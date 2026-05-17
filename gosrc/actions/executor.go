package actions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gosrc/database"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// EnsureDependencies checks for essential system tools (like git) and installs them if missing.
func EnsureDependencies() error {
	if _, err := exec.LookPath("git"); err == nil {
		return nil
	}
	logger.Info("Git not found. Attempting automatic installation...", nil)
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		exec.Command("apt-get", "update", "-y").Run()
		return exec.Command("apt-get", "install", "-y", "git").Run()
	} else if _, err := os.Stat("/usr/bin/yum"); err == nil {
		return exec.Command("yum", "install", "-y", "git").Run()
	} else if runtime.GOOS == "windows" {
		return exec.Command("choco", "install", "git", "-y").Run()
	} else if runtime.GOOS == "darwin" {
		return exec.Command("brew", "install", "git").Run()
	} else if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		return exec.Command("dnf", "install", "-y", "git").Run()
	} else if _, err := os.Stat("/usr/bin/pacman"); err == nil {
		return exec.Command("pacman", "-S", "--noconfirm", "git").Run()
	}
	return fmt.Errorf("git is missing and no supported package manager found")
}

// SetupProjectFolder handles the one-time root-level directory setup and ownership transfer.
func SetupProjectFolder(cfg *parser.Config) error {
	path := cfg.ServiceDir
	if !isValidUsername(cfg.ServiceUser) {
		return fmt.Errorf("invalid service username: '%s'", cfg.ServiceUser)
	}
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		logger.Info(fmt.Sprintf("User '%s' not found. Creating...", cfg.ServiceUser), cfg)
		createCmd := exec.Command("useradd", "--system", "--shell", "/usr/sbin/nologin", "-m", cfg.ServiceUser)
		if err := createCmd.Run(); err != nil {
			return fmt.Errorf("failed to create user '%s': %v", cfg.ServiceUser, err)
		}
		u, err = user.Lookup(cfg.ServiceUser)
		if err != nil {
			return fmt.Errorf("user creation succeeded but lookup failed: %v", err)
		}
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	os.Chown(path, uid, gid)

	testPath := filepath.Join(path, ".cicdlog")
	testCmd := exec.Command("sudo", "-u", cfg.ServiceUser, "mkdir", "-p", testPath)
	if err := testCmd.Run(); err != nil {
		parentDir := filepath.Dir(path)
		if err := exec.Command("chmod", "+x", parentDir).Run(); err == nil {
			if err := exec.Command("sudo", "-u", cfg.ServiceUser, "mkdir", "-p", testPath).Run(); err == nil {
				logger.Info(fmt.Sprintf("🛠️  Auto-fixed traversal permissions for %s", parentDir), cfg)
			} else {
				return fmt.Errorf("user '%s' cannot write to %s", cfg.ServiceUser, path)
			}
		}
	}
	os.Chown(filepath.Join(path, ".cicdlog"), uid, gid)
	logger.Info(fmt.Sprintf("✅ Project folder prepared at %s for user %s", path, cfg.ServiceUser), cfg)
	return nil
}

// GrantSudoPrivileges creates a restricted sudoers entry for the service user (IF-6 expanded whitelist).
func GrantSudoPrivileges(username string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	content := fmt.Sprintf("%s ALL=(ALL) NOPASSWD: %s\n", username, GetWhiteListForSUDO())
	sudoersPath := fmt.Sprintf("/etc/sudoers.d/%s", username)
	return os.WriteFile(sudoersPath, []byte(content), 0440)
}

func RemoveSudoPrivileges(username string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	return os.Remove(fmt.Sprintf("/etc/sudoers.d/%s", username))
}

func GetFileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func RunAsUser(cfg *parser.Config, command string, args ...string) error {
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		return err
	}
	uidInt, _ := strconv.Atoi(u.Uid)
	gidInt, _ := strconv.Atoi(u.Gid)
	cmd := exec.Command(command, args...)
	cmd.Dir = cfg.ServiceDir
	cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+u.Username)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setPlatformAttributes(cmd, uint32(uidInt), uint32(gidInt))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%s %v' failed: %v", command, args, err)
	}
	return nil
}

// RunAsUserWithContext runs a command with a context timeout (EC-7)
func RunAsUserWithContext(ctx context.Context, cfg *parser.Config, command string, args ...string) error {
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		return err
	}
	uidInt, _ := strconv.Atoi(u.Uid)
	gidInt, _ := strconv.Atoi(u.Gid)
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Dir = cfg.ServiceDir
	cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+u.Username)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	setPlatformAttributes(cmd, uint32(uidInt), uint32(gidInt))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%s %v' failed: %v", command, args, err)
	}
	return nil
}

// ═══════════════════════════════════════════════════════
//  F12 — Disk Space Check
// ═══════════════════════════════════════════════════════

// checkDiskSpace is implemented in platform-specific files (executor_linux.go, executor_windows.go)

// ═══════════════════════════════════════════════════════
//  F6 — Per-project deploy timeout
// ═══════════════════════════════════════════════════════

func getDeployTimeout(codebasePath string, globalSecs int) time.Duration {
	// Check for per-project override: .cicd/timeout
	data, err := os.ReadFile(filepath.Join(codebasePath, ".cicd", "timeout"))
	if err == nil {
		if secs, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	if globalSecs <= 0 {
		globalSecs = 1800
	}
	return time.Duration(globalSecs) * time.Second
}

// ═══════════════════════════════════════════════════════
//  F11 — Visual Log Separators
// ═══════════════════════════════════════════════════════

func writeSeparator(f *os.File, title, service string) {
	line := "══════════════════════════════════════════════════════════"
	ts := time.Now().Format("2006-01-02 15:04:05")
	fmt.Fprintf(f, "\n%s\n  %s  │  %s  │  %s\n%s\n\n", line, title, service, ts, line)
}

// ═══════════════════════════════════════════════════════
//  SyncRepo — EC-2, EC-7, EC-15
// ═══════════════════════════════════════════════════════

func SyncRepo(cfg *parser.Config) error {
	codebasePath := filepath.Join(cfg.ServiceDir, "codebase")

	// F12: Disk space check
	if err := checkDiskSpace(cfg.ServiceDir); err != nil {
		Notify(cfg, "🚨 Disk Space Critical", fmt.Sprintf("Deployment aborted for '%s': %v", cfg.ServiceName, err))
		return err
	}

	if _, err := os.Stat(codebasePath); os.IsNotExist(err) {
		os.MkdirAll(codebasePath, 0755)
		u, _ := user.Lookup(cfg.ServiceUser)
		if u != nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			os.Chown(codebasePath, uid, gid)
		}
	}

	// Safe directory fix
	RunAsUser(cfg, "git", "config", "--global", "--add", "safe.directory", codebasePath)

	// Prepare authenticated URL
	repoURL := cfg.RepoURL
	if cfg.GitUsername != "" && cfg.GitPassword != "" {
		if after, ok := strings.CutPrefix(repoURL, "https://"); ok {
			repoURL = "https://" + cfg.GitUsername + ":" + cfg.GitPassword + "@" + after
		}
	}

	// EC-7: 10-minute timeout for git operations
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	if _, err := os.Stat(filepath.Join(codebasePath, ".git")); os.IsNotExist(err) {
		logger.Info("Cloning repository into codebase/...", cfg)
		oldDir := cfg.ServiceDir
		cfg.ServiceDir = codebasePath
		err := RunAsUserWithContext(ctx, cfg, "git", "clone", repoURL, ".")
		cfg.ServiceDir = oldDir
		return err
	}

	// EC-2: Reset tracked file modifications before pull (preserves node_modules)
	logger.Info("🧹 Resetting working tree before pull...", cfg)
	oldDir := cfg.ServiceDir
	cfg.ServiceDir = codebasePath
	defer func() { cfg.ServiceDir = oldDir }()

	RunAsUserWithContext(ctx, cfg, "git", "reset", "--hard", "HEAD")
	// EC-4: Reset to branch tip for correct state after rollback
	RunAsUserWithContext(ctx, cfg, "git", "fetch", "origin", cfg.Branch)
	return RunAsUserWithContext(ctx, cfg, "git", "reset", "--hard", "origin/"+cfg.Branch)
}

// ═══════════════════════════════════════════════════════
//  RunDeployment — F6, F11, F8
// ═══════════════════════════════════════════════════════

func RunDeployment(cfg *parser.Config) error {
	originalDir := cfg.ServiceDir
	codebasePath := filepath.Join(originalDir, "codebase")
	logDir := filepath.Join(originalDir, ".cicdlog")
	logPath := filepath.Join(logDir, "deploy.log")

	// F10: Rotate log if needed
	logger.RotateLogIfNeeded(logPath)

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open project log: %v", err)
	}
	defer logFile.Close()

	// Get deploy timeout
	timeout := getDeployTimeout(codebasePath, cfg.DeployTimeout)

	// 1. Run Install Script (if exists and changed)
	installScript := filepath.Join(codebasePath, ".cicd", "install.sh")
	if _, err := os.Stat(installScript); err == nil {
		currentHash, _ := GetFileHash(installScript)
		hashFile := filepath.Join(logDir, "install.hash")
		lastHash, _ := os.ReadFile(hashFile)

		if string(lastHash) == currentHash {
			logger.Info("⏩ install.sh hasn't changed. Skipping build step.", cfg)
		} else {
			// F11: Write INSTALL separator
			writeSeparator(logFile, "📦 INSTALL PHASE", cfg.ServiceName)

			// Determine sudo authentication strategy BEFORE building env so that
			// SUDO_ASKPASS is set correctly in the child process environment.
			askPassValue := "/bin/false" // default: block interactive prompts
			if cfg.SudoPass != "" {
				// Strategy A: temp askpass.sh helper. install.sh must use `sudo -A`.
				askPassPath := filepath.Join(logDir, "askpass.sh")
				helperContent := fmt.Sprintf("#!/bin/bash\necho '%s'\n", cfg.SudoPass)
				if err := os.WriteFile(askPassPath, []byte(helperContent), 0700); err != nil {
					logger.Error(fmt.Sprintf("⚠️ Failed to write askpass helper: %v", err), cfg)
				} else {
					defer os.Remove(askPassPath)
					askPassValue = askPassPath
					logger.Info("🔑 Using SudoPass via ASKPASS helper for install.sh", cfg)
				}
			} else {
				// Strategy B: grant JIT NOPASSWD entry in /etc/sudoers.d/.
				if err := GrantSudoPrivileges(cfg.ServiceUser); err != nil {
					logger.Error(fmt.Sprintf("⚠️ JIT Sudo failed: %v", err), cfg)
				} else {
					defer RemoveSudoPrivileges(cfg.ServiceUser)
					logger.Info("🔑 Using JIT NOPASSWD sudo for install.sh", cfg)
				}
			}


			// Build env AFTER askpass is ready so SUDO_ASKPASS has the correct value.
			env := append(os.Environ(),
				"DEBIAN_FRONTEND=noninteractive",
				"SUDO_ASKPASS="+askPassValue,
				"UCF_FORCE_CONFNEW=1",
				"PYTHONUNBUFFERED=1",
				"PORT="+fmt.Sprintf("%d", cfg.Webhook),
				"CICD_LOG_DIR="+logDir,
				"CICD_PID_FILE="+filepath.Join(logDir, "app.pid"),
				"CICD_SERVICE_NAME="+cfg.ServiceName,
				"CICD_SERVICE_DIR="+cfg.ServiceDir,
			)
			logger.Info(fmt.Sprintf("📋 Config for this deployment: %s", cfg.String()), cfg)

			os.Chmod(installScript, 0755)


			// F6: Run with timeout via context
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()

			cmd := exec.CommandContext(ctx, "bash", installScript)
			cmd.Dir = codebasePath
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			cmd.Env = env

			u, _ := user.Lookup(cfg.ServiceUser)
			if u != nil {
				uid, _ := strconv.Atoi(u.Uid)
				gid, _ := strconv.Atoi(u.Gid)
				setPlatformAttributes(cmd, uint32(uid), uint32(gid))
			}
			prepareProcessGroup(cmd)

			if err := cmd.Run(); err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					writeSeparator(logFile, "⏰ TIMEOUT — install.sh killed", cfg.ServiceName)
					logger.Error(fmt.Sprintf("⏰ install.sh exceeded timeout (%v)", timeout), cfg)
					sendNotification(cfg, fmt.Sprintf("⏰ TIMEOUT: install.sh for '%s' exceeded %v", cfg.ServiceName, timeout))
					return fmt.Errorf("install.sh timed out after %v", timeout)
				}
				return fmt.Errorf("install.sh failed: %v (Check .cicdlog/deploy.log)", err)
			}

			os.WriteFile(hashFile, []byte(currentHash), 0644)
			writeSeparator(logFile, "✅ INSTALL COMPLETE", cfg.ServiceName)
			logger.Info("✅ install.sh executed and hash updated", cfg)
		}
	} else {
		logger.Warn("⏩ install.sh not found. Skipping build step.", cfg)
	}

	// 2. Start the application
	writeSeparator(logFile, "🚀 RUN PHASE", cfg.ServiceName)
	logger.Info("⚙️  Launching app via internal Go Process Engine...", cfg)
	if err := StartAppProcess(cfg); err != nil {
		return fmt.Errorf("process start failed: %v", err)
	}

	logger.Info("🚀 Application is now LIVE and managed by Go!", cfg)
	return nil
}

// ═══════════════════════════════════════════════════════
//  F4 — Rollback to Any Commit
// ═══════════════════════════════════════════════════════

func RollbackToCommit(cfg *parser.Config, commitHash string) error {
	Notify(cfg, "⏪ Manual Rollback", fmt.Sprintf("Initiating manual rollback for service '%s' to commit %s", cfg.ServiceName, truncHash(commitHash)))
	codebasePath := filepath.Join(cfg.ServiceDir, "codebase")
	logPath := filepath.Join(cfg.ServiceDir, ".cicdlog", "deploy.log")
	logFile, _ := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if logFile != nil {
		writeSeparator(logFile, "⏪ ROLLBACK TO "+commitHash[:8], cfg.ServiceName)
		logFile.Close()
	}

	// Update deploy status
	projectID, _ := database.GetProjectIDByDir(cfg.ServiceDir)
	database.SetDeployStatusByDir(cfg.ServiceDir, "deploying")

	// 1. Stop current process
	StopAppProcess(cfg.ServiceDir)

	// 2. Git operations with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	oldDir := cfg.ServiceDir
	cfg.ServiceDir = codebasePath

	// EC-15: Unshallow if needed for old commits
	RunAsUserWithContext(ctx, cfg, "git", "fetch", "--unshallow")
	RunAsUserWithContext(ctx, cfg, "git", "fetch", "origin")
	RunAsUserWithContext(ctx, cfg, "git", "checkout", commitHash)

	// EC-11: Full clean for rollback only (removes node_modules etc.)
	// EC-14: Only clean if install.sh differs
	hashFile := filepath.Join(oldDir, ".cicdlog", "install.hash")
	newInstallHash, _ := GetFileHash(filepath.Join(codebasePath, ".cicd", "install.sh"))
	oldInstallHash, _ := os.ReadFile(hashFile)

	if string(oldInstallHash) != newInstallHash {
		logger.Info("🧹 install.sh differs — running full clean for rollback", cfg)
		RunAsUserWithContext(ctx, cfg, "git", "clean", "-fdx")
		os.Remove(hashFile) // Force reinstall
	}

	cfg.ServiceDir = oldDir

	// 3. Run deployment (force install by clearing hash if needed)
	if err := RunDeployment(cfg); err != nil {
		logger.Error("Rollback deployment failed: "+err.Error(), cfg)
		database.SetDeployStatusByDir(cfg.ServiceDir, "failed")
		sendNotification(cfg, fmt.Sprintf("🔥 Rollback FAILED for '%s' to %s: %v", cfg.ServiceName, commitHash[:8], err))
		return err
	}

	// 4. Update history
	if projectID > 0 {
		database.MarkCommitRolledBack(projectID, commitHash)
		database.SetCurrentCommit(projectID, commitHash)
	}
	database.UpdateCommitHashLocal(cfg.ServiceDir, commitHash)

	// 5. Post-deploy health check
	go PostDeployHealthCheck(cfg, commitHash, "rollback", 1)

	logger.Info(fmt.Sprintf("⏪ Rollback to %s complete", commitHash[:8]), cfg)
	return nil
}

// ═══════════════════════════════════════════════════════
//  F8 — Post-Deploy Health Check + Auto-Rollback
// ═══════════════════════════════════════════════════════

// PostDeployHealthCheck waits 15s then verifies the process is alive.
// depth parameter prevents infinite rollback loops (max depth 1).
func PostDeployHealthCheck(cfg *parser.Config, hash, trigger string, depth int) {
	// EC-8: Skip for install-only or externally managed
	runMode := GetRunMode(cfg.ServiceDir)
	if runMode == RunModeInstallOnly {
		database.SetDeployStatusByDir(cfg.ServiceDir, "installed")
		return
	}

	time.Sleep(15 * time.Second)

	projectID, _ := database.GetProjectIDByDir(cfg.ServiceDir)
	alive, _, _, restarts := GetProcessStatus(cfg.ServiceDir)

	// Stability check: If it's alive but has restarted, it's not a successful deploy
	if alive && restarts > 0 {
		logger.Warn(fmt.Sprintf("⚠️  Service '%s' is alive but has restarted %d times during health check. Marking as UNSTABLE.", cfg.ServiceName, restarts), cfg)
		alive = false // Trigger rollback logic below
	}

	if alive {
		if projectID > 0 {
			database.SetCurrentCommit(projectID, hash)
		}
		status := "running"
		if runMode == RunModeDelegating {
			status = "externally_managed"
		}
		database.SetDeployStatusByDir(cfg.ServiceDir, status)
		logger.Info(fmt.Sprintf("✅ Health check PASSED for %s (commit %s)", cfg.ServiceName, truncHash(hash)), cfg)
		Notify(cfg, "✅ Deployment Success", fmt.Sprintf("Service '%s' is running (commit %s). Trigger: %s", cfg.ServiceName, truncHash(hash), trigger))
		return
	}

	// Health check failed
	logger.Error(fmt.Sprintf("⚠️  HEALTH CHECK FAILED for %s after 15s", cfg.ServiceName), cfg)
	database.SetDeployStatusByDir(cfg.ServiceDir, "failed")

	// Auto-rollback if we have a previous good commit and haven't already rolled back
	if depth < 1 && projectID > 0 {
		prevHash, err := database.GetPreviousCurrentCommit(projectID)
		if err == nil && prevHash != "" && prevHash != hash {
			logger.Warn(fmt.Sprintf("⏪ Auto-rolling back to previous commit %s...", truncHash(prevHash)), cfg)
			Notify(cfg, "⚠️ Health Check Failed", fmt.Sprintf("Service '%s' failed health check. Auto-rolling back to commit %s.", cfg.ServiceName, truncHash(prevHash)))
			RollbackToCommit(cfg, prevHash)
			return
		}
	}

	Notify(cfg, "🔥 Deployment Critical", fmt.Sprintf("Service '%s' failed health check and no rollback target is available. Manual intervention required.", cfg.ServiceName))
}

// ═══════════════════════════════════════════════════════
//  RunCustomCommand — unchanged
// ═══════════════════════════════════════════════════════

func RunCustomCommand(cfg *parser.Config, command string) (string, error) {
	if !isValidUsername(cfg.ServiceUser) {
		return "", fmt.Errorf("invalid service user")
	}
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		return "", err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = cfg.ServiceDir
	setPlatformAttributes(cmd, uint32(uid), uint32(gid))
	cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+cfg.ServiceUser)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func isValidUsername(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

// GetRepoHash returns the current HEAD commit hash of the project's codebase
func GetRepoHash(cfg *parser.Config) (string, error) {
	codebasePath := filepath.Join(cfg.ServiceDir, "codebase")
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		return "", err
	}
	uidInt, _ := strconv.Atoi(u.Uid)
	gidInt, _ := strconv.Atoi(u.Gid)
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = codebasePath
	cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+u.Username)
	setPlatformAttributes(cmd, uint32(uidInt), uint32(gidInt))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git rev-parse failed: %v (output: %s)", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

// FullDeployPipeline is the standard deployment function used by the queue.
func FullDeployPipeline(cfg *parser.Config, commitHash, commitMsg string) {
	Notify(cfg, "🚀 Deployment Started", fmt.Sprintf("Triggering build for %s on branch %s", cfg.ServiceName, cfg.Branch))
	projectID, _ := database.GetProjectIDByDir(cfg.ServiceDir)
	database.SetDeployStatusByDir(cfg.ServiceDir, "deploying")

	if err := SyncRepo(cfg); err != nil {
		logger.Error("Sync Failed: "+err.Error(), cfg)
		database.SetDeployStatusByDir(cfg.ServiceDir, "failed")
		Notify(cfg, "🔥 Sync Failed", fmt.Sprintf("Repository synchronization failed for '%s': %v", cfg.ServiceName, err))
		return
	}

	trigger := "webhook"
	// If hash is missing (e.g. manual create), detect it after sync
	if commitHash == "" {
		trigger = "manual"
		if h, err := GetRepoHash(cfg); err == nil {
			commitHash = h
		}
	}

	// Record commit in history
	if projectID > 0 {
		database.AddCommitToHistory(projectID, commitHash, commitMsg, trigger)
	}

	if err := RunDeployment(cfg); err != nil {
		logger.Error("Deployment Failed: "+err.Error(), cfg)
		database.SetDeployStatusByDir(cfg.ServiceDir, "failed")
		Notify(cfg, "🔥 Build Failed", fmt.Sprintf("Deployment script or installation failed for '%s': %v", cfg.ServiceName, err))
		return
	}

	database.UpdateCommitHashLocal(cfg.ServiceDir, commitHash)
	database.UpdateLastCommitInfo(cfg.ServiceDir, commitHash, commitMsg)
	if cfg.MongoDBURI != "" {
		go database.UpdateCommitHash(cfg, commitHash)
	}

	go PostDeployHealthCheck(cfg, commitHash, trigger, 0)
}
