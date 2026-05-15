package api

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"gosrc/actions"
	"gosrc/database"
	"gosrc/deploypath"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"path/filepath"
	"time"

	"bufio"
	"encoding/json"
	"io"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/golang-jwt/jwt/v5"
	"github.com/ktbsomen/jsjson"
)

//go:embed dashboard.html
var dashboardHTML []byte

var jwtSecret = []byte("change-me-to-something-very-secure")

// StartUnifiedServer launches the single-port Gateway for Webhooks & Dashboard
func StartUnifiedServer(cfg *parser.Config) {
	app := fiber.New(fiber.Config{
		BodyLimit: 2 * 1024 * 1024, // 2MB Limit
	})

	// Security: Rate limiting for all public routes
	app.Use(limiter.New(limiter.Config{
		Max:        20,
		Expiration: 1 * time.Minute,
	}))

	// Security: CORS
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST"},
	}))

	// 0. HEALTH CHECK & LANDING PAGE
	app.Get("/", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return c.Send([]byte("🚀 CICD Unified Gateway is LIVE!\n\nAccess the Dashboard at: <a href='/dashboard'>/dashboard</a>"))
	})

	// --- 1. WEBHOOK ROUTE (Public, but HMAC Secured) ---
	app.Post("/", func(c fiber.Ctx) error {
		if c.Get("X-Github-Event") == "ping" {
			return c.SendString("Pong")
		}

		// Verify GitHub Signature
		if err := verifySignature(c, cfg.WebhookSecret); err != nil {
			logger.Warn("Unauthorized webhook: "+err.Error(), cfg)
			return c.Status(401).SendString(err.Error())
		}

		// Parse Payload
		data, err := parseAndValidateWebhook(c, cfg)
		if err != nil {
			return c.Status(200).SendString(err.Error())
		}

		// Notify MongoDB
		go database.UpdateCommitHash(cfg, data.Hash)
		logger.Info(fmt.Sprintf("🚀 WEBHOOK TRIGGERED: %s (Commit: %s)", cfg.ServiceName, data.Hash[:8]), cfg)

		return c.SendString("Deployment initiated")
	})

	// --- 2. DASHBOARD ROUTES (UI) ---
	app.Get("/dashboard", func(c fiber.Ctx) error {
		logger.Info("🖥️  Dashboard requested (Internal Asset)", cfg)
		c.Set("Content-Type", "text/html")
		return c.Send(dashboardHTML)
	})

	app.Post("/auth/magic-link", func(c fiber.Ctx) error {
		type request struct {
			Email string `json:"email"`
		}
		var req request
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(400).SendString("Invalid")
		}

		if req.Email != cfg.AdminEmail {
			return c.Status(401).SendString("Unauthorized")
		}

		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"email": req.Email,
			"exp":   time.Now().Add(time.Minute * 15).Unix(),
		})

		tokenString, _ := token.SignedString(jwtSecret)

		magicLink := fmt.Sprintf("http://%s:%d/auth/verify?token=%s", cfg.PublicIP, cfg.Webhook, tokenString)
		logger.SendMail(cfg, "Dashboard Login Link", magicLink)
		logger.Info("📧 MAGIC LINK: "+magicLink, cfg)

		return c.SendString("Sent")
	})

	app.Get("/auth/verify", func(c fiber.Ctx) error {
		tokenString := c.Query("token")
		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) { return jwtSecret, nil })
		if err != nil || !token.Valid {
			return c.Status(401).SendString("Invalid")
		}

		c.Set("Content-Type", "text/html")
		return c.SendString(fmt.Sprintf(`<script>localStorage.setItem('cicd_token', '%s'); window.location.href='/dashboard';</script>`, tokenString))
	})

	// --- 3. PROTECTED API ROUTES ---
	api := app.Group("/api", authMiddleware)

	api.Get("/fleet", func(c fiber.Ctx) error {
		ips, err := database.GetUniqueServerIPs(cfg)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(ips)
	})

	api.Get("/projects", func(c fiber.Ctx) error {
		projects, err := database.GetAllProjects()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		// Enrich projects with real-time process status
		type projectStatus struct {
			database.Project
			IsRunning bool
			PID       int
			Uptime    string
		}

		var enriched []projectStatus
		for _, p := range projects {
			isRunning, pid, uptime := actions.GetProcessStatus(p.ServiceDir)
			enriched = append(enriched, projectStatus{
				Project:   p,
				IsRunning: isRunning,
				PID:       pid,
				Uptime:    uptime,
			})
		}

		return c.JSON(enriched)
	})

	api.Post("/projects", func(c fiber.Ctx) error {
		type CreateRequest struct {
			RepoURL       string `json:"repo_url"`
			Branch        string `json:"branch"`
			ServiceName   string `json:"service_name"`
			ServiceUser   string `json:"service_user"`
			BaseDir       string `json:"base_dir"`
			AdminEmail    string `json:"admin_email"`
			GitUsername   string `json:"git_username"`
			GitPassword   string `json:"git_password"`
			WebhookSecret string `json:"webhook_secret"`
			WebhookPort   int    `json:"webhook_port"`
			MongoDBURI    string `json:"mongodb_uri"`
			SMTPHost      string `json:"smtp_host"`
			SMTPPort      int    `json:"smtp_port"`
			SMTPUser      string `json:"smtp_user"`
			SMTPPass      string `json:"smtp_pass"`
			SudoPass      string `json:"sudo_pass"`
			PublicIP      string `json:"public_ip"`
		}

		var req CreateRequest
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(400).SendString("Invalid request format")
		}

		// 1. Validation
		if req.RepoURL == "" || req.ServiceName == "" || req.ServiceUser == "" {
			return c.Status(400).SendString("Missing required fields: repo_url, service_name, service_user")
		}

		if req.Branch == "" {
			req.Branch = "main"
		}
		if req.BaseDir == "" {
			req.BaseDir = cfg.ServiceDir
		}
		if req.WebhookPort == 0 {
			req.WebhookPort = 9641 // Default port for UI-created projects
		}

		logger.Info(fmt.Sprintf("🚀 UI TRIGGER: Creating project %s...", req.ServiceName), cfg)

		// 2. Resolve Secure Path
		existingPaths, _ := database.GetAllDeploymentPaths()
		res, err := deploypath.ResolveDeploymentPath(deploypath.Config{
			ServiceUser:         req.ServiceUser,
			BaseDir:             req.BaseDir,
			ProjectName:         req.ServiceName,
			ExistingDeployments: existingPaths,
		})
		if err != nil {
			return c.Status(500).SendString("Path Resolution Failed: " + err.Error())
		}

		// 3. Create a temporary config for this project
		projectCfg := *cfg // Clone global config
		projectCfg.RepoURL = req.RepoURL
		projectCfg.Branch = req.Branch
		projectCfg.ServiceName = req.ServiceName
		projectCfg.ServiceUser = req.ServiceUser
		projectCfg.ServiceDir = res.FinalPath

		// Override with provided optional flags
		if req.AdminEmail != "" {
			projectCfg.AdminEmail = req.AdminEmail
		}
		if req.GitUsername != "" {
			projectCfg.GitUsername = req.GitUsername
		}
		if req.GitPassword != "" {
			projectCfg.GitPassword = req.GitPassword
		}
		if req.WebhookSecret != "" {
			projectCfg.WebhookSecret = req.WebhookSecret
		}
		if req.WebhookPort != 0 {
			projectCfg.Webhook = req.WebhookPort
		}
		if req.MongoDBURI != "" {
			projectCfg.MongoDBURI = req.MongoDBURI
		}
		if req.PublicIP != "" {
			projectCfg.PublicIP = req.PublicIP
		}
		if req.SMTPHost != "" {
			projectCfg.SMTPHost = req.SMTPHost
		}
		if req.SMTPPort != 0 {
			projectCfg.SMTPPort = req.SMTPPort
		}
		if req.SMTPUser != "" {
			projectCfg.SMTPUser = req.SMTPUser
		}
		if req.SMTPPass != "" {
			projectCfg.SMTPPass = req.SMTPPass
		}
		if req.SudoPass != "" {
			projectCfg.SudoPass = req.SudoPass
		}

		// 4. Setup Folder (Root operation)
		if err := actions.SetupProjectFolder(&projectCfg); err != nil {
			return c.Status(500).SendString("Folder Setup Failed: " + err.Error())
		}

		// 5. Register in DBs
		database.RegisterProject(&projectCfg)
		database.RegisterToMongo(&projectCfg)

		// 6. Trigger Initial Deployment in background
		go func() {
			logger.Info(fmt.Sprintf("🎬 Initializing deployment for %s...", projectCfg.ServiceName), &projectCfg)
			actions.SyncRepo(&projectCfg)
			actions.RunDeployment(&projectCfg)
		}()

		return c.Status(201).JSON(fiber.Map{
			"message":     "Project created and deployment started",
			"service_dir": projectCfg.ServiceDir,
		})
	})

	api.Get("/projects/:id/logs", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}

		data, err := os.ReadFile(filepath.Join(project.ServiceDir, ".cicdlog", "deploy.log"))
		if err != nil {
			return c.Status(404).SendString("No logs")
		}
		return c.SendString(string(data))
	})

	api.Get("/system/logs", func(c fiber.Ctx) error {
		data, err := os.ReadFile("/var/log/cicd/system.log")
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		return c.SendString(string(data))
	})

	api.Post("/projects/:id/stop", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		actions.StopAppProcess(project.ServiceDir)
		return c.SendString("Stopped")
	})

	api.Post("/projects/:id/deploy", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		appCfg := project.ToConfig()
		go func() {
			actions.SyncRepo(appCfg)
			actions.RunDeployment(appCfg)
		}()
		return c.SendString("Deployment Started")
	})

	api.Post("/projects/:id/run", func(c fiber.Ctx) error {
		type request struct {
			Command string `json:"command"`
		}
		var req request
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(400).SendString("Invalid request")
		}

		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}

		output, err := actions.RunCustomCommand(project.ToConfig(), req.Command)
		if err != nil {
			return c.Status(500).SendString(fmt.Sprintf("Error: %v\nOutput: %s", err, output))
		}
		return c.SendString(output)
	})
	api.Delete("/projects/:id", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}

		// Stop process first
		actions.StopAppProcess(project.ServiceDir)

		// Delete from database
		if err := database.DeleteProject(c.Params("id")); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		return c.SendString("Deleted")
	})

	// --- SSE REAL-TIME STREAMS ---

	api.Get("/stream/fleet", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		c.Response().SetBodyStreamWriter(func(w *bufio.Writer) {
			for {
				projects, _ := database.GetAllProjects()
				type projectStatus struct {
					database.Project
					IsRunning bool
					PID       int
					Uptime    string
				}
				var enriched []projectStatus
				for _, p := range projects {
					isRunning, pid, uptime := actions.GetProcessStatus(p.ServiceDir)
					enriched = append(enriched, projectStatus{
						Project:   p,
						IsRunning: isRunning,
						PID:       pid,
						Uptime:    uptime,
					})
				}
				data, _ := json.Marshal(enriched)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if err := w.Flush(); err != nil {
					return
				}
				time.Sleep(2 * time.Second)
			}
		})
		return nil
	})

	api.Get("/projects/:id/logs/stream", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no") // Disable Nginx buffering

		logPath := filepath.Join(project.ServiceDir, ".cicdlog", "deploy.log")

		c.Response().SetBodyStreamWriter(func(w *bufio.Writer) {
			// 1. Send an initial "Connected" message so the browser knows it's alive
			fmt.Fprintf(w, ": heartbeat\n\n")
			w.Flush()

			file, err := os.Open(logPath)
			if err != nil {
				fmt.Fprintf(w, "data: ❌ Failed to open log file: %v\n\n", err)
				w.Flush()
				return
			}
			defer file.Close()

			// 2. Read the LAST 20 lines for immediate context
			// (Simplistic approach: seek back a bit and find lines)
			stat, _ := file.Stat()
			offset := int64(2048) // Look back 2KB
			if stat.Size() < offset {
				offset = 0
			} else {
				offset = stat.Size() - offset
			}
			file.Seek(offset, io.SeekStart)

			// Skip first partial line
			scanner := bufio.NewScanner(file)
			if offset > 0 {
				scanner.Scan()
			}

			for scanner.Scan() {
				fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
			}
			w.Flush()

			// 3. Continue tailing for NEW lines
			reader := bufio.NewReader(file)
			lastHeartbeat := time.Now()

			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						// Send heartbeat every 15s to keep connection alive
						if time.Since(lastHeartbeat) > 15*time.Second {
							fmt.Fprintf(w, ": heartbeat\n\n")
							w.Flush()
							lastHeartbeat = time.Now()
						}
						time.Sleep(500 * time.Millisecond)
						continue
					}
					return
				}

				fmt.Fprintf(w, "data: %s\n\n", strings.TrimSpace(line))
				if err := w.Flush(); err != nil {
					return
				}
				lastHeartbeat = time.Now()
			}
		})
		return nil
	})

	// --- GLOBAL SETTINGS API ---

	// GET current global settings
	api.Get("/settings", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"mongodb_uri":    cfg.MongoDBURI,
			"admin_email":    cfg.AdminEmail,
			"webhook_secret": cfg.WebhookSecret,
			"smtp_host":      cfg.SMTPHost,
			"smtp_port":      cfg.SMTPPort,
			"smtp_user":      cfg.SMTPUser,
			"public_ip":      cfg.PublicIP,
		})
	})

	// UPDATE global settings and restart
	api.Put("/settings", func(c fiber.Ctx) error {
		type SettingsRequest struct {
			MongoDBURI    string `json:"mongodb_uri"`
			AdminEmail    string `json:"admin_email"`
			WebhookSecret string `json:"webhook_secret"`
			SMTPHost      string `json:"smtp_host"`
			SMTPPort      int    `json:"smtp_port"`
			SMTPUser      string `json:"smtp_user"`
			SMTPPass      string `json:"smtp_pass"`
			PublicIP      string `json:"public_ip"`
		}

		var req SettingsRequest
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(400).SendString("Invalid format")
		}

		// Update the in-memory config
		if req.MongoDBURI != "" {
			cfg.MongoDBURI = req.MongoDBURI
		}
		if req.AdminEmail != "" {
			cfg.AdminEmail = req.AdminEmail
		}
		if req.WebhookSecret != "" {
			cfg.WebhookSecret = req.WebhookSecret
		}
		if req.SMTPHost != "" {
			cfg.SMTPHost = req.SMTPHost
		}
		if req.SMTPPort != 0 {
			cfg.SMTPPort = req.SMTPPort
		}
		if req.SMTPUser != "" {
			cfg.SMTPUser = req.SMTPUser
		}
		if req.SMTPPass != "" {
			cfg.SMTPPass = req.SMTPPass
		}
		if req.PublicIP != "" {
			cfg.PublicIP = req.PublicIP
		}

		// Save to SQLite
		database.SaveGlobalSettings(cfg)

		logger.Info("⚙️  Global settings updated via UI. Restarting orchestrator to apply changes...", cfg)

		// Trigger restart after a small delay
		go func() {
			time.Sleep(1 * time.Second)
			logger.Info("🛑 Shutting down for systemd restart...", cfg)
			os.Exit(0)
		}()

		return c.JSON(fiber.Map{
			"message": "Settings saved. Orchestrator is restarting...",
		})
	})

	addr := fmt.Sprintf(":%d", cfg.Webhook)
	logger.Info(fmt.Sprintf("🛰️  Unified Gateway starting on %s", addr), cfg)
	if err := app.Listen(addr); err != nil {
		logger.Error(fmt.Sprintf("❌ CRITICAL: Failed to start API Gateway: %v", err), cfg)
		// We panic here because the Gateway is essential for Control Plane access
		panic(fmt.Sprintf("PORT %d IS ALREADY IN USE! Check if an old version is running.", cfg.Webhook))
	}
}

// --- HELPERS ---

func authMiddleware(c fiber.Ctx) error {
	tokenString := c.Get("Authorization")
	if tokenString == "" {
		tokenString = c.Query("token")
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) { return jwtSecret, nil })
	if err != nil || !token.Valid {
		return c.Status(401).SendString("Invalid token")
	}
	return c.Next()
}

func verifySignature(c fiber.Ctx, secret string) error {
	if secret == "" {
		return nil
	}
	signature := c.Get("X-Hub-Signature-256")
	if signature == "" {
		return fmt.Errorf("missing signature")
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(c.Body())
	expected := "sha256=" + hex.EncodeToString(h.Sum(nil))
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

type WebhookData struct {
	Hash   string
	Branch string
	Repo   string
}

func parseAndValidateWebhook(c fiber.Ctx, cfg *parser.Config) (*WebhookData, error) {
	p := jsjson.MustParse(string(c.Body()))
	data := &WebhookData{}
	data.Hash, _ = p.Get("head_commit", "id").String()
	data.Branch, _ = p.Get("ref").String()
	data.Repo, _ = p.Get("repository", "html_url").String()

	if data.Repo != cfg.RepoURL {
		return nil, fmt.Errorf("repo mismatch")
	}
	if data.Branch != "refs/heads/"+cfg.Branch {
		return nil, fmt.Errorf("branch mismatch")
	}
	if data.Hash == "" {
		return nil, fmt.Errorf("no hash")
	}
	return data, nil
}
