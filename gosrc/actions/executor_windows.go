//go:build windows

package actions

import (
	"fmt"
	"gosrc/parser"
	"net/http"
	"os/exec"
	"strings"
)

// setPlatformAttributes is a no-op on Windows as syscall.Credential is not supported
func setPlatformAttributes(cmd *exec.Cmd, uid uint32, gid uint32) {
	// Windows does not support UID/GID dropping via SysProcAttr.Credential
}

// prepareProcessGroup is a no-op on Windows as process groups work differently
func prepareProcessGroup(cmd *exec.Cmd) {
	// No specific SysProcAttr needed for basic tree killing via taskkill
}

// killProcessGroup terminates the entire process tree using taskkill
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	// /F = Force, /T = Tree (children), /PID = Process ID
	killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", cmd.Process.Pid))
	killCmd.Run()
}

// isProcessAlive checks if a PID is still running on Windows
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), fmt.Sprintf("%d", pid))
}

// getChildrenInGroup is not applicable on Windows — returns empty
func getChildrenInGroup(pgid int) []int {
	return nil
}

// sendNotification sends a POST to the configured notify URL
func sendNotification(cfg *parser.Config, msg string) {
	if cfg.NotifyURL == "" {
		return
	}
	payload := fmt.Sprintf(`{"text": "%s"}`, strings.ReplaceAll(msg, `"`, `\"`))
	resp, err := http.Post(cfg.NotifyURL, "application/json", strings.NewReader(payload))
	if err != nil {
		return
	}
	resp.Body.Close()
}

// checkDiskSpace is a no-op on Windows for now
func checkDiskSpace(path string) error {
	return nil
}
