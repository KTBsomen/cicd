package actions

import (
	"fmt"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type ManagedProcess struct {
	Cmd       *exec.Cmd
	Config    *parser.Config
	StartTime time.Time
}

var (
	// registry tracks all active processes by ServiceName
	registry sync.Map
)

// StartAppProcess handles the lifecycle of the application process
func StartAppProcess(cfg *parser.Config) error {
	// 1. Kill any existing instance of this service (by its unique directory)
	StopAppProcess(cfg.ServiceDir)

	// code path inside {cfg.ServiceDir}/codebase
	codebasePath := filepath.Join(cfg.ServiceDir, "codebase")
	if _, err := os.Stat(codebasePath); os.IsNotExist(err) {
		return fmt.Errorf("codebase not found in %s. Codebase might not be ready", codebasePath)
	}
	// run.sh  path inside {cfg.ServiceDir}/codebase/.cicd/run.sh
	runScript := filepath.Join(codebasePath, ".cicd", "run.sh")
	if _, err := os.Stat(runScript); os.IsNotExist(err) {
		return fmt.Errorf("run.sh not found in %s. Codebase might not be ready", codebasePath)
	}

	// 2. Prepare the command
	cmd := exec.Command("bash", runScript)
	cmd.Dir = codebasePath

	// Direct logs to the project's deploy.log
	logPath := filepath.Join(cfg.ServiceDir, ".cicdlog", "deploy.log")
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// 3. Environment setup
	cmd.Env = append(os.Environ(), "PORT="+fmt.Sprintf("%d", cfg.Webhook))

	// 4. Platform-specific Process Group Isolation
	prepareProcessGroup(cmd)

	// 5. Launch as a background process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start process: %v", err)
	}

	// 6. Register and Watch
	proc := &ManagedProcess{
		Cmd:       cmd,
		Config:    cfg,
		StartTime: time.Now(),
	}
	registry.Store(cfg.ServiceDir, proc)

	go monitorProcess(proc)

	logger.Info(fmt.Sprintf("🚀 App '%s' started with PID %d (Managed by Go)", cfg.ServiceName, cmd.Process.Pid), cfg)
	return nil
}

// StopAppProcess safely terminates a running app and its children
func StopAppProcess(id string) {
	if val, ok := registry.Load(id); ok {
		proc := val.(*ManagedProcess)
		logger.Info(fmt.Sprintf("🛑 Stopping service: %s (PID %d) at %s", proc.Config.ServiceName, proc.Cmd.Process.Pid, id), proc.Config)

		// 1. Remove from registry FIRST to prevent watchdog from restarting it
		registry.Delete(id)

		// 2. Kill the entire process group/tree
		killProcessGroup(proc.Cmd)
	}
}

// monitorProcess waits for the process to exit and handles restarts
func monitorProcess(p *ManagedProcess) {
	err := p.Cmd.Wait()

	// If it's still in the registry, it means it exited unexpectedly (not via StopAppProcess)
	if _, ok := registry.Load(p.Config.ServiceDir); ok {
		logger.Error(fmt.Sprintf("⚠️  Service '%s' exited unexpectedly: %v. Restarting...", p.Config.ServiceName, err), p.Config)
		time.Sleep(2 * time.Second) // Prevent rapid-fire crashing
		StartAppProcess(p.Config)
	}
}

// GetProcessStatus returns info for the dashboard
func GetProcessStatus(id string) (bool, int, string) {
	if val, ok := registry.Load(id); ok {
		proc := val.(*ManagedProcess)
		uptime := time.Since(proc.StartTime).String()
		return true, proc.Cmd.Process.Pid, uptime
	}
	return false, 0, "Not Running"
}
