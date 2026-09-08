package dataprovider

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const validSnapshot = `{
  "contractVersion": "1.0",
  "generatedAt": "2026-08-31T05:04:48.240Z",
  "teams": [{"id": "team-1", "name": "Platform", "healthCheckEnabled": false, "teamLeadId": "user-1"}],
  "users": [{"id": "user-1", "username": "alice", "displayName": "Alice", "email": "alice@example.com", "hierarchyLevelId": "level-5"}],
  "memberships": [{"userId": "user-1", "teamId": "team-1"}]
}`

func newSnapshotClient(t *testing.T, baseURL, token string) *Client {
	t.Helper()
	client, err := NewClient(&Config{BaseURL: baseURL, APIToken: token})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

func TestFetchSnapshot_IssuesOneGetToTheContractEndpoint(t *testing.T) {
	var requests int
	var gotMethod, gotPath, gotAPIKey, gotAuth, gotAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotAuth = r.Header.Get("Authorization")
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(validSnapshot))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "secret-token")
	if _, err := client.FetchSnapshot(context.Background()); err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if requests != 1 {
		t.Errorf("provider requests = %d, want exactly 1", requests)
	}
	if gotMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", gotMethod)
	}
	if gotPath != "/org-snapshot" {
		t.Errorf("path = %q, want /org-snapshot", gotPath)
	}
	if gotAPIKey != "secret-token" {
		t.Errorf("x-api-key = %q, want %q", gotAPIKey, "secret-token")
	}
	if gotAuth != "" {
		t.Errorf("Authorization = %q, want empty (the machine credential is x-api-key only)", gotAuth)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q, want application/json", gotAccept)
	}
}

func TestFetchSnapshot_DecodesContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validSnapshot))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	snapshot, err := client.FetchSnapshot(context.Background())
	if err != nil {
		t.Fatalf("FetchSnapshot() error = %v", err)
	}

	if snapshot.ContractVersion != "1.0" {
		t.Errorf("ContractVersion = %q, want 1.0", snapshot.ContractVersion)
	}
	if len(snapshot.Users) != 1 || len(snapshot.Teams) != 1 || len(snapshot.Memberships) != 1 {
		t.Fatalf("got %d users, %d teams, %d memberships; want 1 each",
			len(snapshot.Users), len(snapshot.Teams), len(snapshot.Memberships))
	}
	if snapshot.Teams[0].HealthCheckEnabled == nil || *snapshot.Teams[0].HealthCheckEnabled {
		t.Error("healthCheckEnabled=false did not survive decoding as an explicit false")
	}
}

func TestFetchSnapshot_ClosesTheResponseBody(t *testing.T) {
	// A body that is fully read and closed returns its connection to the idle
	// pool, so two sequential fetches share one connection. A leaked body would
	// force the second fetch onto a new one.
	var newConnections int
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validSnapshot))
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			newConnections++
		}
	}
	server.Start()
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	for i := range 2 {
		if _, err := client.FetchSnapshot(context.Background()); err != nil {
			t.Fatalf("FetchSnapshot() call %d error = %v", i+1, err)
		}
	}

	if newConnections != 1 {
		t.Errorf("server saw %d new connections for 2 fetches, want 1 (an unclosed body prevents reuse)", newConnections)
	}
}

func TestFetchSnapshot_RejectsNonOKStatus(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"upstream detail that must not leak"}`))
		}))

		client := newSnapshotClient(t, server.URL, "token")
		_, err := client.FetchSnapshot(context.Background())
		server.Close()

		if err == nil {
			t.Fatalf("status %d: expected an error", status)
		}
		if strings.Contains(err.Error(), "upstream detail") {
			t.Errorf("status %d: error leaked the upstream body: %v", status, err)
		}
	}
}

func TestFetchSnapshot_ErrorsNeverContainTheToken(t *testing.T) {
	const token = "super-secret-token"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, token)
	_, err := client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), token) {
		t.Errorf("error leaked the API token: %v", err)
	}
}

func TestFetchSnapshot_RejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"contractVersion": "1.0", "users": [ this is not json`))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	_, err := client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
	if strings.Contains(err.Error(), "this is not json") {
		t.Errorf("error leaked the response fragment: %v", err)
	}
}

func TestFetchSnapshot_RejectsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	if _, err := client.FetchSnapshot(context.Background()); err == nil {
		t.Fatal("expected an error for an empty response body")
	}
}

func TestFetchSnapshot_RejectsOversizedResponse(t *testing.T) {
	// Exercise the real limit by lowering it, rather than generating a payload
	// of the production size.
	original := maxSnapshotBytes
	maxSnapshotBytes = int64(len(validSnapshot)) - 1
	defer func() { maxSnapshotBytes = original }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validSnapshot))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	_, err := client.FetchSnapshot(context.Background())
	if !errors.Is(err, ErrSnapshotTooLarge) {
		t.Fatalf("FetchSnapshot() error = %v, want ErrSnapshotTooLarge", err)
	}
}

func TestFetchSnapshot_AcceptsAResponseAtTheSizeLimit(t *testing.T) {
	original := maxSnapshotBytes
	maxSnapshotBytes = int64(len(validSnapshot))
	defer func() { maxSnapshotBytes = original }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validSnapshot))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	if _, err := client.FetchSnapshot(context.Background()); err != nil {
		t.Fatalf("a response exactly at the limit should decode, got error = %v", err)
	}
}

func TestFetchSnapshot_RejectsCrossOriginRedirect(t *testing.T) {
	var otherHostReceivedKey string
	otherHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHostReceivedKey = r.Header.Get("x-api-key")
		w.WriteHeader(http.StatusOK)
	}))
	defer otherHost.Close()

	originHost := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, otherHost.URL+"/org-snapshot", http.StatusFound)
	}))
	defer originHost.Close()

	client := newSnapshotClient(t, originHost.URL, "secret-token")
	_, err := client.FetchSnapshot(context.Background())
	if err == nil {
		t.Fatal("expected an error rejecting the cross-origin redirect")
	}
	if otherHostReceivedKey != "" {
		t.Fatalf("x-api-key leaked to the redirect target: %q", otherHostReceivedKey)
	}
}

func TestFetchSnapshot_HonoursContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(validSnapshot))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.FetchSnapshot(ctx); err == nil {
		t.Error("expected an error when the context is already cancelled")
	}
}

func TestFetchSnapshot_RespectsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write([]byte(validSnapshot))
	}))
	defer server.Close()

	client := newSnapshotClient(t, server.URL, "token")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	if _, err := client.FetchSnapshot(ctx); err == nil {
		t.Error("expected a timeout error")
	}
}
