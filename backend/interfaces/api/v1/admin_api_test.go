// SPDX-License-Identifier: Apache-2.0

package v1_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/gin-gonic/gin"
	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

var _ = Describe("Admin API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		adminToken string
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		databaseURL := "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
		var err error
		db, err = sql.Open("postgres", databaseURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		// Clean schema and run migrations fresh
		_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
		Expect(err).NotTo(HaveOccurred())

		driver, err := migratePostgres.WithInstance(db, &migratePostgres.Config{})
		Expect(err).NotTo(HaveOccurred())

		migrationEngine, err := migrate.NewWithDatabaseInstance(
			"file://../../../infrastructure/persistence/postgres/migrations",
			"postgres",
			driver,
		)
		Expect(err).NotTo(HaveOccurred())

		err = migrationEngine.Up()
		Expect(err).NotTo(HaveOccurred())

		// Clean up test data
		db.Exec("DELETE FROM team_members WHERE user_id LIKE 'test-%' OR team_id LIKE 'test-%'")
		db.Exec("DELETE FROM team_supervisors WHERE user_id LIKE 'test-%' OR team_id LIKE 'test-%'")
		db.Exec("DELETE FROM health_check_responses")
		db.Exec("DELETE FROM health_check_sessions WHERE user_id LIKE 'test-%'")
		db.Exec("DELETE FROM teams WHERE id LIKE 'test-%'")
		db.Exec("DELETE FROM users WHERE id LIKE 'test-%'")
		db.Exec("DELETE FROM hierarchy_levels WHERE id LIKE 'test-%'")
		db.Exec("DELETE FROM health_dimensions WHERE id LIKE 'test-%' OR id LIKE 'e2e-%' OR id LIKE 'dim-%'")

		// Create JWT service and generate admin token
		jwtService := services.NewJWTService()
		tokenPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = tokenPair.AccessToken

		// Create repositories and router
		orgRepo := postgres.NewOrganizationRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)

		router = gin.New()
		v1.SetupAdminRoutes(router, orgRepo, userRepo, teamRepo, jwtService)
	})

	AfterEach(func() {
		if db != nil {
			db.Exec("DELETE FROM team_members WHERE team_id LIKE 'test-%'")
			db.Exec("DELETE FROM team_supervisors WHERE team_id LIKE 'test-%'")
			db.Exec("DELETE FROM teams WHERE id LIKE 'test-%'")
			db.Exec("DELETE FROM hierarchy_levels WHERE id LIKE 'test-%'")
			db.Exec("DELETE FROM users WHERE id LIKE 'test-%'")
			db.Close()
		}
	})

	Describe("GET /api/v1/admin/hierarchy-levels", func() {
		It("should return all hierarchy levels ordered by position", func() {
			req := httptest.NewRequest("GET", "/api/v1/admin/hierarchy-levels", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.HierarchyLevelsResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(response.Levels)).To(BeNumerically(">=", 5))

			// Verify order
			for i := 1; i < len(response.Levels); i++ {
				Expect(response.Levels[i].Position).To(BeNumerically(">=", response.Levels[i-1].Position))
			}
		})
	})

	Describe("POST /api/v1/admin/hierarchy-levels", func() {
		It("should create a new hierarchy level", func() {
			reqBody := dto.CreateHierarchyLevelRequest{
				ID:   "test-level-1",
				Name: "Test Level",
				Permissions: dto.HierarchyPermissionsDTO{
					CanViewAllTeams:  true,
					CanEditTeams:     false,
					CanManageUsers:   false,
					CanTakeSurvey:    true,
					CanViewAnalytics: false,
				},
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/hierarchy-levels", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))

			var response dto.HierarchyLevelDTO
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.ID).To(Equal("test-level-1"))
			Expect(response.Name).To(Equal("Test Level"))
		})
	})

	Describe("GET /api/v1/admin/users", func() {
		It("should return all users with total count", func() {
			req := httptest.NewRequest("GET", "/api/v1/admin/users", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.UsersResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Users).NotTo(BeEmpty())
			Expect(response.Total).To(Equal(len(response.Users)))
		})
	})

	Describe("POST /api/v1/admin/users", func() {
		It("should create a new user", func() {
			reqBody := dto.CreateUserRequest{
				ID:             "test-user-1",
				Username:       "testuser",
				Email:          "test@example.com",
				FullName:       "Test User",
				Password:       "password123",
				HierarchyLevel: "level-5",
				ReportsTo:      nil,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("POST", "/api/v1/admin/users", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))

			var response dto.AdminUserDTO
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Username).To(Equal("testuser"))
		})
	})

	Describe("GET /api/v1/admin/teams", func() {
		It("should return all teams with cadence and total count", func() {
			// Seed a test team so the response is non-empty
			db.Exec("INSERT INTO teams (id, name) VALUES ('test-team-1', 'Test Team') ON CONFLICT DO NOTHING")

			req := httptest.NewRequest("GET", "/api/v1/admin/teams", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.TeamsResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Teams).NotTo(BeEmpty())
			Expect(response.Total).To(Equal(len(response.Teams)))
		})
	})

	Describe("GET /api/v1/admin/settings/dimensions", func() {
		It("should return all 11 health dimensions", func() {
			req := httptest.NewRequest("GET", "/api/v1/admin/settings/dimensions", nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.DimensionsResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Dimensions).To(HaveLen(11))

			for _, dim := range response.Dimensions {
				Expect(dim.ID).NotTo(BeEmpty())
				Expect(dim.Name).NotTo(BeEmpty())
				Expect(dim.Weight).To(BeNumerically(">", 0))
			}
		})
	})

	Describe("PUT /api/v1/admin/settings/dimensions/:id", func() {
		It("should update dimension weight and active status", func() {
			isActive := false
			weight := 2.5
			reqBody := dto.UpdateDimensionRequest{
				IsActive: &isActive,
				Weight:   &weight,
			}

			body, _ := json.Marshal(reqBody)
			req := httptest.NewRequest("PUT", "/api/v1/admin/settings/dimensions/mission", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response dto.HealthDimensionDTO
			err := json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())
			Expect(response.IsActive).To(BeFalse())
			Expect(response.Weight).To(Equal(2.5))

			// Restore original values
			isActive = true
			weight = 1.0
			reqBody = dto.UpdateDimensionRequest{IsActive: &isActive, Weight: &weight}
			body, _ = json.Marshal(reqBody)
			req = httptest.NewRequest("PUT", "/api/v1/admin/settings/dimensions/mission", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)
			w = httptest.NewRecorder()
			router.ServeHTTP(w, req)
		})
	})
})
