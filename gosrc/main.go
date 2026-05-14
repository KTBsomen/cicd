package main

import (
	"fmt"
	"gosrc/actions"
	"gosrc/api"
	"gosrc/database"
	"gosrc/deploypath"
	"gosrc/logger"
	"gosrc/parser"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/shirou/gopsutil/v4/mem"
)

func GetResourceLimits(cpuPercent int, ramPercent int) (int, int64) {
	period := 100000
	cores := runtime.NumCPU()

	cpuQuota := (cpuPercent * period * cores) / 100

	v, err := mem.VirtualMemory()
	if err != nil {
		fmt.Printf("Error getting memory info: %v\n", err)
		return 0, 0
	}

	ramLimit := (int64(v.Total) * int64(ramPercent)) / 100

	return cpuQuota, ramLimit
}
func main() {
	// Initialize the new multi-channel logger
	logger.InitLogger()

	// Ensure system dependencies (like git) are present
	if err := actions.EnsureDependencies(); err != nil {
		fmt.Printf("⚠️ Warning: Dependency check failed: %v\n", err)
	}

	if len(os.Args) > 1 && os.Args[1] == "list" {
		database.InitDB(&parser.Config{})
		projects, _ := database.GetAllProjects()
		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)

		// 1. Add Header with Bold/Cyan styling
		t.AppendHeader(table.Row{"ID", "SERVICE NAME", "REPO URL", "BRANCH", "SERVICE DIR", "SERVICE USER"})
		// 2. Add Rows with "Zebra" stripes using colors
		for i, p := range projects {
			// Switch between Cyan and White for the text
			rowColor := text.FgHiCyan
			if i%2 == 0 {
				rowColor = text.FgHiWhite
			}
			t.AppendRow(table.Row{
				rowColor.Sprint(p.ID),
				rowColor.Sprint(p.ServiceName),
				rowColor.Sprint(p.RepoURL),
				rowColor.Sprint(p.Branch),
				rowColor.Sprint(p.ServiceDir),
				rowColor.Sprint(p.ServiceUser),
			})
			t.AppendSeparator() // The magic "Continuous Line"
		}
		// 3. Customize the look (Colors & Borders)
		style := table.StyleRounded
		style.Format.Header = text.FormatUpper                       // Make headers UPPERCASE
		style.Color.Header = text.Colors{text.FgHiYellow, text.Bold} // Yellow Bold Header
		style.Color.Border = text.Colors{text.FgHiBlack}             // Subtle Dim Borders

		t.SetStyle(style)
		t.Render()
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "stop" {
		err1 := exec.Command("systemctl", "stop", "cicd").Run()
		err2 := exec.Command("systemctl", "disable", "cicd").Run()
		if err1 != nil || err2 != nil {
			fmt.Println("❌ Failed to stop service:", err1, err2)
			os.Exit(1)
		}
		fmt.Println("✅ Service stopped and disabled")
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "start" {
		err1 := exec.Command("systemctl", "start", "cicd").Run()
		if err1 != nil {
			fmt.Println("❌ Failed to start service:", err1)
			os.Exit(1)
		}
		fmt.Println("✅ Service started and enabled")
		os.Exit(0)
	}

	var config parser.Config
	config.Parse()
	database.InitDB(&config)

	if !isRunningUnderSystemd(&config) {
		newPath, err := InstallBinaryToSystem(&config)
		if err != nil {
			newPath, err = os.Executable()
			if err != nil {
				fmt.Println("❌ Failed to get executable path:", err)
				os.Exit(1)
			}

		}
		// Resolve a secure, unique, and collision-free deployment path
		existingPaths, _ := database.GetAllDeploymentPaths()
		res, err := deploypath.ResolveDeploymentPath(deploypath.Config{
			ServiceUser:         config.ServiceUser,
			BaseDir:             config.ServiceDir,
			ProjectName:         config.ServiceName,
			ExistingDeployments: existingPaths,
		})
		if err != nil {
			logger.Error("Secure Path Resolution Failed: "+err.Error(), &config)
			fmt.Printf("❌ Secure Path Error: %v\n", err)
			os.Exit(1)
		}

		// Update the config with the final resolved path (e.g., /home/ubuntu/api-a81f2c)
		config.ServiceDir = res.FinalPath

		// Natively create the directory and set permissions (Root Operation)
		if err := actions.SetupProjectFolder(&config); err != nil {
			logger.Error("Failed to prepare project folder: "+err.Error(), &config)
			fmt.Printf("❌ Permission Error: %v\n", err)
			os.Exit(1)
		}

		database.RegisterProject(&config)
		database.RegisterToMongo(&config)
		fmt.Println("🛠️  Running in Setup Mode...")
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

		os.Exit(0)
	}
	// 1. start the server in background
	go api.StartUnifiedServer(&config)
	// 2. LOAD all projects (including ones from yesterday)
	projects, err := database.GetAllProjects()
	if err != nil {
		logger.Error("Failed to get all projects: "+err.Error(), &config)
		os.Exit(1)
	}
	// --- FLEET RESURRECTION ---
	// Automatically sync and start all registered projects on boot
	logger.Info(fmt.Sprintf("🔋 Restoring fleet: %d projects found", len(projects)), &config)
	for _, p := range projects {
		go func(proj database.Project) {
			appCfg := proj.ToConfig()
			logger.Info(fmt.Sprintf("🛠️  Auto-deploying %s...", appCfg.ServiceName), appCfg)

			// 1. Sync the Repository
			if err := actions.SyncRepo(appCfg); err != nil {
				logger.Error("Auto-sync Failed: "+err.Error(), appCfg)
				return
			}

			// 2. Execute Deployment (which includes starting the process)
			if err := actions.RunDeployment(appCfg); err != nil {
				logger.Error("Auto-deployment Failed: "+err.Error(), appCfg)
				return
			}
		}(p)
	}

	go database.WatchChanges(&config, func(project *parser.Config) {
		logger.Info(fmt.Sprintf("🔔 REMOTE TRIGGER: Deployment signal received for %s", project.ServiceName), project)

		// 1. Sync the Repository (Clone or Pull as ServiceUser)
		if err := actions.SyncRepo(project); err != nil {
			logger.Error("Sync Failed: "+err.Error(), project)
			return
		}

		// 2. Execute Deployment Scripts (install.sh)
		if err := actions.RunDeployment(project); err != nil {
			logger.Error("Execution Failed: "+err.Error(), project)
			return
		}

		logger.Info("✅ DEPLOYMENT CYCLE COMPLETE for "+project.ServiceName, project)
	})

	fmt.Println(config.AdminEmail)
	fmt.Println(config.PublicIP)
	fmt.Println(config.Webhook)
	fmt.Println(config.RepoURL)
	fmt.Println(config.ServiceDir)
	fmt.Println(GetResourceLimits(20, 50))
	logger.SendErrorEmail(&config, "Test", "Test Error", "Test Step")
	stop := make(chan os.Signal, 1)
	// systemd sends SIGTERM on stop, SIGINT on Ctrl+C
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	fmt.Println("🚀 Services running under systemd...")

	// Block main
	<-stop
	fmt.Println("🛑 Stopped by system signal")
}
func InstallBinaryToSystem(cfg *parser.Config) (string, error) {
	targetPath := "/usr/local/bin/cicd"
	currentPath, _ := os.Executable()

	// 1. Check if we are already in the right place
	if currentPath == targetPath {
		return targetPath, nil
	}

	logger.Info(parser.MustParseTemplate("Moving binary to {{.Path}} for permanence...", map[string]any{"Path": targetPath}), cfg)

	// 2. Open the source
	input, err := os.Open(currentPath)
	if err != nil {
		return "", fmt.Errorf("failed to open source binary: %v", err)
	}
	defer input.Close()
	os.Remove(targetPath)
	// 3. Create the destination (using sudo power)
	// We use os.OpenFile to set 0755 permissions (executable)
	output, err := os.OpenFile(targetPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		logger.Error(parser.MustParseTemplate("Failed to install to {{.Path}}: {{.Err}} (try running with sudo)", map[string]any{"Path": targetPath, "Err": err}), cfg)
		return "", fmt.Errorf("failed to install to %s: %v (try running with sudo)", targetPath, err)
	}
	defer output.Close()

	// 4. Copy the bytes
	_, err = io.Copy(output, input)
	return targetPath, err
}

// isRunningUnderSystemd returns true if the process was started by systemd.
func isRunningUnderSystemd(cfg *parser.Config) bool {
	if strings.EqualFold(cfg.Setup, "run") {
		return true
	}
	// systemd always sets INVOCATION_ID for services
	if os.Getenv("INVOCATION_ID") != "" {
		return true
	}

	// Fallback: systemd also sets JOURNAL_STREAM
	if os.Getenv("JOURNAL_STREAM") != "" {
		return true
	}

	return false
}
