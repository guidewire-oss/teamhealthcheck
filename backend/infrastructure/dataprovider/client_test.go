package dataprovider

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

func TestNewClient_RejectsNilConfig(t *testing.T) {
	client, err := NewClient(nil)
	if err != ErrNilConfig {
		t.Fatalf("err = %v, want %v", err, ErrNilConfig)
	}
	if client != nil {
		t.Fatalf("expected nil client, got %+v", client)
	}
}

func TestNewClient_RejectsInvalidBaseURL(t *testing.T) {
	_, err := NewClient(&Config{BaseURL: "://not-a-valid-url", APIToken: "secret-token"})
	if err == nil {
		t.Fatal("expected an error for an invalid base URL, got nil")
	}
}

func TestNewClient_RejectsEmptyAPIToken(t *testing.T) {
	client, err := NewClient(&Config{BaseURL: "https://provider.example.com", APIToken: ""})
	if err != ErrEmptyAPIToken {
		t.Fatalf("err = %v, want %v", err, ErrEmptyAPIToken)
	}
	if client != nil {
		t.Fatalf("expected nil client, got %+v", client)
	}
}

func TestValidateBaseURL_AcceptsWhatNewClientAccepts(t *testing.T) {
	if err := ValidateBaseURL("https://provider.example.com"); err != nil {
		t.Fatalf("expected a valid base URL to pass, got %v", err)
	}
}

func TestValidateBaseURL_RejectsWhatNewClientRejects(t *testing.T) {
	if err := ValidateBaseURL("://not-a-valid-url"); err == nil {
		t.Fatal("expected an error for an invalid base URL, got nil")
	}
}

func TestValidateBaseURL_RejectsRelativeURL(t *testing.T) {
	if err := ValidateBaseURL("/no-host"); err == nil {
		t.Fatal("expected an error for a relative URL, got nil")
	}
}

func TestNewClient_RejectsWhitespaceOnlyAPIToken(t *testing.T) {
	client, err := NewClient(&Config{BaseURL: "https://provider.example.com", APIToken: "   "})
	if err != ErrEmptyAPIToken {
		t.Fatalf("err = %v, want %v", err, ErrEmptyAPIToken)
	}
	if client != nil {
		t.Fatalf("expected nil client, got %+v", client)
	}
}

func TestNewClient_RejectsRelativeBaseURL(t *testing.T) {
	_, err := NewClient(&Config{BaseURL: "/resource", APIToken: "secret-token"})
	if err == nil {
		t.Fatal("expected an error for a relative base URL, got nil")
	}
}

func TestNewClient_RejectsNonHTTPBaseURL(t *testing.T) {
	_, err := NewClient(&Config{BaseURL: "ftp://provider.example.com", APIToken: "secret-token"})
	if err == nil {
		t.Fatal("expected an error for a non-http(s) base URL, got nil")
	}
}

func TestNewClient_RejectsHostlessBaseURL(t *testing.T) {
	_, err := NewClient(&Config{BaseURL: "https:///provider", APIToken: "secret-token"})
	if err == nil {
		t.Fatal("expected an error for a hostless base URL, got nil")
	}
}

func TestClient_Do_SendsAPIKeyHeader(t *testing.T) {
	var receivedHeader, receivedPath, receivedAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeader = r.Header.Get("x-api-key")
		receivedPath = r.URL.Path
		receivedAccept = r.Header.Get("Accept")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(&Config{BaseURL: server.URL, APIToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := client.Do(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if receivedHeader != "secret-token" {
		t.Errorf("x-api-key header = %q, want %q", receivedHeader, "secret-token")
	}
	if receivedPath != "/resource" {
		t.Errorf("path = %q, want %q", receivedPath, "/resource")
	}
	if receivedAccept != "application/json" {
		t.Errorf("Accept = %q, want %q", receivedAccept, "application/json")
	}
}

func TestClient_Do_JoinsBaseURLAndPathRegardlessOfSlashes(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(&Config{BaseURL: server.URL + "/", APIToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := client.Do(context.Background(), http.MethodGet, "/resource", nil)
	if err != nil {
		t.Fatalf("Do returned error: %v", err)
	}
	defer resp.Body.Close()

	if receivedPath != "/resource" {
		t.Errorf("path = %q, want %q", receivedPath, "/resource")
	}
}

func TestClient_Do_RejectsCrossOriginRedirect(t *testing.T) {
	var otherHostReceivedHeader string
	otherHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHostReceivedHeader = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer otherHost.Close()

	originHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, otherHost.URL+"/resource", http.StatusFound)
	}))
	defer originHost.Close()

	client, err := NewClient(&Config{BaseURL: originHost.URL, APIToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	resp, err := client.Do(context.Background(), http.MethodGet, "/resource", nil)
	if err == nil {
		t.Fatal("expected an error rejecting the cross-origin redirect, got nil")
	}
	if resp != nil {
		resp.Body.Close()
	}
	if otherHostReceivedHeader != "" {
		t.Fatalf("x-api-key leaked to the redirect target: %q", otherHostReceivedHeader)
	}
}

func TestClient_Do_RespectsContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewClient(&Config{BaseURL: server.URL, APIToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = client.Do(ctx, http.MethodGet, "/resource", nil)
	if err == nil {
		t.Fatal("expected an error from a cancelled context, got nil")
	}
}

// captureLogs routes the global logger into a buffer for one test and
// restores a default logger afterwards.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger.Init(logger.Config{Level: "debug", Output: &buf})
	t.Cleanup(func() { logger.Init(logger.Config{Level: "info"}) })
	return &buf
}

func assertNoneLogged(t *testing.T, logs string, forbidden ...string) {
	t.Helper()
	for _, f := range forbidden {
		if strings.Contains(logs, f) {
			t.Errorf("log output leaked %q: %s", f, logs)
		}
	}
}

func TestClient_Do_TransportErrorLogsNoURLOrToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	baseURL := server.URL
	server.Close() // connection refused: *url.Error embeds the full URL

	logs := captureLogs(t)
	client, err := NewClient(&Config{BaseURL: baseURL, APIToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = client.Do(context.Background(), http.MethodGet, "/resource?apikey=query-secret", nil)
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("err = %v, want ErrRequestFailed", err)
	}
	if strings.Contains(err.Error(), "127.0.0.1") || strings.Contains(err.Error(), "query-secret") {
		t.Errorf("caller-facing error leaked request details: %v", err)
	}

	out := logs.String()
	assertNoneLogged(t, out, baseURL, "127.0.0.1", "/resource", "query-secret", "secret-token", "connection refused")
	if !strings.Contains(out, `"failure":"transport"`) || !strings.Contains(out, `"method":"GET"`) {
		t.Errorf("expected sanitized category and method in log, got: %s", out)
	}
}

func TestClient_Do_RedirectErrorLogsNoURLs(t *testing.T) {
	otherHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer otherHost.Close()
	originHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, otherHost.URL+"/leak-target?token=redirect-secret", http.StatusFound)
	}))
	defer originHost.Close()

	logs := captureLogs(t)
	client, err := NewClient(&Config{BaseURL: originHost.URL, APIToken: "secret-token"})
	if err != nil {
		t.Fatalf("NewClient returned error: %v", err)
	}

	_, err = client.Do(context.Background(), http.MethodGet, "/resource", nil)
	if !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("err = %v, want ErrRequestFailed", err)
	}

	out := logs.String()
	assertNoneLogged(t, out, originHost.URL, otherHost.URL, "leak-target", "redirect-secret", "secret-token", "127.0.0.1")
	if !strings.Contains(out, `"failure":"cross_origin_redirect"`) {
		t.Errorf("expected cross_origin_redirect category in log, got: %s", out)
	}
}

func TestClient_Do_CancelledContextIsCategorizedWithoutURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	logs := captureLogs(t)
	client, _ := NewClient(&Config{BaseURL: server.URL, APIToken: "secret-token"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Do(ctx, http.MethodGet, "/resource", nil); !errors.Is(err, ErrRequestFailed) {
		t.Fatalf("err = %v, want ErrRequestFailed", err)
	}
	out := logs.String()
	assertNoneLogged(t, out, server.URL, "127.0.0.1", "/resource")
	if !strings.Contains(out, `"failure":"canceled"`) {
		t.Errorf("expected canceled category, got: %s", out)
	}
}

func TestParseBaseURL_RejectsUserinfoWithoutEchoingIt(t *testing.T) {
	for _, raw := range []string{"https://user:hunter2@provider.example.com", "https://hunter2@provider.example.com"} {
		if err := ValidateBaseURL(raw); !errors.Is(err, ErrBaseURLHasCredentials) {
			t.Fatalf("ValidateBaseURL(%q) = %v, want ErrBaseURLHasCredentials", raw, err)
		}
		if _, err := NewClient(&Config{BaseURL: raw, APIToken: "secret-token"}); !errors.Is(err, ErrBaseURLHasCredentials) {
			t.Fatalf("NewClient(%q) err = %v, want ErrBaseURLHasCredentials", raw, err)
		}
	}
}

func TestParseBaseURL_ErrorsNeverEchoTheInput(t *testing.T) {
	// Unparseable (control character) and structurally invalid inputs that
	// carry a secret must not appear in the returned error.
	for _, raw := range []string{"http://us\x7fer:hunter2@host", "ftp://hunter2.example.com", "hunter2"} {
		err := ValidateBaseURL(raw)
		if err == nil {
			t.Fatalf("ValidateBaseURL(%q) = nil, want an error", raw)
		}
		if strings.Contains(err.Error(), "hunter2") {
			t.Errorf("error echoed the input: %v", err)
		}
	}
}
