// +build windows

package actions

import "os/exec"

// setPlatformAttributes is a no-op on Windows as syscall.Credential is not supported
func setPlatformAttributes(cmd *exec.Cmd, uid uint32, gid uint32) {
	// Windows does not support UID/GID dropping via SysProcAttr.Credential
}
