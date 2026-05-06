package main

import (
	"fmt"
	"gosrc/actions"
	"gosrc/database"
	"gosrc/logger"
	"gosrc/parser"
	"gosrc/webhook"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

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
	var config parser.Config
	config.Parse()
	if !isRunningUnderSystemd() {
		newPath, err := InstallBinaryToSystem(&config)
		if err != nil {
			newPath, err = os.Executable()
			if err != nil {
				fmt.Println("❌ Failed to get executable path:", err)
				os.Exit(1)
			}

		}
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
		if err := exec.Command("systemctl", "enable", config.ServiceName).Run(); err != nil {
			logger.Error(fmt.Sprintf("Failed to enable service: %v", err), &config)
			panic(err)
		}
		if err := exec.Command("systemctl", "start", config.ServiceName).Run(); err != nil {
			logger.Error(fmt.Sprintf("Failed to start service: %v", err), &config)
			panic(err)
		}
		logger.Info("🚀 Service started and enabled on boot!", &config)

		os.Exit(0)
	}

	database.InitDB(&config)
	go webhook.StartWebhook(&config)

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
func isRunningUnderSystemd() bool {
	if runtime.GOOS == "windows" {
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
