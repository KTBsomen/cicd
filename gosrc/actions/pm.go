package actions

import (
	"context"
	"fmt"
	"gosrc/database"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ═══════════════════════════════════════════════════════
//  4-Tier Process Model (B4)
//
//  Tier 1 — Blocking: run.sh never exits (node app.js)
//  Tier 2 — Background: exits 0, children in our PGID
//  Tier 3 — Delegating: exits 0, user wrote $CICD_PID_FILE
//  Tier 4 — Install-only: no run.sh at all
// ═══════════════════════════════════════════════════════

type RunMode string

const (
	RunModeBlocking    RunMode = "blocking"
	RunModeBackground  RunMode = "background"
	RunModeDelegating  RunMode = "delegating"
	RunModeInstallOnly RunMode = "install"
)

type ManagedProcess struct {
	Cmd          *exec.Cmd
	Config       *parser.Config
	StartTime    time.Time
	Mode         RunMode
	ExtPID       int // PID from $CICD_PID_FILE (tier 3)
	ProcessGroup int // PGID to watch (tier 2)
	RestartCount int
	LastRestart  time.Time
}

var (
	// registry tracks all active processes by ServiceDir
	registry sync.Map
)

// F7 — Restart backoff schedule (seconds)
var backoffSchedule = []time.Duration{
	2 * time.Second,
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

const maxRestarts = 10
const stableRunDuration = 5 * time.Minute // Reset restart count after stable run

// StartAppProcess handles the lifecycle of the application process
func StartAppProcess(cfg *parser.Config) error {
	codebasePath := filepath.Join(cfg.ServiceDir, "codebase")
	if _, err := os.Stat(codebasePath); os.IsNotExist(err) {
		return fmt.Errorf("codebase not found in %s. Codebase might not be ready", codebasePath)
	}

	runScript := filepath.Join(codebasePath, ".cicd", "run.sh")
	pidPath := filepath.Join(cfg.ServiceDir, ".cicdlog", "app.pid")

	// Read the old PID BEFORE deleting, so StopAppProcess and the
	// port-release wait below can actually check if the old process is dead.
	oldPID := readPIDFile(pidPath)

	// Tier 4: No run.sh = install-only mode
	if _, err := os.Stat(runScript); os.IsNotExist(err) {
		logger.Info("✅ No run.sh found. Install-only deployment complete.", cfg)
		registry.Delete(cfg.ServiceDir)
		return nil // Not an error
	}

	// EC-1: Check if process is already alive via PID file (daemon restart case)
	if oldPID > 0 {
		if isProcessAlive(oldPID) {
			logger.Info(fmt.Sprintf("♻️  Re-adopting existing process PID %d (still alive from before restart)", oldPID), cfg)
			proc := &ManagedProcess{
				Config:    cfg,
				StartTime: time.Now(),
				Mode:      RunModeDelegating,
				ExtPID:    oldPID,
			}
			registry.Store(cfg.ServiceDir, proc)
			// Write PID back (it was deleted above, but process is alive)
			writePIDFile(pidPath, oldPID, cfg.ServiceUser)
			go watchSinglePID(proc)
			return nil
		}
		// PID file exists but process dead — proceed to start fresh
	}

	// 1. Kill any existing instance of this service
	StopAppProcess(cfg.ServiceDir)

	// NOW remove the stale PID file after the process is stopped
	os.Remove(pidPath)

	// Wait up to 5 seconds for the old process to release its port before starting the new one.
	// Without this wait, the new process tries to bind the same port while the old one is still
	// in graceful shutdown → "address already in use" → crash → restart loop.
	if oldPID > 0 {
		for i := 0; i < 10; i++ {
			time.Sleep(500 * time.Millisecond)
			if !isProcessAlive(oldPID) {
				break
			}
		}
	}

	// EC-5: Ensure the script is executable
	os.Chmod(runScript, 0755)

	// Patch the run script to inject safety flags (set -e / pipefail) and sudo -A normalization if present
	patchedRunScript := filepath.Join(cfg.ServiceDir, ".cicdlog", "run_patched.sh")
	scriptToRun, _ := patchShellScript(runScript, patchedRunScript)
	if scriptToRun == patchedRunScript {
		logger.Info("🩹 run.sh patched: injected set -e + set -o pipefail", cfg)
	}

	// 2. Prepare the command
	cmd := exec.Command("bash", scriptToRun)
	cmd.Dir = codebasePath

	// Direct logs to the project's deploy.log
	logPath := filepath.Join(cfg.ServiceDir, ".cicdlog", "deploy.log")
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Lookup service user to drop privileges and set proper HOME/USER context (EC-7/EC-9)
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		logFile.Close()
		return fmt.Errorf("failed to lookup service user %s: %v", cfg.ServiceUser, err)
	}
	uidInt, _ := strconv.Atoi(u.Uid)
	gidInt, _ := strconv.Atoi(u.Gid)

	// 3. Environment setup — inject CICD env vars for all tiers and service user context
	cmd.Env = append(os.Environ(),
		"HOME="+u.HomeDir,
		"USER="+u.Username,
		"CICD_PORT="+fmt.Sprintf("%d", cfg.Webhook),
		"CICD_LOG_DIR="+filepath.Join(cfg.ServiceDir, ".cicdlog"),
		"CICD_PID_FILE="+pidPath,
		"CICD_SERVICE_NAME="+cfg.ServiceName,
		"CICD_SERVICE_DIR="+cfg.ServiceDir,
	)

	// 4. Platform-specific Process Group Isolation & Privilege Drop
	setPlatformAttributes(cmd, uint32(uidInt), uint32(gidInt))
	prepareProcessGroup(cmd)

	// 5. Launch as a background process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("failed to start process: %v", err)
	}

	// 6. Register and Watch
	proc := &ManagedProcess{
		Cmd:       cmd,
		Config:    cfg,
		StartTime: time.Now(),
		Mode:      RunModeBlocking, // Default, will be refined in monitorProcess
	}
	registry.Store(cfg.ServiceDir, proc)

	// Write the bash process PID (tier 1 and 2 both start as bash)
	writePIDFile(pidPath, cmd.Process.Pid, cfg.ServiceUser)

	go monitorProcess(proc, logFile)

	logger.Info(fmt.Sprintf("🚀 App '%s' started with PID %d (Managed by Go)", cfg.ServiceName, cmd.Process.Pid), cfg)
	return nil
}

// StopAppProcess safely terminates a running app and its children
func StopAppProcess(id string) {
	if val, ok := registry.Load(id); ok {
		proc := val.(*ManagedProcess)

		// 1. Remove from registry FIRST to prevent watchdog from restarting it
		registry.Delete(id)

		// Handle different modes
		switch proc.Mode {
		case RunModeDelegating:
			if proc.ExtPID > 0 {
				logger.Info(fmt.Sprintf("🛑 Stopping external process PID %d for %s", proc.ExtPID, proc.Config.ServiceName), proc.Config)
				if p, err := os.FindProcess(proc.ExtPID); err == nil {
					p.Signal(os.Interrupt)
					time.Sleep(2 * time.Second)
					p.Kill() // Force if still alive
				}
			}
		case RunModeBackground:
			logger.Info(fmt.Sprintf("🛑 Killing process group %d for %s", proc.ProcessGroup, proc.Config.ServiceName), proc.Config)
			if proc.Cmd != nil {
				killProcessGroup(proc.Cmd)
			}
		default: // RunModeBlocking
			if proc.Cmd != nil {
				logger.Info(fmt.Sprintf("🛑 Stopping service: %s (PID %d)", proc.Config.ServiceName, proc.Cmd.Process.Pid), proc.Config)
				killProcessGroup(proc.Cmd)
			}
		}

		// Clean up PID file
		pidPath := filepath.Join(id, ".cicdlog", "app.pid")
		os.Remove(pidPath)
	}
}

// monitorProcess watches the bash process and detects the correct tier on exit
func monitorProcess(p *ManagedProcess, logFile *os.File) {
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	pgid := 0
	if p.Cmd != nil && p.Cmd.Process != nil {
		pgid = p.Cmd.Process.Pid // PGID == bash PID (set by prepareProcessGroup)
	}

	err := p.Cmd.Wait()

	exitCode := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		exitCode = -1
	}

	pidPath := filepath.Join(p.Config.ServiceDir, ".cicdlog", "app.pid")

	if exitCode != 0 {
		// Non-zero exit = crash → restart with backoff.
		// IMPORTANT: verify we are still the CURRENT registered process.
		// A new deploy may have already replaced us in the registry — in that
		// case we must NOT restart, otherwise we'd kill the new deploy's process.
		if isCurrentProcess(p) {
			logger.Error(fmt.Sprintf("⚠️  Service '%s' crashed (exit code %d). Restarting with backoff...", p.Config.ServiceName, exitCode), p.Config)
			Notify(p.Config, "⚠️ Service Crashed", fmt.Sprintf("Service '%s' exited unexpectedly with code %d. Attempting automated restart.", p.Config.ServiceName, exitCode))
			restartWithBackoff(p)
		} else {
			logger.Info(fmt.Sprintf("🔄 Old watchdog for '%s' exiting — superseded by newer deploy", p.Config.ServiceName), p.Config)
		}
		return
	}

	// Exit 0 — wait for any daemonization forks to settle
	time.Sleep(1 * time.Second)

	// TIER 3: Check for PID file written by run.sh user
	if pidData, err := os.ReadFile(pidPath); err == nil {
		pidStr := strings.TrimSpace(string(pidData))
		if pid, err := strconv.Atoi(pidStr); err == nil && pid > 0 {
			// Verify it's not the bash PID we wrote ourselves
			if pid != pgid && isProcessAlive(pid) && isCurrentProcess(p) {
				p.Mode = RunModeDelegating
				p.ExtPID = pid
				registry.Store(p.Config.ServiceDir, p)
				go watchSinglePID(p)
				logger.Info(fmt.Sprintf("📌 Delegating mode: watching PID %d from $CICD_PID_FILE", pid), p.Config)
				return
			}
		}
	}

	// TIER 2: Auto-detect background children still in our PGID
	if pgid > 0 {
		if children := getChildrenInGroup(pgid); len(children) > 0 && isCurrentProcess(p) {
			p.Mode = RunModeBackground
			p.ProcessGroup = pgid
			// Write the first child PID for CLI status
			writePIDFile(pidPath, children[0], p.Config.ServiceUser)
			registry.Store(p.Config.ServiceDir, p)
			go watchProcessGroup(p)
			logger.Info(fmt.Sprintf("🔍 Background mode: auto-detected %d child(ren) in process group %d", len(children), pgid), p.Config)
			return
		}
	}

	// TIER 4 fallback: check if custom health check script exists to monitor it independently in a loop (F7 / Tier 4 upgrade)
	healthScript := filepath.Join(p.Config.ServiceDir, "codebase", ".cicd", "health.sh")
	if _, err := os.Stat(healthScript); err == nil {
		p.Mode = RunModeDelegating // Treat as delegating/monitored
		p.ExtPID = 0               // No static PID
		registry.Store(p.Config.ServiceDir, p)
		go watchHealthScript(p, healthScript)
		logger.Info("🩺 Custom health script (.cicd/health.sh) detected. Spawning background loop watchdog.", p.Config)
		return
	}

	// TIER 4 fallback: fully external, nothing to track
	logger.Warn("⚠️  WARNING: run.sh exited with no traceable children and no PID file written.", p.Config)
	logger.Warn("👉 If your application runs as a detached background daemon, the orchestrator cannot monitor its status!", p.Config)
	logger.Warn("👉 To enable robust PID monitoring, please write your application's PID to $CICD_PID_FILE inside your run.sh.", p.Config)
	logger.Warn("   [Standard Background Example]:\n     node index.js &\n     echo $! > \"$CICD_PID_FILE\"", p.Config)
	logger.Warn("   [PM2 Process Manager Example]:\n     pm2 start index.js --name \"my-app\"\n     pm2 pid my-app > \"$CICD_PID_FILE\"", p.Config)
	logger.Warn("   [Tmux Session Example]:\n     tmux new-session -d -s my-app \"node index.js\"\n     tmux list-panes -t my-app -F '#{pane_pid}' > \"$CICD_PID_FILE\"", p.Config)
	logger.Info("✅ run.sh exited 0 with no traceable children. Externally managed (monitoring disabled).", p.Config)
	os.Remove(pidPath) // Clean up — no process to track
	registry.Delete(p.Config.ServiceDir)
}

// watchSinglePID monitors an external process by PID (Tier 3)
func watchSinglePID(p *ManagedProcess) {
	for {
		time.Sleep(30 * time.Second)

		// Exit if we are no longer the current process for this service
		if !isCurrentProcess(p) {
			return
		}

		// Reset restart count if stable
		if time.Since(p.StartTime) > stableRunDuration && p.RestartCount > 0 {
			p.RestartCount = 0
		}

		if !isProcessAlive(p.ExtPID) {
			logger.Error(fmt.Sprintf("💀 External process PID %d died. Triggering restart...", p.ExtPID), p.Config)
			registry.Delete(p.Config.ServiceDir)
			restartWithBackoff(p)
			return
		}
	}
}

// watchHealthScript runs .cicd/health.sh in a background loop for independent monitoring (Tier 4)
func watchHealthScript(p *ManagedProcess, healthScript string) {
	for {
		time.Sleep(30 * time.Second)

		// Exit if we are no longer the current process for this service
		if !isCurrentProcess(p) {
			return
		}

		// Reset restart count if stable
		if time.Since(p.StartTime) > stableRunDuration && p.RestartCount > 0 {
			p.RestartCount = 0
		}

		// Execute health script with 10-second timeout, passing CICD env vars
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		os.Chmod(healthScript, 0755)
		err := RunHealthCheckWithEnv(ctx, p.Config, healthScript)
		cancel()

		if err != nil {
			logger.Error(fmt.Sprintf("❌ Custom health check script failed: %v. Triggering restart...", err), p.Config)
			registry.Delete(p.Config.ServiceDir)

			// Mark status as failed in SQLite
			database.SetDeployStatusByDir(p.Config.ServiceDir, "failed")

			// Trigger restart pipeline
			restartWithBackoff(p)
			return
		}
	}
}

// watchProcessGroup monitors all children in a PGID (Tier 2)
func watchProcessGroup(p *ManagedProcess) {
	for {
		time.Sleep(30 * time.Second)

		// Exit if we are no longer the current process for this service
		if !isCurrentProcess(p) {
			return
		}

		// Reset restart count if stable
		if time.Since(p.StartTime) > stableRunDuration && p.RestartCount > 0 {
			p.RestartCount = 0
		}

		children := getChildrenInGroup(p.ProcessGroup)
		if len(children) == 0 {
			logger.Error("💀 All children in process group died. Triggering restart...", p.Config)
			registry.Delete(p.Config.ServiceDir)
			restartWithBackoff(p)
			return
		}
	}
}

// restartWithBackoff restarts with exponential backoff (F7)
func restartWithBackoff(p *ManagedProcess) {
	p.RestartCount++

	if p.RestartCount > maxRestarts {
		logger.Error(fmt.Sprintf("🔥 Max restarts (%d) exceeded for %s. Giving up. Manual intervention required.", maxRestarts, p.Config.ServiceName), p.Config)
		Notify(p.Config, "🔥 Max Restarts Exceeded", fmt.Sprintf("Service '%s' has crashed too many times (%d). Automation has been suspended to prevent loops. Manual intervention required.", p.Config.ServiceName, maxRestarts))
		return
	}

	// Get backoff delay
	idx := p.RestartCount - 1
	if idx >= len(backoffSchedule) {
		idx = len(backoffSchedule) - 1
	}
	delay := backoffSchedule[idx]

	logger.Warn(fmt.Sprintf("⏱️  Restart %d/%d for %s. Waiting %v...", p.RestartCount, maxRestarts, p.Config.ServiceName, delay), p.Config)
	time.Sleep(delay)

	// After the backoff sleep, a new deploy may have started — only restart if we are still current
	if !isCurrentProcess(p) {
		logger.Info(fmt.Sprintf("🔄 Backoff completed for '%s' but a newer deploy is active — not restarting", p.Config.ServiceName), p.Config)
		return
	}

	if err := StartAppProcess(p.Config); err != nil {
		logger.Error(fmt.Sprintf("Failed to restart %s: %v", p.Config.ServiceName, err), p.Config)
	}
}

// GetProcessStatus returns info for the daemon's in-memory API.
// Returns: isRunning, pid, uptime, restartCount
func GetProcessStatus(id string) (bool, int, string, int) {
	if val, ok := registry.Load(id); ok {
		proc := val.(*ManagedProcess)
		uptime := formatUptime(time.Since(proc.StartTime))
		switch proc.Mode {
		case RunModeDelegating:
			return true, proc.ExtPID, uptime, proc.RestartCount
		case RunModeBackground:
			children := getChildrenInGroup(proc.ProcessGroup)
			if len(children) > 0 {
				return true, children[0], uptime, proc.RestartCount
			}
		default:
			if proc.Cmd != nil && proc.Cmd.Process != nil {
				return true, proc.Cmd.Process.Pid, uptime, proc.RestartCount
			}
		}
	}

	// Fallback: If not tracked in active process memory, check database deployment status.
	// This ensures that Tier 4 / externally managed processes (e.g. launched via custom health checks or fully detached)
	// correctly show as Online / running on the dashboard!
	if status, err := database.GetDeployStatusByDir(id); err == nil {
		if status == "running" || status == "externally_managed" {
			return true, 0, "Externally Managed", 0
		}
	}

	return false, 0, "Not Running", 0
}

// GetProcessStatusFromDisk checks the PID file on disk (B3 — for CLI use)
func GetProcessStatusFromDisk(serviceDir string) (bool, int, string, int) {
	pidPath := filepath.Join(serviceDir, ".cicdlog", "app.pid")
	pid := readPIDFile(pidPath)
	if pid <= 0 {
		return false, 0, "Not Running", 0
	}
	if !isProcessAlive(pid) {
		return false, 0, "Not Running (stale PID)", 0
	}
	return true, pid, "Running", 0
}

// GetRunMode returns the current run mode for a project
func GetRunMode(serviceDir string) RunMode {
	if val, ok := registry.Load(serviceDir); ok {
		proc := val.(*ManagedProcess)
		return proc.Mode
	}
	return RunModeInstallOnly
}

// ═══════════════════════════════════════════════════════
//  Helpers
// ═══════════════════════════════════════════════════════

func writePIDFile(path string, pid int, serviceUser string) {
	_ = os.WriteFile(path, []byte(strconv.Itoa(pid)), 0644)
	if u, err := user.Lookup(serviceUser); err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		_ = os.Chown(path, uid, gid)
	}
}

func readPIDFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

func formatUptime(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	hours := int(d.Hours())
	mins := int(d.Minutes()) % 60
	if hours >= 24 {
		days := hours / 24
		hours = hours % 24
		return fmt.Sprintf("%dd%dh%dm", days, hours, mins)
	}
	return fmt.Sprintf("%dh%dm", hours, mins)
}

// isCurrentProcess returns true if p is still the active registered process for its service.
// Watchdog goroutines call this before taking any action so that stale goroutines
// from a previous deploy silently exit instead of interfering with a newer deploy.
func isCurrentProcess(p *ManagedProcess) bool {
	val, ok := registry.Load(p.Config.ServiceDir)
	if !ok {
		return false // service was intentionally stopped
	}
	return val.(*ManagedProcess) == p // pointer equality — must be the exact same instance
}
