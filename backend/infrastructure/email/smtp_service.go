// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package email

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/smtp"
	"os"
	"strconv"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// Config holds SMTP configuration loaded from environment variables.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// LoadConfig reads SMTP settings from environment variables.
// Returns nil if SMTP_HOST is not set (disables email).
func LoadConfig() *Config {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return nil
	}

	port := 587
	if p := os.Getenv("SMTP_PORT"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}

	from := os.Getenv("SMTP_FROM")
	if from == "" {
		from = "noreply@teams360.example.com"
	}

	return &Config{
		Host:     host,
		Port:     port,
		Username: os.Getenv("SMTP_USERNAME"),
		Password: os.Getenv("SMTP_PASSWORD"),
		From:     from,
	}
}

// SMTPEmailService sends emails via SMTP.
type SMTPEmailService struct {
	config *Config
}

// NewSMTPEmailService creates a new SMTP email service.
func NewSMTPEmailService(config *Config) *SMTPEmailService {
	return &SMTPEmailService{config: config}
}

// SendHTML sends an HTML email to the specified recipient.
func (s *SMTPEmailService) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	log := logger.Get()

	// Sanitize to and subject to prevent header injection
	to = strings.NewReplacer("\r", "", "\n", "").Replace(to)
	subject = strings.NewReplacer("\r", "", "\n", "").Replace(subject)

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	headers := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n",
		s.config.From, to, subject)
	msg := []byte(headers + htmlBody)

	var auth smtp.Auth
	if s.config.Username != "" {
		auth = smtp.PlainAuth("", s.config.Username, s.config.Password, s.config.Host)
	}

	// Use TLS for port 465, STARTTLS for others
	if s.config.Port == 465 {
		tlsConfig := &tls.Config{ServerName: s.config.Host}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, s.config.Host)
		if err != nil {
			return fmt.Errorf("failed to create SMTP client: %w", err)
		}
		defer client.Close()

		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth failed: %w", err)
			}
		}

		if err := client.Mail(s.config.From); err != nil {
			return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
		}
		if err := client.Rcpt(to); err != nil {
			return fmt.Errorf("SMTP RCPT TO failed: %w", err)
		}

		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("SMTP DATA failed: %w", err)
		}
		if _, err := w.Write(msg); err != nil {
			return fmt.Errorf("SMTP write failed: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("SMTP close failed: %w", err)
		}

		return client.Quit()
	}

	// Standard SMTP with optional STARTTLS
	if err := smtp.SendMail(addr, auth, s.config.From, []string{to}, msg); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	log.WithField("to", to).WithField("subject", subject).Debug("email sent successfully")
	return nil
}

// SendPasswordResetEmail implements the EmailService interface from password_reset_service.go.
func (s *SMTPEmailService) SendPasswordResetEmail(ctx context.Context, emailAddr, resetToken string) error {
	subject := "Teams360 - Password Reset"
	body := fmt.Sprintf(`<html><body>
<h2>Password Reset Request</h2>
<p>You requested a password reset for your Teams360 account.</p>
<p>Your reset token is: <strong>%s</strong></p>
<p>This token will expire in 1 hour.</p>
<p>If you did not request this, please ignore this email.</p>
</body></html>`, resetToken)

	return s.SendHTML(ctx, emailAddr, subject, body)
}
