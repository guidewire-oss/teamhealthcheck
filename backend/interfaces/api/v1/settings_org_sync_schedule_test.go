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

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// scheduleRepo implements only the settings methods these specs exercise --
// same "embed and panic on anything else" convention as thresholdRepo.
type scheduleRepo struct {
	organization.Repository

	schedule *organization.OrgSyncSchedule
	getErr   error
	saveErr  error

	writes []struct {
		enabled   bool
		frequency string
		nextRunAt *time.Time
	}
}

func (r *scheduleRepo) GetOrgSyncSchedule(_ context.Context) (*organization.OrgSyncSchedule, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.schedule == nil {
		return &organization.OrgSyncSchedule{}, nil
	}
	return r.schedule, nil
}

func (r *scheduleRepo) UpdateOrgSyncSchedule(_ context.Context, enabled bool, frequency string, nextRunAt *time.Time) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.writes = append(r.writes, struct {
		enabled   bool
		frequency string
		nextRunAt *time.Time
	}{enabled, frequency, nextRunAt})
	r.schedule = &organization.OrgSyncSchedule{Enabled: enabled, Frequency: frequency, NextRunAt: nextRunAt}
	return nil
}

var _ = Describe("Admin settings: organization-sync schedule", func() {
	const schedulePath = "/api/v1/admin/settings/organization-provider/schedule"

	var (
		router      *gin.Engine
		repo        *scheduleRepo
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

	decode := func(w *httptest.ResponseRecorder) dto.OrgSyncScheduleDTO {
		var body dto.OrgSyncScheduleDTO
		Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
		return body
	}

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-schedule-tests")
		gin.SetMode(gin.TestMode)

		repo = &scheduleRepo{}

		jwtService := services.NewJWTService()
		adminPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = adminPair.AccessToken

		memberPair, err := jwtService.GenerateTokenPair(context.Background(), "member", "member", "member@test.com", "level-5", nil)
		Expect(err).NotTo(HaveOccurred())
		memberToken = memberPair.AccessToken

		syncService = services.NewOrganizationSyncService(&fakeSyncRepository{}, &fakeFetcher{}, nil, nil)
		router = gin.New()
		v1.SetupOrganizationProviderRoutes(router, syncService, repo, jwtService, time.UTC)
	})

	AfterEach(func() {
		os.Unsetenv("JWT_SECRET")
	})

	Describe("GET", func() {
		It("reports disabled with no frequency/nextRunAt on an empty repo", func() {
			body := decode(request(http.MethodGet, schedulePath, adminToken, ""))
			Expect(body.Enabled).To(BeFalse())
			Expect(body.Frequency).To(BeEmpty())
			Expect(body.NextRunAt).To(BeNil())
		})

		It("always serves the fixed available frequencies and the default", func() {
			body := decode(request(http.MethodGet, schedulePath, adminToken, ""))
			Expect(body.AvailableFrequencies).To(ConsistOf("daily", "weekly", "monthly"))
			Expect(body.DefaultFrequency).To(Equal("weekly"))
		})

		It("refuses a non-admin", func() {
			Expect(request(http.MethodGet, schedulePath, memberToken, "").Code).To(Equal(http.StatusForbidden))
		})

		It("refuses a request with no token", func() {
			Expect(request(http.MethodGet, schedulePath, "", "").Code).To(Equal(http.StatusUnauthorized))
		})

		It("returns 500 on a repository read error", func() {
			repo.getErr = errors.New("connection reset")
			Expect(request(http.MethodGet, schedulePath, adminToken, "").Code).To(Equal(http.StatusInternalServerError))
		})
	})

	Describe("PUT enabling", func() {
		It("stores the Weekly default when enabling with no frequency given", func() {
			w := request(http.MethodPut, schedulePath, adminToken, `{"enabled": true}`)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(repo.writes).To(HaveLen(1))
			Expect(repo.writes[0].enabled).To(BeTrue())
			Expect(repo.writes[0].frequency).To(Equal("weekly"))
			Expect(repo.writes[0].nextRunAt).NotTo(BeNil())
		})

		DescribeTable("round-trips each valid frequency",
			func(frequency string) {
				w := request(http.MethodPut, schedulePath, adminToken, `{"enabled": true, "frequency": "`+frequency+`"}`)
				Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
				Expect(repo.writes[len(repo.writes)-1].frequency).To(Equal(frequency))
				Expect(decode(w).Frequency).To(Equal(frequency))
			},
			Entry("daily", "daily"),
			Entry("weekly", "weekly"),
			Entry("monthly", "monthly"),
		)

		It("trims and normalizes an unexpected case, or rejects an invalid frequency naming all three -- with zero writes", func() {
			w := request(http.MethodPut, schedulePath, adminToken, `{"enabled": true, "frequency": "hourly"}`)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(w.Body.String()).To(ContainSubstring("daily"))
			Expect(w.Body.String()).To(ContainSubstring("weekly"))
			Expect(w.Body.String()).To(ContainSubstring("monthly"))
			Expect(repo.writes).To(BeEmpty())
		})

		It("rejects a missing enabled field with zero writes", func() {
			w := request(http.MethodPut, schedulePath, adminToken, `{}`)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(repo.writes).To(BeEmpty())
		})

		It("rejects a null enabled field with zero writes", func() {
			w := request(http.MethodPut, schedulePath, adminToken, `{"enabled": null}`)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(repo.writes).To(BeEmpty())
		})

		It("rejects malformed JSON with zero writes", func() {
			w := request(http.MethodPut, schedulePath, adminToken, `not json`)
			Expect(w.Code).To(Equal(http.StatusBadRequest))
			Expect(repo.writes).To(BeEmpty())
		})

		It("refuses a non-admin with zero writes", func() {
			w := request(http.MethodPut, schedulePath, memberToken, `{"enabled": true}`)
			Expect(w.Code).To(Equal(http.StatusForbidden))
			Expect(repo.writes).To(BeEmpty())
		})

		It("refuses a request with no token", func() {
			Expect(request(http.MethodPut, schedulePath, "", `{"enabled": true}`).Code).To(Equal(http.StatusUnauthorized))
		})

		It("re-enabling with no frequency reuses the previously saved one, not the Weekly default", func() {
			repo.schedule = &organization.OrgSyncSchedule{Enabled: false, Frequency: "daily"}
			w := request(http.MethodPut, schedulePath, adminToken, `{"enabled": true}`)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(repo.writes[0].frequency).To(Equal("daily"))
		})
	})

	Describe("PUT disabling", func() {
		It("preserves the saved frequency but clears nextRunAt", func() {
			repo.schedule = &organization.OrgSyncSchedule{
				Enabled: true, Frequency: "monthly", NextRunAt: func() *time.Time { t := time.Now(); return &t }(),
			}
			w := request(http.MethodPut, schedulePath, adminToken, `{"enabled": false}`)
			Expect(w.Code).To(Equal(http.StatusOK), w.Body.String())
			Expect(repo.writes).To(HaveLen(1))
			Expect(repo.writes[0].enabled).To(BeFalse())
			Expect(repo.writes[0].frequency).To(Equal("monthly"))
			Expect(repo.writes[0].nextRunAt).To(BeNil())
		})
	})

	Describe("GET last-run", func() {
		const lastRunPath = "/api/v1/admin/organization-provider/sync/last-run"

		It("reports no attempt and no skip when nothing has ever run", func() {
			var body dto.OrgSyncLastRunDTO
			w := request(http.MethodGet, lastRunPath, adminToken, "")
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			Expect(body.LastAttempt).To(BeNil())
			Expect(body.LastSkip).To(BeNil())
		})

		It("refuses a non-admin", func() {
			Expect(request(http.MethodGet, lastRunPath, memberToken, "").Code).To(Equal(http.StatusForbidden))
		})

		It("refuses a request with no token", func() {
			Expect(request(http.MethodGet, lastRunPath, "", "").Code).To(Equal(http.StatusUnauthorized))
		})
	})
})
