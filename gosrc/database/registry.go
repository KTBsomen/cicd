package database

import (
	"database/sql"
	"fmt"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

type User struct {
	ID        int
	Name      string
	Email     string
	ServiceID int
	CreatedAt time.Time
}

var DB *sql.DB

func InitDB(cfg *parser.Config) error {
	// Add this before sql.Open
	dbDir := "/etc/cicd/data"
	if _, err := os.Stat(dbDir); os.IsNotExist(err) {
		os.MkdirAll(dbDir, 0755)
	}

	var err error
	dbPath := filepath.Join(dbDir, "cicd.db")

	// 1. Open/Create SQLite DB
	DB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		logger.Error(parser.MustParseTemplate("Failed to open database:{{.err}}", map[string]any{
			"err": err,
		}), cfg)
		panic(err)
	}

	// 2. Create Table
	query := `
    CREATE TABLE IF NOT EXISTS users (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
		serviceName TEXT,
		serviceUser TEXT,
		repoURL TEXT,
		branch TEXT DEFAULT 'main',
		publicIp TEXT,
		webhook TEXT,
		adminEmail TEXT,
		serviceDir TEXT,
		lastCommitHash TEXT,
		lastCommitMsg TEXT,
		history TEXT DEFAULT '[]',

		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repoURL,branch,serviceName)
    );`

	_, err = DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create table: %v", err)
	}

	// 3. FIX PERMISSIONS (The Grounding Fix)
	// Change ownership of the DB file from root -> App User
	// This ensures the app (running as ServiceUser) can read/write to its own DB
	u, err := user.Lookup(cfg.ServiceUser)
	if err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		os.Chown(dbPath, uid, gid)
	}

	logger.Info("Database initialized", cfg)
	fmt.Println(dbPath)
	return nil
}
