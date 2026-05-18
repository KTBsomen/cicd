package cli

import (
	"fmt"
	"gosrc/actions"
	"gosrc/database"
	"gosrc/parser"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// HandleCommands checks if the user is running a utility command (log, status, stop, etc.)
// and executes it. This keeps main.go clean for the actual orchestration logic.
func HandleCommands(cfg *parser.Config) {
	if len(os.Args) < 2 {
		return
	}

	cmd := strings.ToLower(os.Args[1])

	switch cmd {
	case "commands":
		handleHelp()
		os.Exit(0)

	case "ls", "list":
		handleList(cfg)
		os.Exit(0)

	case "status":
		runCommand("systemctl", "status", "cicd")
		os.Exit(0)

	case "start":
		runCommand("systemctl", "start", "cicd")
		fmt.Println("✅ Service started")
		os.Exit(0)

	case "stop":
		runCommand("systemctl", "stop", "cicd")
		fmt.Println("✅ Service stopped")
		os.Exit(0)

	case "restart":
		runCommand("systemctl", "restart", "cicd")
		fmt.Println("✅ Service restarted")
		os.Exit(0)

	case "uninstall":
		runCommand("systemctl", "stop", "cicd")
		runCommand("systemctl", "disable", "cicd")
		fmt.Println("✅ Service stopped and disabled")
		os.Exit(0)

	case "log", "logs":
		handleLogs(cfg)
		os.Exit(0)

	case "token":
		handleToken(cfg)
		os.Exit(0)

	case "pin":
		handlePin(cfg, true)
		os.Exit(0)

	case "unpin":
		handlePin(cfg, false)
		os.Exit(0)

	case "rollback":
		handleRollback(cfg)
		os.Exit(0)

	case "history":
		handleHistory(cfg)
		os.Exit(0)

	case "reinstall":
		handleReinstall(cfg)
		os.Exit(0)

	case "doctor":
		handleDoctor(cfg)
		os.Exit(0)

	case "info":
		handleInfo(cfg)
		os.Exit(0)
	}
}

// ═══════════════════════════════════════════════════════
//  cicd token [--force]
// ═══════════════════════════════════════════════════════

func handleToken(cfg *parser.Config) {
	database.InitDB(cfg)
	database.LoadGlobalSettings(cfg)

	force := len(os.Args) > 2 && (os.Args[2] == "--force" || os.Args[2] == "-f")

	if cfg.AdminEmail != "" && !force {
		fmt.Println(text.FgHiCyan.Sprint("ℹ️  Admin email is configured: ") + text.Bold.Sprint(cfg.AdminEmail))
		fmt.Println(text.Faint.Sprint("Use magic link to log in. Run 'cicd token --force' for emergency bypass."))
		return
	}

	token, err := database.GenerateSetupToken()
	if err != nil {
		fmt.Printf("❌ Failed to generate token: %v\n", err)
		return
	}

	fmt.Println(text.FgHiGreen.Sprint("\n🔑 SETUP TOKEN GENERATED"))
	fmt.Println(text.Bold.Sprint("  Token: ") + text.FgHiYellow.Sprint(token))
	fmt.Println(text.Faint.Sprint("  Expires in 15 minutes. One-time use."))
	fmt.Println(text.Faint.Sprint("  Paste this token in the dashboard login screen.\n"))
}

// ═══════════════════════════════════════════════════════
//  cicd pin / unpin <name|id>
// ═══════════════════════════════════════════════════════

func handlePin(cfg *parser.Config, pin bool) {
	if len(os.Args) < 3 {
		action := "pin"
		if !pin {
			action = "unpin"
		}
		fmt.Printf("Usage: cicd %s <name|id>\n", action)
		return
	}
	database.InitDB(cfg)
	project := resolveProject(os.Args[2])
	if project == nil {
		return
	}

	if err := database.SetProjectPinned(fmt.Sprintf("%d", project.ID), pin); err != nil {
		fmt.Printf("❌ Failed: %v\n", err)
		return
	}
	if pin {
		fmt.Printf("📌 Project '%s' is now PINNED. Webhooks will record commits but NOT deploy.\n", project.ServiceName)
	} else {
		fmt.Printf("📌 Project '%s' is now UNPINNED. Normal deployments will resume.\n", project.ServiceName)
	}
}

// ═══════════════════════════════════════════════════════
//  cicd rollback <name|id> <hash>
// ═══════════════════════════════════════════════════════

func handleRollback(cfg *parser.Config) {
	if len(os.Args) < 4 {
		fmt.Println("Usage: cicd rollback <name|id> <commit-hash>")
		return
	}
	database.InitDB(cfg)
	project := resolveProject(os.Args[2])
	if project == nil {
		return
	}
	commitHash := os.Args[3]

	appCfg, err := project.ToConfig()
	if err != nil {
		fmt.Printf("❌ Config error: %v\n", err)
		return
	}

	fmt.Printf("⏪ Rolling back '%s' to commit %s...\n", project.ServiceName, commitHash[:8])
	if err := actions.RollbackToCommit(appCfg, commitHash); err != nil {
		fmt.Printf("❌ Rollback failed: %v\n", err)
		return
	}
	fmt.Println("✅ Rollback complete")
}

// ═══════════════════════════════════════════════════════
//  cicd history <name|id>
// ═══════════════════════════════════════════════════════

func handleHistory(cfg *parser.Config) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: cicd history <name|id>")
		return
	}
	database.InitDB(cfg)
	project := resolveProject(os.Args[2])
	if project == nil {
		return
	}

	entries, err := database.GetCommitHistory(project.ID, 25)
	if err != nil || len(entries) == 0 {
		fmt.Println("No commit history found.")
		return
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"#", "Commit", "Message", "Status", "Triggered", "Time"})

	for _, e := range entries {
		statusColor := text.FgHiBlack
		switch e.Status {
		case "current":
			statusColor = text.FgHiGreen
		case "rolled_back":
			statusColor = text.FgHiRed
		}
		hash := e.CommitHash
		if len(hash) > 8 {
			hash = hash[:8]
		}
		msg := e.CommitMsg
		if len(msg) > 40 {
			msg = msg[:40] + "..."
		}
		t.AppendRow(table.Row{
			e.ID,
			hash,
			msg,
			statusColor.Sprint(strings.ToUpper(e.Status)),
			e.TriggeredBy,
			e.StartedAt.Format("Jan 02 15:04"),
		})
	}

	style := table.StyleRounded
	style.Color.Header = text.Colors{text.FgHiCyan, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	t.Render()
}

// ═══════════════════════════════════════════════════════
//  cicd reinstall <name|id>
// ═══════════════════════════════════════════════════════

func handleReinstall(cfg *parser.Config) {
	if len(os.Args) < 3 {
		fmt.Println("Usage: cicd reinstall <name|id>")
		return
	}
	database.InitDB(cfg)
	project := resolveProject(os.Args[2])
	if project == nil {
		return
	}

	hashFile := filepath.Join(project.ServiceDir, ".cicdlog", "install.hash")
	os.Remove(hashFile)
	fmt.Printf("🗑️  Deleted install.hash for '%s'. Next deployment will force reinstall.\n", project.ServiceName)
	fmt.Println(text.Faint.Sprint("Trigger a deployment via webhook or 'cicd deploy' to apply."))
}

// ═══════════════════════════════════════════════════════
//  cicd doctor — System health checks
// ═══════════════════════════════════════════════════════

func handleDoctor(cfg *parser.Config) {
	fmt.Println(text.FgHiCyan.Sprint("\n🩺 CICD DOCTOR — System Health Check\n"))
	passed := 0
	total := 0

	check := func(name string, fn func() error) {
		total++
		if err := fn(); err != nil {
			fmt.Printf("  %s %s: %s\n", text.FgHiRed.Sprint("✗"), name, err)
		} else {
			passed++
			fmt.Printf("  %s %s\n", text.FgHiGreen.Sprint("✓"), name)
		}
	}

	check("Git binary present", func() error {
		_, err := exec.LookPath("git")
		return err
	})

	check("SQLite database accessible", func() error {
		database.InitDB(cfg)
		return database.DB.Ping()
	})

	check("MongoDB reachable", func() error {
		database.LoadGlobalSettings(cfg)
		if cfg.MongoDBURI == "" {
			return fmt.Errorf("not configured")
		}
		conn, err := net.DialTimeout("tcp", "localhost:27017", 5*time.Second)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	})

	database.InitDB(cfg)
	projects, _ := database.GetAllProjects()

	check(fmt.Sprintf("All project dirs exist (%d projects)", len(projects)), func() error {
		var missing []string
		for _, p := range projects {
			if _, err := os.Stat(p.ServiceDir); os.IsNotExist(err) {
				missing = append(missing, p.ServiceName)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing dirs: %s", strings.Join(missing, ", "))
		}
		return nil
	})

	check("All PIDs alive", func() error {
		var dead []string
		for _, p := range projects {
			isRunning, _, _, _ := actions.GetProcessStatusFromDisk(p.ServiceDir)
			pidPath := filepath.Join(p.ServiceDir, ".cicdlog", "app.pid")
			if _, err := os.Stat(pidPath); err == nil && !isRunning {
				dead = append(dead, p.ServiceName)
			}
		}
		if len(dead) > 0 {
			return fmt.Errorf("stale PIDs: %s", strings.Join(dead, ", "))
		}
		return nil
	})

	check("SMTP reachable", func() error {
		database.LoadGlobalSettings(cfg)
		if cfg.SMTPHost == "" {
			return fmt.Errorf("not configured")
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort), 5*time.Second)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	})

	fmt.Printf("\n  %s %d/%d checks passed\n\n", text.FgHiCyan.Sprint("Result:"), passed, total)
}

// ═══════════════════════════════════════════════════════
//  Existing handlers
// ═══════════════════════════════════════════════════════

func handleLogs(cfg *parser.Config) {
	if len(os.Args) == 2 {
		runCommand("journalctl", "-u", "cicd", "-n", "100")
		return
	}

	arg := os.Args[2]

	if arg == "-f" {
		runCommand("journalctl", "-u", "cicd", "-f")
		return
	}

	database.InitDB(cfg)
	project := resolveProject(arg)
	if project == nil {
		return
	}

	logPath := filepath.Join(project.ServiceDir, ".cicdlog", "deploy.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Printf("❌ Log file not found at: %s\n", logPath)
		os.Exit(1)
	}

	if len(os.Args) > 3 && os.Args[3] == "-f" {
		runCommand("tail", "-f", logPath)
	} else {
		runCommand("cat", logPath)
	}
}

func handleList(cfg *parser.Config) {
	database.InitDB(cfg)
	projects, err := database.GetAllProjects()
	if err != nil {
		fmt.Println("❌ Failed to fetch fleet:", err)
		return
	}

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"ID", "Service Name", "Branch", "Status", "PID", "Uptime", "Deploy", "Pinned"})

	for _, p := range projects {
		// B3: Use PID file on disk (not in-memory registry)
		isRunning, pid, uptime, _ := actions.GetProcessStatusFromDisk(p.ServiceDir)
		statusIcon := text.FgHiRed.Sprint("● STOPPED")
		if isRunning {
			statusIcon = text.FgHiGreen.Sprint("● RUNNING")
		}
		pinnedIcon := ""
		if p.IsPinned {
			pinnedIcon = text.FgHiYellow.Sprint("📌")
		}

		t.AppendRow(table.Row{
			p.ID,
			p.ServiceName,
			p.Branch,
			statusIcon,
			pid,
			uptime,
			p.DeployStatus,
			pinnedIcon,
		})
		t.AppendSeparator()
	}

	style := table.StyleRounded
	style.Format.Header = text.FormatUpper
	style.Color.Header = text.Colors{text.FgHiYellow, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	t.Render()
}

// resolveProject finds a project by ID or name
func resolveProject(idOrName string) *database.Project {
	project, err := database.GetProjectByID(idOrName)
	if err != nil {
		project, err = database.GetProjectByName(idOrName)
	}
	if err != nil || project == nil {
		fmt.Printf("❌ Project not found: %s\n", idOrName)
		return nil
	}
	return project
}

func runCommand(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		fmt.Printf("❌ Command failed (%s): %v\n", name, err)
	}
}

// ═══════════════════════════════════════════════════════
//  cicd info — Display Gateway Credentials & Guides
// ═══════════════════════════════════════════════════════

func handleInfo(cfg *parser.Config) {
	database.InitDB(cfg)
	database.LoadGlobalSettings(cfg)
	PrintGatewayInfo(cfg)
}

// PrintGatewayInfo renders a highly polished, guide-style dashboard summary of the orchestrator.
// It is shared between the first setup boot and the on-demand 'cicd info' command.
func PrintGatewayInfo(cfg *parser.Config) {
	token := "Not Required (Admin Email Configured)"
	if cfg.AdminEmail == "" {
		if t, err := database.GetOrGenerateSetupToken(); err == nil {
			token = t
		} else {
			token = "Failed to load active setup token"
		}
	}

	fmt.Println(text.FgHiCyan.Sprint("\n======================================================================"))
	fmt.Println(text.Bold.Sprint(" 🖥️  STEP 1: ACCESS YOUR ADMIN DASHBOARD"))
	fmt.Println("======================================================================")
	fmt.Println(text.Faint.Sprint("  1. Open your browser and navigate to the Dashboard URL."))
	fmt.Println(text.Faint.Sprint("  2. Copy and paste the Setup Token below to log in (expires in 15m).\n"))
	fmt.Println(text.Faint.Sprint("  3. In the login page you have to click `USE SETUP TOKEN` button and paste the token.\n"))

	t1 := table.NewWriter()
	t1.SetOutputMirror(os.Stdout)
	t1.AppendRow(table.Row{text.Bold.Sprint("🌐 Dashboard URL"), text.FgHiCyan.Sprintf("http://%s:%d/dashboard", cfg.PublicIP, cfg.Webhook)})
	if cfg.AdminEmail == "" {
		t1.AppendRow(table.Row{text.Bold.Sprint("🔑 Setup Token "), text.FgHiYellow.Sprint(token)})
	} else {
		t1.AppendRow(table.Row{text.Bold.Sprint("📧 Admin Email "), text.Faint.Sprint(cfg.AdminEmail)})
	}
	style1 := table.StyleRounded
	style1.Color.Border = text.Colors{text.FgHiBlack}
	t1.SetStyle(style1)
	t1.Render()

	fmt.Println(text.FgHiGreen.Sprint("\n======================================================================"))
	fmt.Println(text.Bold.Sprint(" 🛰️  STEP 2: CONNECT GITHUB (WEBHOOK SETUP)"))
	fmt.Println("======================================================================")
	fmt.Println(text.Faint.Sprint("  1. Go to your GitHub repository -> Settings -> Webhooks -> Add Webhook."))
	fmt.Println(text.Faint.Sprint("  2. Set Payload URL, select 'application/json', and paste the Secret key.\n"))

	t2 := table.NewWriter()
	t2.SetOutputMirror(os.Stdout)
	t2.AppendRow(table.Row{text.Bold.Sprint("🔌 Payload URL  "), text.FgHiCyan.Sprintf("http://%s:%d/", cfg.PublicIP, cfg.Webhook)})
	t2.AppendRow(table.Row{text.Bold.Sprint("⚙️  Content Type "), text.Bold.Sprint("application/json")})
	t2.AppendRow(table.Row{text.Bold.Sprint("🔒 Secret Key   "), text.FgHiYellow.Sprint(cfg.WebhookSecret)})
	style2 := table.StyleRounded
	style2.Color.Border = text.Colors{text.FgHiBlack}
	t2.SetStyle(style2)
	t2.Render()

	fmt.Println(text.FgHiYellow.Sprint("\n======================================================================"))
	fmt.Println(text.Bold.Sprint(" 🎥 STEP 3: NEED HELP? WATCH VIDEO GUIDE"))
	fmt.Println("======================================================================")
	fmt.Println("  Watch the latest YouTube guide on configuring GitHub webhooks:")
	fmt.Println("  " + text.Bold.Sprint("https://www.youtube.com/watch?v=MyEkKp3VRwo") + text.Faint.Sprint(" (GitHub Webhooks Tutorial by Behind Tools)"))
	fmt.Println("======================================================================")

	fmt.Println(text.Faint.Sprint("⚙️  SYSTEMD SERVICE MANAGEMENT:"))
	fmt.Println("  - To view gateway info:  " + text.Bold.Sprint("cicd info"))
	fmt.Println("  - To monitor live background logs:  " + text.Bold.Sprint("cicd log -f"))

	fmt.Println("  - To check systemd service status:  " + text.Bold.Sprint("cicd status"))
	fmt.Println("  - To view list of active services:  " + text.Bold.Sprint("cicd ls\n"))
	fmt.Println("  - TO View all commands:             " + text.Bold.Sprintf("cicd commands"))
}

// handleHelp displays a high-fidelity table of all registered CLI commands
func handleHelp() {
	fmt.Println("\n" + text.BgCyan.Sprint("  CICD ORCHESTRATOR CLI COMMANDS  ") + "\n")

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Command", "Usage / Arguments", "Description"})

	t.AppendRows([]table.Row{
		{text.FgHiYellow.Sprint("list / ls"), "cicd list", "List all registered projects and their status"},
		{text.FgHiYellow.Sprint("info"), "cicd info", "Print premium Command Center setup and connection links"},
		{text.FgHiYellow.Sprint("status"), "cicd status", "Check status of the background orchestrator service"},
		{text.FgHiYellow.Sprint("start"), "cicd start", "Start the orchestrator background service"},
		{text.FgHiYellow.Sprint("stop"), "cicd stop", "Stop the orchestrator background service"},
		{text.FgHiYellow.Sprint("restart"), "cicd restart", "Restart the orchestrator background service"},
		{text.FgHiYellow.Sprint("logs / log"), "cicd log [service]", "Tail global orchestrator logs or project deploy logs"},
		{text.FgHiYellow.Sprint("token"), "cicd token [--force]", "Retrieve or regenerate the gateway onboarding token"},
		{text.FgHiYellow.Sprint("pin"), "cicd pin <service> <hash>", "Pin a service deployment to a specific commit hash"},
		{text.FgHiYellow.Sprint("unpin"), "cicd unpin <service>", "Remove the pinned commit constraint from a service"},
		{text.FgHiYellow.Sprint("history"), "cicd history <service>", "View the deployment and rollback history of a service"},
		{text.FgHiYellow.Sprint("rollback"), "cicd rollback <service> <hash>", "Roll back a service to a previous deployment commit"},
		{text.FgHiYellow.Sprint("reinstall"), "cicd reinstall <service>", "Force full clean reinstall of a service dependency tree"},
		{text.FgHiYellow.Sprint("doctor"), "cicd doctor", "Run diagnostic health checks on ports, SQLite, and SMTP"},
		{text.FgHiYellow.Sprint("uninstall"), "cicd uninstall", "Stop, disable, and clean up the orchestrator daemon"},
		{text.FgHiYellow.Sprint("--help"), "cicd --help", "Show other project specific help command list"},
	})

	style := table.StyleRounded
	style.Color.Header = text.Colors{text.FgHiCyan, text.Bold}
	style.Color.Border = text.Colors{text.FgHiBlack}
	t.SetStyle(style)
	t.Render()
	fmt.Println()
}
