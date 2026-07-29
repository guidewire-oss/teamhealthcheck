// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"bytes"
	"database/sql"
	"encoding/base64"
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

// makeTestIDToken returns a minimal unsigned JWT containing the given email.
// The SSO handler uses ParseUnverified so no real signature is needed in tests.
func makeTestIDToken(email string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claimsJSON, _ := json.Marshal(map[string]interface{}{
		"sub":   "test-subject",
		"email": email,
	})
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	return header + "." + payload + ".fakesig"
}

// startFakeTokenServer spins up an httptest server that mimics an OAuth token endpoint.
func startFakeTokenServer(idToken string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
				"access_token": "fake-access-token",
				"id_token":     idToken,
				"token_type":   "Bearer",
			})
		} else {
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"}) //nolint:errcheck
		}
	}))
}

var _ = Describe("SSO Authentication Integration Tests", func() {
	var (
		router  *gin.Engine
		db      *sql.DB
		cleanup func()
	)

	BeforeEach(func() {
		os.Setenv("JWT_SECRET", "test-secret-key-for-sso-tests")
		db, cleanup = testhelpers.SetupTestDatabase()
		gin.SetMode(gin.TestMode)
		router = gin.New()
		userRepo := postgres.NewUserRepository(db)
		orgRepo := postgres.NewOrganizationRepository(db)
		jwtService := services.NewJWTService()
		v1.SetupAuthRoutes(router, userRepo, orgRepo, jwtService)
		v1.SetupSSORoutes(router, userRepo, jwtService, orgRepo)
	})

	AfterEach(func() {
		cleanup()
		os.Unsetenv("JWT_SECRET")
		os.Unsetenv("OAUTH_CLIENT_ID")
		os.Unsetenv("OAUTH_TOKEN_URL")
		os.Unsetenv("OAUTH_REDIRECT_URI")
	})

	// ── helpers ───────────────────────────────────────────────────────────────

	insertUserWithPassword := func(id, username, email, password string) {
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		Expect(err).NotTo(HaveOccurred())
		_, err = db.Exec(
			`INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
			 VALUES ($1, $2, $3, 'Test User', 'level-5', $4)`,
			id, username, email, string(hashed),
		)
		Expect(err).NotTo(HaveOccurred())
	}

	postPasswordLogin := func(username, password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": username, "password": password})
		req, _ := http.NewRequest("POST", "/api/v1/auth/login", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	postSSOCallback := func(code, codeVerifier string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"code": code, "code_verifier": codeVerifier})
		req, _ := http.NewRequest("POST", "/api/v1/auth/sso/callback", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// ── 1. Username/password works when SSO vars are absent ──────────────────

	Describe("POST /api/v1/auth/login — username/password login", func() {
		Context("when no SSO environment variables are present", func() {
			It("should authenticate successfully with valid credentials", func() {
				insertUserWithPassword("pw-test-1", "pwuser1", "pwuser1@test.com", "secret")
				w := postPasswordLogin("pwuser1", "secret")

				Expect(w.Code).To(Equal(http.StatusOK))
				var resp map[string]interface{}
				Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["accessToken"]).NotTo(BeEmpty())
				Expect(resp["refreshToken"]).NotTo(BeEmpty())
			})

			It("should reject wrong credentials", func() {
				insertUserWithPassword("pw-test-2", "pwuser2", "pwuser2@test.com", "secret")
				w := postPasswordLogin("pwuser2", "wrongpassword")

				Expect(w.Code).To(Equal(http.StatusUnauthorized))
				var resp map[string]interface{}
				Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["error"]).To(Equal("Invalid username or password"))
			})
		})

		// ── 2. Username/password still works when SSO vars ARE set ───────────

		Context("when SSO environment variables are present", func() {
			BeforeEach(func() {
				os.Setenv("OAUTH_CLIENT_ID", "test-client-id")
				os.Setenv("OAUTH_TOKEN_URL", "http://fake-provider/token")
				os.Setenv("OAUTH_REDIRECT_URI", "http://localhost:3000/auth/callback")
			})

			It("should still authenticate successfully with valid credentials", func() {
				insertUserWithPassword("pw-sso-1", "pwssouser1", "pwssouser1@test.com", "secret")
				w := postPasswordLogin("pwssouser1", "secret")

				Expect(w.Code).To(Equal(http.StatusOK))
				var resp map[string]interface{}
				Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["accessToken"]).NotTo(BeEmpty())
			})

			It("should still reject wrong credentials", func() {
				insertUserWithPassword("pw-sso-2", "pwssouser2", "pwssouser2@test.com", "secret")
				w := postPasswordLogin("pwssouser2", "wrongpassword")

				Expect(w.Code).To(Equal(http.StatusUnauthorized))
			})
		})
	})

	// ── SSO callback tests ────────────────────────────────────────────────────

	Describe("POST /api/v1/auth/sso/callback", func() {
		Context("when SSO is not configured (no env vars)", func() {
			It("should return 503 with a descriptive error", func() {
				w := postSSOCallback("some-code", "some-verifier")

				Expect(w.Code).To(Equal(http.StatusServiceUnavailable))
				var resp map[string]interface{}
				Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
				Expect(resp["error"]).To(ContainSubstring("not configured"))
			})
		})

		Context("when SSO is configured", func() {
			BeforeEach(func() {
				os.Setenv("OAUTH_CLIENT_ID", "test-client-id")
				os.Setenv("OAUTH_REDIRECT_URI", "http://localhost:3000/auth/callback")
			})

			Context("when required fields are missing from the request", func() {
				BeforeEach(func() {
					os.Setenv("OAUTH_TOKEN_URL", "http://fake-provider/token")
				})

				It("should return 400 when code is missing", func() {
					body, _ := json.Marshal(map[string]string{"code_verifier": "some-verifier"})
					req, _ := http.NewRequest("POST", "/api/v1/auth/sso/callback", bytes.NewBuffer(body))
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					Expect(w.Code).To(Equal(http.StatusBadRequest))
				})

				It("should return 400 when code_verifier is missing", func() {
					body, _ := json.Marshal(map[string]string{"code": "some-code"})
					req, _ := http.NewRequest("POST", "/api/v1/auth/sso/callback", bytes.NewBuffer(body))
					req.Header.Set("Content-Type", "application/json")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, req)

					Expect(w.Code).To(Equal(http.StatusBadRequest))
				})
			})

			Context("when the OAuth provider rejects the code", func() {
				It("should return 401", func() {
					fakeProvider := startFakeTokenServer("", http.StatusBadRequest)
					defer fakeProvider.Close()
					os.Setenv("OAUTH_TOKEN_URL", fakeProvider.URL)

					w := postSSOCallback("bad-code", "some-verifier")

					Expect(w.Code).To(Equal(http.StatusUnauthorized))
					var resp map[string]interface{}
					Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
					Expect(resp["error"]).To(ContainSubstring("Token exchange"))
				})
			})

			Context("when the OAuth provider is unreachable", func() {
				It("should return 401", func() {
					os.Setenv("OAUTH_TOKEN_URL", "http://127.0.0.1:19999/nonexistent")

					w := postSSOCallback("some-code", "some-verifier")

					Expect(w.Code).To(Equal(http.StatusUnauthorized))
				})
			})

			Context("when the provider token has no email claim", func() {
				It("should return 401", func() {
					noEmailToken := func() string {
						header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
						claims, _ := json.Marshal(map[string]interface{}{"sub": "user-with-no-email"})
						payload := base64.RawURLEncoding.EncodeToString(claims)
						return header + "." + payload + ".fakesig"
					}()

					fakeProvider := startFakeTokenServer(noEmailToken, http.StatusOK)
					defer fakeProvider.Close()
					os.Setenv("OAUTH_TOKEN_URL", fakeProvider.URL)

					w := postSSOCallback("some-code", "some-verifier")

					Expect(w.Code).To(Equal(http.StatusUnauthorized))
					var resp map[string]interface{}
					Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
					Expect(resp["error"]).To(ContainSubstring("email"))
				})
			})

			// ── 4. Email not found in userRepo ────────────────────────────────

			Context("when the email from the token does not match any user in the DB", func() {
				It("should return 401 with a clear message directing the user to their administrator", func() {
					fakeProvider := startFakeTokenServer(makeTestIDToken("ghost@external.com"), http.StatusOK)
					defer fakeProvider.Close()
					os.Setenv("OAUTH_TOKEN_URL", fakeProvider.URL)

					w := postSSOCallback("some-code", "some-verifier")

					Expect(w.Code).To(Equal(http.StatusUnauthorized))
					var resp map[string]interface{}
					Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
					Expect(resp["error"]).To(ContainSubstring("No account found"))
					Expect(resp["error"]).To(ContainSubstring("administrator"))
				})
			})

			// ── 3. Email matches a user — login successful ────────────────────

			Context("when the email from the token matches a user in the DB", func() {
				BeforeEach(func() {
					_, err := db.Exec(`
						INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
						VALUES ('sso-user-1', 'ssouser', 'sso@example.com', 'SSO User', 'level-3', 'unused', 'sso')
					`)
					Expect(err).NotTo(HaveOccurred())
				})

				It("should return 200 with JWT tokens and correct user data", func() {
					fakeProvider := startFakeTokenServer(makeTestIDToken("sso@example.com"), http.StatusOK)
					defer fakeProvider.Close()
					os.Setenv("OAUTH_TOKEN_URL", fakeProvider.URL)

					w := postSSOCallback("valid-code", "valid-verifier")

					Expect(w.Code).To(Equal(http.StatusOK))
					var resp map[string]interface{}
					Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())

					user := resp["user"].(map[string]interface{})
					Expect(user["id"]).To(Equal("sso-user-1"))
					Expect(user["username"]).To(Equal("ssouser"))
					Expect(user["email"]).To(Equal("sso@example.com"))
					Expect(user["hierarchyLevel"]).To(Equal("level-3"))
					Expect(user).NotTo(HaveKey("passwordHash"))

					Expect(resp["accessToken"]).NotTo(BeEmpty())
					Expect(resp["refreshToken"]).NotTo(BeEmpty())
					Expect(resp["expiresIn"]).To(BeNumerically(">", 0))
				})

				It("should include the user's team memberships in the response", func() {
					_, err := db.Exec(`INSERT INTO teams (id, name, team_lead_id) VALUES ('sso-team-1', 'SSO Team', NULL)`)
					Expect(err).NotTo(HaveOccurred())
					_, err = db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ('sso-team-1', 'sso-user-1')`)
					Expect(err).NotTo(HaveOccurred())

					fakeProvider := startFakeTokenServer(makeTestIDToken("sso@example.com"), http.StatusOK)
					defer fakeProvider.Close()
					os.Setenv("OAUTH_TOKEN_URL", fakeProvider.URL)

					w := postSSOCallback("valid-code", "valid-verifier")

					Expect(w.Code).To(Equal(http.StatusOK))
					var resp map[string]interface{}
					Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())
					user := resp["user"].(map[string]interface{})
					Expect(user["teamIds"].([]interface{})).To(ContainElement("sso-team-1"))
				})

				It("should produce a valid access token with correct claims", func() {
					fakeProvider := startFakeTokenServer(makeTestIDToken("sso@example.com"), http.StatusOK)
					defer fakeProvider.Close()
					os.Setenv("OAUTH_TOKEN_URL", fakeProvider.URL)

					w := postSSOCallback("valid-code", "valid-verifier")
					Expect(w.Code).To(Equal(http.StatusOK))

					var resp map[string]interface{}
					Expect(json.Unmarshal(w.Body.Bytes(), &resp)).To(Succeed())

					jwtSvc := services.NewJWTService()
					claims, err := jwtSvc.ValidateAccessToken(resp["accessToken"].(string))
					Expect(err).NotTo(HaveOccurred())
					Expect(claims.UserID).To(Equal("sso-user-1"))
					Expect(claims.Email).To(Equal("sso@example.com"))
					Expect(claims.HierarchyLevel).To(Equal("level-3"))
				})
			})
		})
	})
})
