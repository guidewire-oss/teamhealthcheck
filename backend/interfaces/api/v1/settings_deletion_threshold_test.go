package v1_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/dataprovider"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// thresholdRepo implements only the two settings methods these specs exercise.
// Embedding the interface keeps the fake small: any other method would panic,
// which is the point -- the deletion-threshold endpoints must not reach for
// anything else.
type thresholdRepo struct {
	organization.Repository

	saved     *float64
	getErr    error
	updateErr error
	writes    []float64
}

func (r *thresholdRepo) GetOrgSyncMaxDeletePercent(_ context.Context) (*float64, error) {
	return r.saved, r.getErr
}

func (r *thresholdRepo) UpdateOrgSyncMaxDeletePercent(_ context.Context, percent float64) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	r.writes = append(r.writes, percent)
	value := percent
	r.saved = &value
	return nil
}

func percentPtr(f float64) *float64 { return &f }

var _ = Describe("Admin settings: organization-sync deletion threshold", func() {
	const thresholdPath = "/api/v1/admin/settings/organization-provider/deletion-threshold"

	var (
		router      *gin.Engine
		repo        *thresholdRepo
		syncService *services.OrganizationSyncService
		adminToken  string
		memberToken string
	)

	request := func(method, url, token, body string) *httptest.ResponseRecorder {
		var req *http.Request
		var err error
		if body == "" {
			req, err = http.NewRequest(method, url, nil)
		} else {
			req, err = http.NewRequest(method, url, strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
		}
		Expect(err).NotTo(HaveOccurred())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	decode := func(w *httptest.ResponseRecorder) dto.OrgSyncDeletionThreshold {
		var body dto.OrgSyncDeletionThreshold
		Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
		return body
	}

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-threshold-tests")
		os.Unsetenv(services.EnvMaxDeletePercent)
		gin.SetMode(gin.TestMode)

		repo = &thresholdRepo{}

		jwtService := services.NewJWTService()
		adminPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = adminPair.AccessToken

		memberPair, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "member@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		memberToken = memberPair.AccessToken

		// The threshold endpoints are served by the provider routes, because
		// whether they may run depends on this sync service's own state.
		syncService = services.NewOrganizationSyncService(&fakeSyncRepository{}, &fakeFetcher{}, nil, nil)
		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, syncService, repo, jwtService)
	})

	AfterEach(func() {
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv(services.EnvMaxDeletePercent)
	})

	Describe("reading the threshold", func() {
		It("reports the 20% default when nothing is configured", func() {
			w := request(http.MethodGet, thresholdPath, adminToken, "")
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			body := decode(w)
			Expect(body.MaxDeletePercent).To(Equal(20.0))
			Expect(body.Source).To(Equal(services.ThresholdSourceDefault))
			Expect(body.DefaultPercent).To(Equal(20.0))
			Expect(body.MinPercent).To(Equal(1.0))
			Expect(body.MaxPercent).To(Equal(100.0))
		})

		It("reports the environment fallback when no admin value is saved", func() {
			os.Setenv(services.EnvMaxDeletePercent, "45")

			body := decode(request(http.MethodGet, thresholdPath, adminToken, ""))
			Expect(body.MaxDeletePercent).To(Equal(45.0))
			Expect(body.Source).To(Equal(services.ThresholdSourceEnvironment))
		})

		It("prefers a saved admin value over the environment", func() {
			os.Setenv(services.EnvMaxDeletePercent, "45")
			repo.saved = percentPtr(12)

			body := decode(request(http.MethodGet, thresholdPath, adminToken, ""))
			Expect(body.MaxDeletePercent).To(Equal(12.0))
			Expect(body.Source).To(Equal(services.ThresholdSourceAdmin))
		})

		It("reports a stored value outside the allowed range as a misconfiguration", func() {
			repo.saved = percentPtr(250)

			w := request(http.MethodGet, thresholdPath, adminToken, "")
			Expect(w.Code).To(Equal(http.StatusInternalServerError), w.Body.String())
		})

		It("surfaces a settings read failure instead of inventing a threshold", func() {
			repo.getErr = errors.New("database down")

			w := request(http.MethodGet, thresholdPath, adminToken, "")
			Expect(w.Code).To(Equal(http.StatusInternalServerError), w.Body.String())
		})

		It("refuses a non-admin", func() {
			Expect(request(http.MethodGet, thresholdPath, memberToken, "").Code).To(Equal(http.StatusForbidden))
		})

		It("refuses an unauthenticated caller", func() {
			Expect(request(http.MethodGet, thresholdPath, "", "").Code).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("updating the threshold", func() {
		It("saves a valid percentage and reports it as admin-configured", func() {
			w := request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 35}`)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())

			body := decode(w)
			Expect(body.MaxDeletePercent).To(Equal(35.0))
			Expect(body.Source).To(Equal(services.ThresholdSourceAdmin))
			Expect(repo.writes).To(Equal([]float64{35}))
		})

		It("accepts both ends of the allowed range", func() {
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 1}`).Code).To(Equal(http.StatusOK))
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 100}`).Code).To(Equal(http.StatusOK))
			Expect(repo.writes).To(Equal([]float64{1, 100}))
		})

		It("rejects values outside 1-100, non-numeric input and a missing field", func() {
			for _, body := range []string{
				`{"maxDeletePercent": 0}`,
				`{"maxDeletePercent": 0.9}`,
				`{"maxDeletePercent": -20}`,
				`{"maxDeletePercent": 100.1}`,
				`{"maxDeletePercent": 1000}`,
				`{"maxDeletePercent": "40"}`,
				`{"maxDeletePercent": NaN}`,
				`{"maxDeletePercent": Infinity}`,
				`{"maxDeletePercent": null}`,
				`{}`,
				`not json`,
			} {
				w := request(http.MethodPut, thresholdPath, adminToken, body)
				Expect(w.Code).To(Equal(http.StatusBadRequest), body+" -> "+w.Body.String())
			}
			Expect(repo.writes).To(BeEmpty())
		})

		It("reports a persistence failure rather than claiming the value was saved", func() {
			repo.updateErr = errors.New("database down")

			w := request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 30}`)
			Expect(w.Code).To(Equal(http.StatusInternalServerError), w.Body.String())
		})

		It("refuses a non-admin and writes nothing", func() {
			Expect(request(http.MethodPut, thresholdPath, memberToken, `{"maxDeletePercent": 90}`).Code).To(Equal(http.StatusForbidden))
			Expect(repo.writes).To(BeEmpty())
		})

		It("refuses an unauthenticated caller and writes nothing", func() {
			Expect(request(http.MethodPut, thresholdPath, "", `{"maxDeletePercent": 90}`).Code).To(Equal(http.StatusUnauthorized))
			Expect(repo.writes).To(BeEmpty())
		})
	})

	// The lock is what stops the obvious way around a mass-deletion hold:
	// start a sync, see it held at 20%, raise the threshold to 100%, retry.
	Describe("while a sync is running or held", func() {
		const syncPath = "/api/v1/admin/organization-provider/sync"
		const holdPath = "/api/v1/admin/organization-provider/sync/hold"

		// heldReportFor builds an over-threshold report at the given threshold,
		// shaped the way orgprovider.BuildMassDeletionReport would produce it
		// for a real repository's guard trip.
		heldReportFor := func(threshold float64) *orgprovider.MassDeletionReport {
			report := orgprovider.BuildMassDeletionReport(20, 6, 10, 3, threshold)
			report.WithIncoming(14, 7, 42)
			return &report
		}

		// rewireWithStore rebuilds the router around a repository and fetcher this
		// spec controls. `store` is what the SYNC service reads its threshold
		// from; `repo` (the outer var) is always what the settings endpoints
		// read and write. Passing repo itself as the store, via rewire below, is
		// the normal case and mirrors production (cmd/api/main.go wires the same
		// orgRepo for both). A spec passes a different store only when it is
		// deliberately testing what happens if the two diverge -- see the
		// shifting-store spec, which models a store whose answer changes out
		// from under a held sync.
		rewireWithStore := func(syncRepo *fakeSyncRepository, fetcher services.SnapshotFetcher, store services.DeleteThresholdStore) {
			jwtService := services.NewJWTService()
			pair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
			Expect(err).NotTo(HaveOccurred())
			adminToken = pair.AccessToken

			syncService = services.NewOrganizationSyncService(syncRepo, fetcher, nil, nil,
				services.WithDeleteThresholdStore(store))
			router = gin.New()
			v1.SetupOrganizationProviderRoutes(router, syncService, repo, jwtService)
		}

		// rewire is the normal case: the sync reads its threshold from the same
		// repo instance the settings endpoints serve, exactly as production
		// wires it.
		rewire := func(syncRepo *fakeSyncRepository, fetcher services.SnapshotFetcher) {
			rewireWithStore(syncRepo, fetcher, repo)
		}

		BeforeEach(func() {
			os.Setenv(dataprovider.EnvBaseURL, "https://provider.invalid")
			os.Setenv(dataprovider.EnvAPIToken, "handler-test-token")
		})

		AfterEach(func() {
			os.Unsetenv(dataprovider.EnvBaseURL)
			os.Unsetenv(dataprovider.EnvAPIToken)
		})

		It("refuses a threshold update while a held sync is unresolved, from any tab", func() {
			syncRepo := &fakeSyncRepository{holdReport: heldReportFor(20)}
			repo.saved = percentPtr(20)
			rewire(syncRepo, &fakeFetcher{})

			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))

			// A second tab is just another request: same refusal.
			w := request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 100}`)
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())

			var body dto.ErrorResponse
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			Expect(body.Code).To(Equal(dto.CodeThresholdLocked))
			Expect(repo.writes).To(BeEmpty())
		})

		It("reports the lock on a fresh read, so a reloaded page stays disabled", func() {
			syncRepo := &fakeSyncRepository{holdReport: heldReportFor(20)}
			repo.saved = percentPtr(20)
			rewire(syncRepo, &fakeFetcher{})
			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))

			body := decode(request(http.MethodGet, thresholdPath, adminToken, ""))
			Expect(body.Locked).To(BeTrue())
			Expect(body.LockReason).To(Equal(dto.ThresholdLockHeld))
			Expect(body.ActiveSyncThreshold).NotTo(BeNil())
			Expect(*body.ActiveSyncThreshold).To(Equal(20.0))
		})

		It("keeps judging later attempts against the held threshold, not a changed setting", func() {
			// The store reports 100% from the second read onwards, standing in
			// for a threshold that somehow changed mid-hold. fakeSyncRepository
			// always reports a hold once holdReport is set, regardless of the
			// MaxDeletePercent it receives, so the 409s below only prove the
			// endpoint kept refusing -- the assertion that actually proves the
			// guard was judged at the frozen value is the one on
			// syncRepo.lastInput.MaxDeletePercent. (The stronger version of this
			// guarantee -- that a repository which genuinely re-evaluates against
			// the threshold it is given would also apply cleanly at 100% but stay
			// held at 20% -- is TestHoldFreezesTheThresholdForLaterAttempts in
			// organization_sync_threshold_test.go.)
			store := &shiftingThresholdStore{first: 20, rest: 100}
			syncRepo := &fakeSyncRepository{holdReport: heldReportFor(20)}
			rewireWithStore(syncRepo, &fakeFetcher{}, store)

			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))
			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))
			Expect(syncRepo.lastInput.MaxDeletePercent).To(Equal(20.0),
				"the second attempt must reuse the held threshold, not the store's now-changed value")
		})

		It("still lets the explicit Sync Anyway override through, and unlocks afterwards", func() {
			syncRepo := &fakeSyncRepository{holdReport: heldReportFor(20)}
			repo.saved = percentPtr(20)
			rewire(syncRepo, &fakeFetcher{})
			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))

			override := `{"overrideMassDeletion": true, "confirmedMassDeletion": {"usersExisting": 20, "usersDeleting": 6, "teamsExisting": 10, "teamsDeleting": 3}}`
			Expect(request(http.MethodPost, syncPath, adminToken, override).Code).To(Equal(http.StatusOK))

			Expect(decode(request(http.MethodGet, thresholdPath, adminToken, "")).Locked).To(BeFalse())
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 40}`).Code).
				To(Equal(http.StatusOK))
		})

		It("unlocks when the admin dismisses the hold instead of overriding it", func() {
			syncRepo := &fakeSyncRepository{holdReport: heldReportFor(20)}
			repo.saved = percentPtr(20)
			rewire(syncRepo, &fakeFetcher{})
			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))

			body := decode(request(http.MethodDelete, holdPath, adminToken, ""))
			Expect(body.Locked).To(BeFalse())

			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 40}`).Code).
				To(Equal(http.StatusOK))
			// Dismissing resolves the review; it must not have applied anything.
			Expect(syncRepo.calls).To(Equal(1))
		})

		It("refuses a non-admin dismissing a hold", func() {
			syncRepo := &fakeSyncRepository{holdReport: heldReportFor(20)}
			repo.saved = percentPtr(20)
			rewire(syncRepo, &fakeFetcher{})
			Expect(request(http.MethodPost, syncPath, adminToken, "").Code).To(Equal(http.StatusConflict))

			Expect(request(http.MethodDelete, holdPath, memberToken, "").Code).To(Equal(http.StatusForbidden))
			Expect(decode(request(http.MethodGet, thresholdPath, adminToken, "")).Locked).To(BeTrue())
		})

		It("refuses a threshold update while a sync is actually running", func() {
			gate := make(chan struct{})
			// Buffered: the fetcher's signal is non-blocking, so an unbuffered
			// channel would drop it before the spec starts listening.
			started := make(chan struct{}, 1)
			syncRepo := &fakeSyncRepository{}
			repo.saved = percentPtr(20)
			rewire(syncRepo, &gatedFetcher{gate: gate, started: started})

			done := make(chan int, 1)
			go func() {
				done <- request(http.MethodPost, syncPath, adminToken, "").Code
			}()

			Eventually(started, time.Second).Should(Receive())

			w := request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 100}`)
			Expect(w.Code).To(Equal(http.StatusConflict), w.Body.String())
			Expect(decode(request(http.MethodGet, thresholdPath, adminToken, "")).LockReason).
				To(Equal(dto.ThresholdLockSyncing))

			close(gate)
			Eventually(done, time.Second).Should(Receive(Equal(http.StatusOK)))

			// The run finished cleanly, so editing is available again.
			Expect(request(http.MethodPut, thresholdPath, adminToken, `{"maxDeletePercent": 40}`).Code).
				To(Equal(http.StatusOK))
		})
	})
})

// shiftingThresholdStore returns one value on its first read and another on
// every read after it, standing in for a setting that changed mid-lifecycle.
// This is the API-layer twin of services_test.shiftingStore in
// organization_sync_threshold_test.go -- duplicated deliberately rather than
// exported, since a test-only helper shared across the services and v1
// packages would need its own package just for this.
type shiftingThresholdStore struct {
	first, rest float64
	reads       int
}

func (s *shiftingThresholdStore) GetOrgSyncMaxDeletePercent(_ context.Context) (*float64, error) {
	s.reads++
	value := s.rest
	if s.reads == 1 {
		value = s.first
	}
	return &value, nil
}

// gatedFetcher blocks inside the sync until its gate is closed, which keeps a
// run genuinely in flight while another request is made against the API.
type gatedFetcher struct {
	gate    chan struct{}
	started chan struct{}
}

func (f *gatedFetcher) FetchSnapshot(ctx context.Context) (*orgsnapshot.Snapshot, error) {
	select {
	case f.started <- struct{}{}:
	default:
	}
	<-f.gate
	return (&fakeFetcher{}).FetchSnapshot(ctx)
}
