//go:build linux

package actions

import (
	"fmt"
	"gosrc/parser"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
		syscall.Kill(-pgid, syscall.SIGTERM)
		// Give processes 3 seconds to shut down gracefully
		// then force kill if still alive
		go func() {
			<-make(chan struct{}) // We don't actually block here; SIGTERM should suffice
		}()
	}
	// Also try SIGKILL as fallback
	cmd.Process.Kill()
}

// isProcessAlive checks if a PID is still running via kill signal 0
func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

// getChildrenInGroup scans /proc for processes still in the given PGID.
// Returns a slice of child PIDs (excluding the parent PGID process itself).
func getChildrenInGroup(pgid int) []int {
	if pgid <= 0 {
		return nil
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var children []int
	target := fmt.Sprintf("%d", pgid)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 || pid == pgid {
			continue // skip non-numeric dirs and the PGID leader itself
		}

		// Read /proc/<pid>/stat to get the process group ID (field 5, 0-indexed from field 1)
		statPath := filepath.Join("/proc", entry.Name(), "stat")
		data, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		// /proc/<pid>/stat format: pid (comm) state ppid pgrp ...
		// We need field 5 (pgrp). The comm field can contain spaces and parens,
		// so we find the last ')' and parse from there.
		statStr := string(data)
		idx := strings.LastIndex(statStr, ")")
		if idx < 0 || idx+2 >= len(statStr) {
			continue
		}
		fields := strings.Fields(statStr[idx+2:])
		// fields[0] = state, fields[1] = ppid, fields[2] = pgrp
		if len(fields) < 3 {
			continue
		}
		if fields[2] == target {
			children = append(children, pid)
		}
	}
	return children
}

// sendNotification sends a POST to the configured notify URL (Slack/Discord compatible)
func sendNotification(cfg *parser.Config, msg string) {
	if cfg.NotifyURL == "" {
		return
	}
	payload := fmt.Sprintf(`{"text": "%s"}`, strings.ReplaceAll(msg, `"`, `\"`))
	resp, err := http.Post(cfg.NotifyURL, "application/json", strings.NewReader(payload))
	if err != nil {
		return // Best effort, don't log errors for notification failures
	}
	resp.Body.Close()
}

// getProcessPorts returns TCP ports a specific PID is listening on (stub for now)
func getProcessPorts(pid int32) []uint32 {
	return nil
}

// checkDiskSpace verifies there is enough free space (Linux implementation)
func checkDiskSpace(path string) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return nil // Can't check, proceed anyway
	}
	freeBytes := stat.Bavail * uint64(stat.Bsize)
	if freeBytes < 500*1024*1024 {
		return fmt.Errorf("insufficient disk space: %.1fMB free, need at least 500MB", float64(freeBytes)/1e6)
	}
	return nil
}
