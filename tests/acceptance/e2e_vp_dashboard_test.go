package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: VP/Executive Dashboard", func() {
	var (
		testVPUserID      = "e2e_vp1"
		testDirectorID    = "e2e_director1"
	)

	BeforeEach(func() {
		// Seed VP and Director users for the tests
		// Create VP user
		_, _ = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
			($1, 'e2e_vp1', 'e2e_vp1@teams360.demo', 'E2E VP User', 'level-1', NULL, $2)
			ON CONFLICT (id) DO NOTHING
		`, testVPUserID, DemoPasswordHash)

		// Create Director user who reports to VP
		_, _ = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
			($1, 'e2e_director1', 'e2e_director1@teams360.demo', 'E2E Director', 'level-2', $2, $3)
			ON CONFLICT (id) DO NOTHING
		`, testDirectorID, testVPUserID, DemoPasswordHash)

		// Add VP to team supervisor chain for teams
		_, _ = db.Exec(`
			INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position) VALUES
			('e2e_team1', $1, 'level-1', 3),
			('e2e_team2', $1, 'level-1', 3),
			('e2e_team1', $2, 'level-2', 2),
			('e2e_team2', $2, 'level-2', 2)
			ON CONFLICT (team_id, user_id) DO NOTHING
		`, testVPUserID, testDirectorID)
	})

	Describe("Hierarchy View Tab", func() {
		Context("when VP views the organizational hierarchy", func() {
			It("should display organization tree with expandable nodes", func() {
				By("Logging in as VP")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_vp1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				// VP should be redirected to manager/dashboard page
				err = page.WaitForURL("**/manager", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying Hierarchy View tab exists")
				hierarchyTab := page.Locator("[data-testid='hierarchy-tab'], button:has-text('Hierarchy View')")
				err = hierarchyTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking on Hierarchy View tab")
				err = hierarchyTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying organization tree is displayed")
				// Should show tree structure with VP at top
				orgTree := page.Locator("[data-testid='org-tree'], [data-testid='hierarchy-tree']")
				err = orgTree.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying VP node shows with expand capability")
				// VP should be at top of tree with children underneath
				vpNode := page.Locator("[data-testid='org-node-vp']").Or(page.Locator("text=VP")).Or(page.Locator("text=Vice President"))
				err = vpNode.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying child nodes (Directors) are visible or expandable")
				// Should show subordinates in tree
				directorNode := page.Locator("[data-testid='org-node-director']").Or(page.Locator("text=Director"))
				count, err := directorNode.Count()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">=", 1), "Should display at least one director node")

				GinkgoWriter.Printf("Hierarchy View displayed successfully\n")
			})
		})
	})

	Describe("Summary View Tab", func() {
		Context("when VP views the summary dashboard", func() {
			It("should display aggregated health metrics across all teams", func() {
				By("Logging in as VP")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_vp1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/manager", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Clicking on Summary View tab")
				summaryTab := page.Locator("[data-testid='summary-tab'], button:has-text('Summary View')")
				err = summaryTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = summaryTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying overall health score is displayed")
				overallScore := page.Locator("[data-testid='overall-health-score']").Or(page.Locator("text=Overall Health")).Or(page.Locator("text=Average"))
				err = overallScore.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying team count is displayed")
				teamCount := page.Locator("[data-testid='total-teams']").Or(page.Locator("text=teams")).Or(page.Locator("text=Total Teams"))
				err = teamCount.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying recent activity section exists")
				recentActivity := page.Locator("[data-testid='recent-activity']").Or(page.Locator("text=Recent Activity")).Or(page.Locator("text=Latest"))
				err = recentActivity.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Summary View displayed successfully\n")
			})
		})
	})

	Describe("Comparison Tab", func() {
		Context("when VP compares teams", func() {
			It("should allow selection of teams for side-by-side comparison", func() {
				By("Logging in as VP")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_vp1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/manager", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Clicking on Comparison tab")
				comparisonTab := page.Locator("[data-testid='comparison-tab'], button:has-text('Comparison')")
				err = comparisonTab.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = comparisonTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying team selection controls exist")
				teamSelector := page.Locator("[data-testid='team-selector'], select, [role='listbox']")
				err = teamSelector.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying comparison chart/table exists")
				comparisonChart := page.Locator("[data-testid='comparison-chart'], [data-testid='comparison-table'], canvas, svg")
				err = comparisonChart.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Comparison tab displayed successfully\n")
			})
		})
	})

	Describe("Assessment Period Filtering", func() {
		Context("when VP filters by assessment period", func() {
			It("should update all views based on selected period", func() {
				By("Logging in as VP")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_vp1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/manager", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying assessment period filter exists")
				periodFilter := page.Locator("[data-testid='period-filter'], select:has-text('Period'), select#period-filter")
				err = periodFilter.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Selecting a specific assessment period")
				_, err = periodFilter.First().SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"2024 - 1st Half"},
				})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(1000 * time.Millisecond)

				By("Verifying data updates after period selection")
				// The page should show data for the selected period or show no data message
				content := page.Locator("[data-testid='team-health-card']").Or(page.Locator("text=No teams")).Or(page.Locator("text=No data"))
				err = content.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Assessment period filtering works correctly\n")
			})
		})
	})

	Describe("VP Cannot Take Surveys", func() {
		// BUSINESS RULE: Only Team Members (level-5) and Team Leads (level-4) can take surveys.
		// VPs supervise multiple teams, making it ambiguous which team their survey response
		// should apply to. VPs should use the manager dashboard to view team health data instead.

		Context("when VP tries to access survey page", func() {
			It("should redirect VP away from survey page", func() {
				By("Logging in as VP")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_vp1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				// VP should be redirected to manager dashboard
				err = page.WaitForURL("**/manager", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Attempting to navigate to survey page")
				_, err = page.Goto(frontendURL + "/survey")
				Expect(err).NotTo(HaveOccurred())

				// Wait for potential redirect
				time.Sleep(2 * time.Second)

				By("Verifying VP is NOT on survey page")
				currentURL := page.URL()
				Expect(currentURL).NotTo(ContainSubstring("/survey"), "VP should not be able to access survey page")

				GinkgoWriter.Printf("VP survey restriction verified\n")
			})
		})
	})
})
