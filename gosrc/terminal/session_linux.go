//go:build linux

package terminal

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"syscall"
	"time"

	"github.com/creack/pty"
)

type PTYSession struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// NewSession creates an interactive PTY session on Linux running as the project's ServiceUser
func NewSession(dir, username string) (Session, error) {
	shell := "/bin/bash"
	if _, err := os.Stat(shell); err != nil {
		shell = "/bin/sh"
	}

	cmd := exec.Command(shell)
	if dir != "" {
		cmd.Dir = dir
	}

	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		homeDir = "/tmp"
	}

	// Privilege separation: Only drop privileges if currently running as root (EUID == 0)
	// Non-root processes cannot setuid/setgid and will fail with EPERM (operation not permitted)
	if os.Geteuid() == 0 && username != "" {
		if u, err := user.Lookup(username); err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)
			if u.HomeDir != "" {
				homeDir = u.HomeDir
			}
			if uid != 0 {
				cmd.SysProcAttr = &syscall.SysProcAttr{
					Credential: &syscall.Credential{
						Uid:         uint32(uid),
						Gid:         uint32(gid),
						NoSetGroups: true,
					},
				}
			}
		}
	}

	// Prepend interceptor shims (nano, vim, etc.) to PATH
	pathEnv := os.Getenv("PATH")
	if interceptorDir, err := GetInterceptorDir(); err == nil && interceptorDir != "" {
		pathEnv = interceptorDir + ":" + pathEnv
	}

	cmd.Env = []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"HOME=" + homeDir,
		"USER=" + username,
		"PATH=" + pathEnv,
		"CICD_TERMINAL=1",
		"SHLVL=1",
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to start pty: %w", err)
	}

	// Set initial standard terminal window size (80x24)
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})

	return &PTYSession{
		ptmx: ptmx,
		cmd:  cmd,
	}, nil
}

func (s *PTYSession) Read(p []byte) (int, error) {
	return s.ptmx.Read(p)
}

func (s *PTYSession) Write(p []byte) (int, error) {
	return s.ptmx.Write(p)
}

func (s *PTYSession) Resize(cols, rows uint16) error {
	if s.ptmx == nil {
		return nil
	}
	return pty.Setsize(s.ptmx, &pty.Winsize{
		Rows: rows,
		Cols: cols,
	})
}

func (s *PTYSession) Close() error {
	if s.cmd != nil && s.cmd.Process != nil {
		pid := s.cmd.Process.Pid
		pgid, err := syscall.Getpgid(pid)
		if err == nil {
			// Gracefully terminate entire process group
			syscall.Kill(-pgid, syscall.SIGTERM)
			go func() {
				time.Sleep(1 * time.Second)
				syscall.Kill(-pgid, syscall.SIGKILL)
			}()
		} else {
			s.cmd.Process.Kill()
		}
	}

	if s.ptmx != nil {
		return s.ptmx.Close()
	}
	return nil
}
