package parser

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type Config struct {
	// Required
	Setup      string
	RepoURL    string
	Branch     string
	AdminEmail string

	// MongoDB
	MongoDBURI string

	// Git Credentials
	GitUsername string
	GitPassword string

	// SMTP
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	User     string
	SudoPass string

	// Service
	ServiceName  string
	ServiceDir   string
	ServiceUser  string
	ServiceReset string

	// Webhook
	Webhook       int
	WebhookSecret string

	// Network
	PublicIP string

	// Notifications
	NotifyURL string

	// Deployment
	DeployTimeout int // seconds, default 1800 (30 min)
}

// ExplicitFlags tracks which flags were explicitly provided on the command line.
// Used by LoadGlobalSettings to decide: "only override from SQLite if user didn't pass this flag."
var ExplicitFlags = map[string]bool{}

// MustCompile is preferred for global variables; it panics if the regex is invalid
var emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)

func isValid(email string) bool {
	return emailRegex.MatchString(email)
}

// generate secret key
func GenerateSecretKey() string {
	b := make([]byte, 16) // 16 bytes = 32 hex chars
	if _, err := rand.Read(b); err != nil {
		fmt.Println("failed to generate random secret key: ", err)
		return ""
	}
	return hex.EncodeToString(b)
}

// GetPublicIP attempts to find the public IP address of the machine.
func GetPublicIP() string {
	// 1. Try ipify API
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err == nil && resp.StatusCode == http.StatusOK {
		defer resp.Body.Close()
		if ip, err := io.ReadAll(resp.Body); err == nil {
			return strings.TrimSpace(string(ip))
		}
	}

	// 2. Fallback to Socket
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	return ""
}

// Parse flags receiver method for Config struct
func (c *Config) Parse() {
	// 1. Setup Categorized "Mini Manual" Documentation
	flag.Usage = func() {
		fmt.Println(text.FgHiCyan.Sprint("\n🛰️  CICD ORCHESTRATOR MANAGER"))
		fmt.Println(text.Faint.Sprint("High-performance Go-based automated deployment engine\n"))

		fmt.Println(text.Bold.Sprint("USAGE:"))
		fmt.Printf("  %s %s\n\n", text.FgHiYellow.Sprint("cicd"), text.Faint.Sprint("--repo-url [url] [options...]"))

		// --- SECTION 1: MANDATORY REQUIREMENTS ---
		fmt.Println(text.BgRed.Sprint(text.FgWhite.Sprint(" ⚠️  MANDATORY REQUIREMENTS ")))
		tReq := table.NewWriter()
		tReq.AppendHeader(table.Row{"Flag", "Description", "Default"})
		tReq.AppendRows([]table.Row{
			{"--mongodb-uri", "Central sync URL (Must be same on all nodes)", "mongodb+srv://..."},
			{"--repo-url", "Git Repository URL", "https://github.com/user/app"},
			{"--admin-email", "System alert recipient", "admin@domain.com"},
			{"--webhook", "Port for GitHub webhook listener", "9641"},

			{"--webhook-secret", "HMAC secret for security", "my_secret_key"},
		})
		renderTable(tReq, text.FgHiRed)

		// --- SECTION 2: SOURCE CONTROL ---
		printCategory("📦 SOURCE CONTROL", []table.Row{
			{"--branch", "Branch to monitor (Default: main)", "production"},
			{"--git-username", "Username for private git access", "myuser"},
			{"--git-password", "Token/Pass for private git access", "ghp_123..."},
		}, text.FgHiCyan)

		// --- SECTION 3: SYSTEM & PRIVILEGES ---
		printCategory("⚙️  SYSTEM & SERVICE", []table.Row{
			{"--setup", "Set to 'run' for debug or 'service' for systemd", "run"},
			{"--service-name", "Unique ID for the service file", "api-server"},
			{"--service-user", "User that will own the code files", "www-data"},
			{"--service-dir", "Base path for deployment (Default: /home/)", "/var/www/"},
			{"--user", "Operating system user for execution", "ubuntu"},
			{"--sudo-pass", "Sudo password for package management", "********"},
			{"--service-reset", "True/False: Wipe systemd and restart", "False"},
		}, text.FgHiMagenta)

		// --- SECTION 4: WEBHOOK & ALERTS ---
		printCategory("🔔 WEBHOOK & ALERTS", []table.Row{
			{"--smtp-host", "SMTP server for sending alerts", "smtp.gmail.com"},
			{"--smtp-port", "Port for SMTP server", "587"},
			{"--smtp-user", "Username for SMTP authentication", "alerts@domain.com"},
			{"--smtp-pass", "Password for SMTP authentication", "********"},
			{"--public-ip", "Manual Public IP (Auto-detected if empty)", "1.2.3.4"},
			{"--notify-url", "Slack/Discord webhook for failure alerts", "https://hooks.slack.com/..."},
			{"--deploy-timeout", "Max seconds for install.sh execution", "1800"},
		}, text.FgHiYellow)

		// --- SECTION 5: EXAMPLES ---
		fmt.Println(text.Bold.Sprint("💡 EXAMPLES & USE CASES"))
		tEx := table.NewWriter()
		tEx.AppendRows([]table.Row{
			{text.FgGreen.Sprint("Local Debug"), "cicd --setup run --repo-url http://..."},
			{text.FgGreen.Sprint("Production"), "cicd --setup service --repo-url https://... --webhook 9641"},
		})
		renderTable(tEx, text.FgHiGreen)
	}

	// 2. Define Flags
	flag.StringVar(&c.Setup, "setup", "", "debug mode testing use RUN so bypass systemd")
	flag.StringVar(&c.RepoURL, "repo-url", "", "Repository URL for the code")
	flag.StringVar(&c.Branch, "branch", "main", "Branch for the code")
	flag.StringVar(&c.AdminEmail, "admin-email", "", "Admin email to send error logs")
	flag.StringVar(&c.MongoDBURI, "mongodb-uri", "", "MongoDB URI for change monitoring")
	flag.StringVar(&c.GitUsername, "git-username", "", "Git username for private repos")
	flag.StringVar(&c.GitPassword, "git-password", "", "Git password/token for private repos")
	flag.StringVar(&c.SMTPHost, "smtp-host", "", "SMTP host")
	flag.IntVar(&c.SMTPPort, "smtp-port", 587, "SMTP port")
	flag.StringVar(&c.SMTPUser, "smtp-user", "", "SMTP username")
	flag.StringVar(&c.SMTPPass, "smtp-pass", "", "SMTP password")
	flag.StringVar(&c.User, "user", "", "Username of the code runner")
	flag.StringVar(&c.SudoPass, "sudo-pass", "", "Sudo password for package installation")
	flag.StringVar(&c.ServiceName, "service-name", "mycicdapp", "Name of the service")
	flag.StringVar(&c.ServiceDir, "service-dir", "/home/", "Path of the service")
	flag.StringVar(&c.ServiceUser, "service-user", "", "Name of the service user")
	flag.StringVar(&c.ServiceReset, "service-reset", "False", "True/False to delete systemd and restart")
	flag.IntVar(&c.Webhook, "webhook", 9641, "Port number for webhook listener")
	flag.StringVar(&c.WebhookSecret, "webhook-secret", "", "GitHub Webhook secret")
	flag.StringVar(&c.PublicIP, "public-ip", "", "Public IP for management")
	flag.StringVar(&c.NotifyURL, "notify-url", "", "Slack/Discord webhook URL for failure alerts")
	flag.IntVar(&c.DeployTimeout, "deploy-timeout", 1800, "Max seconds for install.sh (default 30 min)")

	flag.Parse()

	// 3. Build the ExplicitFlags set AFTER Parse() — flag.Visit only visits explicitly-set flags
	flag.Visit(func(f *flag.Flag) {
		ExplicitFlags[f.Name] = true
	})

	// 4. Run Visual Validation
	c.validateRequirements()

	if c.PublicIP == "" {
		c.PublicIP = GetPublicIP()
	}
	if c.RepoURL == "" && c.WebhookSecret == "" {
		c.WebhookSecret = GenerateSecretKey()
	}
}

// printCategory is a helper to render a small table for each section
func printCategory(title string, rows []table.Row, color text.Color) {
	fmt.Println(text.Bold.Sprint(title))
	t := table.NewWriter()
	t.AppendHeader(table.Row{"Flag", "Description", "Example"})

	for _, row := range rows {
		t.AppendRow(row)
		t.AppendSeparator()
	}

	renderTable(t, color)
}

func renderTable(t table.Writer, color text.Color) {
	t.SetAllowedRowLength(120)
	style := table.StyleRounded
	style.Color.Header = text.Colors{color, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	fmt.Println(t.Render())
	fmt.Println()
}

func (c *Config) validateRequirements() {
	// If no RepoURL is provided, we assume the user wants to start in "Gateway Mode"
	// (Dashboard & Webhook listener only). We skip validation in this case.
	if c.RepoURL == "" {
		fmt.Println(text.FgHiCyan.Sprint("🛰️  Starting in GATEWAY MODE (Passive Management)"))
		fmt.Println(text.Faint.Sprint("No deployment flags provided. Use the Dashboard to create projects.\n"))
		if c.MongoDBURI == "" {
			fmt.Println(text.FgHiYellow.Sprint("⚠️ MongoDB URI is missing. Running Local only mode."))
			fmt.Println(text.Faint.Sprint("To enable multi server fleet sync consider adding a mongodb atlas url.\n"))
			fmt.Println(text.Faint.Sprint("MongoDB atlas provides free tier which is enough for this tool.\nSign up here : https://www.mongodb.com/cloud/atlas/register"))

		}

		return
	}

	var missing []string
	if c.MongoDBURI == "" {
		fmt.Println(text.FgHiYellow.Sprint("⚠️ MongoDB URI is missing. Running Local only mode."))
		fmt.Println(text.Faint.Sprint("To enable multi server fleet sync consider adding a mongodb atlas url.\n"))
		fmt.Println(text.Faint.Sprint("MongoDB atlas provides free tier which is enough for this tool.\nSign up here : https://www.mongodb.com/cloud/atlas/register"))

	}
	if c.AdminEmail == "" {
		missing = append(missing, "--admin-email")
	}
	if c.WebhookSecret == "" {
		missing = append(missing, "--webhook-secret")
	}

	if len(missing) > 0 {
		fmt.Println(text.FgHiRed.Sprint("\n❌ CONFIGURATION ERROR"))
		fmt.Println(text.Faint.Sprint("The following required flags were not found or are empty:"))

		for _, m := range missing {
			fmt.Printf("  %s %s\n", text.FgRed.Sprint("•"), text.Bold.Sprint(m))
		}

		fmt.Println(text.Bold.Sprint("\nACTION REQUIRED:"))
		fmt.Printf("  Run the command with the missing flags or use %s for full documentation.\n\n", text.FgHiCyan.Sprint("--help"))
		os.Exit(1)
	}

	// Email Validations
	if !isValid(c.AdminEmail) {
		fmt.Printf("\n%s --admin-email is not a valid email address.\n", text.FgRed.Sprint("Error:"))
		os.Exit(1)
	}
}

func (c *Config) String() string {
	sudoStatus := "EMPTY"
	if c.SudoPass != "" {
		sudoStatus = "SET"
	}
	smtpStatus := "EMPTY"
	if c.SMTPPass != "" {
		smtpStatus = "SET"
	}
	return fmt.Sprintf(
		"service-name=%s repo-url=%s branch=%s service-dir=%s service-user=%s "+
			"webhook=%d admin-email=%s public-ip=%s notify-url=%s "+
			"deploy-timeout=%d smtp-host=%s smtp-port=%d smtp-user=%s smtp-pass=[%s] "+
			"git-username=%s sudo-pass=[%s] mongodb-uri=%s",
		c.ServiceName, c.RepoURL, c.Branch, c.ServiceDir, c.ServiceUser,
		c.Webhook, c.AdminEmail, c.PublicIP, c.NotifyURL,
		c.DeployTimeout, c.SMTPHost, c.SMTPPort, c.SMTPUser, smtpStatus,
		c.GitUsername, sudoStatus, c.MongoDBURI,
	)
}
