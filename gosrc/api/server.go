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
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
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
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

//go:embed dashboard.html
var dashboardHTML []byte

// jwtSecret is loaded from DB or env at startup (IF-2)
var jwtSecret []byte

// StartUnifiedServer launches the single-port Gateway for Webhooks & Dashboard
func StartUnifiedServer(cfg *parser.Config) {
	// IF-2: Load JWT secret from DB or generate fresh
	jwtSecret = database.GetJWTSecret()

	app := fiber.New(fiber.Config{
		BodyLimit: 2 * 1024 * 1024,
	})

	app.Use(limiter.New(limiter.Config{
		Max:        20,
		Expiration: 1 * time.Minute,
	}))

	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE"},
	}))

	// 0. HEALTH CHECK
	app.Get("/", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return c.Send([]byte("CICD Unified Gateway is LIVE!\n\nAccess the Dashboard at: <a href='/dashboard'>/dashboard</a>"))
	})

	// --- 1. WEBHOOK ROUTE ---
	app.Post("/", func(c fiber.Ctx) error {
		if c.Get("X-Github-Event") == "ping" {
			return c.SendString("Pong")
		}
		// cfg.WebhookSecret is correctly loaded from DB via LoadGlobalSettings at startup
		if err := verifySignature(c, cfg.WebhookSecret); err != nil {
			logger.Warn("Unauthorized webhook: "+err.Error(), cfg)
			actions.Notify(cfg, "🛡️ Security: Unauthorized Webhook", fmt.Sprintf("A webhook hit from %s was rejected: %v", c.IP(), err))
			return c.Status(401).SendString(err.Error())
		}

		// Step 1: Extract repo URL from payload to identify which project this is for
		data, err := parseWebhookPayload(c)
		if err != nil {
			return c.Status(200).SendString(err.Error())
		}

		// Step 2: Look up the project from the DB by repo URL
		// cfg has no RepoURL — projects are registered dynamically via dashboard/CLI
		dbProject, err := database.GetProjectByRepoURL(data.Repo)
		if err != nil || dbProject == nil {
			logger.Warn(fmt.Sprintf("Webhook for unknown repo: %s", data.Repo), cfg)
			return c.Status(200).SendString("repo not registered")
		}

		// Step 3: Validate branch against this specific project's configured branch
		if data.Branch != "refs/heads/"+dbProject.Branch {
			logger.Info(fmt.Sprintf("Branch skip for %s: got %s, watching refs/heads/%s", dbProject.ServiceName, data.Branch, dbProject.Branch), cfg)
			return c.Status(200).SendString("branch mismatch")
		}

		if data.Hash == "" {
			return c.Status(200).SendString("no commit hash")
		}

		// Step 4: Build project-specific config from DB row.
		// Global settings (SMTP, NotifyURL, DeployTimeout, etc.) are now
		// automatically merged inside ToConfig() via loadGlobalInto().
		projectCfg, err := dbProject.ToConfig()
		if err != nil {
			logger.Error("Webhook: failed to build project config: "+err.Error(), cfg)
			return c.Status(500).SendString("project config error")
		}


		// F5: Check if project is pinned — record commit but skip deployment
		// (FullDeployPipeline calls AddCommitToHistory internally, so we don't here)
		if dbProject.IsPinned {
			database.AddCommitToHistory(dbProject.ID, data.Hash, data.CommitMsg, "webhook")
			database.UpdateLastCommitInfo(dbProject.ServiceDir, data.Hash, data.CommitMsg)
			logger.Info(fmt.Sprintf("📌 Project '%s' pinned. Commit %s recorded, NOT deployed.", dbProject.ServiceName, data.Hash[:8]), projectCfg)
			return c.SendString("Pinned — commit recorded")
		}

		// Step 6: Update last commit info on the project row, then enqueue.
		// AddCommitToHistory is called inside FullDeployPipeline — do NOT call it here too.
		database.UpdateLastCommitInfo(dbProject.ServiceDir, data.Hash, data.CommitMsg)
		actions.EnqueueDeployment(projectCfg, data.Hash, data.CommitMsg, actions.FullDeployPipeline)
		logger.Info(fmt.Sprintf("🚀 WEBHOOK TRIGGERED: %s (Commit: %s)", projectCfg.ServiceName, data.Hash[:8]), projectCfg)
		return c.SendString("Deployment initiated")
	})

	// --- 2. DASHBOARD & AUTH ---
	app.Get("/dashboard", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return c.Send(dashboardHTML)
	})

	// B1: Token-based login for first run
	app.Post("/auth/token-login", func(c fiber.Ctx) error {
		type request struct {
			Token string `json:"token"`
		}
		var req request
		if err := c.Bind().Body(&req); err != nil {
			return c.Status(400).SendString("Invalid request")
		}
		valid, err := database.ValidateSetupToken(req.Token)
		if !valid {
			errMsg := "Invalid token"
			if err != nil {
				errMsg = err.Error()
			}
			return c.Status(401).JSON(fiber.Map{"error": errMsg})
		}
		// Generate JWT
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"role": "admin",
			"exp":  time.Now().Add(24 * time.Hour).Unix(),
		})
		tokenString, _ := token.SignedString(jwtSecret)
		actions.Notify(cfg, "🔑 Security: Login Success", fmt.Sprintf("Admin session started via Setup Token from IP: %s", c.IP()))
		return c.JSON(fiber.Map{"token": tokenString, "message": "Authenticated via setup token"})
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
		actions.Notify(cfg, "📧 Security: Magic Link Requested", fmt.Sprintf("A login link was requested for %s from IP: %s", req.Email, c.IP()))
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

	// F14: Projects with resource stats
	api.Get("/projects", func(c fiber.Ctx) error {
		projects, err := database.GetAllProjects()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		type projectStatus struct {
			database.Project
			IsRunning  bool     `json:"is_running"`
			PID        int      `json:"pid"`
			Uptime     string   `json:"uptime"`
			CPUPercent float64  `json:"cpu_percent"`
			MemoryMB   float64  `json:"memory_mb"`
			Ports      []uint32 `json:"ports"`
		}

		var enriched []projectStatus
		for _, p := range projects {
			isRunning, pid, uptime, _ := actions.GetProcessStatus(p.ServiceDir)
			ps := projectStatus{
				Project:   p,
				IsRunning: isRunning,
				PID:       pid,
				Uptime:    uptime,
			}
			// F14: Get resource stats for running processes
			if isRunning && pid > 0 {
				ps.CPUPercent, ps.MemoryMB = getProcessStats(pid)
				ps.Ports = getProcessListeningPorts(int32(pid))
			}
			enriched = append(enriched, ps)
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
		if req.RepoURL == "" || req.ServiceName == "" {
			return c.Status(400).SendString("Missing required fields: repo_url, service_name, service_user")
		}
		if req.ServiceUser == "" {

			// Generate clean slug from project name
			slug := deploypath.Slugify(req.ServiceName)

			// Generate random suffix to avoid any collisions
			suffix, err := deploypath.RandomHex(6)
			if err != nil {
				logger.Error(fmt.Sprintf("failed to generate random hex: %v", err), cfg)
				suffix = strconv.Itoa(rand.Intn(1000000))
			}

			serviceUserName := fmt.Sprintf("%s-%s", slug, suffix)
			req.ServiceUser = "cicd_usr_" + serviceUserName
		}
		if req.Branch == "" {
			req.Branch = "main"
		}
		if req.BaseDir == "" {
			req.BaseDir = cfg.ServiceDir
		}

		// EC-16: Port conflict check (only if a specific port is requested)
		if req.WebhookPort > 0 {
			existing, _ := database.CheckPortConflict(req.WebhookPort, 0)
			if existing != "" {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Port %d already in use by project '%s'", req.WebhookPort, existing)})
			}
		}

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

		projectCfg := *cfg
		projectCfg.RepoURL = req.RepoURL
		projectCfg.Branch = req.Branch
		projectCfg.ServiceName = req.ServiceName
		projectCfg.ServiceUser = req.ServiceUser
		projectCfg.ServiceDir = res.FinalPath
		if req.AdminEmail != "" {
			projectCfg.AdminEmail = req.AdminEmail
		}
		if req.GitUsername != "" {
			projectCfg.GitUsername = req.GitUsername
		}
		if req.GitPassword != "" {
			projectCfg.GitPassword = req.GitPassword
		}
		if req.PublicIP == "" {
			req.PublicIP = cfg.PublicIP // Use the server's own detected IP as default
		}

		// Validate IP format (simple check)
		if net.ParseIP(req.PublicIP) == nil && req.PublicIP != "localhost" && req.PublicIP != "127.0.0.1" {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid Public IP address format"})
		}

		if req.PublicIP != "" {
			projectCfg.PublicIP = req.PublicIP
		}
		if req.SudoPass != "" {
			projectCfg.SudoPass = req.SudoPass
		}
		if req.WebhookPort > 0 {
			projectCfg.Webhook = req.WebhookPort
		}
		if req.WebhookSecret != "" {
			projectCfg.WebhookSecret = req.WebhookSecret
		}

		if err := actions.SetupProjectFolder(&projectCfg); err != nil {
			return c.Status(500).SendString("Folder Setup Failed: " + err.Error())
		}
		database.RegisterProject(&projectCfg)
		database.RegisterToMongo(&projectCfg)

		// Use deployment queue
		actions.EnqueueDeployment(&projectCfg, "", "", actions.FullDeployPipeline)

		return c.Status(201).JSON(fiber.Map{
			"message":     "Project created and deployment started",
			"service_dir": projectCfg.ServiceDir,
		})
	})

	api.Put("/projects/:id", func(c fiber.Ctx) error {
		id := c.Params("id")
		project, err := database.GetProjectByID(id)
		if err != nil {
			return c.Status(404).SendString("Project not found")
		}

		// Use *string pointers: nil = "not sent, keep current value"
		//                        "" = "explicitly cleared"
		type UpdateRequest struct {
			RepoURL       *string `json:"repo_url"`
			Branch        *string `json:"branch"`
			ServiceName   *string `json:"service_name"`
			ServiceUser   *string `json:"service_user"`
			AdminEmail    *string `json:"admin_email"`
			GitUsername   *string `json:"git_username"`
			GitPassword   *string `json:"git_password"`
			WebhookPort   *int    `json:"webhook_port"`
			PublicIP      *string `json:"public_ip"`
			SudoPass      *string `json:"sudo_pass"`
			WebhookSecret *string `json:"webhook_secret"`
			MongoDBURI    *string `json:"mongodb_uri"`
		}

		var req UpdateRequest
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(400).SendString("Invalid request format")
		}

		// Model after existing but update only sent fields
		appCfg, err := project.ToConfig()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}

		// For each field: if the pointer is non-nil (field was sent), apply the value.
		// An empty string explicitly clears the field (e.g., removing SudoPass).
		if req.RepoURL       != nil { appCfg.RepoURL       = *req.RepoURL }
		if req.Branch        != nil { appCfg.Branch        = *req.Branch }
		if req.ServiceName   != nil { appCfg.ServiceName   = *req.ServiceName }
		if req.ServiceUser   != nil { appCfg.ServiceUser   = *req.ServiceUser }
		if req.AdminEmail    != nil { appCfg.AdminEmail    = *req.AdminEmail }
		if req.GitUsername   != nil { appCfg.GitUsername   = *req.GitUsername }
		if req.GitPassword   != nil { appCfg.GitPassword   = *req.GitPassword }
		if req.PublicIP      != nil { appCfg.PublicIP      = *req.PublicIP }
		if req.SudoPass      != nil { appCfg.SudoPass      = *req.SudoPass }
		if req.WebhookSecret != nil { appCfg.WebhookSecret = *req.WebhookSecret }
		if req.MongoDBURI    != nil { appCfg.MongoDBURI    = *req.MongoDBURI }

		// Port conflict check
		if req.WebhookPort != nil && *req.WebhookPort > 0 {
			existing, _ := database.CheckPortConflict(*req.WebhookPort, project.ID)
			if existing != "" {
				return c.Status(400).JSON(fiber.Map{"error": fmt.Sprintf("Port %d already in use by project '%s'", *req.WebhookPort, existing)})
			}
			appCfg.Webhook = *req.WebhookPort
		}


		if err := database.UpdateProject(id, appCfg); err != nil {
			return c.Status(500).SendString(err.Error())
		}

		// Trigger a redeploy with the updated config so changes take effect immediately.
		// This is equivalent to clicking "Deploy" after saving.
		actions.EnqueueDeployment(appCfg, "", "", func(cfg *parser.Config, h, m string) {
			actions.SyncRepo(cfg)
			actions.RunDeployment(cfg)
		})

		return c.JSON(fiber.Map{
			"message": "Project updated and redeployment triggered",
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
		f, err := os.Open("/var/log/cicd/system.log")
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		defer f.Close()
		stat, _ := f.Stat()
		const tailBytes = 100 * 1024 // last 100 KB only — never blocks
		if stat.Size() > tailBytes {
			f.Seek(stat.Size()-tailBytes, io.SeekStart)
			// skip the partial first line
			buf := make([]byte, 1)
			for {
				n, e := f.Read(buf)
				if n > 0 && buf[0] == '\n' {
					break
				}
				if e != nil {
					break
				}
			}
		}
		data, _ := io.ReadAll(f)
		return c.SendString(string(data))
	})

	// SSE: live-tail of the system log — same pattern as project log stream
	api.Get("/system/logs/stream", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("X-Accel-Buffering", "no")
		const sysLog = "/var/log/cicd/system.log"
		c.Response().SetBodyStreamWriter(func(w *bufio.Writer) {
			fmt.Fprintf(w, ": heartbeat\n\n")
			w.Flush()
			file, err := os.Open(sysLog)
			if err != nil {
				fmt.Fprintf(w, "data: ❌ Cannot open system log: %v\n\n", err)
				w.Flush()
				return
			}
			defer file.Close()
			stat, _ := file.Stat()
			const tailBytes = 50 * 1024
			offset := int64(0)
			if stat.Size() > tailBytes {
				offset = stat.Size() - tailBytes
			}
			file.Seek(offset, io.SeekStart)
			scanner := bufio.NewScanner(file)
			if offset > 0 {
				scanner.Scan() // discard partial first line
			}
			for scanner.Scan() {
				fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
			}
			w.Flush()
			reader := bufio.NewReader(file)
			lastHeartbeat := time.Now()
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
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

	api.Post("/projects/:id/stop", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		actions.StopAppProcess(project.ServiceDir)
		database.SetDeployStatus(c.Params("id"), "idle")
		actions.Notify(cfg, "🛑 Service Stopped", fmt.Sprintf("Service '%s' was manually stopped via Dashboard by Admin.", project.ServiceName))
		return c.SendString("Stopped")
	})

	api.Post("/projects/:id/deploy", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		appCfg, err := project.ToConfig()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		actions.EnqueueDeployment(appCfg, "", "", func(cfg *parser.Config, h, m string) {
			actions.SyncRepo(cfg)
			actions.RunDeployment(cfg)
		})
		return c.SendString("Deployment Started")
	})

	// F2: Commit history
	api.Get("/projects/:id/history", func(c fiber.Ctx) error {
		id, _ := strconv.Atoi(c.Params("id"))
		entries, err := database.GetCommitHistory(id, 50)
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		return c.JSON(entries)
	})

	// F4: Rollback
	api.Post("/projects/:id/rollback", func(c fiber.Ctx) error {
		type req struct {
			CommitHash string `json:"commit_hash"`
		}
		var r req
		if err := c.Bind().Body(&r); err != nil || r.CommitHash == "" {
			return c.Status(400).SendString("Missing commit_hash")
		}
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		appCfg, err := project.ToConfig()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		go actions.RollbackToCommit(appCfg, r.CommitHash)
		return c.JSON(fiber.Map{"message": "Rollback initiated to " + r.CommitHash[:8]})
	})

	// F5: Pin/Unpin
	api.Post("/projects/:id/pin", func(c fiber.Ctx) error {
		return database.SetProjectPinned(c.Params("id"), true)
	})
	api.Post("/projects/:id/unpin", func(c fiber.Ctx) error {
		return database.SetProjectPinned(c.Params("id"), false)
	})

	// F13: Force reinstall
	api.Post("/projects/:id/reinstall", func(c fiber.Ctx) error {
		project, err := database.GetProjectByID(c.Params("id"))
		if err != nil {
			return c.Status(404).SendString("Not found")
		}
		hashFile := filepath.Join(project.ServiceDir, ".cicdlog", "install.hash")
		os.Remove(hashFile)
		appCfg, err := project.ToConfig()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		actions.EnqueueDeployment(appCfg, "", "", func(cfg *parser.Config, h, m string) {
			actions.SyncRepo(cfg)
			actions.RunDeployment(cfg)
		})
		return c.JSON(fiber.Map{"message": "Force reinstall triggered"})
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
		appCfg, err := project.ToConfig()
		if err != nil {
			return c.Status(500).SendString(err.Error())
		}
		output, err := actions.RunCustomCommand(appCfg, req.Command)
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
		actions.StopAppProcess(project.ServiceDir)
		if err := database.DeleteProject(c.Params("id")); err != nil {
			return c.Status(500).SendString(err.Error())
		}
		actions.Notify(cfg, "🗑️ Project Deleted", fmt.Sprintf("Project '%s' (Path: %s) was permanently removed from node %s.", project.ServiceName, project.ServiceDir, cfg.PublicIP))
		return c.SendString("Deleted")
	})

	// F15: Server-wide open ports
	api.Get("/server/ports", func(c fiber.Ctx) error {
		ports := getServerListeningPorts()
		return c.JSON(fiber.Map{"listening": ports})
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
					IsRunning  bool     `json:"is_running"`
					PID        int      `json:"pid"`
					Uptime     string   `json:"uptime"`
					CPUPercent float64  `json:"cpu_percent"`
					MemoryMB   float64  `json:"memory_mb"`
					Ports      []uint32 `json:"ports"`
				}
				var enriched []projectStatus
				for _, p := range projects {
					isRunning, pid, uptime, _ := actions.GetProcessStatus(p.ServiceDir)
					ps := projectStatus{
						Project:   p,
						IsRunning: isRunning,
						PID:       pid,
						Uptime:    uptime,
					}
					if isRunning && pid > 0 {
						ps.CPUPercent, ps.MemoryMB = getProcessStats(pid)
						ps.Ports = getProcessListeningPorts(int32(pid))
					}
					enriched = append(enriched, ps)
				}
				data, _ := json.Marshal(enriched)
				fmt.Fprintf(w, "data: %s\n\n", data)
				if err := w.Flush(); err != nil {
					return
				}
				time.Sleep(5 * time.Second) // 3s instead of 2s to reduce CPU
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
		c.Set("X-Accel-Buffering", "no")

		logPath := filepath.Join(project.ServiceDir, ".cicdlog", "deploy.log")
		c.Response().SetBodyStreamWriter(func(w *bufio.Writer) {
			fmt.Fprintf(w, ": heartbeat\n\n")
			w.Flush()
			file, err := os.Open(logPath)
			if err != nil {
				fmt.Fprintf(w, "data: ❌ Failed to open log file: %v\n\n", err)
				w.Flush()
				return
			}
			defer file.Close()
			stat, _ := file.Stat()
			offset := int64(2048)
			if stat.Size() < offset {
				offset = 0
			} else {
				offset = stat.Size() - offset
			}
			file.Seek(offset, io.SeekStart)
			scanner := bufio.NewScanner(file)
			if offset > 0 {
				scanner.Scan()
			}
			for scanner.Scan() {
				fmt.Fprintf(w, "data: %s\n\n", scanner.Text())
			}
			w.Flush()
			reader := bufio.NewReader(file)
			lastHeartbeat := time.Now()
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
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
	api.Get("/settings", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"mongodb_uri":    cfg.MongoDBURI,
			"admin_email":    cfg.AdminEmail,
			"webhook_port":   cfg.Webhook,
			"webhook_secret": cfg.WebhookSecret,
			"smtp_host":      cfg.SMTPHost,
			"smtp_port":      cfg.SMTPPort,
			"smtp_user":      cfg.SMTPUser,
			"smtp_pass":      cfg.SMTPPass,
			"public_ip":      cfg.PublicIP,
			"notify_url":     cfg.NotifyURL,
			"deploy_timeout": cfg.DeployTimeout,
		})
	})

	api.Put("/settings", func(c fiber.Ctx) error {
		type SettingsRequest struct {
			MongoDBURI    string `json:"mongodb_uri"`
			AdminEmail    string `json:"admin_email"`
			WebhookPort   int    `json:"webhook_port"`
			WebhookSecret string `json:"webhook_secret"`
			SMTPHost      string `json:"smtp_host"`
			SMTPPort      int    `json:"smtp_port"`
			SMTPUser      string `json:"smtp_user"`
			SMTPPass      string `json:"smtp_pass"`
			PublicIP      string `json:"public_ip"`
			NotifyURL     string `json:"notify_url"`
			DeployTimeout int    `json:"deploy_timeout"`
		}
		var req SettingsRequest
		if err := c.Bind().JSON(&req); err != nil {
			return c.Status(400).SendString("Invalid format")
		}
		if req.MongoDBURI != "" {
			cfg.MongoDBURI = req.MongoDBURI
		}
		if req.AdminEmail != "" {
			cfg.AdminEmail = req.AdminEmail
		}
		if req.WebhookPort != 0 {
			cfg.Webhook = req.WebhookPort
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

		if req.NotifyURL != "" {
			cfg.NotifyURL = req.NotifyURL
		}
		if req.DeployTimeout != 0 {
			cfg.DeployTimeout = req.DeployTimeout
		}
		if strings.ToUpper(req.PublicIP) == "AUTO" || req.PublicIP == "" {
			cfg.PublicIP = parser.GetPublicIP()
		}
		database.SaveGlobalSettings(cfg)
		logger.Info("⚙️  Global settings updated via UI", cfg)
		actions.Notify(cfg, "⚙️ Infrastructure: Settings Updated", fmt.Sprintf("Global configuration (Ports/SMTP/Secrets) was modified via Dashboard from IP: %s. System is restarting.", c.IP()))
		go func() {
			time.Sleep(1 * time.Second)
			os.Exit(0)
		}()
		return c.JSON(fiber.Map{"message": "Settings saved. Orchestrator is restarting..."})
	})

	addr := fmt.Sprintf(":%d", cfg.Webhook)
	logger.Info(fmt.Sprintf("🛰️  Unified Gateway starting on %s", addr), cfg)
	if err := app.Listen(addr); err != nil {
		logger.Error(fmt.Sprintf("❌ CRITICAL: Failed to start API Gateway: %v", err), cfg)
		panic(fmt.Sprintf("PORT %d IS ALREADY IN USE!", cfg.Webhook))
	}
}

// ═══════════════════════════════════════════════════════
//  HELPERS
// ═══════════════════════════════════════════════════════

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
	Hash      string
	Branch    string
	Repo      string
	CommitMsg string
}

// parseWebhookPayload extracts the raw fields from a GitHub push event body.
// All validation (repo match, branch match) is done by the caller after a DB lookup.
func parseWebhookPayload(c fiber.Ctx) (*WebhookData, error) {
	p := jsjson.MustParse(string(c.Body()))
	data := &WebhookData{}
	data.Hash, _ = p.Get("head_commit", "id").String()
	data.Branch, _ = p.Get("ref").String()
	data.Repo, _ = p.Get("repository", "html_url").String()
	data.CommitMsg, _ = p.Get("head_commit", "message").String()
	if data.Repo == "" {
		return nil, fmt.Errorf("missing repository url in payload")
	}
	return data, nil
}

// F14: Get CPU% and Memory for a process
func getProcessStats(pid int) (cpuPct float64, memMB float64) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, 0
	}
	cpu, _ := p.CPUPercent()
	mem, _ := p.MemoryInfo()
	if mem != nil {
		memMB = float64(mem.RSS) / 1024 / 1024
	}
	return cpu, memMB
}

// F15: Get listening ports for a specific PID.
// Uses process-level Connections() which reads /proc/<pid>/net/tcp directly,
// correctly populating Port info without needing the global /proc/net/tcp Pid scan.
func getProcessListeningPorts(pid int32) []uint32 {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil
	}
	conns, err := p.Connections()
	if err != nil {
		// Fallback: scan global connections and match by pid
		allConns, err2 := gnet.Connections("tcp")
		if err2 != nil {
			return nil
		}
		var ports []uint32
		for _, c := range allConns {
			if c.Status == "LISTEN" && c.Pid == pid {
				ports = append(ports, c.Laddr.Port)
			}
		}
		return ports
	}
	var ports []uint32
	for _, c := range conns {
		if c.Status == "LISTEN" {
			ports = append(ports, c.Laddr.Port)
		}
	}
	return ports
}

// F15: Get all listening ports server-wide
func getServerListeningPorts() []uint32 {
	conns, err := gnet.Connections("tcp")
	if err != nil {
		return nil
	}
	seen := make(map[uint32]bool)
	var ports []uint32
	for _, c := range conns {
		if c.Status == "LISTEN" && !seen[c.Laddr.Port] {
			seen[c.Laddr.Port] = true
			ports = append(ports, c.Laddr.Port)
		}
	}
	return ports
}
