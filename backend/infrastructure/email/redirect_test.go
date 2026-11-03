package email

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordingSender struct {
	calls    []sentMail
	failFor  map[string]bool
	resetCnt int
}

type sentMail struct{ to, subject, body string }

func (r *recordingSender) SendHTML(_ context.Context, to, subject, body string) error {
	r.calls = append(r.calls, sentMail{to, subject, body})
	if r.failFor[to] {
		return errors.New("rejected")
	}
	return nil
}

func (r *recordingSender) SendPasswordResetEmail(context.Context, string, string) error {
	r.resetCnt++
	return nil
}

func TestRedirectTargetsParsing(t *testing.T) {
	cases := []struct {
		name       string
		raw        string
		want       []string
		configured bool
		wantErr    bool
	}{
		{"unset is not configured", "", nil, false, false},
		{"whitespace only is not configured", "   ", nil, false, false},
		{"one address", "a@x.com", []string{"a@x.com"}, true, false},
		{"comma, semicolon and space separators", "a@x.com, b@x.com;c@x.com d@x.com", []string{"a@x.com", "b@x.com", "c@x.com", "d@x.com"}, true, false},
		{"duplicates collapse, case-insensitively", "a@x.com,A@X.com", []string{"a@x.com"}, true, false},
		{"a typo is rejected", "a@x.com,not-an-email", nil, true, true},
		{"a display-name form is rejected", "Jane <jane@x.com>", nil, true, true},
		{"only separators is rejected", ",;,", nil, true, true},
	}
	for _, c := range cases {
		got, configured, err := RedirectTargets(c.raw)
		if configured != c.configured {
			t.Errorf("%s: configured = %v, want %v", c.name, configured, c.configured)
		}
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", c.name, err, c.wantErr)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: targets = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestRedirectSenderNeverUsesTheRealRecipient(t *testing.T) {
	inner := &recordingSender{}
	s := NewRedirectSender(inner, []string{"cw1@x.com", "cw2@x.com"})

	if err := s.SendHTML(context.Background(), "realteamlead@x.com", "Survey reminder", "<html><body><p>hi</p></body></html>"); err != nil {
		t.Fatal(err)
	}

	if len(inner.calls) != 2 {
		t.Fatalf("expected one copy per test address, got %d", len(inner.calls))
	}
	for _, c := range inner.calls {
		if c.to == "realteamlead@x.com" {
			t.Fatalf("the real recipient was passed to the real sender: %+v", c)
		}
	}
	if inner.calls[0].to != "cw1@x.com" || inner.calls[1].to != "cw2@x.com" {
		t.Errorf("unexpected recipients: %s, %s", inner.calls[0].to, inner.calls[1].to)
	}
}

func TestRedirectSenderLeavesTheEmailUnchanged(t *testing.T) {
	inner := &recordingSender{}
	s := NewRedirectSender(inner, []string{"cw1@x.com"})
	body := "<html><body style=\"x\"><p>hi</p></body></html>"

	_ = s.SendHTML(context.Background(), "real@x.com", "Survey reminder", body)

	c := inner.calls[0]
	if c.subject != "Survey reminder" || c.body != body {
		t.Errorf("email was altered: subject=%q body=%s", c.subject, c.body)
	}
	if strings.Contains(c.subject+c.body, "real@x.com") {
		t.Error("the real recipient must not appear in what is delivered")
	}
}

func TestRedirectSenderKeepsGoingAfterOneTargetFails(t *testing.T) {
	inner := &recordingSender{failFor: map[string]bool{"cw1@x.com": true}}
	s := NewRedirectSender(inner, []string{"cw1@x.com", "cw2@x.com"})

	err := s.SendHTML(context.Background(), "real@x.com", "s", "<body></body>")

	var partial *PartialDeliveryError
	if !errors.As(err, &partial) {
		t.Fatalf("one success and one failure must be a *PartialDeliveryError, got %v", err)
	}
	if partial.Delivered != 1 || partial.Failed != 1 {
		t.Errorf("counts = delivered %d, failed %d; want 1 and 1", partial.Delivered, partial.Failed)
	}
	if len(inner.calls) != 2 {
		t.Errorf("the second address must still be tried, got %d calls", len(inner.calls))
	}
}

func TestRedirectSenderReportsPlainErrorWhenEveryTargetFails(t *testing.T) {
	inner := &recordingSender{failFor: map[string]bool{"cw1@x.com": true, "cw2@x.com": true}}
	s := NewRedirectSender(inner, []string{"cw1@x.com", "cw2@x.com"})

	err := s.SendHTML(context.Background(), "real@x.com", "s", "<body></body>")

	var partial *PartialDeliveryError
	if err == nil || errors.As(err, &partial) {
		t.Errorf("total failure must be a plain error so it can be retried, got %v", err)
	}
}

func TestRedirectSenderNeverForwardsPasswordResets(t *testing.T) {
	inner := &recordingSender{}
	s := NewRedirectSender(inner, []string{"cw1@x.com"})

	if err := s.SendPasswordResetEmail(context.Background(), "real@x.com", "secret-token"); err != nil {
		t.Fatal(err)
	}
	if inner.resetCnt != 0 || len(inner.calls) != 0 {
		t.Fatal("a password reset token must never reach a test address")
	}
}

func TestRedirectSenderReportsItsTargetsAndDelivery(t *testing.T) {
	s := NewRedirectSender(&recordingSender{}, []string{"cw1@x.com"})

	if got := RedirectTargetsOf(s); !reflect.DeepEqual(got, []string{"cw1@x.com"}) {
		t.Errorf("RedirectTargetsOf = %v", got)
	}
	if RedirectTargetsOf(&recordingSender{}) != nil {
		t.Error("an ordinary sender has no redirect targets")
	}
	if !Delivers(s) {
		t.Error("wrapping a delivering sender must still report delivery")
	}
	if Delivers(NewRedirectSender(LogSender{}, []string{"cw1@x.com"})) {
		t.Error("wrapping the log sender must not claim delivery")
	}
}
