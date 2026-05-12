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

// SendErrorEmail is a placeholder for your alert system
