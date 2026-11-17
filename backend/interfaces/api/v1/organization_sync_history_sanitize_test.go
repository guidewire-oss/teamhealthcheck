package v1

import (
	"strings"
	"testing"
)

// The Authorization/Bearer header shape is covered by the handler spec that
// reads back run-2's sanitized message, so it is not repeated here.
//
// secrets lists fragments that must all be gone. A value with spaces or an
// escaped quote is listed piece by piece, so a partial leak cannot pass by
// leaving only part of it behind.
func TestSanitizeRunErrorRedactsCredentialShapes(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		secrets []string
	}{
		{"plain equals", "password=hunter2", []string{"hunter2"}},
		{"colon with space", "token: abc12345", []string{"abc12345"}},
		{"underscore prefix", "db_password=hunter2", []string{"hunter2"}},
		{"upper snake case", "ACCESS_TOKEN=abc12345", []string{"abc12345"}},
		{"camel case", "clientSecret: s3cr3t-value", []string{"s3cr3t-value"}},
		{"api key header", "x-api-key: abc12345", []string{"abc12345"}},
		{"json double quoted", `{"password": "hunter2"}`, []string{"hunter2"}},
		{"json no space", `{"password":"hunter2"}`, []string{"hunter2"}},
		{"json prefixed key", `{"db_password": "hunter2", "user": "bob"}`, []string{"hunter2"}},
		{"json camel case key", `{"clientSecret":"s3cr3t-value"}`, []string{"s3cr3t-value"}},
		{"single quoted key and value", `{'password': 'hunter2'}`, []string{"hunter2"}},
		{"escaped json inside a string", `provider said {\"password\":\"hunter2\"}`, []string{"hunter2"}},
		{"quoted value with spaces", `password="hunter 2 x"`, []string{"hunter", "2 x"}},
		{"escaped quoted value with spaces", `{\"password\": \"hunter 2 x\"}`, []string{"hunter", "2 x"}},
		{"escaped single quoted value with spaces", `{\'password\': \'Zq9 Xk3\'}`, []string{"Zq9", "Xk3"}},
		{"double quoted value with an escaped quote", `password: "Zq9\"Xk3"`, []string{"Zq9", "Xk3"}},
		{"single quoted value with an escaped quote", `password: 'Zq9\'Xk3'`, []string{"Zq9", "Xk3"}},
		{"spaced separator", "pwd  =  hunter2", []string{"hunter2"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeRunError(c.in)
			for _, secret := range c.secrets {
				if strings.Contains(got, secret) {
					t.Fatalf("sanitizeRunError(%q) = %q, still contains %q", c.in, got, secret)
				}
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
	cases := []struct {
		name, in, prefix, suffix string
	}{
		{"plain quoted", `Provider rejected {"password": "hunter2"} for sync`, "Provider rejected", "for sync"},
		{"escaped quote inside the value", `password: "Zq9\"Xk3" then retry`, "", "then retry"},
		{"escaped quoted value with spaces", `rejected {\"password\": \"hunter 2 x\"} for sync`, "rejected", "for sync"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeRunError(c.in)
			if !strings.HasPrefix(got, c.prefix) || !strings.HasSuffix(got, c.suffix) {
				t.Errorf("sanitizeRunError(%q) = %q, want it to start with %q and end with %q", c.in, got, c.prefix, c.suffix)
			}
		})
	}
}

// An unquoted value ends at the first whitespace: nothing in the text says where
// a secret containing a space would end, and redacting to the end of the message
// would also drop the context an administrator needs. This test pins that
// documented limit so it is a decision, not an accident.
func TestSanitizeRunErrorUnquotedValueEndsAtWhitespace(t *testing.T) {
	got := sanitizeRunError("password=hunter 2")
	if strings.Contains(got, "hunter") {
		t.Errorf("the first token of an unquoted value must be redacted: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Errorf("want a [redacted] marker: %q", got)
	}
}
