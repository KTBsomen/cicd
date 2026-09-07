//go:build windows

package terminal

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

type PipeSession struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

// NewSession creates an interactive shell session on Windows
func NewSession(dir, username string) (Session, error) {
	// Look for powershell or cmd
	cmdName := "cmd.exe"
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		cmdName = "powershell.exe"
	}

	cmd := exec.Command(cmdName)
	if dir != "" {
		cmd.Dir = dir
	}

	// Prepend interceptor dir to PATH
	pathEnv := os.Getenv("PATH")
	if interceptorDir, err := GetInterceptorDir(); err == nil && interceptorDir != "" {
		pathEnv = interceptorDir + string(os.PathListSeparator) + pathEnv
	}
	cmd.Env = append(os.Environ(), "PATH="+pathEnv)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	cmd.Stderr = cmd.Stdout // combine stderr into stdout

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("failed to start process %s: %w", cmdName, err)
	}

	return &PipeSession{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func (s *PipeSession) Read(p []byte) (int, error) {
	return s.stdout.Read(p)
}

func (s *PipeSession) Write(p []byte) (int, error) {
	return s.stdin.Write(p)
}

func (s *PipeSession) Resize(cols, rows uint16) error {
	// No-op for basic Windows pipes
	return nil
}

func (s *PipeSession) Close() error {
	if s.stdin != nil {
		s.stdin.Close()
	}
	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}
	if s.stdout != nil {
		s.stdout.Close()
	}
	return nil
}
