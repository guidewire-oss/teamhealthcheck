// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("E2E: Authorization Security", func() {
	var (
		adminToken      string
		managerToken    string
		teamMemberToken string
	)

	BeforeEach(func() {
		// Get tokens for different user roles
		var err error

		// Admin user (level-1, has all permissions)
		adminToken, err = loginAndGetToken("admin", "admin")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as admin")
		Expect(adminToken).NotTo(BeEmpty(), "Admin token should not be empty")

		// Manager user (level-3)
		managerToken, err = loginAndGetToken("manager1", "demo")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as manager1")
		Expect(managerToken).NotTo(BeEmpty(), "Manager token should not be empty")

		// Team member user (level-5)
		teamMemberToken, err = loginAndGetToken("demo", "demo")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as demo user")
		Expect(teamMemberToken).NotTo(BeEmpty(), "Team member token should not be empty")
	})

	Describe("Issue #1: Admin Endpoint Authorization", func() {
		Context("when a non-admin user tries to access admin endpoints", func() {
			It("should return 403 Forbidden for team member accessing hierarchy levels", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/hierarchy-levels", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team member should not be able to access admin hierarchy-levels endpoint")
			})

			It("should return 403 Forbidden for manager accessing admin users list", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/users", managerToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Manager should not be able to access admin users endpoint")
			})

			It("should return 403 Forbidden for team member creating hierarchy level", func() {
				body := `{"id":"test-level","name":"Test Level","permissions":{}}`
				resp, err := makeAuthenticatedRequest("POST", "/api/v1/admin/hierarchy-levels", teamMemberToken, strings.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team member should not be able to create hierarchy levels")
			})

			It("should return 403 Forbidden for manager accessing admin teams list", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/teams", managerToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Manager should not be able to access admin teams endpoint")
			})

			It("should return 403 Forbidden for team member accessing dimensions settings", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/settings/dimensions", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team member should not be able to access admin settings")
			})
		})

		Context("when an admin user accesses admin endpoints", func() {
			It("should return 200 OK for admin accessing hierarchy levels", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/hierarchy-levels", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Admin should be able to access hierarchy-levels endpoint")
			})

			It("should return 200 OK for admin accessing users list", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/users", adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Admin should be able to access users endpoint")
			})
		})
	})

	Describe("Issue #2: Manager Dashboard Role-Based Access", func() {
		Context("when a team member tries to access manager dashboard", func() {
			It("should return 403 Forbidden for team member accessing manager teams health", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/managers/manager1/teams/health", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team member should not be able to access manager dashboard data")
			})

			It("should return 403 Forbidden for team member accessing manager trends", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/managers/manager1/dashboard/trends?period=2024+-+1st+Half", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team member should not be able to access manager trends")
			})
		})

		Context("when a manager tries to access another manager's data", func() {
			It("should return 403 Forbidden for manager accessing other manager's teams", func() {
				// manager1 should not be able to access manager2's data
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/managers/manager2/teams/health", managerToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Manager should not be able to access another manager's team data")
			})
		})

		Context("when a manager accesses their own dashboard", func() {
			It("should return 200 OK for manager accessing their own teams health", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/managers/manager1/teams/health", managerToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"Manager should be able to access their own team data")
			})
		})
	})

	Describe("Issue #3: Authentication Status Codes", func() {
		Context("when making unauthenticated requests to protected endpoints", func() {
			It("should return 401 Unauthorized for admin endpoint without token", func() {
				resp, err := makeRequest("GET", "/api/v1/admin/hierarchy-levels", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Unauthenticated request should return 401, not 400 or 200")
			})

			It("should return 401 Unauthorized for manager endpoint without token", func() {
				resp, err := makeRequest("GET", "/api/v1/managers/manager1/teams/health", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Unauthenticated request should return 401")
			})

			It("should return 401 Unauthorized for team sessions endpoint without token", func() {
				resp, err := makeRequest("GET", "/api/v1/teams/team-phoenix/sessions", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Unauthenticated request should return 401")
			})

			It("should return 401 Unauthorized for health check submission without token", func() {
				body := `{"teamId":"team-alpha","responses":[]}`
				resp, err := makeRequest("POST", "/api/v1/health-checks", strings.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Unauthenticated health check submission should return 401")
			})
		})

		Context("when making requests with invalid token", func() {
			It("should return 401 Unauthorized for request with invalid token", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/hierarchy-levels", "invalid-token", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Invalid token should return 401")
			})

			It("should return 401 Unauthorized for request with expired token", func() {
				// Note: We can't easily create an expired token in the test,
				// but we can test with a malformed JWT
				expiredToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJleHAiOjE2MDAwMDAwMDB9.invalid"
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/admin/hierarchy-levels", expiredToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Expired/invalid token should return 401")
			})
		})
	})

	Describe("Issue #5: Team Membership Validation", func() {
		Context("when a user tries to access sessions of a team they don't belong to", func() {
			It("should return 403 Forbidden for user accessing other team's sessions", func() {
				// demo user is in team-alpha, should not access team-dragon
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/teams/team-dragon/sessions", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"User should not be able to access sessions of teams they don't belong to")
			})

			It("should return 403 Forbidden for user accessing other team's health data", func() {
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/teams/team-dragon/info", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"User should not be able to access health data of teams they don't belong to")
			})
		})

		Context("when a user accesses their own team's data", func() {
			It("should return 200 OK for user accessing their own team's sessions", func() {
				// demo user is in team-alpha
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/teams/team-phoenix/sessions", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				// Should be 200 or 404 (if no sessions), but NOT 403
				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusOK),
					Equal(http.StatusNotFound),
				), "User should be able to access their own team's sessions")
			})
		})

		Context("when a manager accesses teams in their supervisor chain", func() {
			It("should return 200 OK for manager accessing supervised team's sessions", func() {
				// manager1 supervises team-alpha through the supervisor chain
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/teams/team-phoenix/sessions", managerToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusOK),
					Equal(http.StatusNotFound),
				), "Manager should be able to access supervised team's sessions")
			})
		})
	})

	Describe("Fix 3: Team Dashboard and User Routes Protection", func() {
		Context("when making unauthenticated requests to previously unprotected routes", func() {
			It("should return 401 for team dashboard health-summary without token", func() {
				resp, err := makeRequest("GET", "/api/v1/teams/e2e_team1/dashboard/health-summary", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Team dashboard health-summary should require authentication")
			})

			It("should return 401 for team dashboard response-distribution without token", func() {
				resp, err := makeRequest("GET", "/api/v1/teams/e2e_team1/dashboard/response-distribution", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Team dashboard response-distribution should require authentication")
			})

			It("should return 401 for team dashboard individual-responses without token", func() {
				resp, err := makeRequest("GET", "/api/v1/teams/e2e_team1/dashboard/individual-responses", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Team dashboard individual-responses should require authentication")
			})

			It("should return 401 for team dashboard trends without token", func() {
				resp, err := makeRequest("GET", "/api/v1/teams/e2e_team1/dashboard/trends", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"Team dashboard trends should require authentication")
			})

			It("should return 401 for user survey-history without token", func() {
				resp, err := makeRequest("GET", "/api/v1/users/e2e_demo/survey-history", nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized),
					"User survey-history should require authentication")
			})
		})

		Context("when authenticated user accesses team dashboard for their team", func() {
			It("should allow team member to access their team's dashboard", func() {
				// demo user (seed data) is in team-phoenix
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/teams/team-phoenix/dashboard/health-summary", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(SatisfyAny(
					Equal(http.StatusOK),
					Equal(http.StatusNotFound),
				), "Team member should be able to access their team's dashboard")
			})
		})

		Context("when authenticated user accesses another team's dashboard", func() {
			It("should return 403 for user accessing non-member team dashboard", func() {
				// demo user is NOT in team-dragon
				resp, err := makeAuthenticatedRequest("GET", "/api/v1/teams/team-dragon/dashboard/health-summary", teamMemberToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"User should not be able to access dashboard of teams they don't belong to")
			})
		})
	})

	Describe("Issue #6: Team Lead Team Modification Restriction", func() {
		var teamLeadToken string

		BeforeEach(func() {
			var err error
			teamLeadToken, err = loginAndGetToken("teamlead1", "demo")
			Expect(err).NotTo(HaveOccurred(), "Failed to login as teamlead1")
		})

		Context("when a team lead tries to modify a team they don't lead", func() {
			It("should return 403 Forbidden for team lead updating other team", func() {
				// teamlead1 leads team-alpha, should not modify team-dragon
				body := `{"name":"Hacked Team Name"}`
				resp, err := makeAuthenticatedRequest("PUT", "/api/v1/admin/teams/team-dragon", teamLeadToken, strings.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team lead should not be able to modify teams they don't lead")
			})

			It("should return 403 Forbidden for team lead deleting teams (admin only)", func() {
				resp, err := makeAuthenticatedRequest("DELETE", "/api/v1/admin/teams/team-dragon", teamLeadToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
					"Team lead should not be able to add members to teams they don't lead")
			})
		})
	})
})

// Helper function to login and get JWT token
func loginAndGetToken(username, password string) (string, error) {
	body := fmt.Sprintf(`{"username":"%s","password":"%s"}`, username, password)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(
		backendURL+"/api/v1/auth/login",
		"application/json",
		strings.NewReader(body),
	)
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		AccessToken string `json:"accessToken"` // camelCase as per API response
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse login response: %w", err)
	}

	return result.AccessToken, nil
}

// Helper function to make authenticated HTTP request
func makeAuthenticatedRequest(method, path, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, backendURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}

// Helper function to make unauthenticated HTTP request
func makeRequest(method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, backendURL+path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	return client.Do(req)
}
