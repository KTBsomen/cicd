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

type Project struct {
	ID             int
	ServiceName    string
	ServiceUser    string
	RepoURL        string
	Branch         string
	PublicIp       string
	Webhook        string
	AdminEmail     string
	ServiceDir     string
	LastCommitHash string
	LastCommitMsg  string
	History        any
	GithubToken    string
	GithubUsername string
	CreatedAt      time.Time
}

var DB *sql.DB

func InitDB(cfg *parser.Config) error {
	if DB != nil {
		return nil
	}
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
		githubToken TEXT,
		githubUsername TEXT,
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

// RegisterProject saves the configuration from CLI/Dashboard into SQLite.
// It uses an "Upsert" (Insert or Update) so it doesn't create duplicates.
func RegisterProject(cfg *parser.Config) error {
	query := `
	INSERT INTO users (
		serviceName, serviceUser, repoURL, branch, publicIp, 
		webhook, adminEmail, serviceDir, githubToken, githubUsername
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(repoURL, branch, serviceName) DO UPDATE SET
		serviceUser=excluded.serviceUser,
		publicIp=excluded.publicIp,
		adminEmail=excluded.adminEmail,
		serviceDir=excluded.serviceDir,
		githubToken=excluded.githubToken,
		githubUsername=excluded.githubUsername;`

	_, err := DB.Exec(query,
		cfg.ServiceName,
		cfg.ServiceUser,
		cfg.RepoURL,
		cfg.Branch,
		cfg.PublicIP,
		strconv.Itoa(cfg.Webhook),
		cfg.AdminEmail,
		cfg.ServiceDir,
		cfg.GitPassword,
		cfg.GitUsername,
	)

	if err != nil {
		logger.Error(fmt.Sprintf("❌ DB Register Failed: %v", err), cfg)
		return err
	}

	logger.Info(fmt.Sprintf("📝 Project '%s' registered in SQLite", cfg.ServiceName), cfg)
	return nil
}

// GetAllProjects reads every project from the database so the orchestrator can start them.
func GetAllProjects() ([]Project, error) {
	rows, err := DB.Query("SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir FROM users")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		err := rows.Scan(&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir)
		if err != nil {
			continue
		}
		projects = append(projects, p)
	}
	return projects, nil
}
func GetProjectByID(id string) (*Project, error) {
	var p Project
	err := DB.QueryRow("SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir FROM users WHERE id = ?", id).Scan(&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
func (p *Project) ToConfig() *parser.Config {
	wh, _ := strconv.Atoi(p.Webhook)
	errCount := 0
	if p.AdminEmail == "" {
		logger.Error("Admin Email is empty", nil)
		errCount++
	}
	if p.ServiceDir == "" {
		logger.Error("Service Dir is empty", nil)
		errCount++
	}
	if p.PublicIp == "" {
		logger.Warn("Public IP is empty", nil)
	}
	if p.RepoURL == "" {
		logger.Error("Repo URL is empty", nil)
		errCount++
	}
	if p.ServiceName == "" {
		logger.Error("Service Name is empty", nil)
		errCount++
	}
	if p.ServiceUser == "" {
		logger.Error("Service User is empty", nil)
		errCount++
	}

	if errCount > 0 {
		panic("Some Configs are empty")
	}
	return &parser.Config{
		ServiceName: p.ServiceName,
		ServiceUser: p.ServiceUser,
		RepoURL:     p.RepoURL,
		PublicIP:    p.PublicIp,
		Webhook:     wh,
		Branch:      p.Branch,
		AdminEmail:  p.AdminEmail,
		ServiceDir:  p.ServiceDir,
		GitPassword: p.GithubToken,
		GitUsername: p.GithubUsername,
	}
}

// GetAllDeploymentPaths retrieves all currently used project directories
func GetAllDeploymentPaths() ([]string, error) {

	rows, err := DB.Query("SELECT service_dir FROM projects")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil {
			paths = append(paths, p)
		}
	}
	return paths, nil
}
