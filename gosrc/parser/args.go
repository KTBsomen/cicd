package parser

import (
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type Config struct {
	// Required
	Setup      string
	RepoURL    string
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
}

// MustCompile is preferred for global variables; it panics if the regex is invalid
var emailRegex = regexp.MustCompile(`^[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,4}$`)

func isValid(email string) bool {
	return emailRegex.MatchString(email)
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
	} else {
		fmt.Printf("Error fetching IP from ipify: %v\n", err)
	}

	// 2. Fallback to Socket
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err == nil {
		defer conn.Close()
		// Extracts the IP from the local address of the connection
		localAddr := conn.LocalAddr().(*net.UDPAddr)
		return localAddr.IP.String()
	}

	fmt.Printf("Error fetching IP using socket: %v\n", err)
	return ""
}

// Parse flags reciver method for Config struct
func (c *Config) Parse() {
	// Required Flags
	flag.StringVar(&c.Setup, "setup", "", "Type of setup (e.g., node, python, manual) [Required]")
	flag.StringVar(&c.RepoURL, "repo-url", "", "Repository URL for the code [Required]")
	flag.StringVar(&c.AdminEmail, "admin-email", "", "Admin email to send error logs [Required]")

	// MongoDB with default
	flag.StringVar(&c.MongoDBURI, "mongodb-uri", "mongodb+srv://default-url", "MongoDB URI for change monitoring")

	// Git
	flag.StringVar(&c.GitUsername, "git-username", "", "Git username for private repos")
	flag.StringVar(&c.GitPassword, "git-password", "", "Git password/token for private repos")

	// SMTP with defaults
	flag.StringVar(&c.SMTPHost, "smtp-host", "smtpout.secureserver.net", "SMTP host")
	flag.IntVar(&c.SMTPPort, "smtp-port", 465, "SMTP port")
	flag.StringVar(&c.SMTPUser, "smtp-user", "test@wowcircle.in", "SMTP username")
	flag.StringVar(&c.SMTPPass, "smtp-pass", "Epassword", "SMTP password")

	// System
	flag.StringVar(&c.User, "user", "", "Username of the code runner")
	flag.StringVar(&c.SudoPass, "sudo-pass", "", "Sudo password for package installation")

	// Service with defaults
	flag.StringVar(&c.ServiceName, "service-name", "myapp", "Name of the service")
	flag.StringVar(&c.ServiceDir, "service-dir", "/home/", "Path of the service")
	flag.StringVar(&c.ServiceUser, "service-user", "root", "Name of the service user")
	flag.StringVar(&c.ServiceReset, "service-reset", "False", "True/False to delete systemd and restart")

	// Webhook
	flag.IntVar(&c.Webhook, "webhook", 8002, "Port number for webhook listener")
	flag.StringVar(&c.WebhookSecret, "webhook-secret", "", "GitHub Webhook secret")

	// Public IP (You would call your getPublicIP function here)
	flag.StringVar(&c.PublicIP, "public-ip", "", "Public IP for management")

	flag.Parse()

	// Logic for Required Fields
	if c.Setup == "" || c.RepoURL == "" || c.AdminEmail == "" || c.Webhook <= 0 || c.WebhookSecret == "" {
		fmt.Println("Error: --setup, --repo-url, --admin-email, --webhook and --webhook-secret are required.")
		flag.Usage()
		os.Exit(1)
	}
	if !isValid(c.AdminEmail) {
		fmt.Println("Error: --admin-email is not a valid email address.")
		os.Exit(1)
	}
	if !isValid(c.SMTPUser) {
		fmt.Println("Error: --smtp-user is not a valid email address.")
		os.Exit(1)
	}

	if c.PublicIP == "" {
		c.PublicIP = GetPublicIP()
	}
}
