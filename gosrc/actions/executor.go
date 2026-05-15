package actions

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// EnsureDependencies checks for essential system tools (like git) and installs them if missing.
func EnsureDependencies() error {
	if _, err := exec.LookPath("git"); err == nil {
		return nil
	}

	logger.Info("Git not found. Attempting automatic installation...", nil)

	// Detect Package Manager and Install
	if _, err := os.Stat("/usr/bin/apt-get"); err == nil {
		cmd := exec.Command("apt-get", "update", "-y")
		cmd.Run() // Best effort update
		return exec.Command("apt-get", "install", "-y", "git").Run()
	} else if _, err := os.Stat("/usr/bin/yum"); err == nil {
		return exec.Command("yum", "install", "-y", "git").Run()
	} else if runtime.GOOS == "windows" {
		// On Windows, git is usually in PATH if installed, or we can try chocolatey
		if _, err := exec.LookPath("git"); err == nil {
			return nil
		}
		logger.Info("Git not found. Attempting Chocolatey installation...", nil)
		return exec.Command("choco", "install", "git", "-y").Run()
	} else if runtime.GOOS == "darwin" {
		// On macOS, git is usually in PATH if installed, or we can try Homebrew
		if _, err := exec.LookPath("git"); err == nil {
			return nil
		}
		logger.Info("Git not found. Attempting Homebrew installation...", nil)
		return exec.Command("brew", "install", "git").Run()
	} else if _, err := os.Stat("/usr/bin/dnf"); err == nil {
		return exec.Command("dnf", "install", "-y", "git").Run()
	} else if _, err := os.Stat("/usr/bin/zypper"); err == nil {
		return exec.Command("zypper", "install", "-y", "git").Run()
	} else if _, err := os.Stat("/usr/bin/pacman"); err == nil {
		return exec.Command("pacman", "-S", "--noconfirm", "git").Run()
	} else {
		return fmt.Errorf("git is missing and no supported package manager (apt/yum/choco/brew/dnf/pacman) was found")
	}

}

// SetupProjectFolder handles the one-time root-level directory setup and ownership transfer.
// This is the "Option B" strategy: Root creates, User owns, everything else runs as User.
func SetupProjectFolder(cfg *parser.Config) error {
	path := cfg.ServiceDir
	// 1. Resolve UID/GID for the target user (with Auto-Creation)
	if !isValidUsername(cfg.ServiceUser) {
		return fmt.Errorf("invalid service username: '%s'. Use only alphanumeric characters and dashes", cfg.ServiceUser)
	}

	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		logger.Info(fmt.Sprintf("User '%s' not found. Creating secure system user...", cfg.ServiceUser), cfg)

		// Create a system user with no login shell and a home directory
		// --system: creates a system account
		// --shell /usr/sbin/nologin: prevents interactive login
		createCmd := exec.Command("useradd", "--system", "--shell", "/usr/sbin/nologin", "-m", cfg.ServiceUser)
		if err := createCmd.Run(); err != nil {
			return fmt.Errorf("failed to create system user '%s': %v (Make sure you are running as root)", cfg.ServiceUser, err)
		}

		// Try looking up the user again after creation
		u, err = user.Lookup(cfg.ServiceUser)
		if err != nil {
			return fmt.Errorf("user creation succeeded but lookup failed: %v", err)
		}
	}
	// 2. Create the directory tree as Root
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)

	// 3. Grant Ownership to the ServiceUser
	if err := os.Chown(path, uid, gid); err != nil {
		return fmt.Errorf("failed to set ownership: %v", err)
	}

	// 4. THE REAL TEST: Verify the user can actually reach and write to this folder.
	// We attempt to create the .cicdlog directory AS the target user via sudo.
	testPath := filepath.Join(path, ".cicdlog")
	testCmd := exec.Command("sudo", "-u", cfg.ServiceUser, "mkdir", "-p", testPath)

	if err := testCmd.Run(); err != nil {
		logger.Warn(fmt.Sprintf("⚠️  Visibility Issue: User '%s' cannot reach %s. Attempting auto-fix...", cfg.ServiceUser, path), cfg)

		// Self-Healing: Attempt to grant execute (+x) permission to the parent directory
		parentDir := filepath.Dir(path)
		if err := exec.Command("chmod", "+x", parentDir).Run(); err == nil {
			// Retry the test
			if err := exec.Command("sudo", "-u", cfg.ServiceUser, "mkdir", "-p", testPath).Run(); err == nil {
				logger.Info(fmt.Sprintf("🛠️  Auto-fixed traversal permissions for %s", parentDir), cfg)
			} else {
				return fmt.Errorf("permission check failed: user '%s' still cannot write to %s. Ensure parent directories have '+x' (execute) permission", cfg.ServiceUser, path)
			}
		} else {
			return fmt.Errorf("failed to auto-fix permissions for %s: %v", parentDir, err)
		}
	}

	// Success! We also ensure the newly created .cicdlog folder is owned by the user
	os.Chown(filepath.Join(path, ".cicdlog"), uid, gid)

	logger.Info(fmt.Sprintf("✅ Project folder prepared at %s for user %s", path, cfg.ServiceUser), cfg)
	return nil
}

// GrantSudoPrivileges creates a restricted sudoers entry for the service user.
// This allows the user to run only specific administrative commands (like apt and systemctl)
// without a password, enabling install.sh to manage dependencies safely.
func GrantSudoPrivileges(username string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	// Define the whitelist of commands the service user is allowed to run via sudo
	whitelist := "/usr/bin/apt, /usr/bin/apt-get, /usr/bin/systemctl, /usr/bin/service, /usr/bin/dnf, /usr/bin/zypper, /usr/bin/pacman, /usr/bin/yum, /usr/bin/git, /usr/bin/curl, /usr/bin/wget, /usr/bin/unzip, /usr/bin/tar, /usr/bin/node, /usr/bin/npm, /usr/bin/npx, /usr/bin/npm, /usr/bin/npx, /usr/bin/node_modules"
	content := fmt.Sprintf("%s ALL=(ALL) NOPASSWD: %s\n", username, whitelist)

	sudoersPath := fmt.Sprintf("/etc/sudoers.d/cicd-%s", username)

	// Write the restricted policy with 0440 permissions (required by sudo)
	err := os.WriteFile(sudoersPath, []byte(content), 0440)
	if err != nil {
		return fmt.Errorf("failed to write sudoers file: %v", err)
	}

	return nil
}

// RemoveSudoPrivileges deletes the restricted sudoers entry for the service user.
func RemoveSudoPrivileges(username string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	sudoersPath := fmt.Sprintf("/etc/sudoers.d/cicd-%s", username)
	return os.Remove(sudoersPath)
}

// GetFileHash calculates the SHA-256 hash of a file to detect changes.
func GetFileHash(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// RunAsUser drops root privileges to execute a command as a specific user
func RunAsUser(cfg *parser.Config, command string, args ...string) error {
	u, err := user.Lookup(cfg.ServiceUser)
	if err != nil {
		return err
	}

	uidInt, _ := strconv.Atoi(u.Uid)
	gidInt, _ := strconv.Atoi(u.Gid)

	cmd := exec.Command(command, args...)
	cmd.Dir = cfg.ServiceDir

	// Set environment variables for the user
	cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+u.Username)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Set platform-specific attributes (UID/GID on Linux/WSL)
	setPlatformAttributes(cmd, uint32(uidInt), uint32(gidInt))

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command '%s %v' failed: %v", command, args, err)
	}
	return nil
}

// SyncRepo ensures the project folder is up-to-date with the latest code
func SyncRepo(cfg *parser.Config) error {
	codebasePath := filepath.Join(cfg.ServiceDir, "codebase")

	// Ensure codebase directory exists
	if _, err := os.Stat(codebasePath); os.IsNotExist(err) {
		os.MkdirAll(codebasePath, 0755)
		// Change ownership to the service user so git can work
		u, _ := user.Lookup(cfg.ServiceUser)
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		os.Chown(codebasePath, uid, gid)
	}

	// 0. The "Safe Directory" Fix (Prevents Git Error 128)
	err := RunAsUser(cfg, "git", "config", "--global", "--add", "safe.directory", codebasePath)
	if err != nil {
		logger.Warn(fmt.Sprintf("⚠️ Failed to set safe.directory: %v", err), cfg)
	}

	// Prepare authenticated URL if credentials exist
	repoURL := cfg.RepoURL
	if cfg.GitUsername != "" && cfg.GitPassword != "" {
		// Replace https:// with https://user:pass@
		if after, ok := strings.CutPrefix(repoURL, "https://"); ok {
			repoURL = "https://" + cfg.GitUsername + ":" + cfg.GitPassword + "@" + after
		}
	}

	// Check if .git exists to decide between clone and pull
	if _, err := os.Stat(filepath.Join(codebasePath, ".git")); os.IsNotExist(err) {
		logger.Info(fmt.Sprintf("Cloning repository %s into codebase/...", repoURL), cfg)
		return RunAsUser(cfg, "git", "clone", repoURL, codebasePath)
	}

	logger.Info("Pulling latest changes from git...", cfg)
	// We need to tell RunAsUser to execute inside the codebase folder
	oldDir := cfg.ServiceDir
	cfg.ServiceDir = codebasePath
	defer func() { cfg.ServiceDir = oldDir }()

	return RunAsUser(cfg, "git", "pull", "origin", cfg.Branch)
}

// RunDeployment executes the install and run scripts within the project directory.
func RunDeployment(cfg *parser.Config) error {
	originalDir := cfg.ServiceDir
	codebasePath := filepath.Join(originalDir, "codebase")

	logPath := filepath.Join(originalDir, ".cicdlog", "deploy.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return fmt.Errorf("failed to open project log: %v", err)
	}
	defer logFile.Close()

	// 1. Run Install Script (if exists and changed)
	// We check inside codebase/.cicd/install.sh only
	installScript := filepath.Join(codebasePath, ".cicd", "install.sh")

	if _, err := os.Stat(installScript); err == nil {
		currentHash, _ := GetFileHash(installScript)
		hashFile := filepath.Join(originalDir, ".cicdlog", "install.hash")
		lastHash, _ := os.ReadFile(hashFile)

		if string(lastHash) == currentHash {
			logger.Info("⏩ install.sh hasn't changed. Skipping build step.", cfg)
		} else {
			// Pre-configure environment
			env := append(os.Environ(),
				"DEBIAN_FRONTEND=noninteractive",
				"SUDO_ASKPASS=/bin/false",
				"UCF_FORCE_CONFNEW=1",
				"PYTHONUNBUFFERED=1",
			)

			// DECISION: Choose the Privilege Path
			if cfg.SudoPass != "" {
				// PATH 1: Password-based Sudo (AskPass Wrapper)
				logger.Info("Using Password-based Sudo automation (AskPass)...", cfg)
				askPassPath := filepath.Join(originalDir, ".cicdlog", "askpass.sh")
				helperContent := fmt.Sprintf("#!/bin/bash\necho '%s'\n", cfg.SudoPass)
				os.WriteFile(askPassPath, []byte(helperContent), 0700)
				defer os.Remove(askPassPath)

				cfg.ServiceDir = codebasePath // Pivot to codebase for execution
				os.Setenv("SUDO_ASKPASS", askPassPath)
				err = RunAsUser(cfg, "sudo", "-A", "bash", installScript)
				cfg.ServiceDir = originalDir // Return to root
			} else {
				// PATH 2: Non-interactive/NOPASSWD Sudo
				logger.Info("Using JIT Whitelist (NOPASSWD) for installation...", cfg)
				logger.Info(fmt.Sprintf("script running as %s", cfg.ServiceUser), cfg)
				if err := GrantSudoPrivileges(cfg.ServiceUser); err != nil {
					logger.Error(fmt.Sprintf("⚠️ JIT Sudo failed: %v", err), cfg)
				} else {
					defer RemoveSudoPrivileges(cfg.ServiceUser)
				}
			}

			// Ensure the script is executable
			os.Chmod(installScript, 0755)

			cmd := exec.Command("bash", installScript)
			cmd.Dir = cfg.ServiceDir
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			cmd.Env = env

			// Drop privileges for script execution (it will use sudo inside if needed)
			u, _ := user.Lookup(cfg.ServiceUser)
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			setPlatformAttributes(cmd, uint32(uid), uint32(gid))

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("install.sh failed: %v (Check .cicdlog/deploy.log)", err)
			}

			// Store new hash
			os.WriteFile(hashFile, []byte(currentHash), 0644)
			logger.Info("✅ install.sh executed and hash updated", cfg)
		}
	} else {
		logger.Warn("⏩ install.sh not found. Skipping build step.", cfg)
		return fmt.Errorf("install.sh not found")
	}

	// 2. Start the application via the native Go Process Manager
	logger.Info("⚙️  Launching app via internal Go Process Engine...", cfg)
	if err := StartAppProcess(cfg); err != nil {
		return fmt.Errorf("process start failed: %v", err)
	}

	logger.Info("🚀 Application is now LIVE and managed by Go!", cfg)
	return nil
}

// RunCustomCommand executes a single command as the service user and returns the output
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

	// We use bash -c to allow for complex commands/pipes if needed,
	// but wrapped in RunAsUser for security.
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = cfg.ServiceDir

	// Set identity
	setPlatformAttributes(cmd, uint32(uid), uint32(gid))

	// Set environment
	cmd.Env = append(os.Environ(),
		"HOME="+u.HomeDir,
		"USER="+cfg.ServiceUser,
	)

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// isValidUsername ensures the username is safe for command line usage
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
