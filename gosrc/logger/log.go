package logger

import (
	"fmt"
	"gosrc/parser"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	systemLog *os.File
	once      sync.Once
	mu        sync.Mutex
)

const (
	maxLogSize     = 50 * 1024 * 1024 // 50MB
	maxLogRotation = 5                // Keep 5 rotated files
)

// InitLogger sets up the global system log directory and file
func InitLogger() {
	once.Do(func() {
		logDir := "/var/log/cicd"
		// Ensure system log directory exists
		if _, err := os.Stat(logDir); os.IsNotExist(err) {
			os.MkdirAll(logDir, 0755)
		}

		path := filepath.Join(logDir, "system.log")
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("❌ Failed to open system log: %v\n", err)
			return
		}
		systemLog = f
	})
}

func emit(level, color, msg string, cfg *parser.Config) {
	mu.Lock()
	defer mu.Unlock()

	timestamp := time.Now().Format("2006-01-02 15:04:05")
	serviceName := "SYSTEM"
	if cfg != nil && cfg.ServiceName != "" {
		serviceName = cfg.ServiceName
	}

	// 1. Console Output (With ANSI Colors)
	fmt.Printf("%s[%s]%s [%s] [%s] - %s\n", color, level, "\033[0m", timestamp, serviceName, msg)

	// 2. Global System Log
	if systemLog != nil {
		line := fmt.Sprintf("[%s] [%s] [%s] - %s\n", level, timestamp, serviceName, msg)
		systemLog.WriteString(line)
	}

	// 3. Project-Specific Log (If context is provided)
	if cfg != nil && cfg.ServiceDir != "" {
		projectLogDir := filepath.Join(cfg.ServiceDir, ".cicdlog")
		// Ensure project log dir exists (Safety Check)
		if _, err := os.Stat(projectLogDir); err == nil {
			projectLogPath := filepath.Join(projectLogDir, "deploy.log")
			f, err := os.OpenFile(projectLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
			if err == nil {
				line := fmt.Sprintf("[%s] [%s] - %s\n", level, timestamp, msg)
				f.WriteString(line)
				f.Close()
			}
		}
	}
}

// Color constants
const (
	colorInfo  = "\033[1;32m" // Green
	colorWarn  = "\033[1;33m" // Yellow
	colorError = "\033[1;31m" // Red
	colorReset = "\033[0m"
)

func Info(msg string, cfg *parser.Config)  { emit("INFO", colorInfo, msg, cfg) }
func Warn(msg string, cfg *parser.Config)  { emit("WARN", colorWarn, msg, cfg) }
func Error(msg string, cfg *parser.Config) { emit("ERROR", colorError, msg, cfg) }

// RotateLogIfNeeded checks if a log file exceeds maxLogSize and rotates it.
// Rotation: .log.4 deleted, .3→.4, .2→.3, .1→.2, .log→.1, new empty .log
func RotateLogIfNeeded(logPath string) {
	info, err := os.Stat(logPath)
	if err != nil || info.Size() < maxLogSize {
		return
	}

	// Rotate: delete oldest, shift others
	for i := maxLogRotation; i >= 1; i-- {
		src := fmt.Sprintf("%s.%d", logPath, i)
		if i == maxLogRotation {
			os.Remove(src) // Delete the oldest
			continue
		}
		dst := fmt.Sprintf("%s.%d", logPath, i+1)
		os.Rename(src, dst) // Shift: .3→.4, .2→.3, .1→.2
	}

	// Move current log to .1
	os.Rename(logPath, logPath+".1")

	// Create fresh empty log
	f, err := os.Create(logPath)
	if err == nil {
		f.Close()
	}
}
