package v1_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncrun"
	"github.com/agopalakrishnan/teams360/backend/domain/orgsyncschedule"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type schedStore struct {
	mu sync.Mutex
	s  orgsyncschedule.Schedule
}

func (m *schedStore) Get(context.Context) (*orgsyncschedule.Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.s
	return &c, nil
}
func (m *schedStore) Save(_ context.Context, en bool, f orgsyncschedule.Frequency, next *time.Time, _ string) (*orgsyncschedule.Schedule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.s.Enabled, m.s.Frequency, m.s.NextRunAt = en, f, next
	c := m.s
	return &c, nil
}
func (m *schedStore) Claim(context.Context, time.Time, time.Time) (bool, error) { return false, nil }

type noHistory struct{}

func (noHistory) LatestByTrigger(context.Context, orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	return nil, nil
}
func (noHistory) LatestSuccessByTrigger(context.Context, orgsyncrun.Trigger) (*orgsyncrun.Run, error) {
	return nil, nil
}

type neverStarter struct{ called bool }

func (n *neverStarter) Configured() bool { return true }
func (n *neverStarter) StartRun(context.Context, services.SyncOptions, string) (*orgsyncrun.Run, error) {
	n.called = true
	return &orgsyncrun.Run{}, nil
}

var _ = Describe("Auto-sync schedule endpoints", func() {
	const path = "/api/v1/admin/organization-provider/auto-sync"
	var (
		router              *gin.Engine
		store               *schedStore
		starter             *neverStarter
		adminTok, memberTok string
	)

	do := func(method, tok, body string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, path, bytes.NewBufferString(body))
		Expect(err).NotTo(HaveOccurred())
		req.Header.Set("Content-Type", "application/json")
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
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

	BeforeEach(func() {
		GinkgoT().Setenv("JWT_SECRET", "test-secret-key-for-handler-tests")
		gin.SetMode(gin.TestMode)
		store = &schedStore{s: orgsyncschedule.Schedule{Frequency: orgsyncschedule.Daily}}
		starter = &neverStarter{}
		sched := services.NewOrganizationSyncScheduler(store, noHistory{}, starter)
		jwtService := services.NewJWTService()
		a, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "a@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		m, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "m@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		adminTok, memberTok = a.AccessToken, m.AccessToken
		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, services.NewOrganizationSyncService(nil, nil, nil, nil), &thresholdRepo{}, jwtService,
			v1.WithAutoSyncScheduler(sched))
	})

	It("requires authentication and admin rights for both read and update", func() {
		Expect(do(http.MethodGet, "", "").Code).To(Equal(http.StatusUnauthorized))
		Expect(do(http.MethodPut, "", `{"enabled":true,"frequency":"DAILY"}`).Code).To(Equal(http.StatusUnauthorized))
		Expect(do(http.MethodGet, memberTok, "").Code).To(Equal(http.StatusForbidden))
		Expect(do(http.MethodPut, memberTok, `{"enabled":true,"frequency":"DAILY"}`).Code).To(Equal(http.StatusForbidden))
		Expect(store.s.Enabled).To(BeFalse(), "a forbidden update must not persist")
	})

	It("defaults to disabled with no next run and UTC timezone", func() {
		body := decode(do(http.MethodGet, adminTok, ""))
		Expect(body["enabled"]).To(BeFalse())
		Expect(body["nextRunAt"]).To(BeNil())
		Expect(body["lastAttempt"]).To(BeNil())
		Expect(body["lastSuccessAt"]).To(BeNil())
		Expect(body["timezone"]).To(Equal("UTC"))
	})

	It("persists enable and frequency, computes a future next run and starts no sync", func() {
		w := do(http.MethodPut, adminTok, `{"enabled":true,"frequency":"WEEKLY"}`)
		Expect(w.Code).To(Equal(http.StatusOK))
		body := decode(w)
		Expect(body["enabled"]).To(BeTrue())
		Expect(body["frequency"]).To(Equal("WEEKLY"))
		next, err := time.Parse(time.RFC3339, body["nextRunAt"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(next).To(BeTemporally(">", time.Now()))
		Expect(next.Weekday()).To(Equal(time.Monday))
		Expect(next.Hour()).To(Equal(2))
		Expect(starter.called).To(BeFalse())

		Expect(decode(do(http.MethodGet, adminTok, ""))["frequency"]).To(Equal("WEEKLY"))

		off := decode(do(http.MethodPut, adminTok, `{"enabled":false}`))
		Expect(off["enabled"]).To(BeFalse())
		Expect(off["nextRunAt"]).To(BeNil())
		Expect(off["frequency"]).To(Equal("WEEKLY"), "disabling keeps the chosen frequency")
	})

	DescribeTable("rejects invalid input without saving",
		func(body string) {
			Expect(do(http.MethodPut, adminTok, body).Code).To(Equal(http.StatusBadRequest))
			Expect(store.s.Enabled).To(BeFalse())
		},
		Entry("unknown frequency", `{"enabled":true,"frequency":"HOURLY"}`),
		Entry("cron expression", `{"enabled":true,"frequency":"0 2 * * *"}`),
		Entry("enabling without frequency", `{"enabled":true}`),
		Entry("missing enabled", `{"frequency":"DAILY"}`),
		Entry("wrong type", `{"enabled":"yes","frequency":"DAILY"}`),
		Entry("malformed json", `{`),
	)
})
