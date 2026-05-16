package actions

import (
	"fmt"
	"gosrc/database"
	"gosrc/logger"
	"gosrc/parser"
	"net/http"
	"strings"
	"time"
)

// Notify handles asynchronous delivery of system alerts via Webhooks and Email.
// It runs in a short-lived goroutine to prevent blocking critical deployment paths.
func Notify(cfg *parser.Config, subject string, message string) {
	if cfg == nil {
		return
	}

	// Capture project-specific values before potentially merging global settings
	notifyURL := cfg.NotifyURL
	adminEmail := cfg.AdminEmail
	serviceName := cfg.ServiceName

	// If SMTP settings are missing (common for project-specific configs), load global ones
	if cfg.SMTPHost == "" {
		database.LoadGlobalSettings(cfg)
		// Restore project-specific overrides if the global settings replaced them
		if adminEmail != "" {
			cfg.AdminEmail = adminEmail
		}
		if notifyURL != "" {
			cfg.NotifyURL = notifyURL
		}
	}

	logger.Info(fmt.Sprintf("[NOTIFY TO] notifyURL: %s, adminEmail: %s, serviceName: %s, SMTP: %s:%d\n",
		notifyURL, adminEmail, serviceName, cfg.SMTPHost, cfg.SMTPPort), cfg)

	go func() {
		// 1. Send Webhook (Slack/Discord compatible)
		if notifyURL != "" {
			logger.Info(fmt.Sprintf("📡 Attempting Webhook: %s", subject), cfg)
			payload := fmt.Sprintf(`{"text": "*[%s]* %s\n%s"}`, serviceName, subject, strings.ReplaceAll(message, `"`, `\"`))

			client := &http.Client{
				Timeout: 10 * time.Second,
			}
			resp, err := client.Post(notifyURL, "application/json", strings.NewReader(payload))
			if err != nil {
				logger.Error(fmt.Sprintf("❌ Webhook Failed: %v", err), cfg)
			} else {
				defer resp.Body.Close()
				if resp.StatusCode >= 400 {
					logger.Error(fmt.Sprintf("❌ Webhook Error [HTTP %d]", resp.StatusCode), cfg)
				} else {
					logger.Info(fmt.Sprintf("✅ Webhook Sent: %s", subject), cfg)
				}
			}
		}

		// 2. Send Email
		if adminEmail != "" {
			logger.Info(fmt.Sprintf("📧 Attempting Email: %s", subject), cfg)
			err := logger.SendMail(cfg, fmt.Sprintf("[%s] %s", serviceName, subject), message)
			if err != nil {
				logger.Error(fmt.Sprintf("❌ Email Failed: %v", err), cfg)
			} else {
				logger.Info(fmt.Sprintf("✅ Email Sent: %s", subject), cfg)
			}
		}
	}()
}

// sendNotification is a internal helper for simple string-only alerts (compat with old code)
func sendNotification(cfg *parser.Config, msg string) {
	Notify(cfg, "System Alert", msg)
}
