package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Manager Dashboard", Label("e2e"), func() {
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
			DELETE FROM health_check_responses WHERE session_id LIKE 'e2e_%test';
			DELETE FROM health_check_sessions WHERE id LIKE 'e2e_%test';
			DELETE FROM team_supervisors WHERE team_id LIKE 'e2e_%test';
			DELETE FROM team_members WHERE team_id LIKE 'e2e_%test';
			DELETE FROM teams WHERE id LIKE 'e2e_%test';
			DELETE FROM users WHERE id LIKE 'e2e_%test';
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

	Describe("Manager views supervised teams health", func() {
		Context("when manager has multiple teams with different health levels", func() {
			It("should display aggregated health metrics sorted by health (worst first)", func() {
				// Given: Create organizational hierarchy
				By("Setting up organizational hierarchy with manager, leads, and members")

				// Create users (using e2e_ prefix to avoid conflicts with seed data)
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash)
					VALUES
						('e2e_mgr_test', 'e2e_mgr_test', 'e2e_mgr_test@test.com', 'E2E Manager Test', 'level-3', NULL, $1),
						('e2e_ld1_test', 'e2e_ld1_test', 'e2e_ld1_test@test.com', 'E2E Lead One Test', 'level-4', 'e2e_mgr_test', $1),
						('e2e_ld2_test', 'e2e_ld2_test', 'e2e_ld2_test@test.com', 'E2E Lead Two Test', 'level-4', 'e2e_mgr_test', $1),
						('e2e_mem1_test', 'e2e_mem1_test', 'e2e_mem1_test@test.com', 'E2E Member One Test', 'level-5', 'e2e_ld1_test', $1),
						('e2e_mem2_test', 'e2e_mem2_test', 'e2e_mem2_test@test.com', 'E2E Member Two Test', 'level-5', 'e2e_ld1_test', $1),
						('e2e_mem3_test', 'e2e_mem3_test', 'e2e_mem3_test@test.com', 'E2E Member Three Test', 'level-5', 'e2e_ld2_test', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Create teams (using e2e_ prefix)
				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES
						('e2e_tm1_test', 'E2E Alpha Squad', 'e2e_ld1_test'),
						('e2e_tm2_test', 'E2E Beta Squad', 'e2e_ld2_test')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add team members
				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id)
					VALUES
						('e2e_tm1_test', 'e2e_ld1_test'),
						('e2e_tm1_test', 'e2e_mem1_test'),
						('e2e_tm1_test', 'e2e_mem2_test'),
						('e2e_tm2_test', 'e2e_ld2_test'),
						('e2e_tm2_test', 'e2e_mem3_test')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Add supervisor chains
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES
						('e2e_tm1_test', 'e2e_ld1_test', 'level-4', 1),
						('e2e_tm1_test', 'e2e_mgr_test', 'level-3', 2),
						('e2e_tm2_test', 'e2e_ld2_test', 'level-4', 1),
						('e2e_tm2_test', 'e2e_mgr_test', 'level-3', 2)
				`)
				Expect(err).NotTo(HaveOccurred())

				By("Creating health check sessions with different health levels")
				currentDate := time.Now().Format("2006-01-02")

				// Create health check sessions for Team 1 (good health - avg 2.5)
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('e2e_sess1_test', 'e2e_tm1_test', 'e2e_mem1_test', $1, '2024 - 2nd Half', true),
						('e2e_sess2_test', 'e2e_tm1_test', 'e2e_mem2_test', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				// Add responses for Team 1 (average score ~2.5 - good)
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('e2e_sess1_test', 'mission', 3, 'improving', 'Clear goals'),
						('e2e_sess1_test', 'value', 2, 'stable', 'Good value'),
						('e2e_sess2_test', 'mission', 3, 'stable', 'Clear mission'),
						('e2e_sess2_test', 'value', 2, 'improving', 'Delivering well')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create health check sessions for Team 2 (needs support - avg 1.5)
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('e2e_sess3_test', 'e2e_tm2_test', 'e2e_mem3_test', $1, '2024 - 2nd Half', true)
				`, currentDate)
				Expect(err).NotTo(HaveOccurred())

				// Add responses for Team 2 (average score ~1.5 - needs support)
				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES
						('e2e_sess3_test', 'mission', 2, 'declining', 'Unclear direction'),
						('e2e_sess3_test', 'value', 1, 'declining', 'Struggling')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager logs in and navigates to dashboard
				By("Manager logging in to the application")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// Fill in login form (using e2e test manager)
				err = page.Locator("input[name='username']").Fill("e2e_mgr_test")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to manager dashboard")
				// Wait for redirect to manager dashboard
				Eventually(func() string {
					url := page.URL()
					return url
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				// Then: Verify dashboard shows aggregated team health
				By("Verifying team health data is displayed")

				// Wait for dashboard to load
				_, err = page.WaitForSelector("text=Team Health Overview", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Verify we see 2 teams - wait for cards to render after API call
				By("Verifying 2 teams are displayed")
				teamCards := page.Locator("[data-testid='team-health-card']")
				// Wait for at least one card to appear (API is async)
				err = teamCards.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				count, err := teamCards.Count()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(2))

				// Verify teams are sorted by health (worst first for attention)
				By("Verifying teams are sorted by health (worst first)")
				firstTeamName, err := page.Locator("[data-testid='team-health-card']").First().Locator("[data-testid='team-name']").TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(firstTeamName).To(ContainSubstring("E2E Beta Squad")) // Lower health (1.5) should appear first

				// Verify health scores are displayed
				By("Verifying health scores are visible")

				// Beta Squad should show lower health
				betaCard := page.Locator("[data-testid='team-health-card']").Filter(playwright.LocatorFilterOptions{
					HasText: "E2E Beta Squad",
				})
				betaHealth, err := betaCard.Locator("[data-testid='team-health-score']").TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(betaHealth).To(ContainSubstring("1.5")) // Average of (2+1)/2

				// Alpha Squad should show better health
				alphaCard := page.Locator("[data-testid='team-health-card']").Filter(playwright.LocatorFilterOptions{
					HasText: "E2E Alpha Squad",
				})
				alphaHealth, err := alphaCard.Locator("[data-testid='team-health-score']").TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(alphaHealth).To(ContainSubstring("2.5")) // Average of (3+2+3+2)/4

				// Verify submission counts
				By("Verifying submission counts are displayed")
				betaSubmissions, err := betaCard.Locator("[data-testid='submission-count']").TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(betaSubmissions).To(ContainSubstring("1")) // 1 submission

				alphaSubmissions, err := alphaCard.Locator("[data-testid='submission-count']").TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(alphaSubmissions).To(ContainSubstring("2")) // 2 submissions

				// Verify dimension breakdown is available
				By("Verifying dimension-level details are accessible")
				// Re-locate alphaCard to avoid stale reference
				alphaCardForClick := page.Locator("[data-testid='team-health-card']").Filter(playwright.LocatorFilterOptions{
					HasText: "E2E Alpha Squad",
				})
				viewDetailsBtn := alphaCardForClick.Locator("[data-testid='view-details-button']")

				// Verify the View Details button exists and shows dimension count
				btnText, err := viewDetailsBtn.TextContent()
				Expect(err).NotTo(HaveOccurred())
				GinkgoWriter.Printf("View Details button text: %s\n", btnText)
				Expect(btnText).To(ContainSubstring("Details"))

				err = viewDetailsBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait a moment for React state update
				time.Sleep(500 * time.Millisecond)

				// Should see dimension breakdown after expansion
				// Look for the dimension grid that appears when expanded
				Eventually(func() bool {
					// Look for any dimension card by checking for the capitalize class element
					dimensionCards := alphaCardForClick.Locator(".bg-gray-50.rounded-lg.p-3.border")
					count, _ := dimensionCards.Count()
					GinkgoWriter.Printf("Found %d dimension cards\n", count)
					return count > 0
				}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Verify specific dimensions are visible
				Eventually(func() bool {
					missionDim := alphaCardForClick.Locator("h4.capitalize:has-text('mission')")
					visible, _ := missionDim.IsVisible()
					return visible
				}, 3*time.Second, 300*time.Millisecond).Should(BeTrue())

				// Check for "Delivering Value" (special case for 'value' dimension)
				valueDim := alphaCardForClick.Locator("h4:has-text('Delivering Value')")
				visible, err := valueDim.IsVisible()
				Expect(err).NotTo(HaveOccurred())
				Expect(visible).To(BeTrue())
			})

			It("should allow filtering by assessment period", func() {
				// Given: Manager with teams that have health checks in different periods
				By("Setting up teams with health checks in multiple assessment periods")

				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_mgr_period', 'e2e_mgr_period', 'e2e_mgr_period@test.com', 'E2E Manager Period', 'level-3', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('e2e_tm_period', 'E2E Alpha Squad Period', 'e2e_mgr_period')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ('e2e_tm_period', 'e2e_mgr_period', 'level-3', 1)
				`)
				Expect(err).NotTo(HaveOccurred())

				// Create sessions in different periods
				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
					VALUES
						('e2e_sess_per1', 'e2e_tm_period', 'e2e_mgr_period', '2024-01-15', '2023 - 2nd Half', true),
						('e2e_sess_per2', 'e2e_tm_period', 'e2e_mgr_period', '2024-07-15', '2024 - 1st Half', true)
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES
						('e2e_sess_per1', 'mission', 2, 'stable'),
						('e2e_sess_per2', 'mission', 3, 'improving')
				`)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager logs in and filters by period
				By("Manager logging in")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_mgr_period")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for manager dashboard
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				By("Filtering by assessment period '2024 - 1st Half'")
				_, err = page.Locator("[data-testid='period-filter']").SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"2024 - 1st Half"},
				})
				Expect(err).NotTo(HaveOccurred())

				// Then: Should only show data from that period
				By("Verifying only 2024 - 1st Half data is shown")
				teamCard := page.Locator("[data-testid='team-health-card']").First()

				// Health score should be 3.0 (only session2) — wait for API response after filter change
				Eventually(func() (string, error) {
					return teamCard.Locator("[data-testid='team-health-score']").TextContent()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("3.0"))

				// Submission count should be 1 (only session2)
				Eventually(func() (string, error) {
					return teamCard.Locator("[data-testid='submission-count']").TextContent()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("1"))
			})
		})

		Context("when manager has no supervised teams", func() {
			It("should display empty state message", func() {
				// Given: Manager with no teams
				By("Creating manager with no team assignments")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_mgr_empty', 'e2e_mgr_empty', 'e2e_mgr_empty@test.com', 'E2E Manager Empty', 'level-3', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// When: Manager logs in
				By("Manager logging in")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_mgr_empty")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: Should see empty state
				By("Verifying empty state message is displayed")
				Eventually(func() bool {
					emptyState := page.Locator("text=No teams found")
					visible, _ := emptyState.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
			})
		})
	})
})
