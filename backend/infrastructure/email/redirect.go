package email

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// EnvRedirectTo lists the addresses every reminder email is delivered to
// INSTEAD of its real recipient, so a team can test real delivery without
// anyone outside the test receiving anything. Comma, semicolon or space
// separated.
const EnvRedirectTo = "EMAIL_REDIRECT_TO"

// RedirectTargets parses EMAIL_REDIRECT_TO. configured is false when the
// variable is empty or unset. An error means it is set but unusable (an entry
// that is not a plain email address, or nothing left after splitting); the
// caller must then send nothing rather than fall back to real recipients.
func RedirectTargets(raw string) (targets []string, configured bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}

	seen := map[string]bool{}
	for _, token := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	}) {
		addr, perr := mail.ParseAddress(token)
		if perr != nil || addr.Address != token {
			return nil, true, fmt.Errorf("%s has an entry that is not a plain email address: %q", EnvRedirectTo, token)
		}
		key := strings.ToLower(token)
		if !seen[key] {
			seen[key] = true
			targets = append(targets, token)
		}
	}
	if len(targets) == 0 {
		return nil, true, fmt.Errorf("%s is set but contains no addresses", EnvRedirectTo)
	}
	return targets, true, nil
}

// RedirectSender wraps a real Sender and delivers every email to a fixed list
// of test addresses instead of the address it was meant for. The email itself
// is sent unchanged, so it reads exactly as a real one would. The original
// recipient is only recorded in the server log and is never passed to the
// wrapped sender.
type RedirectSender struct {
	inner Sender
	to    []string
}

// NewRedirectSender redirects everything sent through inner to the given
// addresses.
func NewRedirectSender(inner Sender, to []string) *RedirectSender {
	return &RedirectSender{inner: inner, to: append([]string(nil), to...)}
}

// RedirectTargets returns the addresses everything is delivered to.
func (r *RedirectSender) RedirectTargets() []string { return append([]string(nil), r.to...) }

// Delivers reports whether the wrapped sender delivers anything.
func (r *RedirectSender) Delivers() bool { return Delivers(r.inner) }

// PartialDeliveryError means at least one redirect target received the email
// and at least one did not. Retrying would send the successful targets a
// duplicate, so callers should treat the email as sent.
type PartialDeliveryError struct {
	Delivered, Failed int
	Err               error
}

func (e *PartialDeliveryError) Error() string {
	return fmt.Sprintf("delivered to %d redirect target(s), failed for %d: %v", e.Delivered, e.Failed, e.Err)
}

func (e *PartialDeliveryError) Unwrap() error { return e.Err }

// SendHTML delivers one copy of the email to each test address.
func (r *RedirectSender) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	logger.Get().
		WithField("intended_to", strings.NewReplacer("\r", "", "\n", "").Replace(to)).
		WithField("delivered_to", r.to).
		Info("reminder email redirected")

	var firstErr error
	failed := 0
	for _, target := range r.to {
		if err := r.inner.SendHTML(ctx, target, subject, htmlBody); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	if failed > 0 && failed < len(r.to) {
		return &PartialDeliveryError{Delivered: len(r.to) - failed, Failed: failed, Err: firstErr}
	}
	return firstErr
}

// SendPasswordResetEmail is never forwarded. A reset token is a credential
// for the real account, and sending it to a test address would hand that
// account to whoever reads the mailbox.
func (r *RedirectSender) SendPasswordResetEmail(_ context.Context, emailAddr, _ string) error {
	logger.Get().
		WithField("to", emailAddr).
		Warn("password reset email suppressed: EMAIL_REDIRECT_TO is set, and reset tokens are never redirected")
	return nil
}

// RedirectTargetsOf returns the test addresses a sender redirects to, or nil
// when it does not redirect.
func RedirectTargetsOf(s Sender) []string {
	if r, ok := s.(interface{ RedirectTargets() []string }); ok {
		return r.RedirectTargets()
	}
	return nil
}
