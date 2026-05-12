package api

import (
	"crypto/hmac"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"gosrc/database"
	"gosrc/logger"
	"gosrc/parser"
	"os"
	"path/filepath"
	"time"

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
		return c.SendString("🚀 CICD Unified Gateway is LIVE!\n\nAccess the Dashboard at: /dashboard")
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
		return c.JSON(projects)
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
