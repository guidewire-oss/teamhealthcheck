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

var _ = Describe("E2E: Team Member Management", func() {
	var adminToken string
	var testTeamID string

	BeforeEach(func() {
		var err error
		adminToken, err = loginAndGetToken("admin", "admin")
		Expect(err).NotTo(HaveOccurred(), "Failed to login as admin")
		Expect(adminToken).NotTo(BeEmpty())

		// Create a fresh test team for member management tests
		testTeamID = fmt.Sprintf("e2e_members_team_%d", time.Now().UnixNano())
		_, err = db.Exec(`
			INSERT INTO teams (id, name, team_lead_id) VALUES ($1, 'E2E Members Test Team', 'e2e_lead1')
			ON CONFLICT (id) DO NOTHING
		`, testTeamID)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test team")

		// Verify team was actually created
		var teamExists bool
		err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1)`, testTeamID).Scan(&teamExists)
		Expect(err).NotTo(HaveOccurred(), "Failed to verify team exists")
		Expect(teamExists).To(BeTrue(), "Team should exist after creation")
	})

	AfterEach(func() {
		// Clean up test team members and team
		if testTeamID != "" {
			db.Exec(`DELETE FROM team_members WHERE team_id = $1`, testTeamID)
			db.Exec(`DELETE FROM team_supervisors WHERE team_id = $1`, testTeamID)
			db.Exec(`DELETE FROM teams WHERE id = $1`, testTeamID)
		}
	})

	Describe("GET /api/v1/admin/teams/:id/members", func() {
		Context("when team has no members", func() {
			It("should return empty members list", func() {
				resp, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result struct {
					Members []struct {
						UserID   string `json:"userId"`
						UserName string `json:"userName"`
						Email    string `json:"email"`
					} `json:"members"`
					Total int `json:"total"`
				}
				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &result)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Members).To(BeEmpty())
				Expect(result.Total).To(Equal(0))
			})
		})

		Context("when team has members", func() {
			BeforeEach(func() {
				// Add e2e_member1 to the test team
				_, err := db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, 'e2e_member1') ON CONFLICT (team_id, user_id) DO NOTHING`, testTeamID)
				Expect(err).NotTo(HaveOccurred(), "Failed to add e2e_member1 to team")
			})

			It("should return member list with user details", func() {
				resp, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				var result struct {
					Members []struct {
						UserID   string `json:"userId"`
						UserName string `json:"userName"`
						Email    string `json:"email"`
					} `json:"members"`
					Total int `json:"total"`
				}
				body, err := io.ReadAll(resp.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &result)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Total).To(Equal(1))
				Expect(result.Members).To(HaveLen(1))
				Expect(result.Members[0].UserID).To(Equal("e2e_member1"))
			})
		})
	})

	Describe("POST /api/v1/admin/teams/:id/members", func() {
		Context("when adding a valid member", func() {
			It("should add the member and return 201", func() {
				body := `{"userId":"e2e_member2"}`
				resp, err := makeAuthenticatedRequest("POST", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, strings.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusCreated))

				By("Verifying member was added via GET")
				resp2, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp2.Body.Close()

				var result struct {
					Members []struct {
						UserID string `json:"userId"`
					} `json:"members"`
					Total int `json:"total"`
				}
				body2, err := io.ReadAll(resp2.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body2, &result)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Total).To(Equal(1))
				Expect(result.Members[0].UserID).To(Equal("e2e_member2"))
			})
		})

		Context("when adding without userId", func() {
			It("should return 400 Bad Request", func() {
				body := `{}`
				resp, err := makeAuthenticatedRequest("POST", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, strings.NewReader(body))
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Describe("DELETE /api/v1/admin/teams/:id/members/:userId", func() {
		BeforeEach(func() {
			_, err := db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, 'e2e_member3') ON CONFLICT (team_id, user_id) DO NOTHING`, testTeamID)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when removing an existing member", func() {
			It("should remove the member and return 200", func() {
				resp, err := makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/admin/teams/%s/members/e2e_member3", testTeamID), adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp.Body.Close()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				By("Verifying member was removed via GET")
				resp2, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, nil)
				Expect(err).NotTo(HaveOccurred())
				defer resp2.Body.Close()

				var result struct {
					Members []struct{} `json:"members"`
					Total   int        `json:"total"`
				}
				body, err := io.ReadAll(resp2.Body)
				Expect(err).NotTo(HaveOccurred())
				err = json.Unmarshal(body, &result)
				Expect(err).NotTo(HaveOccurred())

				Expect(result.Total).To(Equal(0))
			})
		})
	})

	Describe("Full member lifecycle", func() {
		It("should support add, list, and remove flow", func() {
			By("Adding two members")
			body1 := `{"userId":"e2e_member1"}`
			resp1, err := makeAuthenticatedRequest("POST", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, strings.NewReader(body1))
			Expect(err).NotTo(HaveOccurred())
			resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusCreated))

			body2 := `{"userId":"e2e_member2"}`
			resp2, err := makeAuthenticatedRequest("POST", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, strings.NewReader(body2))
			Expect(err).NotTo(HaveOccurred())
			resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusCreated))

			By("Listing members - should have 2")
			resp3, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp3.Body.Close()

			var result struct {
				Total int `json:"total"`
			}
			listBody, err := io.ReadAll(resp3.Body)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(listBody, &result)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Total).To(Equal(2))

			By("Removing one member")
			resp4, err := makeAuthenticatedRequest("DELETE", fmt.Sprintf("/api/v1/admin/teams/%s/members/e2e_member1", testTeamID), adminToken, nil)
			Expect(err).NotTo(HaveOccurred())
			resp4.Body.Close()
			Expect(resp4.StatusCode).To(Equal(http.StatusOK))

			By("Listing members - should have 1")
			resp5, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), adminToken, nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp5.Body.Close()

			var result2 struct {
				Members []struct {
					UserID string `json:"userId"`
				} `json:"members"`
				Total int `json:"total"`
			}
			listBody2, err := io.ReadAll(resp5.Body)
			Expect(err).NotTo(HaveOccurred())
			err = json.Unmarshal(listBody2, &result2)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.Total).To(Equal(1))
			Expect(result2.Members[0].UserID).To(Equal("e2e_member2"))
		})
	})

	Describe("Non-admin access restriction", func() {
		It("should deny team member access to member management endpoints", func() {
			memberToken, err := loginAndGetToken("demo", "demo")
			Expect(err).NotTo(HaveOccurred())

			resp, err := makeAuthenticatedRequest("GET", fmt.Sprintf("/api/v1/admin/teams/%s/members", testTeamID), memberToken, nil)
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
				"Non-admin users should not access team member management endpoints")
		})
	})
})
