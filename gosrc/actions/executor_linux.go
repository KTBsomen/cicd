// +build linux

package actions

import (
	"os/exec"
	"syscall"
)

// setPlatformAttributes configures the command to run with specific UID/GID on Linux
func setPlatformAttributes(cmd *exec.Cmd, uid uint32, gid uint32) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid:         uid,
			Gid:         gid,
			NoSetGroups: true, // Recommended to prevent "operation not permitted" errors
		},
	}
}

// prepareProcessGroup enables process group isolation for clean termination
func prepareProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup terminates the entire process group
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err == nil {
		syscall.Kill(-pgid, syscall.SIGKILL)
	}
}
