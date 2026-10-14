package v1_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// historyRecorder implements only ListOrgSyncHistory -- same "embed and panic
// on anything else" convention as thresholdRepo and scheduleRepo.
type historyRecorder struct {
	services.SyncRunRecorder

	entries  []orgprovider.OrgSyncHistoryEntry
	gotLimit int
}

func (r *historyRecorder) ListOrgSyncHistory(_ context.Context, limit int) ([]orgprovider.OrgSyncHistoryEntry, error) {
	r.gotLimit = limit
	return r.entries, nil
}

var _ = Describe("Admin: organization-sync run IDs and history", func() {
	const (
		syncPath    = "/api/v1/admin/organization-provider/sync"
		historyPath = "/api/v1/admin/organization-provider/sync/history"
	)

	var (
		router      *gin.Engine
		syncRepo    *fakeSyncRepository
		recorder    *historyRecorder
		adminToken  string
		memberToken string
	)

	do := func(method, url, token string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, url, nil)
		Expect(err).NotTo(HaveOccurred())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// build wires the routes around a sync service that does (recorder != nil)
	// or does not have a history recorder.
	build := func(withRecorder bool) {
		jwtService := services.NewJWTService()

		adminPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = adminPair.AccessToken

		memberPair, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "member@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		memberToken = memberPair.AccessToken

		syncRepo = &fakeSyncRepository{}
		var opts []services.Option
		if withRecorder {
			recorder = &historyRecorder{}
			opts = append(opts, services.WithSyncRunRecorder(recorder))
		}
		syncService := services.NewOrganizationSyncService(syncRepo, &fakeFetcher{}, nil, nil, opts...)

		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, syncService, &scheduleRepo{}, jwtService, time.UTC)
	}

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-run-id-tests")
		gin.SetMode(gin.TestMode)
	})

	AfterEach(func() {
		os.Unsetenv("JWT_SECRET")
	})

	Describe("POST sync", func() {
		It("returns the run ID in a header and in the body, and it is the ID the sync used", func() {
			build(false)

			w := do(http.MethodPost, syncPath, adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))

			headerID := w.Header().Get(v1.SyncRunIDHeader)
			_, err := uuid.Parse(headerID)
			Expect(err).NotTo(HaveOccurred(), "the header must carry a UUID")

			var body struct {
				RunID string `json:"runId"`
			}
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			Expect(body.RunID).To(Equal(headerID))
			Expect(syncRepo.lastInput.RunID).To(Equal(headerID), "the persistence layer must receive the same ID")
		})

		It("gives each request its own run ID", func() {
			build(false)

			first := do(http.MethodPost, syncPath, adminToken).Header().Get(v1.SyncRunIDHeader)
			second := do(http.MethodPost, syncPath, adminToken).Header().Get(v1.SyncRunIDHeader)

			Expect(first).NotTo(BeEmpty())
			Expect(second).NotTo(BeEmpty())
			Expect(first).NotTo(Equal(second))
		})
	})

	Describe("GET history", func() {
		It("is an empty list, not null, when nothing has been recorded", func() {
			build(false)

			w := do(http.MethodGet, historyPath, adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Body.String()).To(MatchJSON(`{"entries":[]}`))
		})

		It("returns the recorded entries with their run IDs, and defaults the limit to 50", func() {
			build(true)
			id := uuid.NewString()
			recorder.entries = []orgprovider.OrgSyncHistoryEntry{{
				OrgSyncAttempt: orgprovider.OrgSyncAttempt{
					RunID:        id,
					Trigger:      orgprovider.TriggerScheduled,
					Status:       orgprovider.StatusFailed,
					StartedAt:    time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC),
					FinishedAt:   time.Date(2026, 3, 10, 2, 0, 5, 0, time.UTC),
					WritesStatus: orgprovider.WritesNone,
					Message:      "provider unreachable",
				},
			}}

			w := do(http.MethodGet, historyPath, adminToken)
			Expect(w.Code).To(Equal(http.StatusOK))

			var body dto.OrgSyncHistoryDTO
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			Expect(body.Entries).To(HaveLen(1))
			Expect(body.Entries[0].RunID).To(Equal(id))
			Expect(body.Entries[0].Trigger).To(Equal("scheduled"))
			Expect(body.Entries[0].Status).To(Equal("failed"))
			Expect(body.Entries[0].Message).To(Equal("provider unreachable"))
			Expect(recorder.gotLimit).To(Equal(50))
		})

		It("honors a limit and caps it at 200", func() {
			build(true)

			Expect(do(http.MethodGet, historyPath+"?limit=7", adminToken).Code).To(Equal(http.StatusOK))
			Expect(recorder.gotLimit).To(Equal(7))

			Expect(do(http.MethodGet, historyPath+"?limit=5000", adminToken).Code).To(Equal(http.StatusOK))
			Expect(recorder.gotLimit).To(Equal(200))
		})

		It("rejects a limit that is not a positive integer", func() {
			build(true)

			Expect(do(http.MethodGet, historyPath+"?limit=abc", adminToken).Code).To(Equal(http.StatusBadRequest))
			Expect(do(http.MethodGet, historyPath+"?limit=0", adminToken).Code).To(Equal(http.StatusBadRequest))
			Expect(do(http.MethodGet, historyPath+"?limit=-3", adminToken).Code).To(Equal(http.StatusBadRequest))
		})

		It("refuses a non-admin", func() {
			build(true)
			Expect(do(http.MethodGet, historyPath, memberToken).Code).To(Equal(http.StatusForbidden))
		})

		It("refuses a request with no token", func() {
			build(true)
			Expect(do(http.MethodGet, historyPath, "").Code).To(Equal(http.StatusUnauthorized))
		})
	})
})
