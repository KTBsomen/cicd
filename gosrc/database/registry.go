package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
	ID             int       `json:"id"`
	ServiceName    string    `json:"service_name"`
	ServiceUser    string    `json:"service_user"`
	RepoURL        string    `json:"repo_url"`
	Branch         string    `json:"branch"`
	PublicIp       string    `json:"public_ip"`
	Webhook        string    `json:"webhook_port"`
	AdminEmail     string    `json:"admin_email"`
	ServiceDir     string    `json:"service_dir"`
	User           string    `json:"user"`
	LastCommitHash string    `json:"last_commit_hash"`
	LastCommitMsg  string    `json:"last_commit_msg"`
	History        any       `json:"history"`
	GithubToken    string    `json:"github_token"`
	GithubUsername string    `json:"github_username"`
	SudoPass       string    `json:"sudo_pass"`
	WebhookSecret  string    `json:"webhook_secret"`
	MongoDBURI     string    `json:"mongodb_uri"`
	IsPinned       bool      `json:"is_pinned"`
	DeployStatus   string    `json:"deploy_status"`
	CreatedAt      time.Time `json:"created_at"`
}

// CommitEntry represents a single deployment event in the commit history table
type CommitEntry struct {
	ID          int       `json:"id"`
	ProjectID   int       `json:"project_id"`
	CommitHash  string    `json:"commit_hash"`
	CommitMsg   string    `json:"commit_msg"`
	Status      string    `json:"status"`       // current | intermediate | rolled_back
	TriggeredBy string    `json:"triggered_by"` // webhook | manual | rollback | boot
	StartedAt   time.Time `json:"started_at"`
	FinishedAt  time.Time `json:"finished_at"`
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

	// Connection pool settings for memory efficiency
	DB.SetMaxOpenConns(1) // SQLite only supports one writer
	DB.SetMaxIdleConns(1)
	DB.SetConnMaxLifetime(0) // Keep open forever

	// 2. Create Tables (all columns defined here — no ALTER TABLE migrations)
	query := `
    CREATE TABLE IF NOT EXISTS users (
        id              INTEGER PRIMARY KEY AUTOINCREMENT,
		serviceName     TEXT,
		serviceUser     TEXT,
		repoURL         TEXT,
		branch          TEXT DEFAULT 'main',
		publicIp        TEXT,
		webhook         TEXT,
		adminEmail      TEXT,
		serviceDir      TEXT,
		user            TEXT,
		lastCommitHash  TEXT,
		lastCommitMsg   TEXT,
		githubToken     TEXT,
		githubUsername   TEXT,
		sudoPass        TEXT DEFAULT '',
		webhookSecret   TEXT DEFAULT '',
		mongodb_uri     TEXT DEFAULT '',
		history         TEXT DEFAULT '[]',
		is_pinned       INTEGER DEFAULT 0,
		deploy_status   TEXT DEFAULT 'idle',
		created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(repoURL,branch,serviceName)
    );
	CREATE TABLE IF NOT EXISTS settings (
		id               INTEGER PRIMARY KEY CHECK (id = 1),
		mongodb_uri      TEXT DEFAULT '',
		admin_email      TEXT DEFAULT '',
		webhook_port     INTEGER DEFAULT 9641,
		webhook_secret   TEXT DEFAULT '',
		smtp_host        TEXT DEFAULT '',
		smtp_port        INTEGER DEFAULT 587,
		smtp_user        TEXT DEFAULT '',
		smtp_pass        TEXT DEFAULT '',
		public_ip        TEXT DEFAULT '',
		notify_url       TEXT DEFAULT '',
		deploy_timeout   INTEGER DEFAULT 1800,
		jwt_secret       TEXT DEFAULT '',
		setup_token      TEXT DEFAULT '',
		setup_token_expires DATETIME,
		updated_at       DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS commit_history (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id    INTEGER NOT NULL,
		commit_hash   TEXT NOT NULL,
		commit_msg    TEXT DEFAULT '',
		status        TEXT DEFAULT 'intermediate',
		triggered_by  TEXT DEFAULT 'webhook',
		started_at    DATETIME,
		finished_at   DATETIME,
		FOREIGN KEY(project_id) REFERENCES users(id) ON DELETE CASCADE
	);`

	_, err = DB.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create tables: %v", err)
	}

	// Enable WAL mode for better concurrent read performance
	DB.Exec("PRAGMA journal_mode=WAL")
	DB.Exec("PRAGMA foreign_keys=ON")

	// 3. FIX PERMISSIONS (The Grounding Fix)
	// Change ownership of the DB file from root -> App User
	u, err := user.Lookup(cfg.ServiceUser)
	if err == nil {
		uid, _ := strconv.Atoi(u.Uid)
		gid, _ := strconv.Atoi(u.Gid)
		os.Chown(dbPath, uid, gid)
		// Also chown WAL and SHM files
		os.Chown(dbPath+"-wal", uid, gid)
		os.Chown(dbPath+"-shm", uid, gid)
	}

	logger.Info("Database initialized", cfg)
	return nil
}

// ═══════════════════════════════════════════════════════
//  GLOBAL SETTINGS — Save / Load / Setup Token
// ═══════════════════════════════════════════════════════

// SaveGlobalSettings stores the current configuration into the settings table
func SaveGlobalSettings(cfg *parser.Config) {
	query := `
	INSERT INTO settings (id, mongodb_uri, admin_email, webhook_port, webhook_secret, smtp_host, smtp_port, smtp_user, smtp_pass, public_ip, notify_url, deploy_timeout, updated_at)
	VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
	ON CONFLICT(id) DO UPDATE SET
		mongodb_uri=excluded.mongodb_uri,
		admin_email=excluded.admin_email,
		webhook_port=excluded.webhook_port,
		webhook_secret=excluded.webhook_secret,
		smtp_host=excluded.smtp_host,
		smtp_port=excluded.smtp_port,
		smtp_user=excluded.smtp_user,
		smtp_pass=excluded.smtp_pass,
		public_ip=excluded.public_ip,
		notify_url=excluded.notify_url,
		deploy_timeout=excluded.deploy_timeout,
		updated_at=CURRENT_TIMESTAMP;`

	_, err := DB.Exec(query, cfg.MongoDBURI, cfg.AdminEmail, cfg.Webhook, cfg.WebhookSecret, cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUser, cfg.SMTPPass, cfg.PublicIP, cfg.NotifyURL, cfg.DeployTimeout)
	if err != nil {
		logger.Error("Failed to save global settings: "+err.Error(), cfg)
	}
}

// LoadGlobalSettings restores the configuration from the settings table.
// B2 FIX: Uses parser.ExplicitFlags — SQLite values only override when the user
// did NOT explicitly pass that flag on the command line.
func LoadGlobalSettings(cfg *parser.Config) {
	query := `SELECT mongodb_uri, admin_email, webhook_port, webhook_secret, smtp_host, smtp_port, smtp_user, smtp_pass, public_ip, notify_url, deploy_timeout FROM settings WHERE id = 1`
	row := DB.QueryRow(query)

	var mongo, admin, secret, host, smtpUser, pass, ip, notifyURL string
	var port, deployTimeout, webhookport int

	err := row.Scan(&mongo, &admin, &webhookport, &secret, &host, &port, &smtpUser, &pass, &ip, &notifyURL, &deployTimeout)
	if err != nil {
		return // No settings row yet — first boot
	}

	// Track what we're overriding for the warning log
	var overrides []string

	// Helper: apply DB value unless user explicitly passed this flag
	apply := func(flagName string, dbVal string, cfgPtr *string) {
		if parser.ExplicitFlags[flagName] {
			if dbVal != "" && *cfgPtr != dbVal {
				overrides = append(overrides, fmt.Sprintf("  --%s: %s → %s (flag wins)", flagName, dbVal, *cfgPtr))
			}
			return // CLI flag wins
		}
		if dbVal != "" {
			*cfgPtr = dbVal
		}
	}

	apply("mongodb-uri", mongo, &cfg.MongoDBURI)
	apply("admin-email", admin, &cfg.AdminEmail)
	apply("webhook-secret", secret, &cfg.WebhookSecret)
	apply("smtp-host", host, &cfg.SMTPHost)
	apply("smtp-user", smtpUser, &cfg.SMTPUser)
	apply("smtp-pass", pass, &cfg.SMTPPass)
	apply("public-ip", ip, &cfg.PublicIP)
	apply("notify-url", notifyURL, &cfg.NotifyURL)

	// Int fields
	if !parser.ExplicitFlags["smtp-port"] && port > 0 {
		cfg.SMTPPort = port
	}
	if !parser.ExplicitFlags["deploy-timeout"] && deployTimeout > 0 {
		cfg.DeployTimeout = deployTimeout
	}
	if !parser.ExplicitFlags["webhook"] && webhookport > 0 {
		cfg.Webhook = webhookport
	}

	if len(overrides) > 0 {
		logger.Warn("⚠️  Overriding existing database config with provided flags:\n"+strings.Join(overrides, "\n"), cfg)
	}
}

// GetOrGenerateSetupToken checks if there is a valid, non-expired setup token in the database.
// If active, it returns it; otherwise, it generates and stores a new one.
func GetOrGenerateSetupToken() (string, error) {
	var token string
	var expiresStr sql.NullString
	err := DB.QueryRow(`SELECT setup_token, setup_token_expires FROM settings WHERE id = 1`).Scan(&token, &expiresStr)
	if err == nil && token != "" {
		if expiresStr.Valid {
			var expires time.Time
			for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05-07:00", "2006-01-02T15:04:05Z", "2006-01-02 15:04:05"} {
				if t, err := time.Parse(layout, expiresStr.String); err == nil {
					expires = t
					break
				}
			}
			if !expires.IsZero() && time.Now().Before(expires) {
				return token, nil // Still active, return it
			}
		}
	}
	return GenerateSetupToken()
}

// GenerateSetupToken creates a 32-char hex random token, stores it in SQLite with a 15-minute expiry.
// Returns the token string. Called on boot when AdminEmail is empty.
func GenerateSetupToken() (string, error) {
	b := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random token: %v", err)
	}
	token := hex.EncodeToString(b)
	expires := time.Now().Add(15 * time.Minute)

	_, err := DB.Exec(`UPDATE settings SET setup_token = ?, setup_token_expires = ? WHERE id = 1`, token, expires)
	if err != nil {
		// Settings row might not exist yet, insert it
		_, err = DB.Exec(`INSERT OR IGNORE INTO settings (id, setup_token, setup_token_expires) VALUES (1, ?, ?)`, token, expires)
		if err != nil {
			return "", err
		}
	}
	return token, nil
}

// ValidateSetupToken checks the token against the DB. Returns true if valid and not expired.
// On success, deletes the token (one-time use).
func ValidateSetupToken(token string) (bool, error) {
	var stored string
	var expiresStr sql.NullString

	err := DB.QueryRow(`SELECT setup_token, setup_token_expires FROM settings WHERE id = 1`).Scan(&stored, &expiresStr)
	if err != nil {
		return false, fmt.Errorf("no settings found")
	}

	if stored == "" || stored != token {
		return false, fmt.Errorf("invalid token")
	}

	if expiresStr.Valid {
		// Try multiple time formats since SQLite can store in different formats
		var expires time.Time
		for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05-07:00", "2006-01-02T15:04:05Z"} {
			if t, err := time.Parse(layout, expiresStr.String); err == nil {
				expires = t
				break
			}
		}
		if !expires.IsZero() && time.Now().After(expires) {
			return false, fmt.Errorf("token expired. Run 'cicd token' on server to get a new one")
		}
	}

	// One-time use: delete the token
	DB.Exec(`UPDATE settings SET setup_token = '', setup_token_expires = NULL WHERE id = 1`)
	return true, nil
}

// GetJWTSecret loads or generates the JWT signing secret
func GetJWTSecret() []byte {
	// 1. Check env var first
	if s := os.Getenv("CICD_JWT_SECRET"); s != "" {
		return []byte(s)
	}

	// 2. Try loading from DB
	var secret string
	err := DB.QueryRow(`SELECT jwt_secret FROM settings WHERE id = 1`).Scan(&secret)
	if err == nil && secret != "" {
		return []byte(secret)
	}

	// 3. Generate new random secret and persist it
	b := make([]byte, 32)
	rand.Read(b)
	secret = hex.EncodeToString(b)
	DB.Exec(`UPDATE settings SET jwt_secret = ? WHERE id = 1`, secret)
	// Ensure the row exists
	DB.Exec(`INSERT OR IGNORE INTO settings (id, jwt_secret) VALUES (1, ?)`, secret)
	return []byte(secret)
}

// ═══════════════════════════════════════════════════════
//  PROJECT CRUD
// ═══════════════════════════════════════════════════════

// RegisterProject saves the configuration from CLI/Dashboard into SQLite.
// It uses an "Upsert" (Insert or Update) so it doesn't create duplicates.
func RegisterProject(cfg *parser.Config) error {
	query := `
	INSERT INTO users (
		serviceName, serviceUser, repoURL, branch, publicIp, 
		webhook, adminEmail, serviceDir, user, githubToken, githubUsername, sudoPass, webhookSecret, mongodb_uri
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(repoURL, branch, serviceName) DO UPDATE SET
		serviceName=excluded.serviceName, serviceUser=excluded.serviceUser, publicIp=excluded.publicIp, 
		webhook=excluded.webhook, adminEmail=excluded.adminEmail, serviceDir=excluded.serviceDir, 
		user=excluded.user, githubToken=excluded.githubToken, githubUsername=excluded.githubUsername,
		sudoPass=excluded.sudoPass, webhookSecret=excluded.webhookSecret, mongodb_uri=excluded.mongodb_uri`

	_, err := DB.Exec(query,
		cfg.ServiceName,
		cfg.ServiceUser,
		cfg.RepoURL,
		cfg.Branch,
		cfg.PublicIP,
		strconv.Itoa(cfg.Webhook),
		cfg.AdminEmail,
		cfg.ServiceDir,
		cfg.User,
		cfg.GitPassword,
		cfg.GitUsername,
		cfg.SudoPass,
		cfg.WebhookSecret,
		cfg.MongoDBURI,
	)

	if err != nil {
		logger.Error(fmt.Sprintf("❌ DB Register Failed: %v", err), cfg)
		return err
	}

	logger.Info(fmt.Sprintf("📝 Project '%s' registered in SQLite", cfg.ServiceName), cfg)
	return nil
}

// UpdateProject modifies an existing project's configuration by ID.
func UpdateProject(id string, cfg *parser.Config) error {
	query := `
	UPDATE users SET 
		serviceName=?, serviceUser=?, repoURL=?, branch=?, publicIp=?, 
		webhook=?, adminEmail=?, serviceDir=?, user=?, githubToken=?, githubUsername=?,
		sudoPass=?, webhookSecret=?, mongodb_uri=?
	WHERE id=?`

	_, err := DB.Exec(query,
		cfg.ServiceName,
		cfg.ServiceUser,
		cfg.RepoURL,
		cfg.Branch,
		cfg.PublicIP,
		strconv.Itoa(cfg.Webhook),
		cfg.AdminEmail,
		cfg.ServiceDir,
		cfg.User,
		cfg.GitPassword,
		cfg.GitUsername,
		cfg.SudoPass,
		cfg.WebhookSecret,
		cfg.MongoDBURI,
		id,
	)

	if err != nil {
		logger.Error(fmt.Sprintf("❌ DB Update Failed: %v", err), cfg)
		return err
	}

	logger.Info(fmt.Sprintf("📝 Project '%s' updated in SQLite", cfg.ServiceName), cfg)
	return nil
}

// GetAllProjects reads every project from the database so the orchestrator can start them.
func GetAllProjects() ([]Project, error) {
	rows, err := DB.Query(`SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir, user,
		githubToken, githubUsername, COALESCE(sudoPass,''), COALESCE(webhookSecret,''), COALESCE(mongodb_uri,''),
		COALESCE(is_pinned,0), COALESCE(deploy_status,'idle'),
		COALESCE(lastCommitHash,''), COALESCE(lastCommitMsg,'') FROM users`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var pinned int
		err := rows.Scan(&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir, &p.User, &p.GithubToken, &p.GithubUsername, &p.SudoPass, &p.WebhookSecret, &p.MongoDBURI, &pinned, &p.DeployStatus, &p.LastCommitHash, &p.LastCommitMsg)
		if err != nil {
			continue
		}
		p.IsPinned = pinned != 0
		projects = append(projects, p)
	}
	return projects, nil
}

func GetProjectByID(id string) (*Project, error) {
	var p Project
	var pinned int
	err := DB.QueryRow(`
		SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir, user, githubToken, githubUsername, 
		COALESCE(sudoPass,''), COALESCE(webhookSecret,''), COALESCE(mongodb_uri,''),
		COALESCE(is_pinned,0), COALESCE(deploy_status,'idle') 
		FROM users WHERE id = ?`, id).Scan(
		&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir, &p.User, &p.GithubToken, &p.GithubUsername,
		&p.SudoPass, &p.WebhookSecret, &p.MongoDBURI,
		&pinned, &p.DeployStatus,
	)
	if err != nil {
		return nil, err
	}
	p.IsPinned = pinned != 0
	return &p, nil
}

func GetProjectByRepoURL(repoURL string) (*Project, error) {
	var p Project
	var pinned int
	err := DB.QueryRow(`
		SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir, user, githubToken, githubUsername, 
		COALESCE(sudoPass,''), COALESCE(webhookSecret,''), COALESCE(mongodb_uri,''),
		COALESCE(is_pinned,0), COALESCE(deploy_status,'idle') 
		FROM users WHERE repoURL = ?`, repoURL).Scan(
		&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir, &p.User, &p.GithubToken, &p.GithubUsername,
		&p.SudoPass, &p.WebhookSecret, &p.MongoDBURI,
		&pinned, &p.DeployStatus,
	)
	if err != nil {
		return nil, err
	}
	p.IsPinned = pinned != 0
	return &p, nil
}

func GetProjectByName(name string) (*Project, error) {
	var p Project
	var pinned int
	err := DB.QueryRow(`
		SELECT id, serviceName, serviceUser, repoURL, branch, publicIp, webhook, adminEmail, serviceDir, user, githubToken, githubUsername, 
		COALESCE(sudoPass,''), COALESCE(webhookSecret,''), COALESCE(mongodb_uri,''),
		COALESCE(is_pinned,0), COALESCE(deploy_status,'idle') 
		FROM users WHERE serviceName = ?`, name).Scan(
		&p.ID, &p.ServiceName, &p.ServiceUser, &p.RepoURL, &p.Branch, &p.PublicIp, &p.Webhook, &p.AdminEmail, &p.ServiceDir, &p.User, &p.GithubToken, &p.GithubUsername,
		&p.SudoPass, &p.WebhookSecret, &p.MongoDBURI,
		&pinned, &p.DeployStatus,
	)
	if err != nil {
		return nil, err
	}
	p.IsPinned = pinned != 0
	return &p, nil
}

// ToConfig converts a Project row to a runtime Config. Returns error instead of panic (IF-4).
// It also merges gateway-level settings (SMTP, notify, timeout, etc.) from the settings table
// so that every caller automatically gets a fully populated config.
func (p *Project) ToConfig() (*parser.Config, error) {
	wh, _ := strconv.Atoi(p.Webhook)
	var missing []string
	if p.ServiceDir == "" {
		missing = append(missing, "ServiceDir")
	}
	if p.RepoURL == "" {
		missing = append(missing, "RepoURL")
	}
	if p.ServiceName == "" {
		missing = append(missing, "ServiceName")
	}
	if p.ServiceUser == "" {
		missing = append(missing, "ServiceUser")
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("project %d is missing required fields: %s", p.ID, strings.Join(missing, ", "))
	}

	cfg := &parser.Config{
		ServiceName:   p.ServiceName,
		ServiceUser:   p.ServiceUser,
		User:          p.User,
		RepoURL:       p.RepoURL,
		PublicIP:      p.PublicIp,
		Webhook:       wh,
		Branch:        p.Branch,
		AdminEmail:    p.AdminEmail,
		ServiceDir:    p.ServiceDir,
		GitPassword:   p.GithubToken,
		GitUsername:   p.GithubUsername,
		SudoPass:      p.SudoPass,
		WebhookSecret: p.WebhookSecret,
		MongoDBURI:    p.MongoDBURI,
	}

	// Enrich with gateway-level settings from the settings table.
	// Only fills fields that are empty/zero in the per-project row.
	loadGlobalInto(cfg)

	return cfg, nil
}

// loadGlobalInto reads the single settings row and fills gateway-level fields
// into a per-project config — but only when the per-project value is empty/zero.
// This ensures SMTP, notifications, and timeouts are always available during deployment.
func loadGlobalInto(cfg *parser.Config) {
	if DB == nil {
		return
	}
	var smtpHost, smtpUser, smtpPass, notifyURL, adminEmail, publicIP, mongoURI, webhookSecret string
	var smtpPort, deployTimeout int
	err := DB.QueryRow(`
		SELECT COALESCE(smtp_host,''), COALESCE(smtp_port,587),
		       COALESCE(smtp_user,''), COALESCE(smtp_pass,''),
		       COALESCE(notify_url,''), COALESCE(admin_email,''),
		       COALESCE(deploy_timeout,1800), COALESCE(public_ip,''),
		       COALESCE(mongodb_uri,''), COALESCE(webhook_secret,'')
		FROM settings WHERE id = 1`).Scan(
		&smtpHost, &smtpPort, &smtpUser, &smtpPass,
		&notifyURL, &adminEmail, &deployTimeout, &publicIP,
		&mongoURI, &webhookSecret,
	)
	if err != nil {
		return // no settings row yet — first boot, skip silently
	}
	if cfg.SMTPHost      == "" { cfg.SMTPHost      = smtpHost }
	if cfg.SMTPPort      == 0  { cfg.SMTPPort      = smtpPort }
	if cfg.SMTPUser      == "" { cfg.SMTPUser      = smtpUser }
	if cfg.SMTPPass      == "" { cfg.SMTPPass      = smtpPass }
	if cfg.NotifyURL     == "" { cfg.NotifyURL     = notifyURL }
	if cfg.AdminEmail    == "" { cfg.AdminEmail    = adminEmail }
	if cfg.DeployTimeout == 0  { cfg.DeployTimeout = deployTimeout }
	if cfg.PublicIP      == "" { cfg.PublicIP      = publicIP }
	if cfg.MongoDBURI    == "" { cfg.MongoDBURI    = mongoURI }
	if cfg.WebhookSecret == "" { cfg.WebhookSecret = webhookSecret }
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

// ═══════════════════════════════════════════════════════
//  PINNING
// ═══════════════════════════════════════════════════════

func SetProjectPinned(id string, pinned bool) error {
	val := 0
	if pinned {
		val = 1
	}
	_, err := DB.Exec("UPDATE users SET is_pinned = ? WHERE id = ?", val, id)
	return err
}

func IsProjectPinned(id string) (bool, error) {
	var pinned int
	err := DB.QueryRow("SELECT COALESCE(is_pinned, 0) FROM users WHERE id = ?", id).Scan(&pinned)
	return pinned != 0, err
}

// ═══════════════════════════════════════════════════════
//  DEPLOY STATUS
// ═══════════════════════════════════════════════════════

func SetDeployStatus(id string, status string) error {
	_, err := DB.Exec("UPDATE users SET deploy_status = ? WHERE id = ?", status, id)
	return err
}

// SetDeployStatusByDir updates deploy status using serviceDir as the key
func SetDeployStatusByDir(serviceDir string, status string) error {
	_, err := DB.Exec("UPDATE users SET deploy_status = ? WHERE serviceDir = ?", status, serviceDir)
	return err
}

// GetDeployStatusByDir fetches deploy status using serviceDir as the key
func GetDeployStatusByDir(serviceDir string) (string, error) {
	var status string
	err := DB.QueryRow("SELECT COALESCE(deploy_status, 'idle') FROM users WHERE serviceDir = ?", serviceDir).Scan(&status)
	return status, err
}

// ═══════════════════════════════════════════════════════
//  COMMIT HISTORY
// ═══════════════════════════════════════════════════════

// AddCommitToHistory inserts a new commit entry and returns its row ID
func AddCommitToHistory(projectID int, hash, msg, triggeredBy string) (int64, error) {
	res, err := DB.Exec(
		`INSERT INTO commit_history (project_id, commit_hash, commit_msg, status, triggered_by, started_at)
		 VALUES (?, ?, ?, 'intermediate', ?, CURRENT_TIMESTAMP)`,
		projectID, hash, msg, triggeredBy,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetCurrentCommit marks the given hash as "current" and all others for this project as "intermediate"
func SetCurrentCommit(projectID int, hash string) error {
	tx, err := DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// 1. Mark all existing "current" entries as intermediate
	_, err = tx.Exec(`UPDATE commit_history SET status = 'intermediate' WHERE project_id = ? AND status = 'current'`, projectID)
	if err != nil {
		return fmt.Errorf("failed to reset current status: %v", err)
	}

	// 2. Mark this specific hash as current (using ID subquery for compatibility)
	query := `
		UPDATE commit_history SET status = 'current', finished_at = CURRENT_TIMESTAMP 
		WHERE id = (
			SELECT id FROM commit_history 
			WHERE project_id = ? AND commit_hash = ? AND status = 'intermediate' 
			ORDER BY id DESC LIMIT 1
		)`
	res, err := tx.Exec(query, projectID, hash)
	if err != nil {
		return fmt.Errorf("failed to set current status: %v", err)
	}

	rows, _ := res.RowsAffected()
	if rows == 0 {
		// Fallback: If for some reason it's not 'intermediate', just find the latest for this hash
		tx.Exec(`UPDATE commit_history SET status = 'current', finished_at = CURRENT_TIMESTAMP 
		         WHERE id = (SELECT id FROM commit_history WHERE project_id = ? AND commit_hash = ? ORDER BY id DESC LIMIT 1)`, 
				 projectID, hash)
	}

	return tx.Commit()
}

// MarkCommitRolledBack marks a specific commit as rolled_back
func MarkCommitRolledBack(projectID int, hash string) error {
	_, err := DB.Exec(`UPDATE commit_history SET status = 'rolled_back' WHERE project_id = ? AND commit_hash = ?`, projectID, hash)
	return err
}

// GetCommitHistory returns the commit history for a project, newest first
func GetCommitHistory(projectID int, limit int) ([]CommitEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := DB.Query(
		`SELECT id, project_id, commit_hash, commit_msg, status, triggered_by, 
		 COALESCE(started_at, ''), COALESCE(finished_at, '')
		 FROM commit_history WHERE project_id = ? ORDER BY id DESC LIMIT ?`,
		projectID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []CommitEntry
	for rows.Next() {
		var e CommitEntry
		var startStr, endStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.CommitHash, &e.CommitMsg, &e.Status, &e.TriggeredBy, &startStr, &endStr); err != nil {
			continue
		}
		// Parse times with best-effort
		e.StartedAt, _ = time.Parse("2006-01-02 15:04:05", startStr)
		e.FinishedAt, _ = time.Parse("2006-01-02 15:04:05", endStr)
		entries = append(entries, e)
	}
	return entries, nil
}

// GetPreviousCurrentCommit returns the commit hash of the most recent "current" entry before a given one
func GetPreviousCurrentCommit(projectID int) (string, error) {
	var hash string
	err := DB.QueryRow(
		`SELECT commit_hash FROM commit_history WHERE project_id = ? AND status = 'current' ORDER BY id DESC LIMIT 1`,
		projectID,
	).Scan(&hash)
	if err != nil {
		return "", err
	}
	return hash, nil
}

// GetProjectIDByDir returns the project ID for a given serviceDir
func GetProjectIDByDir(serviceDir string) (int, error) {
	var id int
	err := DB.QueryRow("SELECT id FROM users WHERE serviceDir = ?", serviceDir).Scan(&id)
	return id, err
}

// CheckPortConflict checks if any other project already uses the given webhook port (EC-16)
func CheckPortConflict(port int, excludeProjectID int) (string, error) {
	var name string
	err := DB.QueryRow(
		"SELECT serviceName FROM users WHERE webhook = ? AND id != ?",
		strconv.Itoa(port), excludeProjectID,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil // No conflict
	}
	if err != nil {
		return "", err
	}
	return name, nil // Conflict found
}

// UpdateCommitHashLocal updates the last commit hash in the users table
func UpdateCommitHashLocal(serviceDir string, hash string) error {
	_, err := DB.Exec("UPDATE users SET lastCommitHash = ? WHERE serviceDir = ?", hash, serviceDir)
	return err
}

// UpdateLastCommitInfo updates both the commit hash and message on the project row
func UpdateLastCommitInfo(serviceDir, hash, msg string) error {
	_, err := DB.Exec("UPDATE users SET lastCommitHash = ?, lastCommitMsg = ? WHERE serviceDir = ?", hash, msg, serviceDir)
	return err
}
