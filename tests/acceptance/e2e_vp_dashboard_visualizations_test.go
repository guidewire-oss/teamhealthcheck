// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: VP Dashboard Visualizations", Label("e2e"), func() {
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

		// Clean up test data before each test
		_, err = db.Exec(`
			DELETE FROM health_check_responses WHERE session_id LIKE 'e2e_vp_%';
			DELETE FROM health_check_sessions WHERE id LIKE 'e2e_vp_%';
			DELETE FROM team_supervisors WHERE team_id LIKE 'e2e_vp_%';
			DELETE FROM team_members WHERE team_id LIKE 'e2e_vp_%';
			DELETE FROM teams WHERE id LIKE 'e2e_vp_%';
			DELETE FROM users WHERE id LIKE 'e2e_vp_%';
		`)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if page != nil {
			page.Close()
		}
		if ctx != nil {
			ctx.Close()
		}
	})

	Describe("VP views aggregated radar chart", func() {
		Context("when VP has multiple teams with health check data", func() {
			It("should display radar chart showing aggregated health scores across all teams", func() {
				// Given: VP with multiple teams having health check submissions
				By("Setting up VP with multiple teams and health check data")

				// Create VP user
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_vp_radar', 'e2e_vp_radar', 'e2e_vp_radar@test.com', 'E2E VP Radar Test', 'level-1', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Create teams under VP
				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES
						('e2e_vp_team1', 'E2E VP Alpha Squad', 'e2e_vp_radar'),
						('e2e_vp_team2', 'E2E VP Beta Squad', 'e2e_vp_radar')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add supervisor chains (VP supervises both teams)
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES
						('e2e_vp_team1', 'e2e_vp_radar', 'level-1', 1),
						('e2e_vp_team2', 'e2e_vp_radar', 'level-1', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create health check sessions
				currentDate := time.Now().Format("2006-01-02")
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('e2e_vp_sess1', 'e2e_vp_team1', 'e2e_vp_radar', $1, '2024 - 2nd Half', true),
						('e2e_vp_sess2', 'e2e_vp_team2', 'e2e_vp_radar', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				// Add responses with different scores per dimension
				// Team 1: mission=3, value=2, speed=3, fun=2 (avg 2.5)
				// Team 2: mission=2, value=3, speed=2, fun=3 (avg 2.5)
				// Aggregated avg per dimension: mission=2.5, value=2.5, speed=2.5, fun=2.5
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('e2e_vp_sess1', 'mission', 3, 'improving', 'Clear goals'),
						('e2e_vp_sess1', 'value', 2, 'stable', 'Good value'),
						('e2e_vp_sess1', 'speed', 3, 'improving', 'Fast delivery'),
						('e2e_vp_sess1', 'fun', 2, 'stable', 'Could be better'),
						('e2e_vp_sess2', 'mission', 2, 'stable', 'Some clarity needed'),
						('e2e_vp_sess2', 'value', 3, 'improving', 'Great value'),
						('e2e_vp_sess2', 'speed', 2, 'declining', 'Need to speed up'),
						('e2e_vp_sess2', 'fun', 3, 'improving', 'Fun team')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: VP logs in and navigates to manager dashboard
				By("VP logging in to the application")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_vp_radar")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for redirect to manager dashboard")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				// Wait for dashboard to load
				_, err = page.WaitForSelector("text=Team Health Overview", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Select the assessment period that matches our test data
				By("Selecting assessment period filter")
				periodFilter := page.Locator("[data-testid='period-filter']")
				_, err = periodFilter.SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"2024 - 2nd Half"},
				})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond) // Wait for data refresh

				By("VP clicking on Radar tab")
				// Click on the Radar tab to view aggregated radar chart
				radarTab := page.Locator("[data-testid='radar-tab']")
				err = radarTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for loading spinner to appear and disappear (indicates data fetch complete)
				By("Waiting for radar data to load")
				Eventually(func() bool {
					loadingSpinner := page.Locator(".animate-spin")
					visible, _ := loadingSpinner.IsVisible()
					return !visible // Wait for spinner to disappear
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Then: Radar chart should be visible with aggregated data
				By("Verifying radar chart is displayed")
				Eventually(func() bool {
					radarChart := page.Locator("[data-testid='vp-radar-chart']")
					visible, _ := radarChart.IsVisible()
					return visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Verify the radar chart title indicates aggregated view
				By("Verifying radar chart shows aggregated title")
				radarTitle := page.Locator("text=Aggregated Health Overview")
				visible, err := radarTitle.IsVisible()
				Expect(err).NotTo(HaveOccurred())
				Expect(visible).To(BeTrue())

				// Verify dimension labels are visible in the chart
				By("Verifying dimension labels are present")
				Eventually(func() bool {
					// Look for at least one dimension name in the radar chart area
					missionLabel := page.Locator("text=Mission")
					visible, _ := missionLabel.IsVisible()
					return visible
				}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
			})
		})
	})

	Describe("VP views health trends over time", func() {
		Context("when VP has teams with health data across multiple assessment periods", func() {
			It("should display trends chart showing health changes over time", func() {
				// Given: VP with teams that have health checks in multiple periods
				By("Setting up VP with historical health check data")

				// Create VP user
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_vp_trends', 'e2e_vp_trends', 'e2e_vp_trends@test.com', 'E2E VP Trends Test', 'level-1', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Create team under VP
				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('e2e_vp_trend_team', 'E2E VP Trends Squad', 'e2e_vp_trends')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add supervisor chain
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('e2e_vp_trend_team', 'e2e_vp_trends', 'level-1', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create health check sessions across different periods (showing improvement)
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('e2e_vp_trend_s1', 'e2e_vp_trend_team', 'e2e_vp_trends', '2023-07-15', '2023 - 1st Half', true),
						('e2e_vp_trend_s2', 'e2e_vp_trend_team', 'e2e_vp_trends', '2024-01-15', '2023 - 2nd Half', true),
						('e2e_vp_trend_s3', 'e2e_vp_trend_team', 'e2e_vp_trends', '2024-07-15', '2024 - 1st Half', true)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add responses showing improvement over time
				// Period 1 (2023 - 1st Half): mission=1, value=2 (avg 1.5)
				// Period 2 (2023 - 2nd Half): mission=2, value=2 (avg 2.0)
				// Period 3 (2024 - 1st Half): mission=3, value=3 (avg 3.0)
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('e2e_vp_trend_s1', 'mission', 1, 'declining', 'Unclear direction'),
						('e2e_vp_trend_s1', 'value', 2, 'stable', 'Some value'),
						('e2e_vp_trend_s2', 'mission', 2, 'improving', 'Getting better'),
						('e2e_vp_trend_s2', 'value', 2, 'stable', 'Consistent'),
						('e2e_vp_trend_s3', 'mission', 3, 'improving', 'Crystal clear'),
						('e2e_vp_trend_s3', 'value', 3, 'improving', 'Great value')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: VP logs in and navigates to trends view
				By("VP logging in to the application")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_vp_trends")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for redirect to manager dashboard")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				// Wait for dashboard to load
				_, err = page.WaitForSelector("text=Team Health Overview", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("VP clicking on Trends tab")
				// Click on the Trends tab to view health trends over time
				trendsTab := page.Locator("[data-testid='trends-tab']")
				err = trendsTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: Trends chart should be visible
				By("Verifying trends chart is displayed")
				Eventually(func() bool {
					trendsChart := page.Locator("[data-testid='vp-trends-chart']")
					visible, _ := trendsChart.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Verify the trends chart title
				By("Verifying trends chart shows appropriate title")
				trendsTitle := page.Locator("text=Health Trends Over Time")
				visible, err := trendsTitle.IsVisible()
				Expect(err).NotTo(HaveOccurred())
				Expect(visible).To(BeTrue())

				// Verify chart has rendered data lines (SVG path elements within the chart)
				By("Verifying trends chart has rendered data lines")
				Eventually(func() bool {
					// Recharts renders trend lines as SVG path elements with class 'recharts-line-curve'
					// Check that at least one line is rendered in the chart
					chartLines := page.Locator("[data-testid='vp-trends-chart'] .recharts-line-curve")
					count, _ := chartLines.Count()
					return count > 0
				}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
			})
		})

		Context("when VP has no historical data", func() {
			It("should display empty state message for trends", func() {
				// Given: VP with team but no health check sessions
				By("Setting up VP with team but no health checks")

				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_vp_empty_trend', 'e2e_vp_empty_trend', 'e2e_vp_empty_trend@test.com', 'E2E VP Empty Trends', 'level-1', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('e2e_vp_empty_team', 'E2E VP Empty Squad', 'e2e_vp_empty_trend')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('e2e_vp_empty_team', 'e2e_vp_empty_trend', 'level-1', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: VP logs in and navigates to trends view
				By("VP logging in")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_vp_empty_trend")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				_, err = page.WaitForSelector("text=Team Health Overview", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("VP clicking on Trends tab")
				trendsTab := page.Locator("[data-testid='trends-tab']")
				err = trendsTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: Should show empty state
				By("Verifying empty state message is displayed")
				Eventually(func() bool {
					emptyMessage := page.Locator("text=No trend data available")
					visible, _ := emptyMessage.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
			})
		})
	})

	Describe("VP navigates between visualization tabs", func() {
		Context("when VP has full dashboard access", func() {
			It("should allow seamless navigation between Teams, Radar, and Trends tabs", func() {
				// Given: VP with team data
				By("Setting up VP with team and health data")

				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_vp_nav', 'e2e_vp_nav', 'e2e_vp_nav@test.com', 'E2E VP Navigation', 'level-1', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('e2e_vp_nav_team', 'E2E VP Nav Squad', 'e2e_vp_nav')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('e2e_vp_nav_team', 'e2e_vp_nav', 'level-1', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				currentDate := time.Now().Format("2006-01-02")
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES ('e2e_vp_nav_sess', 'e2e_vp_nav_team', 'e2e_vp_nav', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('e2e_vp_nav_sess', 'mission', 3, 'improving', 'Clear mission'),
						('e2e_vp_nav_sess', 'value', 2, 'stable', 'Good value')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: VP logs in
				By("VP logging in")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_vp_nav")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				_, err = page.WaitForSelector("text=Team Health Overview", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Select the assessment period that matches our test data
				By("Selecting assessment period filter")
				periodFilter := page.Locator("[data-testid='period-filter']")
				_, err = periodFilter.SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"2024 - 2nd Half"},
				})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond) // Wait for data refresh

				// Then: Should be able to navigate between all tabs
				By("Verifying Team Cards tab is active by default and shows team data")
				teamCard := page.Locator("[data-testid='team-health-card']").First()
				err = teamCard.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking on Radar tab and verifying content")
				radarTab := page.Locator("[data-testid='radar-tab']")
				err = radarTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for loading spinner to disappear
				Eventually(func() bool {
					loadingSpinner := page.Locator(".animate-spin")
					visible, _ := loadingSpinner.IsVisible()
					return !visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				Eventually(func() bool {
					radarContent := page.Locator("[data-testid='vp-radar-chart']")
					visible, _ := radarContent.IsVisible()
					return visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking on Trends tab and verifying content")
				trendsTab := page.Locator("[data-testid='trends-tab']")
				err = trendsTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for loading spinner to disappear
				Eventually(func() bool {
					loadingSpinner := page.Locator(".animate-spin")
					visible, _ := loadingSpinner.IsVisible()
					return !visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Should show either the chart or empty state
				Eventually(func() bool {
					trendsContent := page.Locator("[data-testid='vp-trends-chart']")
					emptyState := page.Locator("text=No trend data available")
					chartVisible, _ := trendsContent.IsVisible()
					emptyVisible, _ := emptyState.IsVisible()
					return chartVisible || emptyVisible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking back to Team Cards tab")
				teamsTab := page.Locator("button:has-text('Team Cards')")
				err = teamsTab.Click()
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() bool {
					teamCard := page.Locator("[data-testid='team-health-card']").First()
					visible, _ := teamCard.IsVisible()
					return visible
				}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
			})
		})
	})
})
