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
