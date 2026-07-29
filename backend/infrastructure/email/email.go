// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package email

import "context"

// Sender sends HTML emails. All transport implementations (SMTP, SES) satisfy this.
type Sender interface {
	SendHTML(ctx context.Context, to, subject, htmlBody string) error
	SendPasswordResetEmail(ctx context.Context, emailAddr, resetToken string) error
}
