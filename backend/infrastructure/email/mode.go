package email

import (
	"context"
	"os"
	"strings"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

const (
	// EnvMode selects how email leaves the app. Unset keeps the normal
	// behavior (SES, then SMTP, then disabled). "log" sends nothing and
	// writes each email to the log instead.
	EnvMode = "EMAIL_MODE"

	// EnvTestPod names the one pod survey reminders may reach. When set,
	// recipients are limited to that pod's team lead and members.
	EnvTestPod = "EMAIL_TEST_POD"

	// ModeLog is the EMAIL_MODE value that turns delivery off.
	ModeLog = "log"
)

// Mode returns the configured EMAIL_MODE, lower-cased and trimmed. "" means
// unset. The caller must treat any value it does not recognize as "do not
// send", never as "fall back to real delivery".
func Mode() string {
	return trimmedEnv(EnvMode, true)
}

// TestPod returns the pod name survey reminders are restricted to, or "".
//
// A variable that is set but only whitespace is NOT treated as unset: that
// would silently lift the restriction. It returns BlankValue, which matches no
// pod, so the handler fails closed.
func TestPod() string {
	return trimmedEnv(EnvTestPod, false)
}

// BlankValue is returned for a variable that is set to whitespace only. It is
// never a valid mode or pod name, so callers fail closed instead of reading it
// as "unset".
const BlankValue = "<blank>"

func trimmedEnv(name string, lower bool) string {
	raw := os.Getenv(name)
	v := strings.TrimSpace(raw)
	if lower {
		v = strings.ToLower(v)
	}
	if v == "" && raw != "" {
		return BlankValue
	}
	return v
}

// LogSender is the Sender used when EMAIL_MODE=log. It never opens a
// connection and never delivers anything: every email is written to the log
// and reported as handled. It covers every kind of email the app sends, so
// nothing leaves the process while it is in use.
type LogSender struct{}

// SendHTML logs the recipient and subject. The body is left out of the log.
func (LogSender) SendHTML(_ context.Context, to, subject, _ string) error {
	logger.Get().
		WithField("to", to).
		WithField("subject", subject).
		Info("email (log mode): NOT sent")
	return nil
}

// SendPasswordResetEmail logs only the recipient. The reset token is a
// credential and is never written to the log.
func (LogSender) SendPasswordResetEmail(_ context.Context, emailAddr, _ string) error {
	logger.Get().
		WithField("to", emailAddr).
		Info("password reset email (log mode): NOT sent")
	return nil
}

// Delivers reports that this sender sends nothing.
func (LogSender) Delivers() bool { return false }

// Delivers reports whether sender actually delivers email. A sender opts out
// by having a Delivers() bool method that returns false (LogSender does);
// everything else is assumed to deliver.
func Delivers(sender Sender) bool {
	if sender == nil {
		return false
	}
	if d, ok := sender.(interface{ Delivers() bool }); ok {
		return d.Delivers()
	}
	return true
}
