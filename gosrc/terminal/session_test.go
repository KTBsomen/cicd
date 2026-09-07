package terminal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetInterceptorDir(t *testing.T) {
	dir, err := GetInterceptorDir()
	if err != nil {
		t.Fatalf("GetInterceptorDir failed: %v", err)
	}

	if dir == "" {
		t.Fatal("Expected non-empty interceptor dir")
	}

	commands := []string{"nano", "vim", "vi", "edit", "pico"}
	for _, cmd := range commands {
		p := filepath.Join(dir, cmd)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("Interceptor file %s does not exist: %v", cmd, err)
			continue
		}
		if info.IsDir() {
			t.Errorf("Interceptor file %s is a directory", cmd)
		}
	}
}

func TestTerminalSessionExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "cicd_term_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	sess, err := NewSession(tempDir, "")
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	defer sess.Close()

	// Write a simple command
	_, err = sess.Write([]byte("echo HELLO_CICD\n"))
	if err != nil {
		t.Fatalf("Failed to write to session: %v", err)
	}

	buf := make([]byte, 1024)
	gotOutput := false
	deadline := time.Now().Add(5 * time.Second)

	for time.Now().Before(deadline) {
		n, err := sess.Read(buf)
		if n > 0 {
			out := string(buf[:n])
			if strings.Contains(out, "HELLO_CICD") {
				gotOutput = true
				break
			}
		}
		if err != nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !gotOutput {
		t.Errorf("Did not receive expected HELLO_CICD in session output within deadline")
	}
}
