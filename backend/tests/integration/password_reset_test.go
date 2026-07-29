// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("Password Reset API Integration Tests", func() {
	var (
		router               *gin.Engine
		db                   *sql.DB
		cleanup              func()
		jwtService           *services.JWTService
		passwordResetService *services.PasswordResetService
	)

	BeforeEach(func() {
		// Set a fixed JWT_SECRET for consistent testing
		os.Setenv("JWT_SECRET", "test-secret-key-for-integration-tests")

		// Setup test database with helpers
		db, cleanup = testhelpers.SetupTestDatabase()

		// Setup Gin router
		gin.SetMode(gin.TestMode)
		router = gin.Default()

		// Create repositories and services
		userRepo := postgres.NewUserRepository(db)
		passwordResetRepo := postgres.NewPasswordResetRepository(db)
		jwtService = services.NewJWTService()

		// Create email service (mock for testing - doesn't actually send emails)
		emailService := testhelpers.NewMockEmailService()
		passwordResetService = services.NewPasswordResetService(passwordResetRepo, userRepo, emailService)

		// Setup auth routes with password reset
		orgRepo := postgres.NewOrganizationRepository(db)
		v1.SetupAuthRoutes(router, userRepo, orgRepo, jwtService)
		v1.SetupPasswordResetRoutes(router, passwordResetService, userRepo)
	})

	AfterEach(func() {
		cleanup()
		os.Unsetenv("JWT_SECRET")
	})

	Describe("POST /api/v1/auth/forgot-password", func() {
		Context("when email exists in the system", func() {
			It("should return 200 OK and create a reset token", func() {
				// Given: A user exists in the database
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.DefaultCost)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('resetuser1', 'resetuser', 'resetuser@test.com', 'Reset Test User', 'level-5', $1)
				`, string(hashedPassword))
				Expect(err).NotTo(HaveOccurred())

				// When: User requests password reset
				requestData := map[string]string{
					"email": "resetuser@test.com",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: A reset token should be created in the database
				var tokenCount int
				err = db.QueryRow("SELECT COUNT(*) FROM password_reset_tokens WHERE user_id = 'resetuser1'").Scan(&tokenCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(tokenCount).To(Equal(1))

				// And: Response should indicate success (without revealing token)
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["message"]).To(ContainSubstring("reset"))
			})
		})

		Context("when email does not exist", func() {
			It("should still return 200 OK for security (prevent email enumeration)", func() {
				// When: Request password reset for non-existent email
				requestData := map[string]string{
					"email": "nonexistent@test.com",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should still be 200 OK (to prevent email enumeration)
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: No token should be created
				var tokenCount int
				err := db.QueryRow("SELECT COUNT(*) FROM password_reset_tokens").Scan(&tokenCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(tokenCount).To(Equal(0))
			})
		})

		Context("when email is missing from request", func() {
			It("should return 400 Bad Request", func() {
				// When: Request is missing email
				requestData := map[string]string{}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 400 Bad Request
				Expect(w.Code).To(Equal(http.StatusBadRequest))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Email is required"))
			})
		})

		Context("when email format is invalid", func() {
			It("should return 400 Bad Request", func() {
				// When: Request has invalid email format
				requestData := map[string]string{
					"email": "not-an-email",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/forgot-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 400 Bad Request
				Expect(w.Code).To(Equal(http.StatusBadRequest))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid email format"))
			})
		})
	})

	Describe("POST /api/v1/auth/reset-password", func() {
		var validToken string

		BeforeEach(func() {
			// Create a user with a valid reset token
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('resetuser1', 'resetuser', 'resetuser@test.com', 'Reset Test User', 'level-5', $1)
			`, string(hashedPassword))
			Expect(err).NotTo(HaveOccurred())

			// Create a reset token using the service
			validToken, err = passwordResetService.CreateResetToken("resetuser@test.com")
			Expect(err).NotTo(HaveOccurred())
			Expect(validToken).NotTo(BeEmpty())
		})

		Context("when token is valid and password meets requirements", func() {
			It("should reset the password successfully", func() {
				// When: User submits valid token and new password
				requestData := map[string]string{
					"token":       validToken,
					"newPassword": "NewSecurePass123!",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: User should be able to login with new password
				loginData := map[string]string{
					"username": "resetuser",
					"password": "NewSecurePass123!",
				}
				loginJson, _ := json.Marshal(loginData)
				loginReq, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(loginJson))
				loginReq.Header.Set("Content-Type", "application/json")

				loginW := httptest.NewRecorder()
				router.ServeHTTP(loginW, loginReq)

				Expect(loginW.Code).To(Equal(http.StatusOK))

				// And: Old password should no longer work
				oldLoginData := map[string]string{
					"username": "resetuser",
					"password": "oldpass",
				}
				oldLoginJson, _ := json.Marshal(oldLoginData)
				oldLoginReq, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(oldLoginJson))
				oldLoginReq.Header.Set("Content-Type", "application/json")

				oldLoginW := httptest.NewRecorder()
				router.ServeHTTP(oldLoginW, oldLoginReq)

				Expect(oldLoginW.Code).To(Equal(http.StatusUnauthorized))

				// And: Token should be marked as used
				var usedAt sql.NullTime
				err := db.QueryRow("SELECT used_at FROM password_reset_tokens WHERE user_id = 'resetuser1'").Scan(&usedAt)
				Expect(err).NotTo(HaveOccurred())
				Expect(usedAt.Valid).To(BeTrue())
			})
		})

		Context("when token is invalid", func() {
			It("should return 401 Unauthorized", func() {
				// When: User submits invalid token
				requestData := map[string]string{
					"token":       "invalid-token-123",
					"newPassword": "NewSecurePass123!",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 401 Unauthorized
				Expect(w.Code).To(Equal(http.StatusUnauthorized))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid or expired reset token"))
			})
		})

		Context("when token has already been used", func() {
			It("should return 401 Unauthorized", func() {
				// Given: Token has been used once
				firstRequestData := map[string]string{
					"token":       validToken,
					"newPassword": "FirstNewPass123!",
				}
				firstJson, _ := json.Marshal(firstRequestData)
				firstReq, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(firstJson))
				firstReq.Header.Set("Content-Type", "application/json")

				firstW := httptest.NewRecorder()
				router.ServeHTTP(firstW, firstReq)
				Expect(firstW.Code).To(Equal(http.StatusOK))

				// When: Same token is used again
				secondRequestData := map[string]string{
					"token":       validToken,
					"newPassword": "SecondNewPass123!",
				}
				secondJson, _ := json.Marshal(secondRequestData)
				secondReq, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(secondJson))
				secondReq.Header.Set("Content-Type", "application/json")

				secondW := httptest.NewRecorder()
				router.ServeHTTP(secondW, secondReq)

				// Then: Response should be 401 Unauthorized
				Expect(secondW.Code).To(Equal(http.StatusUnauthorized))

				var response map[string]interface{}
				err := json.Unmarshal(secondW.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid or expired reset token"))
			})
		})

		Context("when token has expired", func() {
			It("should return 401 Unauthorized", func() {
				// Given: Create an expired token directly in DB
				expiredToken := "expired-token-123"
				hashedToken, _ := bcrypt.GenerateFromPassword([]byte(expiredToken), bcrypt.DefaultCost)

				_, err := db.Exec(`
					INSERT INTO password_reset_tokens (id, user_id, token_hash, expires_at)
					VALUES ('expired-id', 'resetuser1', $1, $2)
				`, string(hashedToken), time.Now().Add(-1*time.Hour)) // Expired 1 hour ago
				Expect(err).NotTo(HaveOccurred())

				// When: User tries to use expired token
				requestData := map[string]string{
					"token":       expiredToken,
					"newPassword": "NewSecurePass123!",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 401 Unauthorized
				Expect(w.Code).To(Equal(http.StatusUnauthorized))

				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid or expired reset token"))
			})
		})

		Context("when new password is too short", func() {
			It("should return 400 Bad Request", func() {
				// When: User submits password that's too short
				requestData := map[string]string{
					"token":       validToken,
					"newPassword": "short",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 400 Bad Request
				Expect(w.Code).To(Equal(http.StatusBadRequest))

				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(ContainSubstring("8 characters"))
			})
		})

		Context("when request is missing required fields", func() {
			It("should return 400 Bad Request when token is missing", func() {
				requestData := map[string]string{
					"newPassword": "NewSecurePass123!",
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})

			It("should return 400 Bad Request when new password is missing", func() {
				requestData := map[string]string{
					"token": validToken,
				}
				jsonData, _ := json.Marshal(requestData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/reset-password", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})
})
