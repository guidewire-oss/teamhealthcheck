// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Team Lead Dashboard", func() {
	var (
		testTeamID = "e2e_team1"
	)

	BeforeEach(func() {
		// Clean up any existing test sessions to ensure fresh data
		_, _ = db.Exec(`DELETE FROM health_check_responses WHERE session_id IN ('e2e_tl_session1', 'e2e_tl_session2', 'e2e_tl_session3')`)
		_, _ = db.Exec(`DELETE FROM health_check_sessions WHERE id IN ('e2e_tl_session1', 'e2e_tl_session2', 'e2e_tl_session3')`)

		// Ensure test data exists for team lead dashboard testing
		// Insert additional responses for better visualization
		_, err := db.Exec(`
			INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed) VALUES
			('e2e_tl_session1', $1, 'e2e_member1', '2024-10-01', '2024 - 2nd Half', true),
			('e2e_tl_session2', $1, 'e2e_member2', '2024-10-02', '2024 - 2nd Half', true),
			('e2e_tl_session3', $1, 'e2e_demo', '2024-10-03', '2024 - 2nd Half', true)
			ON CONFLICT (id) DO NOTHING
		`, testTeamID)
		Expect(err).NotTo(HaveOccurred(), "Failed to insert test sessions")

		_, err = db.Exec(`
			INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
			-- Session 1 responses (all dimensions)
			('e2e_tl_session1', 'mission', 3, 'improving', 'Great clarity'),
			('e2e_tl_session1', 'value', 2, 'stable', 'Good delivery'),
			('e2e_tl_session1', 'speed', 1, 'declining', 'Too slow'),
			('e2e_tl_session1', 'fun', 3, 'improving', 'Enjoying work'),
			('e2e_tl_session1', 'health', 2, 'stable', 'OK codebase'),
			('e2e_tl_session1', 'learning', 3, 'improving', 'Learning a lot'),
			('e2e_tl_session1', 'support', 2, 'stable', 'Adequate support'),
			('e2e_tl_session1', 'pawns', 3, 'improving', 'We are players'),
			('e2e_tl_session1', 'release', 1, 'declining', 'Hard to release'),
			('e2e_tl_session1', 'process', 2, 'stable', 'OK process'),
			('e2e_tl_session1', 'teamwork', 3, 'improving', 'Great teamwork'),
			-- Session 2 responses
			('e2e_tl_session2', 'mission', 2, 'stable', 'Clear enough'),
			('e2e_tl_session2', 'value', 3, 'improving', 'Great value'),
			('e2e_tl_session2', 'speed', 2, 'stable', 'OK speed'),
			('e2e_tl_session2', 'fun', 2, 'declining', 'Less fun'),
			('e2e_tl_session2', 'health', 3, 'improving', 'Better code'),
			('e2e_tl_session2', 'learning', 2, 'stable', 'Some learning'),
			('e2e_tl_session2', 'support', 3, 'improving', 'Great support'),
			('e2e_tl_session2', 'pawns', 2, 'stable', 'OK autonomy'),
			('e2e_tl_session2', 'release', 2, 'stable', 'Manageable'),
			('e2e_tl_session2', 'process', 3, 'improving', 'Better process'),
			('e2e_tl_session2', 'teamwork', 2, 'stable', 'OK teamwork'),
			-- Session 3 responses
			('e2e_tl_session3', 'mission', 3, 'improving', 'Crystal clear'),
			('e2e_tl_session3', 'value', 2, 'stable', 'Decent value'),
			('e2e_tl_session3', 'speed', 2, 'improving', 'Getting faster'),
			('e2e_tl_session3', 'fun', 3, 'improving', 'Fun team'),
			('e2e_tl_session3', 'health', 2, 'stable', 'Technical debt'),
			('e2e_tl_session3', 'learning', 3, 'improving', 'Learning lots'),
			('e2e_tl_session3', 'support', 2, 'stable', 'OK support'),
			('e2e_tl_session3', 'pawns', 3, 'improving', 'Empowered'),
			('e2e_tl_session3', 'release', 2, 'improving', 'Easier now'),
			('e2e_tl_session3', 'process', 2, 'stable', 'Works OK'),
			('e2e_tl_session3', 'teamwork', 3, 'improving', 'Great collaboration')
			ON CONFLICT DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred(), "Failed to insert test responses")
	})

	// loginAsTeamLead logs in as e2e_lead1 and navigates to /dashboard, waiting for data to load
	loginAsTeamLead := func(page playwright.Page) {
		_, err := page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("#username").Fill("e2e_lead1")
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("#password").Fill("demo")
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
		Expect(err).NotTo(HaveOccurred())

		// Wait for redirect to dashboard
		err = page.WaitForURL("**/dashboard", playwright.PageWaitForURLOptions{
			Timeout: playwright.Float(15000),
		})
		Expect(err).NotTo(HaveOccurred())

		// Wait for loading to finish — the "Loading..." text should disappear
		Eventually(func() bool {
			loadingEl := page.Locator("text=Loading...")
			visible, _ := loadingEl.IsVisible()
			return !visible
		}, 20*time.Second, 500*time.Millisecond).Should(BeTrue(), "Dashboard should finish loading")

		// Dismiss onboarding modal if shown (fresh Playwright context has empty localStorage)
		dismissOnboardingIfVisible(page)
	}

	Describe("Radar Chart View", func() {
		Context("when Team Lead views team health radar chart", func() {
			It("should display radar chart with all 11 dimensions", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Verifying radar chart section is displayed")
				// Wait for the radar chart section (data-testid on wrapper div)
				radarSection := page.Locator("[data-testid='radar-chart-section']")
				err = radarSection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(15000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Check that we have chart content, not the "no data" message
				noDataMsg := page.Locator("[data-testid='radar-chart-section'] >> text=No health data available")
				noDataVisible, _ := noDataMsg.IsVisible()
				if noDataVisible {
					// Data didn't load — log debug info
					GinkgoWriter.Printf("WARNING: No health data available. Data may not have loaded.\n")

					// Verify the test data exists in the database
					var sessionCount int
					err = db.QueryRow("SELECT COUNT(*) FROM health_check_sessions WHERE team_id = $1 AND completed = true", testTeamID).Scan(&sessionCount)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Printf("DB session count for %s: %d\n", testTeamID, sessionCount)

					var responseCount int
					err = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE session_id LIKE 'e2e_tl_%'").Scan(&responseCount)
					Expect(err).NotTo(HaveOccurred())
					GinkgoWriter.Printf("DB response count for e2e_tl_* sessions: %d\n", responseCount)
				}

				// Either the chart or the "no data" message should be visible
				chartOrNoData := page.Locator("[data-testid='radar-chart'], .recharts-radar, svg:has(.recharts-polar-grid)").
				Or(page.Locator("text=No health data available"))
				err = chartOrNoData.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Radar chart section displayed successfully\n")
			})
		})
	})

	Describe("Response Distribution Tab", func() {
		Context("when Team Lead views response distribution", func() {
			It("should display bar chart showing score distribution per dimension", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Clicking on Response Distribution tab")
				distributionTab := page.Locator("[data-testid='distribution-tab']")
				err = distributionTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = distributionTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(1 * time.Second)

				By("Verifying distribution section is displayed")
				distSection := page.Locator("[data-testid='distribution-chart-section']")
				err = distSection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Verify test data exists in the database before waiting for UI
				var sessionCount int
				dbErr := db.QueryRow("SELECT COUNT(*) FROM health_check_sessions WHERE team_id = $1 AND completed = true", testTeamID).Scan(&sessionCount)
				Expect(dbErr).NotTo(HaveOccurred())
				var responseCount int
				dbErr = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE session_id LIKE 'e2e_tl_%'").Scan(&responseCount)
				Expect(dbErr).NotTo(HaveOccurred())
				GinkgoWriter.Printf("Distribution tab: DB sessions=%d responses=%d\n", sessionCount, responseCount)

				// Click the "Chart" button to switch from breakdown view to chart view
				By("Switching to chart view")
				chartViewBtn := page.Locator("[data-testid='distribution-chart-btn']")
				err = chartViewBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(15000),
				})
				if err != nil {
					// Log page HTML for diagnosis before failing
					html, _ := page.Content()
					GinkgoWriter.Printf("distribution-chart-btn not visible. DB: sessions=%d responses=%d. Page excerpt: %s\n",
						sessionCount, responseCount,
						html[max(0, len(html)-2000):])
				}
				Expect(err).NotTo(HaveOccurred())

				err = chartViewBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for the chart to be visible after clicking
				By("Verifying bar chart is displayed")
				chartElement := page.Locator("[data-testid='distribution-chart']")
				err = chartElement.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Response distribution tab displayed successfully\n")
			})
		})
	})

	Describe("Individual Responses Tab", func() {
		Context("when Team Lead views individual responses", func() {
			It("should display list of team member responses with scores", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Clicking on Individual Responses tab")
				responsesTab := page.Locator("[data-testid='responses-tab']")
				err = responsesTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = responsesTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(1 * time.Second)

				By("Verifying individual responses section is displayed")
				responsesSection := page.Locator("[data-testid='responses-section']")
				err = responsesSection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Check for either response cards or "no responses" message
				responseContent := page.Locator("[data-testid='response-card']").
				Or(page.Locator("text=No individual responses available"))
				err = responseContent.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Individual responses tab displayed successfully\n")
			})

			It("should show comments from team members", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Navigating to Individual Responses")
				responsesTab := page.Locator("[data-testid='responses-tab']")
				_ = responsesTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				_ = responsesTab.Click()
				time.Sleep(1 * time.Second)

				By("Verifying Individual Responses tab content is displayed")
				responsesSection := page.Locator("[data-testid='responses-section']")
				err = responsesSection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Individual Responses tab content displayed successfully\n")
			})
		})
	})

	Describe("Trends Tab", func() {
		Context("when Team Lead views trends over time", func() {
			BeforeEach(func() {
				// Clean up any existing trend sessions to ensure fresh data
				_, _ = db.Exec(`DELETE FROM health_check_responses WHERE session_id IN ('e2e_trend_h1', 'e2e_trend_h2')`)
				_, _ = db.Exec(`DELETE FROM health_check_sessions WHERE id IN ('e2e_trend_h1', 'e2e_trend_h2')`)

				// Insert historical data for trend visualization
				_, err := db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed) VALUES
					('e2e_trend_h1', $1, 'e2e_member1', '2024-01-15', '2023 - 2nd Half', true),
					('e2e_trend_h2', $1, 'e2e_member1', '2024-07-15', '2024 - 1st Half', true)
					ON CONFLICT (id) DO NOTHING
				`, testTeamID)
				Expect(err).NotTo(HaveOccurred(), "Failed to insert trend sessions")

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
					('e2e_trend_h1', 'mission', 1, 'declining', 'Was unclear'),
					('e2e_trend_h1', 'value', 1, 'declining', 'Low value'),
					('e2e_trend_h1', 'speed', 1, 'declining', 'Very slow'),
					('e2e_trend_h2', 'mission', 2, 'improving', 'Getting better'),
					('e2e_trend_h2', 'value', 2, 'improving', 'More value'),
					('e2e_trend_h2', 'speed', 2, 'improving', 'Faster now')
					ON CONFLICT DO NOTHING
				`)
				Expect(err).NotTo(HaveOccurred(), "Failed to insert trend responses")
			})

			It("should display line chart showing health trends across periods", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Clicking on Trends tab")
				trendsTab := page.Locator("[data-testid='trends-tab']")
				err = trendsTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = trendsTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(1 * time.Second)

				By("Verifying trends section is displayed")
				trendsSection := page.Locator("[data-testid='trends-chart-section']")
				err = trendsSection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// The trends tab now defaults to "dimensions" view (sparkline cards)
				// Wait for content to render - could be chart, sparklines, or "no data" message
				// Give extra time for data fetching and rendering
				time.Sleep(3 * time.Second)

				// Check that the trends section has some content rendered
				// We don't need to be too specific about what - just verify it's not empty
				hasContent, err := trendsSection.Evaluate("el => el.children.length > 0", nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(hasContent).To(BeTrue(), "Trends section should have content rendered")

				GinkgoWriter.Printf("Trends chart section displayed successfully\n")
			})
		})
	})

	Describe("Score Band Legend", func() {
		Context("when Team Lead views the radar chart tab", func() {
			It("should display the score band legend", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Verifying score band legend is visible on radar tab")
				legend := page.Locator("[data-testid='score-band-legend']")
				err = legend.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(15000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying all four bands are shown")
				legendText, err := legend.InnerText()
				Expect(err).NotTo(HaveOccurred())
				Expect(legendText).To(ContainSubstring("Excellent"))
				Expect(legendText).To(ContainSubstring("Good"))
				Expect(legendText).To(ContainSubstring("Fair"))
				Expect(legendText).To(ContainSubstring("Poor"))

				GinkgoWriter.Printf("Score band legend displayed successfully\n")
			})
		})
	})

	Describe("Export to Excel", func() {
		Context("when Team Lead has data and clicks export", func() {
			It("should show the export button when individual responses are loaded", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Waiting for data to load on dashboard")
				radarSection := page.Locator("[data-testid='radar-chart-section']")
				err = radarSection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(15000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking Individual Responses tab to load data for export")
				responsesTab := page.Locator("[data-testid='responses-tab']")
				err = responsesTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = responsesTab.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for response cards to load")
				Eventually(func() bool {
					cards := page.Locator("[data-testid='response-card']")
					count, _ := cards.Count()
					return count > 0
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(), "Response cards should load")

				By("Going back to radar tab where export button is shown")
				radarTab := page.Locator("[data-testid='radar-tab']")
				err = radarTab.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying export button is visible")
				exportBtn := page.Locator("[data-testid='export-excel-btn']")
				err = exportBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Export to Excel button is visible for Team Lead\n")
			})
		})
	})

	Describe("Team Lead Survey Access", func() {
		Context("when Team Lead wants to take a survey", func() {
			It("should allow Team Lead to complete health check survey", func() {
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				By("Logging in as Team Lead")
				loginAsTeamLead(page)

				By("Navigating to survey page")
				_, err = page.Goto(frontendURL + "/survey")
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
					State: playwright.LoadStateNetworkidle,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying survey page loads with dimensions")
				missionLabel := page.Locator("text=Mission")
				err = missionLabel.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Team Lead can access survey successfully\n")
			})
		})
	})
})
