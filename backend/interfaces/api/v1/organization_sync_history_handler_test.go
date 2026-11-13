package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// historyStore adds the read-only HistoryReader to the in-memory run store.
type historyStore struct {
	*handlerRunStore
	listErr error
}

func (s *historyStore) List(_ context.Context, f orgsyncrun.HistoryFilter) ([]orgsyncrun.Run, int, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var all []orgsyncrun.Run
	for _, r := range s.runs {
		if f.Status != "" && r.Status != f.Status {
			continue
		}
		if f.Trigger != "" && r.Trigger != f.Trigger {
			continue
		}
		if f.SnapshotType != "" && r.SnapshotType != f.SnapshotType {
			continue
		}
		all = append(all, *r)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
	total := len(all)
	if f.Offset >= total {
		return []orgsyncrun.Run{}, total, nil
	}
	end := min(f.Offset+f.Limit, total)
	return all[f.Offset:end], total, nil
}

var _ = Describe("Organization Provider Handler: sync run history", func() {
	const path = "/api/v1/admin/organization-provider/sync/runs"
	var (
		router      *gin.Engine
		store       *historyStore
		adminToken  string
		memberToken string
		base        time.Time
	)

	get := func(url, token string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		Expect(err).NotTo(HaveOccurred())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	decode := func(w *httptest.ResponseRecorder) map[string]any {
		var m map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &m)).To(Succeed())
		return m
	}
	runs := func(m map[string]any) []map[string]any {
		out := []map[string]any{}
		for _, r := range m["runs"].([]any) {
			out = append(out, r.(map[string]any))
		}
		return out
	}
	ids := func(m map[string]any) []string {
		var out []string
		for _, r := range runs(m) {
			out = append(out, r["runId"].(string))
		}
		return out
	}
	str := func(s string) *string { return &s }
	at := func(d time.Duration) *time.Time { t := base.Add(d); return &t }

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-handler-tests")
		gin.SetMode(gin.TestMode)
		base = time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)

		store = &historyStore{handlerRunStore: &handlerRunStore{runs: map[string]*orgsyncrun.Run{}}}
		// Inserted out of order on purpose: run-1 is oldest, run-5 newest.
		store.runs["run-3"] = &orgsyncrun.Run{ID: "run-3", Status: orgsyncrun.StatusSuccess, Stage: orgsyncrun.StageCompleted,
			Trigger: orgsyncrun.TriggerScheduled, SnapshotType: orgprovider.SnapshotComplete, InitiatedBy: "scheduler",
			CreatedAt: base.Add(2 * time.Hour), StartedAt: at(2*time.Hour + time.Second), CompletedAt: at(2*time.Hour + 31*time.Second),
			SourceGeneratedAt: at(time.Hour)}
		store.runs["run-1"] = &orgsyncrun.Run{ID: "run-1", Status: orgsyncrun.StatusSuccess, Stage: orgsyncrun.StageCompleted,
			Trigger: orgsyncrun.TriggerManual, SnapshotType: orgprovider.SnapshotComplete, InitiatedBy: "admin",
			CreatedAt: base, StartedAt: at(time.Second), CompletedAt: at(10 * time.Second)}
		store.runs["run-5"] = &orgsyncrun.Run{ID: "run-5", Status: orgsyncrun.StatusRunning, Stage: orgsyncrun.StageUpsertingUsers,
			Trigger: orgsyncrun.TriggerManual, SnapshotType: orgprovider.SnapshotComplete, InitiatedBy: "admin",
			CreatedAt: base.Add(4 * time.Hour), StartedAt: at(4*time.Hour + time.Second)}
		store.runs["run-2"] = &orgsyncrun.Run{ID: "run-2", Status: orgsyncrun.StatusFailed, Stage: orgsyncrun.StageFailed,
			Trigger: orgsyncrun.TriggerManual, SnapshotType: orgprovider.SnapshotComplete, InitiatedBy: "admin",
			CreatedAt: base.Add(time.Hour), StartedAt: at(time.Hour + time.Second), CompletedAt: at(time.Hour + 3*time.Second),
			FailedStage: func() *orgsyncrun.Stage { s := orgsyncrun.StageFetching; return &s }(),
			ErrorCode:   str(orgsyncrun.CodeProviderFetchFailed),
			ErrorMessage: str("Provider call to https://user:pw@provider.example/orgs?apiKey=abc123 failed. " +
				"Authorization: Bearer abcdefgh12345678 password=hunter2")}
		store.runs["run-4"] = &orgsyncrun.Run{ID: "run-4", Status: orgsyncrun.StatusFailed, Stage: orgsyncrun.StageFailed,
			Trigger: orgsyncrun.TriggerScheduled, SnapshotType: orgprovider.SnapshotComplete, InitiatedBy: "scheduler",
			CreatedAt: base.Add(3 * time.Hour), StartedAt: at(3 * time.Hour), CompletedAt: at(3*time.Hour + 20*time.Minute),
			ErrorCode: str(orgsyncrun.CodeStaleRun), ErrorMessage: str("stopped responding")}

		svc := services.NewOrganizationSyncService(&gatedRepo{store: store.handlerRunStore}, &fakeFetcher{}, nil, nil, services.WithRunStore(store))
		jwtService := services.NewJWTService()
		a, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = a.AccessToken
		m, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "member@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		memberToken = m.AccessToken

		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, svc, &thresholdRepo{}, jwtService)
	})

	AfterEach(func() { os.Unsetenv("JWT_SECRET") })

	Describe("authorization", func() {
		It("rejects an unauthenticated request with 401", func() {
			Expect(get(path, "").Code).To(Equal(http.StatusUnauthorized))
		})
		It("rejects a non-admin with 403 and returns no run data", func() {
			w := get(path, memberToken)
			Expect(w.Code).To(Equal(http.StatusForbidden))
			Expect(w.Body.String()).NotTo(ContainSubstring("run-"))
		})
		It("allows an admin", func() {
			Expect(get(path, adminToken).Code).To(Equal(http.StatusOK))
		})
	})

	It("returns runs newest first", func() {
		Expect(ids(decode(get(path, adminToken)))).To(Equal([]string{"run-5", "run-4", "run-3", "run-2", "run-1"}))
	})

	It("paginates server-side and reports totals", func() {
		p1 := decode(get(path+"?pageSize=2&page=1", adminToken))
		Expect(ids(p1)).To(Equal([]string{"run-5", "run-4"}))
		pg := p1["pagination"].(map[string]any)
		Expect(pg["totalItems"]).To(BeEquivalentTo(5))
		Expect(pg["totalPages"]).To(BeEquivalentTo(3))
		Expect(pg["hasNextPage"]).To(BeTrue())
		Expect(pg["hasPreviousPage"]).To(BeFalse())

		p3 := decode(get(path+"?pageSize=2&page=3", adminToken))
		Expect(ids(p3)).To(Equal([]string{"run-1"}))
		Expect(p3["pagination"].(map[string]any)["hasNextPage"]).To(BeFalse())

		beyond := decode(get(path+"?pageSize=2&page=9", adminToken))
		Expect(runs(beyond)).To(BeEmpty())
		Expect(beyond["runs"]).NotTo(BeNil(), "an empty page is [] not null")
	})

	It("defaults the page size to 20 and caps it at 100", func() {
		Expect(decode(get(path, adminToken))["pagination"].(map[string]any)["pageSize"]).To(BeEquivalentTo(20))
		Expect(decode(get(path+"?pageSize=5000", adminToken))["pagination"].(map[string]any)["pageSize"]).To(BeEquivalentTo(100))
	})

	It("filters by status, trigger and snapshot type, combined", func() {
		Expect(ids(decode(get(path+"?status=FAILED", adminToken)))).To(Equal([]string{"run-4", "run-2"}))
		Expect(ids(decode(get(path+"?trigger=SCHEDULED", adminToken)))).To(Equal([]string{"run-4", "run-3"}))
		Expect(ids(decode(get(path+"?status=FAILED&trigger=MANUAL", adminToken)))).To(Equal([]string{"run-2"}))
		Expect(runs(decode(get(path+"?snapshotType=PARTIAL", adminToken)))).To(BeEmpty())
		Expect(decode(get(path+"?status=FAILED", adminToken))["pagination"].(map[string]any)["totalItems"]).To(BeEquivalentTo(2))
	})

	It("rejects unknown filter values with 400 and a message", func() {
		for _, q := range []string{"status=BOGUS", "status=failed", "trigger=nope", "snapshotType=x"} {
			w := get(path+"?"+q, adminToken)
			Expect(w.Code).To(Equal(http.StatusBadRequest), q)
			Expect(decode(w)["error"]).NotTo(BeEmpty())
		}
	})

	It("returns 500 with a generic message when the store fails", func() {
		store.listErr = context.DeadlineExceeded
		w := get(path, adminToken)
		Expect(w.Code).To(Equal(http.StatusInternalServerError))
		Expect(w.Body.String()).NotTo(ContainSubstring("deadline"))
	})

	Describe("DTO", func() {
		find := func(id string) map[string]any {
			for _, r := range runs(decode(get(path, adminToken))) {
				if r["runId"] == id {
					return r
				}
			}
			Fail("run not found: " + id)
			return nil
		}

		It("exposes only the documented fields", func() {
			keys := []string{}
			for k := range find("run-3") {
				keys = append(keys, k)
			}
			Expect(keys).To(ConsistOf("runId", "snapshotType", "trigger", "status", "stage", "initiatedBy",
				"createdAt", "startedAt", "completedAt", "sourceGeneratedAt", "durationSeconds",
				"failedStage", "errorCode", "errorMessage"))
			body := get(path, adminToken).Body.String()
			for _, banned := range []string{"idempotency", "workerId", "heartbeat", "holdReport", "massDeletion"} {
				Expect(strings.ToLower(body)).NotTo(ContainSubstring(strings.ToLower(banned)))
			}
		})

		It("computes duration only from recorded start and completion", func() {
			Expect(find("run-3")["durationSeconds"]).To(BeEquivalentTo(30))
			Expect(find("run-5")["durationSeconds"]).To(BeNil(), "still running: no completion time")
			Expect(find("run-4")["durationSeconds"]).To(BeNil(), "stale-run recovery time is not when work stopped")
		})

		It("keeps source-generated time distinct from created time", func() {
			r := find("run-3")
			Expect(r["sourceGeneratedAt"]).To(Equal("2026-11-01T11:00:00Z"))
			Expect(r["createdAt"]).To(Equal("2026-11-01T12:00:00Z"))
			Expect(find("run-5")["sourceGeneratedAt"]).To(BeNil())
		})

		It("includes failure context only for failed runs, sanitized", func() {
			ok := find("run-3")
			Expect(ok["errorMessage"]).To(BeNil())
			Expect(ok["errorCode"]).To(BeNil())

			f := find("run-2")
			Expect(f["failedStage"]).To(Equal("FETCHING"))
			Expect(f["errorCode"]).To(Equal("PROVIDER_FETCH_FAILED"))
			msg := f["errorMessage"].(string)
			for _, secret := range []string{"hunter2", "abc123", "abcdefgh12345678", "user:pw"} {
				Expect(msg).NotTo(ContainSubstring(secret))
			}
			Expect(msg).To(ContainSubstring("Provider call to"))
			Expect(msg).To(ContainSubstring("[redacted]"))
		})

		It("caps very long error messages", func() {
			store.runs["run-2"].ErrorMessage = str(strings.Repeat("x", 600))
			msg := find("run-2")["errorMessage"].(string)
			Expect(len([]rune(msg))).To(BeNumerically("<=", 301))
		})
	})

	It("returns 404 when persisted runs are not configured", func() {
		svc := services.NewOrganizationSyncService(&gatedRepo{store: store.handlerRunStore}, &fakeFetcher{}, nil, nil)
		jwtService := services.NewJWTService()
		a, _ := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		r := gin.New()
		v1.SetupOrganizationProviderRoutes(r, svc, &thresholdRepo{}, jwtService)
		req, _ := http.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+a.AccessToken)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		Expect(w.Code).To(Equal(http.StatusNotFound))
	})
})
