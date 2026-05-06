package logger

import (
	"fmt"
	"gosrc/parser"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

var (
	logFile *os.File
	once    sync.Once
	mu      sync.Mutex
)

// InitLogger opens the file once and keeps it open for the life of the binary
func InitLogger(cfg *parser.Config) {
	once.Do(func() {
		path := filepath.Join(cfg.ServiceDir, "deployment.log")

		// 1. Open/Create as Root
		f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Printf("❌ Failed to open log: %v\n", err)
			return
		}
		logFile = f

		// 2. Hand over the keys to the Service User
		// This is the "Grounded" fix: Root uses its power to let the User write
		u, err := user.Lookup(cfg.ServiceUser)
		if err == nil {
			uid, _ := strconv.Atoi(u.Uid)
			gid, _ := strconv.Atoi(u.Gid)

			// Change ownership of the log file so the App can write to it
			os.Chown(path, uid, gid)
		}
	})
}

func emit(level, color, msg string, cfg *parser.Config) {
	mu.Lock()
	defer mu.Unlock()
	if cfg == nil {
		cfg = &parser.Config{
			ServiceName: "System",
		}
	}
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	// 1. Console Output (With ANSI Colors)
	// \033[1;32m = Bold Green, \033[0m = Reset
	fmt.Printf("%s[%s]%s [%s] [%s] - %s\n", color, level, "\033[0m", timestamp, cfg.ServiceName, msg)

	// 2. File Output (Plain text, no colors)
	if logFile != nil {
		line := fmt.Sprintf("[%s] [%s] [%s] - %s\n", level, timestamp, cfg.ServiceName, msg)
		logFile.WriteString(line)
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
