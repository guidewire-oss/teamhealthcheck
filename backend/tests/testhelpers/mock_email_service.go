// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package testhelpers

import "context"

// MockEmailService is a mock email service for testing.
// It records all calls so tests can verify email sends.
type MockEmailService struct {
	SentEmails []struct {
		Email string
		Token string
	}
	SentHTMLEmails []struct {
		To      string
		Subject string
		Body    string
	}
}

// NewMockEmailService creates a new mock email service
func NewMockEmailService() *MockEmailService {
	return &MockEmailService{
		SentEmails: make([]struct {
			Email string
			Token string
		}, 0),
		SentHTMLEmails: make([]struct {
			To      string
			Subject string
			Body    string
		}, 0),
	}
}

// SendHTML mock implementation
func (m *MockEmailService) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	m.SentHTMLEmails = append(m.SentHTMLEmails, struct {
		To      string
		Subject string
		Body    string
	}{To: to, Subject: subject, Body: htmlBody})
	return nil
}

// SendPasswordResetEmail mock implementation
func (m *MockEmailService) SendPasswordResetEmail(ctx context.Context, email, resetToken string) error {
	m.SentEmails = append(m.SentEmails, struct {
		Email string
		Token string
	}{Email: email, Token: resetToken})
	return nil
}
