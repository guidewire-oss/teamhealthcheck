package v1

import (
	"strings"
	"testing"
)

// The Authorization/Bearer header shape is covered by the handler spec that
// reads back run-2's sanitized message, so it is not repeated here.
func TestSanitizeRunErrorRedactsCredentialShapes(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{"plain equals", "password=hunter2", "hunter2"},
		{"colon with space", "token: abc12345", "abc12345"},
		{"underscore prefix", "db_password=hunter2", "hunter2"},
		{"upper snake case", "ACCESS_TOKEN=abc12345", "abc12345"},
		{"camel case", "clientSecret: s3cr3t-value", "s3cr3t-value"},
		{"api key header", "x-api-key: abc12345", "abc12345"},
		{"json double quoted", `{"password": "hunter2"}`, "hunter2"},
		{"json no space", `{"password":"hunter2"}`, "hunter2"},
		{"json prefixed key", `{"db_password": "hunter2", "user": "bob"}`, "hunter2"},
		{"json camel case key", `{"clientSecret":"s3cr3t-value"}`, "s3cr3t-value"},
		{"single quoted key and value", `{'password': 'hunter2'}`, "hunter2"},
		{"escaped json inside a string", `provider said {\"password\":\"hunter2\"}`, "hunter2"},
		{"quoted value with spaces", `password="hunter 2 x"`, "hunter 2 x"},
		{"spaced separator", "pwd  =  hunter2", "hunter2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeRunError(c.in)
			if strings.Contains(got, c.secret) {
				t.Fatalf("sanitizeRunError(%q) = %q, still contains %q", c.in, got, c.secret)
			}
			if !strings.Contains(got, "[redacted]") {
				t.Fatalf("sanitizeRunError(%q) = %q, want a [redacted] marker", c.in, got)
			}
		})
	}
}

func TestSanitizeRunErrorKeepsOrdinaryText(t *testing.T) {
	cases := []string{
		"Provider call failed with status 503",
		"tokenizer failed to start",
		"secretary lookup returned 0 rows",
		"the token expired",
		"user bob not found",
	}
	for _, in := range cases {
		if got := sanitizeRunError(in); got != in {
			t.Errorf("sanitizeRunError(%q) = %q, want it unchanged", in, got)
		}
	}
}

func TestSanitizeRunErrorKeepsSurroundingContext(t *testing.T) {
	got := sanitizeRunError(`Provider rejected {"password": "hunter2"} for sync`)
	if !strings.HasPrefix(got, "Provider rejected") {
		t.Errorf("lost the leading context: %q", got)
	}
}
