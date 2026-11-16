package email

import (
	"context"
	"strings"
	"testing"
)

func TestModeReadsAndNormalizesEnv(t *testing.T) {
	cases := map[string]string{"": "", "log": "log", "  LOG ": "log", "smtp": "smtp", "   ": BlankValue}
	for in, want := range cases {
		t.Setenv(EnvMode, in)
		if got := Mode(); got != want {
			t.Errorf("Mode() with %q = %q, want %q", in, got, want)
		}
	}
}

func TestTestPodTrimsWhitespace(t *testing.T) {
	t.Setenv(EnvTestPod, "  Hopefarm ")
	if got := TestPod(); got != "Hopefarm" {
		t.Errorf("TestPod() = %q, want Hopefarm", got)
	}
	t.Setenv(EnvTestPod, "")
	if got := TestPod(); got != "" {
		t.Errorf("TestPod() unset = %q, want empty", got)
	}
	t.Setenv(EnvTestPod, "   ")
	if got := TestPod(); got != BlankValue {
		t.Errorf("TestPod() whitespace-only = %q, want %q so it fails closed", got, BlankValue)
	}
}

func TestDeliversIsFalseForNilSender(t *testing.T) {
	if Delivers(nil) {
		t.Error("Delivers(nil) = true, want false")
	}
}

func TestLogSenderNeverDelivers(t *testing.T) {
	var s Sender = LogSender{}

	if Delivers(s) {
		t.Fatal("LogSender must report that it does not deliver")
	}
	if err := s.SendHTML(context.Background(), "a@x.com", "subject", "<p>body</p>"); err != nil {
		t.Errorf("SendHTML returned %v, want nil", err)
	}
	if err := s.SendPasswordResetEmail(context.Background(), "a@x.com", "secret-token"); err != nil {
		t.Errorf("SendPasswordResetEmail returned %v, want nil", err)
	}
}

type realSender struct{}

func (realSender) SendHTML(context.Context, string, string, string) error { return nil }
func (realSender) SendPasswordResetEmail(context.Context, string, string) error {
	return nil
}

func TestDeliversIsTrueForOrdinarySenders(t *testing.T) {
	if !Delivers(realSender{}) {
		t.Fatal("a sender with no Delivers method is assumed to deliver")
	}
}

func TestReminderIntroDiffersForLeaders(t *testing.T) {
	leader := RenderSurveyReminderText(SurveyReminderEmailData{RecipientName: "A", Role: "leader", AssessmentPeriod: "p"})
	if !strings.Contains(leader, "These pods still need to complete the survey:") {
		t.Errorf("leader text: %s", leader)
	}
}
