// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gin-gonic/gin"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/interfaces/dto"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
)

var _ = Describe("Integration: Supervisor Chain API", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		cleanup    func()
		adminToken string
	)

	BeforeEach(func() {
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		// Generate an admin-level JWT token
		jwtService := services.NewJWTService()
		tokenPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = tokenPair.AccessToken

		// Set up admin routes
		router = gin.New()
		orgRepo := postgres.NewOrganizationRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)
		v1.SetupAdminRoutes(router, orgRepo, userRepo, teamRepo, jwtService)

		// Insert test users needed for supervisor chain tests
		_, err = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash) VALUES
			('manager1', 'manager1', 'manager1@test.com', 'Manager One', 'level-3', 'unused'),
			('manager2', 'manager2', 'manager2@test.com', 'Manager Two', 'level-3', 'unused'),
			('director1', 'director1', 'director1@test.com', 'Director One', 'level-2', 'unused')
			ON CONFLICT (id) DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		// Insert test team
		_, err = db.Exec(`
			INSERT INTO teams (id, name, cadence)
			VALUES ('sc_team', 'SC Integration Team', 'monthly')
			ON CONFLICT (id) DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		db.Exec("DELETE FROM team_supervisors WHERE team_id LIKE 'sc_%'")
		db.Exec("DELETE FROM teams WHERE id LIKE 'sc_%'")
		db.Exec("DELETE FROM users WHERE id IN ('manager1', 'manager2', 'director1')")
		cleanup()
	})

	Describe("GET /api/v1/admin/teams/:id/supervisors", func() {
		Context("when the team has no supervisors", func() {
			It("should return an empty supervisors array", func() {
				req := httptest.NewRequest("GET", "/api/v1/admin/teams/sc_team/supervisors", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var response dto.SupervisorChainResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.TeamID).To(Equal("sc_team"))
				Expect(response.Supervisors).To(HaveLen(0))
			})
		})

		Context("when the team has supervisors", func() {
			BeforeEach(func() {
				_, err := db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('sc_team', 'manager1', 'level-3', 1),
					       ('sc_team', 'director1', 'level-2', 2)
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should return supervisors with enriched names ordered by position", func() {
				req := httptest.NewRequest("GET", "/api/v1/admin/teams/sc_team/supervisors", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var response dto.SupervisorChainResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Supervisors).To(HaveLen(2))

				// First supervisor: manager1 at level-3
				Expect(response.Supervisors[0].UserID).To(Equal("manager1"))
				Expect(response.Supervisors[0].LevelID).To(Equal("level-3"))
				Expect(response.Supervisors[0].UserName).NotTo(BeEmpty())
				Expect(response.Supervisors[0].LevelName).NotTo(BeEmpty())

				// Second supervisor: director1 at level-2
				Expect(response.Supervisors[1].UserID).To(Equal("director1"))
				Expect(response.Supervisors[1].LevelID).To(Equal("level-2"))
			})
		})

		Context("when the team does not exist", func() {
			It("should return 404", func() {
				req := httptest.NewRequest("GET", "/api/v1/admin/teams/nonexistent/supervisors", nil)
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when no auth token is provided", func() {
			It("should return 401", func() {
				req := httptest.NewRequest("GET", "/api/v1/admin/teams/sc_team/supervisors", nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusUnauthorized))
			})
		})
	})

	Describe("PUT /api/v1/admin/teams/:id/supervisors", func() {
		Context("when setting a new supervisor chain", func() {
			It("should save the chain and return enriched data", func() {
				reqBody := dto.UpdateSupervisorChainRequest{
					Supervisors: []dto.SupervisorLinkInput{
						{UserID: "manager1", LevelID: "level-3"},
						{UserID: "director1", LevelID: "level-2"},
					},
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("PUT", "/api/v1/admin/teams/sc_team/supervisors", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var response dto.SupervisorChainResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Supervisors).To(HaveLen(2))
				Expect(response.Supervisors[0].UserID).To(Equal("manager1"))
				Expect(response.Supervisors[1].UserID).To(Equal("director1"))

				// Verify database persistence
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM team_supervisors WHERE team_id = 'sc_team'").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(2))
			})
		})

		Context("when replacing an existing supervisor chain", func() {
			BeforeEach(func() {
				_, err := db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('sc_team', 'manager1', 'level-3', 1),
					       ('sc_team', 'director1', 'level-2', 2)
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should replace the chain completely", func() {
				reqBody := dto.UpdateSupervisorChainRequest{
					Supervisors: []dto.SupervisorLinkInput{
						{UserID: "manager2", LevelID: "level-3"},
					},
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("PUT", "/api/v1/admin/teams/sc_team/supervisors", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var response dto.SupervisorChainResponse
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Supervisors).To(HaveLen(1))
				Expect(response.Supervisors[0].UserID).To(Equal("manager2"))

				// Verify old supervisors were removed
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM team_supervisors WHERE team_id = 'sc_team'").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(1))
			})
		})

		Context("when clearing the supervisor chain", func() {
			BeforeEach(func() {
				_, err := db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('sc_team', 'manager1', 'level-3', 1)
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should remove all supervisors", func() {
				reqBody := dto.UpdateSupervisorChainRequest{
					Supervisors: []dto.SupervisorLinkInput{},
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("PUT", "/api/v1/admin/teams/sc_team/supervisors", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var count int
				err := db.QueryRow("SELECT COUNT(*) FROM team_supervisors WHERE team_id = 'sc_team'").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(0))
			})
		})

		Context("when the team does not exist", func() {
			It("should return 404", func() {
				reqBody := dto.UpdateSupervisorChainRequest{
					Supervisors: []dto.SupervisorLinkInput{
						{UserID: "manager1", LevelID: "level-3"},
					},
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("PUT", "/api/v1/admin/teams/nonexistent/supervisors", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+adminToken)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusNotFound))
			})
		})

		Context("when no auth token is provided", func() {
			It("should return 401", func() {
				reqBody := dto.UpdateSupervisorChainRequest{
					Supervisors: []dto.SupervisorLinkInput{
						{UserID: "manager1", LevelID: "level-3"},
					},
				}
				body, _ := json.Marshal(reqBody)

				req := httptest.NewRequest("PUT", "/api/v1/admin/teams/sc_team/supervisors", bytes.NewBuffer(body))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusUnauthorized))
			})
		})
	})
})
