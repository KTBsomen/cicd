package logger

import (
	"bytes"
	"fmt"
	"gosrc/parser"
	"net/smtp"
	"strings"
	"time"
)

func SendErrorEmail(cfg *parser.Config, subject string, err string, step string) error {
	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("From: %s\r\n", cfg.SMTPUser))
	body.WriteString(fmt.Sprintf("To: %s\r\n", cfg.AdminEmail))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n\r\n", subject))
	body.WriteString(fmt.Sprintf("Error in step: %s\r\n", step))
	body.WriteString(fmt.Sprintf("Timestamp: %s\r\n", time.Now().Format(time.RFC3339)))
	body.WriteString(fmt.Sprintf("Error details: %s\r\n", err))

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.SMTPUser, strings.ReplaceAll(strings.ReplaceAll(cfg.SMTPPass, "\r\n", ""), " ", ""), cfg.SMTPHost)

	return smtp.SendMail(addr, auth, cfg.SMTPUser, []string{cfg.AdminEmail}, body.Bytes())
}
func SendMail(cfg *parser.Config, subject string, message string) error {
	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("From: %s\r\n", cfg.SMTPUser))
	body.WriteString(fmt.Sprintf("To: %s\r\n", cfg.AdminEmail))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n\r\n", subject))
	body.WriteString(message)

	addr := fmt.Sprintf("%s:%d", cfg.SMTPHost, cfg.SMTPPort)
	auth := smtp.PlainAuth("", cfg.SMTPUser, strings.ReplaceAll(strings.ReplaceAll(cfg.SMTPPass, "\r\n", ""), " ", ""), cfg.SMTPHost)

	return smtp.SendMail(addr, auth, cfg.SMTPUser, []string{cfg.AdminEmail}, body.Bytes())
}

//gmail host: smtp.gmail.com
//gmail port: 587
//gmail user : "somen6562@gmail.com"
