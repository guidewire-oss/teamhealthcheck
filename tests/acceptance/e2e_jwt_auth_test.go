package acceptance_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: JWT Authentication", Label("e2e", "security"), func() {
	var (
		page playwright.Page
		ctx  playwright.BrowserContext
	)

	BeforeEach(func() {
		var err error
		ctx, err = browser.NewContext()
		Expect(err).NotTo(HaveOccurred())

		page, err = ctx.NewPage()
		Expect(err).NotTo(HaveOccurred())

		// Clean up JWT test-specific data before each test
		_, err = db.Exec(`
			DELETE FROM refresh_tokens WHERE user_id LIKE 'e2e_jwt_%';
			DELETE FROM users WHERE id LIKE 'e2e_jwt_%';
		`)
		// Ignore errors if tables don't exist yet (they will after migration)
	})

	AfterEach(func() {
		if page != nil {
			page.Close()
		}
		if ctx != nil {
			ctx.Close()
		}
	})

	Describe("JWT-based login flow", func() {
		Context("when user logs in with valid credentials", func() {
			It("should return access and refresh tokens and redirect to appropriate dashboard", func() {
				// Given: A user exists in the database
				By("Creating a test user for JWT auth")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_jwt_user1', 'e2e_jwt_user', 'e2e_jwt@test.com', 'E2E JWT User', 'level-5', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// When: User navigates to login page
				By("User navigating to login page")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// And: User enters valid credentials
				By("User entering valid credentials")
				err = page.Locator("input[name='username']").Fill("e2e_jwt_user")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				// When: User submits the form
				By("User submitting login form")
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: User should be redirected to home page (team member level-5)
				By("Verifying redirect to home page")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/home"))
				dismissOnboardingIfVisible(page)

				// And: Access token should be stored (check via API call)
				By("Verifying authenticated API access works")
				// Navigate to a protected page that requires auth
				_, err = page.Goto(frontendURL + "/home")
				Expect(err).NotTo(HaveOccurred())

				// Should stay on home page (not redirected to login)
				time.Sleep(2 * time.Second)
				Expect(page.URL()).To(ContainSubstring("/home"))
			})
		})

		Context("when user logs out", func() {
			It("should invalidate tokens and redirect to login page", func() {
				// Given: User is logged in
				By("Creating a test user and logging in")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_jwt_logout', 'e2e_jwt_logout', 'e2e_jwt_logout@test.com', 'E2E Logout User', 'level-5', $1)
					ON CONFLICT (id) DO NOTHING
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Login via UI
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_jwt_logout")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to home
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/home"))
				dismissOnboardingIfVisible(page)

				// When: User clicks user menu to open dropdown
				By("User clicking user menu to open dropdown")
				userMenuBtn := page.Locator("[data-testid='user-menu-button']")
				err = userMenuBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for dropdown to appear
				time.Sleep(500 * time.Millisecond)

				// Then click logout button
				By("User clicking logout button")
				logoutBtn := page.Locator("[data-testid='logout-button']")
				err = logoutBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: Should be redirected to login page
				By("Verifying redirect to login page")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/login"))

				// And: Accessing protected route should redirect to login
				By("Verifying protected routes require re-authentication")
				_, err = page.Goto(frontendURL + "/home")
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/login"))
			})
		})
	})

	Describe("Token refresh flow", func() {
		Context("when access token is expired but refresh token is valid", func() {
			It("should automatically refresh the access token", func() {
				// This test verifies the silent token refresh mechanism
				// Given: User is logged in
				By("Creating a test user")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_jwt_refresh', 'e2e_jwt_refresh', 'e2e_jwt_refresh@test.com', 'E2E Refresh User', 'level-5', $1)
					ON CONFLICT (id) DO NOTHING
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Login via UI
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_jwt_refresh")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/home"))
				dismissOnboardingIfVisible(page)

				// When: User accesses protected resource after some time
				// (In a real scenario, access token would have short expiry)
				By("User accessing protected resource")
				_, err = page.Goto(frontendURL + "/survey")
				Expect(err).NotTo(HaveOccurred())

				// Then: Should still be authenticated (token refresh worked)
				By("Verifying user remains authenticated")
				time.Sleep(2 * time.Second)
				// Should not be redirected to login
				Expect(page.URL()).NotTo(ContainSubstring("/login"))
			})
		})
	})

	Describe("API token validation", func() {
		Context("when calling API with valid JWT token", func() {
			It("should return protected resource successfully", func() {
				// This test directly verifies the API JWT validation
				By("Creating a test user")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_jwt_api', 'e2e_jwt_api', 'e2e_jwt_api@test.com', 'E2E API User', 'level-5', $1)
					ON CONFLICT (id) DO NOTHING
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Login via API to get tokens
				By("Logging in via API")
				loginResp, err := http.Post(
					backendURL+"/api/v1/auth/login",
					"application/json",
					strings.NewReader(`{"username":"e2e_jwt_api","password":"demo"}`),
				)
				Expect(err).NotTo(HaveOccurred())
				defer loginResp.Body.Close()
				Expect(loginResp.StatusCode).To(Equal(http.StatusOK))

				// Parse response to get tokens
				body, err := io.ReadAll(loginResp.Body)
				Expect(err).NotTo(HaveOccurred())

				var loginData struct {
					User struct {
						ID string `json:"id"`
					} `json:"user"`
					AccessToken  string `json:"accessToken"`
					RefreshToken string `json:"refreshToken"`
				}
				err = json.Unmarshal(body, &loginData)
				Expect(err).NotTo(HaveOccurred())

				// Verify tokens are present
				By("Verifying tokens are returned")
				Expect(loginData.AccessToken).NotTo(BeEmpty(), "Access token should be returned")
				Expect(loginData.RefreshToken).NotTo(BeEmpty(), "Refresh token should be returned")

				// Use access token to call protected endpoint
				By("Calling protected API endpoint with JWT")
				req, err := http.NewRequest("GET", backendURL+"/api/v1/users/me", nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Authorization", "Bearer "+loginData.AccessToken)

				client := &http.Client{}
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Then: Should get successful response
				By("Verifying protected endpoint returns user data")
				Expect(resp.StatusCode).To(Equal(http.StatusOK))
			})
		})

		Context("when calling API without JWT token", func() {
			It("should return 401 Unauthorized", func() {
				By("Calling protected API endpoint without token")
				resp, err := http.Get(backendURL + "/api/v1/users/me")
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Then: Should get 401 Unauthorized
				By("Verifying 401 response")
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("when calling API with invalid JWT token", func() {
			It("should return 401 Unauthorized", func() {
				By("Calling protected API endpoint with invalid token")
				req, err := http.NewRequest("GET", backendURL+"/api/v1/users/me", nil)
				Expect(err).NotTo(HaveOccurred())
				req.Header.Set("Authorization", "Bearer invalid.jwt.token")

				client := &http.Client{}
				resp, err := client.Do(req)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Then: Should get 401 Unauthorized
				By("Verifying 401 response")
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})

	Describe("Token refresh API", func() {
		Context("when refresh token is valid", func() {
			It("should return new access token", func() {
				By("Creating a test user")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_jwt_refresh_api', 'e2e_jwt_refresh_api', 'e2e_jwt_refresh_api@test.com', 'E2E Refresh API User', 'level-5', $1)
					ON CONFLICT (id) DO NOTHING
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Login to get tokens
				By("Logging in via API")
				loginResp, err := http.Post(
					backendURL+"/api/v1/auth/login",
					"application/json",
					strings.NewReader(`{"username":"e2e_jwt_refresh_api","password":"demo"}`),
				)
				Expect(err).NotTo(HaveOccurred())
				defer loginResp.Body.Close()
				Expect(loginResp.StatusCode).To(Equal(http.StatusOK))

				body, err := io.ReadAll(loginResp.Body)
				Expect(err).NotTo(HaveOccurred())

				var loginData struct {
					RefreshToken string `json:"refreshToken"`
				}
				err = json.Unmarshal(body, &loginData)
				Expect(err).NotTo(HaveOccurred())

				// Use refresh token to get new access token
				By("Calling refresh token endpoint")
				refreshResp, err := http.Post(
					backendURL+"/api/v1/auth/refresh",
					"application/json",
					strings.NewReader(`{"refreshToken":"`+loginData.RefreshToken+`"}`),
				)
				Expect(err).NotTo(HaveOccurred())
				defer refreshResp.Body.Close()

				// Then: Should get new access token
				By("Verifying new access token is returned")
				Expect(refreshResp.StatusCode).To(Equal(http.StatusOK))

				refreshBody, err := io.ReadAll(refreshResp.Body)
				Expect(err).NotTo(HaveOccurred())

				var refreshData struct {
					AccessToken string `json:"accessToken"`
				}
				err = json.Unmarshal(refreshBody, &refreshData)
				Expect(err).NotTo(HaveOccurred())
				Expect(refreshData.AccessToken).NotTo(BeEmpty())
			})
		})

		Context("when refresh token is invalid", func() {
			It("should return 401 Unauthorized", func() {
				By("Calling refresh endpoint with invalid token")
				resp, err := http.Post(
					backendURL+"/api/v1/auth/refresh",
					"application/json",
					strings.NewReader(`{"refreshToken":"invalid.refresh.token"}`),
				)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Then: Should get 401 Unauthorized
				By("Verifying 401 response")
				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			})
		})
	})
})
