package cli

import (
	"fmt"
	"gosrc/actions"
	"gosrc/database"
	"gosrc/parser"

	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	}
}

func handleLogs(cfg *parser.Config) {
	// 1. If no args or just "-f", show system logs
	if len(os.Args) == 2 {
		runCommand("journalctl", "-u", "cicd", "-n", "100")
		return
	}

	arg := os.Args[2]

	if arg == "-f" {
		runCommand("journalctl", "-u", "cicd", "-f")
		return
	}

	// 2. If an ID or ServiceName is provided, find the project logs
	database.InitDB(cfg)
	project, err := database.GetProjectByID(arg)
	if err != nil {
		// Try searching by service name if ID fails
		project, err = database.GetProjectByName(arg)
	}

	if err != nil || project.ServiceDir == "" {
		fmt.Printf("❌ Could not find project or logs for: %s\n", arg)
		os.Exit(1)
	}

	logPath := filepath.Join(project.ServiceDir, ".cicdlog", "deploy.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Printf("❌ Log file not found at: %s\n", logPath)
		os.Exit(1)
	}

	// Check if user wants to follow project logs
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

	// 2. Setup the Table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"ID", "Service Name", "Repo URL", "Branch", "Status", "PID", "Uptime", "User"})

	for _, p := range projects {
		isRunning, pid, uptime := actions.GetProcessStatus(p.ServiceDir)
		statusIcon := text.FgHiRed.Sprint("● STOPPED")
		rowColor := text.Colors{text.FgHiBlack}
		if isRunning {
			statusIcon = text.FgHiGreen.Sprint("● RUNNING")
			rowColor = text.Colors{text.FgHiCyan}
		}

		t.AppendRow(table.Row{
			p.ID,
			p.ServiceName,
			p.RepoURL,
			p.Branch,
			statusIcon,
			pid,
			uptime,
			rowColor.Sprint(p.ServiceUser),
		})
		t.AppendSeparator() // The magic "Continuous Line"
	}
	// 3. Customize the look (Colors & Borders)
	style := table.StyleRounded
	style.Format.Header = text.FormatUpper                       // Make headers UPPERCASE
	style.Color.Header = text.Colors{text.FgHiYellow, text.Bold} // Yellow Bold Header
	style.Color.Border = text.Colors{text.FgHiBlack}             // Subtle Dim Borders

	t.SetStyle(style)
	t.Render()
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
