// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Multi-Team Switcher", Label("e2e"), func() {
	var (
		page     playwright.Page
		ctx      playwright.BrowserContext
		userID   = "e2e_multi_user"
		teamAID  = "e2e_multi_team_a"
		teamBID  = "e2e_multi_team_b"
	)

	BeforeEach(func() {
		// Clean up any previous test data
		_, _ = db.Exec(`DELETE FROM health_check_responses WHERE session_id IN ('e2e_multi_s1', 'e2e_multi_s2')`)
		_, _ = db.Exec(`DELETE FROM health_check_sessions WHERE team_id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM team_members WHERE team_id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM team_supervisors WHERE team_id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM teams WHERE id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID)

		// Create test user (team lead level)
		_, err := db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
			VALUES ($1, $2, $3, 'E2E Multi-Team User', 'level-4', $4)
		`, userID, userID, userID+"@teams360.demo", DemoPasswordHash)
		Expect(err).NotTo(HaveOccurred())

		// Create two teams with this user as lead
		_, err = db.Exec(`
			INSERT INTO teams (id, name, team_lead_id) VALUES
			($1, 'Alpha Squad', $3),
			($2, 'Beta Squad', $3)
		`, teamAID, teamBID, userID)
		Expect(err).NotTo(HaveOccurred())

		// Seed health check sessions for each team
		_, err = db.Exec(`
			INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed) VALUES
			('e2e_multi_s1', $1, 'e2e_member1', '2024-10-01', '2024 - 2nd Half', true),
			('e2e_multi_s2', $2, 'e2e_member2', '2024-10-02', '2024 - 2nd Half', true)
			ON CONFLICT (id) DO NOTHING
		`, teamAID, teamBID)
		Expect(err).NotTo(HaveOccurred())

		// Seed responses for team A session
		_, err = db.Exec(`
			INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
			('e2e_multi_s1', 'mission', 3, 'improving', ''),
			('e2e_multi_s1', 'value', 2, 'stable', ''),
			('e2e_multi_s1', 'speed', 1, 'declining', ''),
			('e2e_multi_s1', 'fun', 3, 'improving', ''),
			('e2e_multi_s1', 'health', 2, 'stable', ''),
			('e2e_multi_s1', 'learning', 3, 'improving', ''),
			('e2e_multi_s1', 'support', 2, 'stable', ''),
			('e2e_multi_s1', 'pawns', 3, 'improving', ''),
			('e2e_multi_s1', 'release', 1, 'declining', ''),
			('e2e_multi_s1', 'process', 2, 'stable', ''),
			('e2e_multi_s1', 'teamwork', 3, 'improving', '')
			ON CONFLICT DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		// Seed responses for team B session
		_, err = db.Exec(`
			INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
			('e2e_multi_s2', 'mission', 2, 'stable', ''),
			('e2e_multi_s2', 'value', 3, 'improving', ''),
			('e2e_multi_s2', 'speed', 2, 'stable', ''),
			('e2e_multi_s2', 'fun', 2, 'declining', ''),
			('e2e_multi_s2', 'health', 3, 'improving', ''),
			('e2e_multi_s2', 'learning', 2, 'stable', ''),
			('e2e_multi_s2', 'support', 3, 'improving', ''),
			('e2e_multi_s2', 'pawns', 2, 'stable', ''),
			('e2e_multi_s2', 'release', 2, 'stable', ''),
			('e2e_multi_s2', 'process', 3, 'improving', ''),
			('e2e_multi_s2', 'teamwork', 2, 'stable', '')
			ON CONFLICT DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		var err2 error
		ctx, err2 = browser.NewContext()
		Expect(err2).NotTo(HaveOccurred())
		page, err2 = ctx.NewPage()
		Expect(err2).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_, _ = db.Exec(`DELETE FROM health_check_responses WHERE session_id IN ('e2e_multi_s1', 'e2e_multi_s2')`)
		_, _ = db.Exec(`DELETE FROM health_check_sessions WHERE team_id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM team_members WHERE team_id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM team_supervisors WHERE team_id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM teams WHERE id IN ($1, $2)`, teamAID, teamBID)
		_, _ = db.Exec(`DELETE FROM users WHERE id = $1`, userID)
		if page != nil {
			page.Close()
		}
		if ctx != nil {
			ctx.Close()
		}
	})

	login := func() {
		_, err := page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(10000),
		})
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").Fill(userID)
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("input[name='password']").Fill("demo")
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("button[type='submit']").Click()
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Dashboard team selector", func() {
		It("should show team selector and switch between teams", func() {
			login()

			By("Waiting for dashboard to load")
			Eventually(func() string {
				return page.URL()
			}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/dashboard"))

			By("Verifying team selector is visible")
			selector := page.Locator("[data-testid='team-selector']")
			Eventually(func() bool {
				visible, _ := selector.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying selector has 2 options")
			options := selector.Locator("option")
			Eventually(func() int {
				count, _ := options.Count()
				return count
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(2))

			By("Verifying current team is the first one")
			selectedValue, err := selector.InputValue()
			Expect(err).NotTo(HaveOccurred())
			Expect(selectedValue).To(Equal(teamAID))

			By("Switching to second team")
			_, err = selector.SelectOption(playwright.SelectOptionValues{
				Values: &[]string{teamBID},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for data to reload with new team")
			Eventually(func() string {
				val, _ := selector.InputValue()
				return val
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(teamBID))

			By("Verifying dashboard re-fetched data for team B")
			// The loading indicator should disappear, confirming new data loaded
			Eventually(func() bool {
				loading := page.Locator("text=Loading...")
				visible, _ := loading.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeFalse())
		})
	})

	Describe("Survey team selector", func() {
		It("should show team selector on survey page", func() {
			login()

			By("Waiting for redirect")
			Eventually(func() string {
				return page.URL()
			}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/dashboard"))

			By("Navigating to survey")
			_, err := page.Goto(frontendURL + "/survey")
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for survey to load")
			err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10000),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying team selector is visible")
			selector := page.Locator("[data-testid='team-selector']")
			Eventually(func() bool {
				visible, _ := selector.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying selector has 2 options")
			options := selector.Locator("option")
			count, _ := options.Count()
			Expect(count).To(Equal(2))

			By("Switching to second team")
			_, err = selector.SelectOption(playwright.SelectOptionValues{
				Values: &[]string{teamBID},
			})
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for survey to reload after team switch")
			// After switching, the survey reloads with the new team — wait for the loading to finish
			Eventually(func() bool {
				loading := page.Locator("text=Fetching your team information")
				visible, _ := loading.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeFalse())

			By("Verifying survey resets (back to first dimension)")
			err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(10000),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the team selector now shows team B")
			newVal, err := selector.InputValue()
			Expect(err).NotTo(HaveOccurred())
			Expect(newVal).To(Equal(teamBID))
		})
	})
})
