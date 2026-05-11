package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gosrc/database"
	"gosrc/logger"
	"gosrc/parser"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/ktbsomen/jsjson"
)

// StartWebhook starts the Fiber-based webhook listener
func StartWebhook(cfg *parser.Config) {
	addr := fmt.Sprintf(":%d", cfg.Webhook)

	app := fiber.New(fiber.Config{
		BodyLimit: 2 * 1024 * 1024, // Reject anything larger than 2MB
	})

	// Rate Limiting (10 requests per minute per IP)
	app.Use(limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
	}))

	app.Post("/", func(c fiber.Ctx) error {
		// 1. Filter out non-deployment events
		if c.Get("X-Github-Event") == "ping" {
			return c.SendString("Pong")
		}

		// 2. Security Verification
		if err := verifySignature(c, cfg.WebhookSecret); err != nil {
			logger.Warn("Unauthorized webhook attempt: "+err.Error(), cfg)
			return c.Status(401).SendString(err.Error())
		}

		// 3. Extract and Validate Payload
		data, err := parseAndValidate(c, cfg)
		if err != nil {
			// Return 200 but ignore the deployment (e.g. wrong branch)
			return c.Status(200).SendString(err.Error())
		}

		// 4. THE ACTION: Notify MongoDB Cloud
		go database.UpdateCommitHash(cfg, data.Hash)

		logger.Info(fmt.Sprintf("🚀 Triggered update for '%s' (Commit: %s) by %s", cfg.ServiceName, data.Hash[:8], data.Author), cfg)
		return c.SendString("Deployment initiated successfully")
	})

	fmt.Printf("Webhook listener starting on %s%s\n", cfg.PublicIP, addr)
	if err := app.Listen(addr, fiber.ListenConfig{
		DisableStartupMessage: false,
		EnablePrefork:         false,
	}); err != nil {
		fmt.Printf("Error starting webhook listener: %v\n", err)
		panic(err)
	}
}

// --- Helper Functions ---

func verifySignature(c fiber.Ctx, secret string) error {
	if secret == "" {
		return nil
	}

	signature := c.Get("X-Hub-Signature-256")
	if signature == "" {
		return fmt.Errorf("missing signature header")
	}

	h := hmac.New(sha256.New, []byte(secret))
	h.Write(c.Body())
	expected := "sha256=" + hex.EncodeToString(h.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return fmt.Errorf("invalid HMAC signature")
	}
	return nil
}

type WebhookData struct {
	Hash   string
	Branch string
	Author string
	Repo   string
}

func parseAndValidate(c fiber.Ctx, cfg *parser.Config) (*WebhookData, error) {
	p := jsjson.MustParse(string(c.Body()))

	data := &WebhookData{}
	data.Hash, _ = p.Get("head_commit", "id").String()
	data.Branch, _ = p.Get("ref").String()
	data.Author, _ = p.Get("head_commit", "author", "name").String()
	data.Repo, _ = p.Get("repository", "html_url").String()

	isDeleted, _ := p.Get("deleted").Bool()

	// Validation Rules
	if isDeleted {
		return nil, fmt.Errorf("ignoring branch deletion event")
	}
	if data.Repo != cfg.RepoURL {
		return nil, fmt.Errorf("repository mismatch: %s", data.Repo)
	}
	if data.Branch != "refs/heads/"+cfg.Branch {
		return nil, fmt.Errorf("ignoring push to branch %s (watching %s)", data.Branch, cfg.Branch)
	}
	if data.Hash == "" {
		return nil, fmt.Errorf("missing commit hash in payload")
	}

	return data, nil
}
