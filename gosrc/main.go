package main

import (
	"fmt"
	"gosrc/actions"
	"gosrc/api"
	"gosrc/cli"
	"gosrc/database"
	"gosrc/deploypath"
	"gosrc/logger"
	"gosrc/parser"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

func main() {
	// Initialize the multi-channel logger
	logger.InitLogger()

	// Ensure system dependencies (like git) are present
	if err := actions.EnsureDependencies(); err != nil {
		fmt.Printf("⚠️ Warning: Dependency check failed: %v\n", err)
	}

	// 1. Handle Utility Commands (log, start, stop, ls, token, doctor, etc.)
	cli.HandleCommands(&parser.Config{})

	var config parser.Config
	config.Parse()
	database.InitDB(&config)
	database.LoadGlobalSettings(&config) // B2: Restore from DB, respecting ExplicitFlags

	if !isRunningUnderSystemd(&config) {
		database.SaveGlobalSettings(&config) // Persist current flags for the background service
		newPath, err := InstallBinaryToSystem(&config)
		if err != nil {
			newPath, err = os.Executable()
			if err != nil {
				fmt.Println("❌ Failed to get executable path:", err)
				os.Exit(1)
			}
		}
		if config.RepoURL != "" {
			existingPaths, _ := database.GetAllDeploymentPaths()
			res, err := deploypath.ResolveDeploymentPath(deploypath.Config{
				ServiceUser:         config.ServiceUser,
				BaseDir:             config.ServiceDir,
				ProjectName:         config.ServiceName,
				ExistingDeployments: existingPaths,
			})
			if err != nil {
				logger.Error("Secure Path Resolution Failed: "+err.Error(), &config)
				os.Exit(1)
			}
			config.ServiceDir = res.FinalPath
			if err := actions.SetupProjectFolder(&config); err != nil {
				logger.Error("Failed to prepare project folder: "+err.Error(), &config)
				os.Exit(1)
			}
			database.RegisterProject(&config)
			database.RegisterToMongo(&config)
			fmt.Println("🛠️  Running in Setup Mode...")
		}
		serviceError := actions.CreateServicefile(&config, newPath)
		if serviceError != nil {
			logger.Error("Cant write service files", &config)
			os.Exit(1)
		}
		logger.Info("✅ Service file created. Now starting it...", &config)
		if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
			logger.Error(fmt.Sprintf("Failed to reload systemd: %v", err), &config)
			panic(err)
		}
		if err := exec.Command("systemctl", "enable", "cicd").Run(); err != nil {
			logger.Error(fmt.Sprintf("Failed to enable service: %v", err), &config)
			panic(err)
		}
		if err := exec.Command("systemctl", "restart", "cicd").Run(); err != nil {
			logger.Error(fmt.Sprintf("Failed to start/restart service: %v", err), &config)
			panic(err)
		}
		logger.Info("🚀 Service started and enabled on boot!", &config)
		cli.PrintGatewayInfo(&config)
		os.Exit(0)
	}

	// ═══════════════════════════════════════════════════════
	//  Running under systemd starts here
	// ═══════════════════════════════════════════════════════

	// B1: On first boot with no admin email, generate and print setup token
	if config.AdminEmail == "" {
		token, err := database.GetOrGenerateSetupToken()
		if err != nil {
			logger.Error("Failed to generate setup token: "+err.Error(), &config)
		} else {
			logger.Info("🔑 FIRST-RUN SETUP TOKEN: "+token, &config)
			fmt.Println("\n🔑 ═══════════════════════════════════════════════════")
			fmt.Println("  SETUP TOKEN: " + token)
			fmt.Println("  Paste this in the dashboard login. Expires in 15 min.")
			fmt.Println("  Or run: cicd token")
			fmt.Println("═══════════════════════════════════════════════════════")
		}
	}

	// 1. Start the API server in background
	go api.StartUnifiedServer(&config)

	// 2. FLEET RESURRECTION — through the deployment queue (EC-3)
	projects, err := database.GetAllProjects()
	if err != nil {
		logger.Error("Failed to get all projects: "+err.Error(), &config)
		os.Exit(1)
	}

	logger.Info(fmt.Sprintf("🔋 Restoring fleet: %d projects found", len(projects)), &config)
	for _, p := range projects {
		go func(proj database.Project) {
			appCfg, err := proj.ToConfig()
			if err != nil {
				logger.Error(fmt.Sprintf("Skipping %s: %v", proj.ServiceName, err), &config)
				return
			}
			logger.Info(fmt.Sprintf("🛠️  Auto-deploying %s...", appCfg.ServiceName), appCfg)

			// EC-3: Use the queue so webhooks during resurrection are handled correctly
			actions.EnqueueDeployment(appCfg, "", "", actions.FullDeployPipeline)
		}(p)
	}

	// 3. Watch MongoDB for remote config changes
	go database.WatchChanges(&config, func(project *parser.Config) {
		logger.Info(fmt.Sprintf("🔔 REMOTE TRIGGER: Deployment signal for %s", project.ServiceName), project)
		actions.EnqueueDeployment(project, "", "", actions.FullDeployPipeline)
	})

	// Block on signal
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	fmt.Println("🚀 Services running under systemd...")
	<-stop
	fmt.Println("🛑 Stopped by system signal")
}

func InstallBinaryToSystem(cfg *parser.Config) (string, error) {
	targetPath := "/usr/local/bin/cicd"
	currentPath, _ := os.Executable()
	if currentPath == targetPath {
		return targetPath, nil
	}
	logger.Info(parser.MustParseTemplate("Moving binary to {{.Path}} for permanence...", map[string]any{"Path": targetPath}), cfg)
	input, err := os.Open(currentPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source binary: %v", err)
	}
	defer input.Close()
	os.Remove(targetPath)
	output, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return "", fmt.Errorf("failed to install to %s: %v (try running with sudo)", targetPath, err)
	}
	defer output.Close()
	_, err = io.Copy(output, input)
	return targetPath, err
}

func isRunningUnderSystemd(cfg *parser.Config) bool {
	if strings.EqualFold(cfg.Setup, "run") {
		return true
	}
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}
	if os.Getenv("JOURNAL_STREAM") != "" {
		return true
	}
	return false
}
