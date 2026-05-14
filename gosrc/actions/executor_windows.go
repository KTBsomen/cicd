// +build windows

package actions

import (
	"fmt"
	"os/exec"
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
