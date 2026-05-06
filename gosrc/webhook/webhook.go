package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"gosrc/parser"

	"github.com/gofiber/fiber/v3"
)

func StartWebhook(cfg *parser.Config) {
	addr := fmt.Sprintf(":%d", cfg.Webhook)

	app := fiber.New()

	app.Post("/", func(c fiber.Ctx) error {
		if cfg.WebhookSecret == "" {
			fmt.Println("Warning: Webhook secret not configured, skipping signature verification")
		} else {
			// Get the signature from the request headers
			signature := c.Get("X-Hub-Signature-256")
			if signature == "" {
				return c.Status(fiber.StatusUnauthorized).SendString("Missing signature")
			}

			// Read the request body
			bodyBytes := c.Body()

			// Compute the expected signature
			h := hmac.New(sha256.New, []byte(cfg.WebhookSecret))
			h.Write(bodyBytes)
			expectedSignature := "sha256=" + hex.EncodeToString(h.Sum(nil))

			// Compare signatures
			if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
				return c.Status(fiber.StatusUnauthorized).SendString("Invalid signature")
			}
		}

		return c.SendString(fmt.Sprintf("Received webhook for repo: %s", cfg.RepoURL))
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
