// SPDX-License-Identifier: Apache-2.0

package email

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/sesv2"
	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// SESConfig holds AWS SES configuration loaded from environment variables.
type SESConfig struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	FromAddress     string
}

// LoadSESConfig reads SES settings from environment variables.
// Returns nil if AWS_SES_REGION is not set (disables SES).
func LoadSESConfig() *SESConfig {
	region := os.Getenv("AWS_SES_REGION")
	if region == "" {
		return nil
	}

	fromAddress := os.Getenv("SES_FROM_ADDRESS")
	if fromAddress == "" {
		return nil
	}

	return &SESConfig{
		Region:          region,
		AccessKeyID:     os.Getenv("AWS_SES_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SES_SECRET_ACCESS_KEY"),
		FromAddress:     fromAddress,
	}
}

// SESEmailService sends emails via AWS SES SDK v2.
type SESEmailService struct {
	client *sesv2.Client
	config *SESConfig
}

// NewSESEmailService creates a new SES email service.
func NewSESEmailService(config *SESConfig) (*SESEmailService, error) {
	opts := sesv2.Options{
		Region: config.Region,
	}

	if config.AccessKeyID != "" && config.SecretAccessKey != "" {
		opts.Credentials = credentials.NewStaticCredentialsProvider(
			config.AccessKeyID,
			config.SecretAccessKey,
			"",
		)
	}

	client := sesv2.New(opts)

	return &SESEmailService{
		client: client,
		config: config,
	}, nil
}

// SendHTML sends an HTML email to the specified recipient via AWS SES.
func (s *SESEmailService) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	log := logger.Get()

	// Sanitize to and subject to prevent injection
	to = strings.NewReplacer("\r", "", "\n", "").Replace(to)
	subject = strings.NewReplacer("\r", "", "\n", "").Replace(subject)

	input := &sesv2.SendEmailInput{
		FromEmailAddress: aws.String(s.config.FromAddress),
		Destination: &types.Destination{
			ToAddresses: []string{to},
		},
		Content: &types.EmailContent{
			Simple: &types.Message{
				Subject: &types.Content{
					Data:    aws.String(subject),
					Charset: aws.String("UTF-8"),
				},
				Body: &types.Body{
					Html: &types.Content{
						Data:    aws.String(htmlBody),
						Charset: aws.String("UTF-8"),
					},
				},
			},
		},
	}

	_, err := s.client.SendEmail(ctx, input)
	if err != nil {
		return fmt.Errorf("SES SendEmail failed: %w", err)
	}

	log.WithField("to", to).WithField("subject", subject).Debug("SES email sent successfully")
	return nil
}

// SendPasswordResetEmail sends a password reset email via SES.
func (s *SESEmailService) SendPasswordResetEmail(ctx context.Context, emailAddr, resetToken string) error {
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
