package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// handlerRunStore is a minimal in-memory orgsyncrun.Store for handler specs.
type handlerRunStore struct {
	mu   sync.Mutex
	runs map[string]*orgsyncrun.Run
	last string
}

func (s *handlerRunStore) CreatePending(_ context.Context, p orgsyncrun.NewParams) (*orgsyncrun.Run, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = map[string]*orgsyncrun.Run{}
	}
	for _, r := range s.runs {
		if !r.Status.Terminal() {
			c := *r
			return &c, false, orgsyncrun.ErrActiveRunExists
		}
	}
	r := &orgsyncrun.Run{ID: p.ID, Status: orgsyncrun.StatusPending, Stage: orgsyncrun.StageQueued, CreatedAt: time.Now()}
	s.runs[p.ID], s.last = r, p.ID
	c := *r
	return &c, true, nil
}
func (s *handlerRunStore) Get(_ context.Context, id string) (*orgsyncrun.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.runs[id]; ok {
		c := *r
		return &c, nil
	}
	return nil, orgsyncrun.ErrNotFound
}
func (s *handlerRunStore) Latest(ctx context.Context) (*orgsyncrun.Run, error) {
	s.mu.Lock()
	id := s.last
	s.mu.Unlock()
	if id == "" {
		return nil, nil
	}
	return s.Get(ctx, id)
}
func (s *handlerRunStore) Active(_ context.Context) (*orgsyncrun.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.runs {
		if !r.Status.Terminal() {
			c := *r
			return &c, nil
		}
	}
	return nil, nil
}
func (s *handlerRunStore) SetStage(_ context.Context, id string, st orgsyncrun.Stage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id].Stage = st
	return nil
}
func (s *handlerRunStore) MarkRunning(_ context.Context, id, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id].Status = orgsyncrun.StatusRunning
	return nil
}
func (s *handlerRunStore) Fail(_ context.Context, id string, f orgsyncrun.Failure) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.runs[id]
	r.Status, r.Stage = orgsyncrun.StatusFailed, orgsyncrun.StageFailed
	return nil
}
func (s *handlerRunStore) succeed(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[id].Status, s.runs[id].Stage = orgsyncrun.StatusSuccess, orgsyncrun.StageCompleted
}
func (s *handlerRunStore) RecoverStale(context.Context, time.Duration) (int, error) { return 0, nil }
func (s *handlerRunStore) TryLock(context.Context) (func(), bool, error) {
	return func() {}, true, nil
}

// gatedRepo blocks ApplySnapshot until released, then marks the run SUCCESS.
type gatedRepo struct {
	store   *handlerRunStore
	gate    chan struct{}
	started chan struct{}
	once    sync.Once
}

func (g *gatedRepo) KnownHierarchyLevelIDs(context.Context) (map[string]bool, error) {
	return map[string]bool{"level-3": true, "level-5": true}, nil
}
func (g *gatedRepo) ApplySnapshot(_ context.Context, in orgprovider.ApplyInput) (*orgprovider.ApplyResult, error) {
	g.once.Do(func() { close(g.started) })
	<-g.gate
	g.store.succeed(in.RunID)
	return &orgprovider.ApplyResult{}, nil
}

var _ = Describe("Organization Provider Handler: persisted sync runs", func() {
	var (
		router *gin.Engine
		store  *handlerRunStore
		repo   *gatedRepo
		token  string
	)

	do := func(method, path string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, path, nil)
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	decode := func(w *httptest.ResponseRecorder) map[string]any {
		var m map[string]any
		Expect(json.Unmarshal(w.Body.Bytes(), &m)).To(Succeed())
		return m
	}

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-handler-tests")
		os.Setenv(dataprovider.EnvBaseURL, "https://provider.invalid")
		os.Setenv(dataprovider.EnvAPIToken, "handler-test-token")
		gin.SetMode(gin.TestMode)

		store = &handlerRunStore{}
		repo = &gatedRepo{store: store, gate: make(chan struct{}), started: make(chan struct{})}
		svc := services.NewOrganizationSyncService(repo, &fakeFetcher{}, nil, nil, services.WithRunStore(store))

		jwtService := services.NewJWTService()
		pair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		token = pair.AccessToken

		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, svc, &thresholdRepo{}, jwtService)
	})

	AfterEach(func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv(dataprovider.EnvBaseURL)
		os.Unsetenv(dataprovider.EnvAPIToken)
	})

	It("answers 202 with a run id and PENDING, then reports RUNNING and SUCCESS from persisted state", func() {
		w := do(http.MethodPost, "/api/v1/admin/organization-provider/sync")
		Expect(w.Code).To(Equal(http.StatusAccepted))
		body := decode(w)
		runID, _ := body["runId"].(string)
		Expect(runID).NotTo(BeEmpty())
		Expect(body["status"]).To(Equal("PENDING"), "accepting the request is not success")

		Eventually(repo.started, 2*time.Second).Should(BeClosed())
		running := decode(do(http.MethodGet, "/api/v1/admin/organization-provider/sync/runs/"+runID))
		Expect(running["status"]).To(Equal("RUNNING"))

		// A duplicate click follows the active run instead of starting another.
		dup := do(http.MethodPost, "/api/v1/admin/organization-provider/sync")
		Expect(dup.Code).To(Equal(http.StatusAccepted))
		Expect(decode(dup)["runId"]).To(Equal(runID))
		Expect(decode(dup)["alreadyActive"]).To(BeTrue())

		close(repo.gate)
		Eventually(func() any {
			return decode(do(http.MethodGet, "/api/v1/admin/organization-provider/sync/runs/"+runID))["status"]
		}, 2*time.Second, 10*time.Millisecond).Should(Equal("SUCCESS"))

		// "latest" restores state after a page refresh.
		Expect(decode(do(http.MethodGet, "/api/v1/admin/organization-provider/sync/runs/latest"))["runId"]).To(Equal(runID))
	})

	It("locks the deactivation threshold while a run is active and unlocks it as soon as the run succeeds", func() {
		const thresholdPath = "/api/v1/admin/settings/organization-provider/deletion-threshold"
		put := func() *httptest.ResponseRecorder {
			req, err := http.NewRequest(http.MethodPut, thresholdPath, strings.NewReader(`{"maxDeletePercent": 30}`))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		Expect(decode(do(http.MethodGet, thresholdPath))["locked"]).To(BeFalse(), "idle: editable")

		runID := decode(do(http.MethodPost, "/api/v1/admin/organization-provider/sync"))["runId"].(string)
		Eventually(repo.started, 2*time.Second).Should(BeClosed())

		locked := decode(do(http.MethodGet, thresholdPath))
		Expect(locked["locked"]).To(BeTrue())
		Expect(locked["lockReason"]).To(Equal("syncing"))
		Expect(put().Code).To(Equal(http.StatusConflict), "cannot change the threshold under a running sync")

		close(repo.gate)
		Eventually(func() any {
			return decode(do(http.MethodGet, "/api/v1/admin/organization-provider/sync/runs/"+runID))["status"]
		}, 2*time.Second, 10*time.Millisecond).Should(Equal("SUCCESS"))

		unlocked := decode(do(http.MethodGet, thresholdPath))
		Expect(unlocked["locked"]).To(BeFalse(), "a finished run must never keep the threshold locked")
		Expect(unlocked).NotTo(HaveKey("lockReason"))
		Expect(put().Code).To(Equal(http.StatusOK), "threshold editing is re-enabled")
	})

	It("returns 404 for an unknown run and 204 for latest when nothing has run", func() {
		Expect(do(http.MethodGet, "/api/v1/admin/organization-provider/sync/runs/nope").Code).To(Equal(http.StatusNotFound))
		Expect(do(http.MethodGet, "/api/v1/admin/organization-provider/sync/runs/latest").Code).To(Equal(http.StatusNoContent))
	})
})
