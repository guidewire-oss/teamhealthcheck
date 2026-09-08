package dataprovider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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
