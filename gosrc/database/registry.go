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
	"strings"
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
    );
	CREATE TABLE IF NOT EXISTS settings (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		mongodb_uri TEXT,
		admin_email TEXT,
		webhook_secret TEXT,
		smtp_host TEXT,
		smtp_port INTEGER,
		smtp_user TEXT,
		smtp_pass TEXT,
		public_ip TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

	_, err = DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tables: %v", err)
	}

	// 3. FIX PERMISSIONS (The Grounding Fix)
	// Change ownership of the DB file from root -> App User
	u, err := user.Lookup(cfg.ServiceUser)
	if err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		os.Chown(dbPath, uid, gid)
	}

	logger.Info("Database initialized", cfg)
	return nil
}

// SaveGlobalSettings stores the current configuration into the settings table
func SaveGlobalSettings(cfg *parser.Config) {
	query := `
	INSERT INTO settings (id, mongodb_uri, admin_email, webhook_secret, smtp_host, smtp_port, smtp_user, smtp_pass, public_ip, updated_at)
	VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		mongodb_uri=excluded.mongodb_uri,
		admin_email=excluded.admin_email,
		webhook_secret=excluded.webhook_secret,
		smtp_host=excluded.smtp_host,
		smtp_port=excluded.smtp_port,
		smtp_user=excluded.smtp_user,
		smtp_pass=excluded.smtp_pass,
		public_ip=excluded.public_ip,
		updated_at=CURRENT_TIMESTAMP;`

	_, err := DB.Exec(query, cfg.MongoDBURI, cfg.AdminEmail, cfg.WebhookSecret, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.PublicIP)
	if err != nil {
		logger.Error("Failed to save global settings: "+err.Error(), cfg)
	}
}

// LoadGlobalSettings restores the configuration from the settings table
func LoadGlobalSettings(cfg *parser.Config) {
	query := `SELECT mongodb_uri, admin_email, webhook_secret, smtp_host, smtp_port, smtp_user, smtp_pass, public_ip FROM settings WHERE id = 1`
	row := DB.QueryRow(query)

	var mongo, admin, secret, host, user, pass, ip string
	var port int

	err := row.Scan(&mongo, &admin, &secret, &host, &port, &user, &pass, &ip)
	if err == nil {
		// Only override if the current config is using defaults or empty
		if cfg.MongoDBURI == "" || strings.Contains(cfg.MongoDBURI, "default-url") || strings.Contains(cfg.MongoDBURI, "cicd.vlqm19g") {
			cfg.MongoDBURI = mongo
		}
		if cfg.AdminEmail == "" {
			cfg.AdminEmail = admin
		}
		if cfg.WebhookSecret == "" {
			cfg.WebhookSecret = secret
		}
		if cfg.SMTPHost == "" || cfg.SMTPHost == "smtpout.secureserver.net" || cfg.SMTPHost == "smtp.gmail.com" {
			cfg.SMTPHost = host
		}
		if cfg.SMTPPort == 0 || cfg.SMTPPort == 465 || cfg.SMTPPort == 587 {
			cfg.SMTPPort = port
		}
		if cfg.SMTPUser == "" {
			cfg.SMTPUser = user
		}
		if cfg.SMTPPass == "" {
			cfg.SMTPPass = pass
		}
		if cfg.PublicIP == "" {
			cfg.PublicIP = ip
		}
	}
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
func GetProjectByRepoURL(repoURL string) (*Project, error) {
	var p Project
	err := DB.QueryRow("SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir FROM users WHERE repoURL = ?", repoURL).Scan(&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func GetProjectByName(name string) (*Project, error) {
	var p Project
	err := DB.QueryRow("SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir FROM users WHERE serviceName = ?", name).Scan(&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir)
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

	rows, err := DB.Query("SELECT serviceDir FROM users")
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
func DeleteProject(id string) error {
	_, err := DB.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}
