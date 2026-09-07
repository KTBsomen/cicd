package terminal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Session represents an interactive terminal session (PTY on Linux, pipe fallback on Windows)
type Session interface {
	io.Reader
	io.Writer
	Resize(cols, rows uint16) error
	Close() error
}

var (
	interceptorDirOnce   sync.Once
	cachedInterceptorDir string
)

// GetInterceptorDir creates and returns the directory containing shim scripts for
// nano, vim, vi, edit, pico. When executed in the terminal session, these shims
// output a specific token with the resolved file path so the web UI intercepts it.
func GetInterceptorDir() (string, error) {
	var initErr error
	interceptorDirOnce.Do(func() {
		// User-scoped temp directory to avoid permission conflicts across different users
		dir := filepath.Join(os.TempDir(), fmt.Sprintf("cicd_term_interceptors_%d", os.Getuid()))
		if err := os.MkdirAll(dir, 0755); err != nil {
			initErr = err
			return
		}

		// Shell script content for Linux/Unix
		scriptContent := `#!/bin/sh
if [ -z "$1" ]; then
    echo "Usage: $0 <filename>"
    exit 1
fi
TARGET="$1"
if [ -d "$TARGET" ]; then
    echo "Cannot edit directory: $TARGET"
    exit 1
fi
# Resolve path safely
REAL_PATH=""
if command -v realpath >/dev/null 2>&1; then
    REAL_PATH=$(realpath -m "$TARGET" 2>/dev/null)
elif command -v readlink >/dev/null 2>&1; then
    REAL_PATH=$(readlink -f "$TARGET" 2>/dev/null)
fi
if [ -z "$REAL_PATH" ]; then
    case "$TARGET" in
        /*) REAL_PATH="$TARGET" ;;
        *)  REAL_PATH="$(pwd)/$TARGET" ;;
    esac
fi
printf "\r\n___CICD_EDIT_FILE___:%s\r\n" "$REAL_PATH"
`

		commands := []string{"nano", "vim", "vi", "edit", "pico"}
		for _, cmd := range commands {
			filePath := filepath.Join(dir, cmd)
			if err := os.WriteFile(filePath, []byte(scriptContent), 0755); err != nil {
				initErr = fmt.Errorf("failed to write interceptor script %s: %w", cmd, err)
				return
			}
		}

		// Also write a .bat/.cmd wrapper for Windows cmd testing
		batchContent := `@echo off
if "%~1"=="" (
    echo Usage: %~n0 ^<filename^>
    exit /b 1
)
echo ___CICD_EDIT_FILE___:%~f1
`
		for _, cmd := range commands {
			batchPath := filepath.Join(dir, cmd+".bat")
			_ = os.WriteFile(batchPath, []byte(batchContent), 0755)
			cmdPath := filepath.Join(dir, cmd+".cmd")
			_ = os.WriteFile(cmdPath, []byte(batchContent), 0755)
		}

		cachedInterceptorDir = dir
	})

	return cachedInterceptorDir, initErr
}
