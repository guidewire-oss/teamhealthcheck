// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/interfaces/api/v1"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	"github.com/gin-gonic/gin"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/bcrypt"
)

var _ = Describe("Authentication API Integration Tests", func() {
	var (
		router     *gin.Engine
		db         *sql.DB
		cleanup    func()
		jwtService *services.JWTService
	)

	BeforeEach(func() {
		// Set a fixed JWT_SECRET for consistent testing
		os.Setenv("JWT_SECRET", "test-secret-key-for-integration-tests")

		// Setup test database with helpers
		db, cleanup = testhelpers.SetupTestDatabase()

		// Setup Gin router
		gin.SetMode(gin.TestMode)
		router = gin.Default()

		// Create user repository and JWT service, then setup auth routes
		userRepo := postgres.NewUserRepository(db)
		orgRepo := postgres.NewOrganizationRepository(db)
		jwtService = services.NewJWTService()
		v1.SetupAuthRoutes(router, userRepo, orgRepo, jwtService)
	})

	AfterEach(func() {
		cleanup()
		os.Unsetenv("JWT_SECRET")
	})

	Describe("POST /api/v1/auth/login", func() {
		Context("when credentials are valid", func() {
			It("should return 200 OK with JWT tokens and user data", func() {
				// Given: A user exists in the database with bcrypt hashed password
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('authtest1', 'authuser', 'authuser@test.com', 'Auth Test User', 'level-4', $1)
				`, string(hashedPassword))
				Expect(err).NotTo(HaveOccurred())

				// When: User submits valid credentials
				loginData := map[string]string{
					"username": "authuser",
					"password": "testpass",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Response should contain user data, access token, and refresh token
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				// Verify user data
				user := response["user"].(map[string]interface{})
				Expect(user["id"]).To(Equal("authtest1"))
				Expect(user["username"]).To(Equal("authuser"))
				Expect(user["email"]).To(Equal("authuser@test.com"))
				Expect(user["fullName"]).To(Equal("Auth Test User"))
				Expect(user["hierarchyLevel"]).To(Equal("level-4"))
				Expect(user).NotTo(HaveKey("passwordHash"))

				// Verify JWT tokens are present
				Expect(response["accessToken"]).NotTo(BeEmpty())
				Expect(response["refreshToken"]).NotTo(BeEmpty())
				Expect(response["expiresIn"]).To(BeNumerically(">", 0))
			})
		})

		Context("when username is invalid", func() {
			It("should return 401 Unauthorized", func() {
				// When: User submits non-existent username
				loginData := map[string]string{
					"username": "nonexistentuser",
					"password": "anypassword",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 401 Unauthorized
				Expect(w.Code).To(Equal(http.StatusUnauthorized))

				// And: Error message should be returned
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid username or password"))
			})
		})

		Context("when password is incorrect", func() {
			It("should return 401 Unauthorized", func() {
				// Given: A user exists in the database with bcrypt hashed password
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte("correctpass"), bcrypt.DefaultCost)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('authtest1', 'authuser', 'authuser@test.com', 'Auth Test User', 'level-4', $1)
				`, string(hashedPassword))
				Expect(err).NotTo(HaveOccurred())

				// When: User submits incorrect password
				loginData := map[string]string{
					"username": "authuser",
					"password": "wrongpassword",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 401 Unauthorized
				Expect(w.Code).To(Equal(http.StatusUnauthorized))

				// And: Error message should be returned
				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid username or password"))
			})
		})

		Context("when credentials are missing", func() {
			It("should return 400 Bad Request when username is missing", func() {
				// When: Request is missing username
				loginData := map[string]string{
					"password": "testpass",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 400 Bad Request
				Expect(w.Code).To(Equal(http.StatusBadRequest))

				// And: Error message should indicate missing field
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Username and password are required"))
			})

			It("should return 400 Bad Request when password is missing", func() {
				// When: Request is missing password
				loginData := map[string]string{
					"username": "testuser",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 400 Bad Request
				Expect(w.Code).To(Equal(http.StatusBadRequest))

				// And: Error message should indicate missing field
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Username and password are required"))
			})
		})
	})

	Describe("POST /api/v1/auth/refresh", func() {
		var validRefreshToken string

		BeforeEach(func() {
			// Create a test user and login to get valid tokens
			hashedPassword, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('refreshtest1', 'refreshuser', 'refreshuser@test.com', 'Refresh Test User', 'level-4', $1)
			`, string(hashedPassword))
			Expect(err).NotTo(HaveOccurred())

			// Login to get tokens
			loginData := map[string]string{
				"username": "refreshuser",
				"password": "testpass",
			}
			jsonData, _ := json.Marshal(loginData)
			req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			Expect(w.Code).To(Equal(http.StatusOK))

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			Expect(err).NotTo(HaveOccurred())

			validRefreshToken = response["refreshToken"].(string)
		})

		Context("when refresh token is valid", func() {
			It("should return 200 OK with new access token", func() {
				// When: User submits valid refresh token
				refreshData := map[string]string{
					"refreshToken": validRefreshToken,
				}
				jsonData, _ := json.Marshal(refreshData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Response should contain new access token
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				Expect(response["accessToken"]).NotTo(BeEmpty())
				Expect(response["expiresIn"]).To(BeNumerically(">", 0))
			})
		})

		Context("when refresh token is invalid", func() {
			It("should return 401 Unauthorized", func() {
				// When: User submits invalid refresh token
				refreshData := map[string]string{
					"refreshToken": "invalid.refresh.token",
				}
				jsonData, _ := json.Marshal(refreshData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 401 Unauthorized
				Expect(w.Code).To(Equal(http.StatusUnauthorized))

				// And: Error message should be returned
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Invalid or expired refresh token"))
			})
		})

		Context("when refresh token is missing", func() {
			It("should return 400 Bad Request", func() {
				// When: Request is missing refresh token
				refreshData := map[string]string{}
				jsonData, _ := json.Marshal(refreshData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/refresh", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 400 Bad Request
				Expect(w.Code).To(Equal(http.StatusBadRequest))

				// And: Error message should indicate missing field
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["error"]).To(Equal("Refresh token is required"))
			})
		})
	})

	Describe("POST /api/v1/auth/logout", func() {
		Context("when user logs out", func() {
			It("should return 200 OK with success message", func() {
				// When: User calls logout endpoint
				req, _ := http.NewRequest("POST", "/api/v1/auth/logout", nil)

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				// Then: Response should be 200 OK
				Expect(w.Code).To(Equal(http.StatusOK))

				// And: Response should contain success message
				var response map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(response["message"]).To(Equal("Logged out successfully"))
			})
		})
	})

	Describe("JWT Token Validation", func() {
		Context("when access token is used to access protected resource", func() {
			It("should validate token claims correctly", func() {
				// Given: A user exists and logs in
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('claimstest1', 'claimsuser', 'claimsuser@test.com', 'Claims Test User', 'level-3', $1)
				`, string(hashedPassword))
				Expect(err).NotTo(HaveOccurred())

				// Login to get access token
				loginData := map[string]string{
					"username": "claimsuser",
					"password": "testpass",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				accessToken := response["accessToken"].(string)

				// When: Token is validated
				claims, err := jwtService.ValidateAccessToken(accessToken)

				// Then: Claims should contain correct user info
				Expect(err).NotTo(HaveOccurred())
				Expect(claims.UserID).To(Equal("claimstest1"))
				Expect(claims.Username).To(Equal("claimsuser"))
				Expect(claims.Email).To(Equal("claimsuser@test.com"))
				Expect(claims.HierarchyLevel).To(Equal("level-3"))
			})
		})

		Context("when access token is tampered with", func() {
			It("should reject the token", func() {
				// When: Tampered token is validated
				tamperedToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VySWQiOiJ0ZXN0IiwidXNlcm5hbWUiOiJ0ZXN0IiwiZW1haWwiOiJ0ZXN0QHRlc3QuY29tIiwiaGllcmFyY2h5TGV2ZWwiOiJsZXZlbC01In0.invalid-signature"
				claims, err := jwtService.ValidateAccessToken(tamperedToken)

				// Then: Should return error
				Expect(err).To(HaveOccurred())
				Expect(claims).To(BeNil())
			})
		})
	})

	Describe("User team membership in JWT", func() {
		Context("when user is member of teams", func() {
			It("should include team IDs in the JWT token", func() {
				// Given: A user exists with team memberships
				hashedPassword, err := bcrypt.GenerateFromPassword([]byte("testpass"), bcrypt.DefaultCost)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('teamtest1', 'teamuser', 'teamuser@test.com', 'Team Test User', 'level-5', $1)
				`, string(hashedPassword))
				Expect(err).NotTo(HaveOccurred())

				// Create teams
				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id) VALUES
					('team-alpha', 'Team Alpha', 'teamtest1'),
					('team-beta', 'Team Beta', NULL)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add user to team
				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id) VALUES
					('team-beta', 'teamtest1')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: User logs in
				loginData := map[string]string{
					"username": "teamuser",
					"password": "testpass",
				}
				jsonData, _ := json.Marshal(loginData)
				req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(jsonData))
				req.Header.Set("Content-Type", "application/json")

				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				Expect(w.Code).To(Equal(http.StatusOK))

				var response map[string]interface{}
				err = json.Unmarshal(w.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())

				// Then: Response should include team IDs
				user := response["user"].(map[string]interface{})
				teamIds := user["teamIds"].([]interface{})

				// User should be in both teams (as member and as lead)
				Expect(teamIds).To(ContainElement("team-alpha")) // As team lead
				Expect(teamIds).To(ContainElement("team-beta"))  // As team member

				// And: Access token should have team IDs in claims
				accessToken := response["accessToken"].(string)
				claims, err := jwtService.ValidateAccessToken(accessToken)
				Expect(err).NotTo(HaveOccurred())
				Expect(claims.TeamIDs).To(ContainElement("team-alpha"))
				Expect(claims.TeamIDs).To(ContainElement("team-beta"))
			})
		})
	})
})
