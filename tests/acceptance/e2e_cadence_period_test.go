// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Cadence-Driven Assessment Periods", Label("e2e"), func() {
	// Test that the assessment period stored in the database matches the team's cadence format.
	// Flow: create team with specific cadence → assign user → user takes survey → verify DB period format.

	var (
		testUserID = "e2e_demo"
	)

	Describe("Survey period computed from team cadence", func() {
		var (
			cadenceTeamID string
		)

		AfterEach(func() {
			// Clean up: remove test sessions, team membership, supervisors, and team
			if cadenceTeamID != "" {
				db.Exec("DELETE FROM health_check_responses WHERE session_id IN (SELECT id FROM health_check_sessions WHERE team_id = $1)", cadenceTeamID)
				db.Exec("DELETE FROM health_check_sessions WHERE team_id = $1", cadenceTeamID)
				db.Exec("DELETE FROM team_members WHERE team_id = $1", cadenceTeamID)
				db.Exec("DELETE FROM team_supervisors WHERE team_id = $1", cadenceTeamID)
				db.Exec("DELETE FROM teams WHERE id = $1", cadenceTeamID)
			}
			// Restore user's original team membership
			db.Exec("INSERT INTO team_members (team_id, user_id) VALUES ('e2e_team1', $1) ON CONFLICT (team_id, user_id) DO NOTHING", testUserID)
		})

		Context("when team has quarterly cadence", func() {
			It("should save assessment period in quarterly format (YYYY Q#)", func() {
				// Capture time before submission to avoid boundary race conditions
				testStartTime := time.Now()
				cadenceTeamID = fmt.Sprintf("cadence-team-%d", testStartTime.UnixNano())

				By("Creating a team with quarterly cadence")
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, 'Cadence Test Team', 'e2e_lead1', 'quarterly')
				`, cadenceTeamID)
				Expect(err).NotTo(HaveOccurred())

				// Add supervisor chain so the team is properly structured
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ($1, 'e2e_lead1', 'level-4', 1)
					ON CONFLICT DO NOTHING
				`, cadenceTeamID)
				Expect(err).NotTo(HaveOccurred())

				By("Assigning test user to the cadence team exclusively")
				// Remove from all other teams first so the survey picks up this team
				_, err = db.Exec("DELETE FROM team_members WHERE user_id = $1", testUserID)
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)", cadenceTeamID, testUserID)
				Expect(err).NotTo(HaveOccurred())

				By("Logging in and navigating to survey")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				// Click Take Survey
				surveyBtn := page.Locator("[data-testid='take-survey-btn']")
				err = surveyBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = surveyBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
					State: playwright.LoadStateNetworkidle,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying survey header shows quarterly period and cadence label")
				// Wait for the team info to load (shows "Period: 2026 Q1" and "Quarterly Check")
				Eventually(func() string {
					text, _ := page.Locator("text=Quarterly Check").TextContent()
					return text
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("Quarterly Check"))

				By("Filling all 11 dimensions and submitting")
				// Note: e2e_demo is a Team Member (level-5) — trend input is hidden for individual surveys.
				// Only score selection is needed.
				fillDimension := func(dimensionID string, score int) {
					scoreSelector := fmt.Sprintf("[data-dimension='%s'][data-score='%d']", dimensionID, score)
					err = page.Locator(scoreSelector).WaitFor(playwright.LocatorWaitForOptions{
						State:   playwright.WaitForSelectorStateVisible,
						Timeout: playwright.Float(5000),
					})
					Expect(err).NotTo(HaveOccurred())
					err = page.Locator(scoreSelector).Click()
					Expect(err).NotTo(HaveOccurred())
					time.Sleep(500 * time.Millisecond)
				}

				clickNext := func() {
					nextButton := page.Locator("button:has-text('Next')")
					err = nextButton.WaitFor(playwright.LocatorWaitForOptions{
						State:   playwright.WaitForSelectorStateVisible,
						Timeout: playwright.Float(3000),
					})
					Expect(err).NotTo(HaveOccurred())
					err = nextButton.Click()
					Expect(err).NotTo(HaveOccurred())
					time.Sleep(500 * time.Millisecond)
				}

				// Wait for first dimension to load
				err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Fill all 11 dimensions: score Green (3)
				dimensions := []string{"mission", "value", "speed", "fun", "health", "learning", "support", "pawns", "release", "process", "teamwork"}
				for i, dim := range dimensions {
					fillDimension(dim, 3)
					if i < len(dimensions)-1 {
						clickNext()
					}
				}

				// Submit on last dimension
				submitButton := page.Locator("button[type='submit']:has-text('Submit')")
				err = submitButton.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(3000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = submitButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for redirect to home page after submission")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying assessment period in database is in quarterly format")
				var assessmentPeriod string
				err = db.QueryRow(`
					SELECT assessment_period
					FROM health_check_sessions
					WHERE team_id = $1 AND user_id = $2
					ORDER BY created_at DESC
					LIMIT 1
				`, cadenceTeamID, testUserID).Scan(&assessmentPeriod)
				Expect(err).NotTo(HaveOccurred())

				// Should match "YYYY Q#" format (e.g., "2026 Q1")
				// Use testStartTime captured before submission to avoid boundary races
				expectedQuarter := (testStartTime.Month()-1)/3 + 1
				expectedPeriod := fmt.Sprintf("%d Q%d", testStartTime.Year(), expectedQuarter)
				Expect(assessmentPeriod).To(Equal(expectedPeriod),
					"Assessment period should be in quarterly format matching current quarter")

				GinkgoWriter.Printf("✅ Cadence-driven period test PASSED\n")
				GinkgoWriter.Printf("   Team cadence: quarterly\n")
				GinkgoWriter.Printf("   Assessment period saved: %s\n", assessmentPeriod)
				GinkgoWriter.Printf("   Expected: %s\n", expectedPeriod)
			})
		})

		Context("when team has monthly cadence", func() {
			It("should save assessment period in monthly format (YYYY Mon)", func() {
				// Capture time before submission to avoid boundary race conditions
				testStartTime := time.Now()
				cadenceTeamID = fmt.Sprintf("cadence-monthly-%d", testStartTime.UnixNano())

				By("Creating a team with monthly cadence and assigning user")
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, 'Monthly Cadence Team', 'e2e_lead1', 'monthly')
				`, cadenceTeamID)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ($1, 'e2e_lead1', 'level-4', 1)
					ON CONFLICT DO NOTHING
				`, cadenceTeamID)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("DELETE FROM team_members WHERE user_id = $1", testUserID)
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)", cadenceTeamID, testUserID)
				Expect(err).NotTo(HaveOccurred())

				By("Logging in and taking survey")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				surveyBtn := page.Locator("[data-testid='take-survey-btn']")
				err = surveyBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = surveyBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
					State: playwright.LoadStateNetworkidle,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying survey header shows monthly cadence label")
				Eventually(func() string {
					text, _ := page.Locator("text=Monthly Check").TextContent()
					return text
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("Monthly Check"))

				By("Filling all 11 dimensions and submitting")
				// Note: e2e_demo is a Team Member (level-5) — trend input is hidden for individual surveys.
				// Only score selection is needed.
				err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				dimensions := []string{"mission", "value", "speed", "fun", "health", "learning", "support", "pawns", "release", "process", "teamwork"}
				for i, dim := range dimensions {
					scoreSelector := fmt.Sprintf("[data-dimension='%s'][data-score='2']", dim)
					err = page.Locator(scoreSelector).WaitFor(playwright.LocatorWaitForOptions{
						State:   playwright.WaitForSelectorStateVisible,
						Timeout: playwright.Float(5000),
					})
					Expect(err).NotTo(HaveOccurred())
					err = page.Locator(scoreSelector).Click()
					Expect(err).NotTo(HaveOccurred())
					time.Sleep(500 * time.Millisecond)

					if i < len(dimensions)-1 {
						nextButton := page.Locator("button:has-text('Next')")
						err = nextButton.Click()
						Expect(err).NotTo(HaveOccurred())
						time.Sleep(500 * time.Millisecond)
					}
				}

				submitButton := page.Locator("button[type='submit']:has-text('Submit')")
				err = submitButton.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(3000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = submitButton.Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying assessment period in database is in monthly format")
				var assessmentPeriod string
				err = db.QueryRow(`
					SELECT assessment_period
					FROM health_check_sessions
					WHERE team_id = $1 AND user_id = $2
					ORDER BY created_at DESC
					LIMIT 1
				`, cadenceTeamID, testUserID).Scan(&assessmentPeriod)
				Expect(err).NotTo(HaveOccurred())

				// Should match "YYYY Mon" format (e.g., "2026 Mar")
				// Use testStartTime captured before submission to avoid boundary races
				monthNames := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
				expectedPeriod := fmt.Sprintf("%d %s", testStartTime.Year(), monthNames[testStartTime.Month()-1])
				Expect(assessmentPeriod).To(Equal(expectedPeriod),
					"Assessment period should be in monthly format matching current month")

				GinkgoWriter.Printf("✅ Monthly cadence period test PASSED\n")
				GinkgoWriter.Printf("   Team cadence: monthly\n")
				GinkgoWriter.Printf("   Assessment period saved: %s\n", assessmentPeriod)
				GinkgoWriter.Printf("   Expected: %s\n", expectedPeriod)
			})
		})
	})
})
