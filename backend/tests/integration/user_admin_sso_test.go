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
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	v1 "github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("Integration: User Admin SSO", func() {
	var (
		db         *sql.DB
		router     *gin.Engine
		cleanup    func()
		adminToken string
	)

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-integration-tests")
		gin.SetMode(gin.TestMode)

		db, cleanup = testhelpers.SetupTestDatabase()

		jwtService := services.NewJWTService()
		tokenPair, err := jwtService.GenerateTokenPair(context.Background(), "admin", "admin", "admin@test.com", "level-admin", nil)
		Expect(err).NotTo(HaveOccurred())
		adminToken = tokenPair.AccessToken

		router = gin.New()
		orgRepo := postgres.NewOrganizationRepository(db)
		userRepo := postgres.NewUserRepository(db)
		teamRepo := postgres.NewTeamRepository(db)
		v1.SetupAdminRoutes(router, orgRepo, userRepo, teamRepo, jwtService)
		v1.SetupAuthRoutes(router, userRepo, orgRepo, jwtService)
	})

	AfterEach(func() {
		db.Exec("DELETE FROM users WHERE id LIKE 'ssotest%'")
		cleanup()
		os.Unsetenv("JWT_SECRET")
	})

	Describe("POST /api/v1/admin/users (create SSO user)", func() {
		It("should create an SSO user without a password", func() {
			payload := map[string]any{
				"username":       "ssotestuser",
				"email":          "ssotestuser@test.com",
				"fullName":       "SSO Test User",
				"hierarchyLevel": "level-4",
				"authType":       "sso",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusCreated))

			var resp map[string]any
			json.Unmarshal(w.Body.Bytes(), &resp)
			Expect(resp["authType"]).To(Equal("sso"))

			// Verify in database
			var authType string
			err := db.QueryRow("SELECT auth_type FROM users WHERE id = 'ssotestuser'").Scan(&authType)
			Expect(err).NotTo(HaveOccurred())
			Expect(authType).To(Equal("sso"))
		})

		It("should reject creating a local user without a password", func() {
			payload := map[string]any{
				"username":       "ssotestlocal",
				"email":          "ssotestlocal@test.com",
				"fullName":       "Local No Pass",
				"hierarchyLevel": "level-4",
				"authType":       "local",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", "/api/v1/admin/users", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("PUT /api/v1/admin/users/:id (update auth type)", func() {
		It("should reject switching SSO to local without a password", func() {
			// Create SSO user first
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
				VALUES ('ssotestswitch', 'ssotestswitch', 'ssotestswitch@test.com', 'Switch User', 'level-4', '', 'sso')
			`)
			Expect(err).NotTo(HaveOccurred())

			// Try switching to local without password
			payload := map[string]any{
				"authType": "local",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("PUT", "/api/v1/admin/users/ssotestswitch", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))

			var resp map[string]any
			json.Unmarshal(w.Body.Bytes(), &resp)
			Expect(resp["error"]).To(ContainSubstring("Password is required"))
		})

		It("should allow switching SSO to local with a password", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
				VALUES ('ssotestswitch2', 'ssotestswitch2', 'ssotestswitch2@test.com', 'Switch User 2', 'level-4', '', 'sso')
			`)
			Expect(err).NotTo(HaveOccurred())

			payload := map[string]any{
				"authType": "local",
				"password": "newpass123",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("PUT", "/api/v1/admin/users/ssotestswitch2", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			// Verify auth_type changed
			var authType string
			err = db.QueryRow("SELECT auth_type FROM users WHERE id = 'ssotestswitch2'").Scan(&authType)
			Expect(err).NotTo(HaveOccurred())
			Expect(authType).To(Equal("local"))
		})

		It("should reject setting password on SSO users", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
				VALUES ('ssotestnopass', 'ssotestnopass', 'ssotestnopass@test.com', 'No Pass SSO', 'level-4', '', 'sso')
			`)
			Expect(err).NotTo(HaveOccurred())

			payload := map[string]any{
				"password": "shouldfail",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("PUT", "/api/v1/admin/users/ssotestnopass", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusBadRequest))

			var resp map[string]any
			json.Unmarshal(w.Body.Bytes(), &resp)
			Expect(resp["error"]).To(ContainSubstring("Cannot set password for SSO users"))
		})

		It("should reject invalid authType values", func() {
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
				VALUES ('ssotestinvalid', 'ssotestinvalid', 'ssotestinvalid@test.com', 'Invalid Type', 'level-4', '', 'local')
			`)
			Expect(err).NotTo(HaveOccurred())

			payload := map[string]any{
				"authType": "oauth",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("PUT", "/api/v1/admin/users/ssotestinvalid", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+adminToken)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// Should be rejected by either binding validation or handler switch/case
			Expect(w.Code).To(BeNumerically(">=", 400))
		})
	})

	Describe("POST /api/v1/auth/login (SSO user guard)", func() {
		It("should reject SSO users from local login with generic error", func() {
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
				VALUES ('ssotestlogin', 'ssotestlogin', 'ssotestlogin@test.com', 'SSO Login User', 'level-4', $1, 'sso')
			`, string(hashedPassword))
			Expect(err).NotTo(HaveOccurred())

			payload := map[string]string{
				"username": "ssotestlogin",
				"password": "testpass",
			}
			body, _ := json.Marshal(payload)
			req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusUnauthorized))

			var resp map[string]any
			json.Unmarshal(w.Body.Bytes(), &resp)
			// Should NOT leak that account is SSO
			Expect(resp["error"]).To(Equal("Invalid username or password"))
		})
	})
})
