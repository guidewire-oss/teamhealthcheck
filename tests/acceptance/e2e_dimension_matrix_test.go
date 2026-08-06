package acceptance_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Dimension Matrix View", Label("e2e"), func() {
	var (
		page          playwright.Page
		matrixTeam    = "e2e_matrix_team"
		matrixLead    = "e2e_matrix_lead"
		matrixMember1 = "e2e_matrix_member1"
		matrixMember2 = "e2e_matrix_member2"
	)

	BeforeEach(func() {
		// Create an isolated user and team for matrix tests to avoid interference from other tests
		_, _ = db.Exec(`DELETE FROM health_check_responses WHERE session_id IN ('e2e_matrix_s1', 'e2e_matrix_s2')`)
		_, _ = db.Exec(`DELETE FROM health_check_sessions WHERE id IN ('e2e_matrix_s1', 'e2e_matrix_s2')`)
		_, _ = db.Exec(`DELETE FROM team_members WHERE team_id = $1`, matrixTeam)
		_, _ = db.Exec(`DELETE FROM team_supervisors WHERE team_id = $1`, matrixTeam)
		_, _ = db.Exec(`DELETE FROM teams WHERE id = $1`, matrixTeam)
		_, _ = db.Exec(`DELETE FROM users WHERE id IN ($1, $2, $3)`, matrixLead, matrixMember1, matrixMember2)

		// Create team lead
		_, err := db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
			VALUES ($1, $2, $3, 'E2E Matrix Lead', 'level-4', $4)
			ON CONFLICT (id) DO NOTHING
		`, matrixLead, matrixLead, matrixLead+"@teams360.demo", DemoPasswordHash)
		Expect(err).NotTo(HaveOccurred())

		// Create team members (isolated IDs to avoid interfering with suite-seeded e2e_member1/e2e_member2)
		_, err = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash) VALUES
			($1, $1, $2, 'E2E Matrix Member 1', 'level-5', $3),
			($4, $4, $5, 'E2E Matrix Member 2', 'level-5', $3)
			ON CONFLICT (id) DO NOTHING
		`, matrixMember1, matrixMember1+"@teams360.demo", DemoPasswordHash, matrixMember2, matrixMember2+"@teams360.demo")
		Expect(err).NotTo(HaveOccurred())

		// Create team
		_, err = db.Exec(`
			INSERT INTO teams (id, name, team_lead_id)
			VALUES ($1, 'E2E Matrix Team', $2)
			ON CONFLICT (id) DO NOTHING
		`, matrixTeam, matrixLead)
		Expect(err).NotTo(HaveOccurred())

		// Add members to team
		_, err = db.Exec(`
			INSERT INTO team_members (team_id, user_id) VALUES
			($1, $2),
			($1, $3)
			ON CONFLICT DO NOTHING
		`, matrixTeam, matrixMember1, matrixMember2)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed) VALUES
			('e2e_matrix_s1', $1, $2, '2024-10-01', '2024 - 2nd Half', true),
			('e2e_matrix_s2', $1, $3, '2024-10-02', '2024 - 2nd Half', true)
			ON CONFLICT (id) DO NOTHING
		`, matrixTeam, matrixMember1, matrixMember2)
		Expect(err).NotTo(HaveOccurred())

		_, err = db.Exec(`
			INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
			('e2e_matrix_s1', 'mission', 3, 'improving', 'Great clarity'),
			('e2e_matrix_s1', 'value', 2, 'stable', ''),
			('e2e_matrix_s1', 'speed', 1, 'declining', 'Too slow'),
			('e2e_matrix_s1', 'fun', 3, 'improving', 'Enjoying work'),
			('e2e_matrix_s1', 'health', 2, 'stable', ''),
			('e2e_matrix_s1', 'learning', 3, 'improving', ''),
			('e2e_matrix_s1', 'support', 2, 'stable', ''),
			('e2e_matrix_s1', 'pawns', 3, 'improving', ''),
			('e2e_matrix_s1', 'release', 1, 'declining', ''),
			('e2e_matrix_s1', 'process', 2, 'stable', ''),
			('e2e_matrix_s1', 'teamwork', 3, 'improving', ''),
			('e2e_matrix_s2', 'mission', 2, 'stable', ''),
			('e2e_matrix_s2', 'value', 3, 'improving', 'Great value'),
			('e2e_matrix_s2', 'speed', 2, 'stable', ''),
			('e2e_matrix_s2', 'fun', 2, 'declining', 'Less fun lately'),
			('e2e_matrix_s2', 'health', 3, 'improving', ''),
			('e2e_matrix_s2', 'learning', 2, 'stable', ''),
			('e2e_matrix_s2', 'support', 3, 'improving', ''),
			('e2e_matrix_s2', 'pawns', 2, 'stable', ''),
			('e2e_matrix_s2', 'release', 2, 'stable', ''),
			('e2e_matrix_s2', 'process', 3, 'improving', ''),
			('e2e_matrix_s2', 'teamwork', 2, 'stable', '')
			ON CONFLICT DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		var err2 error
		page, err2 = browser.NewPage()
		Expect(err2).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		_, _ = db.Exec(`DELETE FROM health_check_responses WHERE session_id IN ('e2e_matrix_s1', 'e2e_matrix_s2')`)
		_, _ = db.Exec(`DELETE FROM health_check_sessions WHERE id IN ('e2e_matrix_s1', 'e2e_matrix_s2')`)
		_, _ = db.Exec(`DELETE FROM team_members WHERE team_id = $1`, matrixTeam)
		_, _ = db.Exec(`DELETE FROM team_supervisors WHERE team_id = $1`, matrixTeam)
		_, _ = db.Exec(`DELETE FROM teams WHERE id = $1`, matrixTeam)
		_, _ = db.Exec(`DELETE FROM users WHERE id IN ($1, $2, $3)`, matrixLead, matrixMember1, matrixMember2)
		if page != nil {
			page.Close()
		}
	})

	loginAndGoToResponses := func() {
		By("Logging in as matrix test lead")
		_, err := page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(10000),
		})
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").Fill(matrixLead)
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("input[name='password']").Fill("demo")
		Expect(err).NotTo(HaveOccurred())
		err = page.Locator("button[type='submit']").Click()
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for dashboard to load")
		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/dashboard"))
		dismissOnboardingIfVisible(page)

		By("Clicking Individual Responses tab")
		responsesTab := page.Locator("[data-testid='responses-tab']")
		err = responsesTab.WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(10000),
		})
		Expect(err).NotTo(HaveOccurred())
		err = responsesTab.Click()
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for responses section")
		err = page.Locator("[data-testid='responses-section']").WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(5000),
		})
		Expect(err).NotTo(HaveOccurred())
	}

	Describe("Dimension matrix toggle", func() {
		It("should toggle between person and dimension views", func() {
			loginAndGoToResponses()

			By("Verifying matrix table is visible by default")
			matrixTable := page.Locator("table")
			Eventually(func() bool {
				visible, _ := matrixTable.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying Matrix button is active (default view)")
			// The Matrix button should have the active class (text-indigo-600)
			matrixBtn := page.Locator("[data-testid='matrix-view-btn']")
			err := matrixBtn.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(5000),
			})
			Expect(err).NotTo(HaveOccurred())

			matrixClass, err := matrixBtn.GetAttribute("class")
			Expect(err).NotTo(HaveOccurred())
			Expect(matrixClass).To(ContainSubstring("text-indigo-600"))

			By("Clicking Cards button to switch view")
			cardsBtn := page.Locator("[data-testid='cards-view-btn']")
			err = cardsBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying response cards are visible (cards view)")
			cards := page.Locator("[data-testid='response-card']")
			Eventually(func() int {
				count, _ := cards.Count()
				return count
			}, 5*time.Second, 500*time.Millisecond).Should(BeNumerically(">", 0))

			By("Verifying matrix table is hidden in cards view")
			Eventually(func() bool {
				visible, _ := matrixTable.IsVisible()
				return !visible
			}, 3*time.Second, 300*time.Millisecond).Should(BeTrue())

			By("Switching back to matrix view")
			err = matrixBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				visible, _ := matrixTable.IsVisible()
				return visible
			}, 3*time.Second, 300*time.Millisecond).Should(BeTrue())
		})
	})

	Describe("Matrix cell content", func() {
		It("should display colored dots with trend icons and comment indicators", func() {
			loginAndGoToResponses()

			// Matrix is default view, no need to switch
			By("Verifying matrix is visible by default")
			matrixTable := page.Locator("table")
			err := matrixTable.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(5000),
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying score cell for e2e_member1 - mission (Green dot, Improving)")
			scoreEl := page.Locator("[data-testid='matrix-score-e2e_matrix_s1-mission']")
			err = scoreEl.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(5000),
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify it's a colored dot (rounded circle with background color)
			scoreClass, err := scoreEl.GetAttribute("class")
			Expect(err).NotTo(HaveOccurred())
			Expect(scoreClass).To(ContainSubstring("rounded-full"))

			// Verify green color via inline style (score 3 = #10B981 or rgb(16, 185, 129))
			scoreStyle, err := scoreEl.GetAttribute("style")
			Expect(err).NotTo(HaveOccurred())
			// Browser may convert hex to rgb, so check for either format
			styleLower := strings.ToLower(scoreStyle)
			isGreen := strings.Contains(styleLower, "10b981") ||
			           strings.Contains(styleLower, "rgb(16, 185, 129)")
			Expect(isGreen).To(BeTrue(), "Expected green color (hex or rgb format), got: %s", scoreStyle)

			// Verify aria-label says "Green"
			ariaLabel, err := scoreEl.GetAttribute("aria-label")
			Expect(err).NotTo(HaveOccurred())
			Expect(ariaLabel).To(Equal("Green"))

			// Verify trend icon is visible
			trendEl := page.Locator("[data-testid='matrix-trend-e2e_matrix_s1-mission']")
			trendVisible, _ := trendEl.IsVisible()
			Expect(trendVisible).To(BeTrue())

			By("Verifying score cell for e2e_member1 - speed (Red dot, Declining)")
			speedScore := page.Locator("[data-testid='matrix-score-e2e_matrix_s1-speed']")
			speedStyle, err := speedScore.GetAttribute("style")
			Expect(err).NotTo(HaveOccurred())
			// Browser may convert hex to rgb, so check for either format (score 1 = #EF4444 or rgb(239, 68, 68))
			speedStyleLower := strings.ToLower(speedStyle)
			isRed := strings.Contains(speedStyleLower, "ef4444") ||
			         strings.Contains(speedStyleLower, "rgb(239, 68, 68)")
			Expect(isRed).To(BeTrue(), "Expected red color (hex or rgb format), got: %s", speedStyle)

			speedLabel, err := speedScore.GetAttribute("aria-label")
			Expect(err).NotTo(HaveOccurred())
			Expect(speedLabel).To(Equal("Red"))

			speedTrend := page.Locator("[data-testid='matrix-trend-e2e_matrix_s1-speed']")
			speedTrendVisible, _ := speedTrend.IsVisible()
			Expect(speedTrendVisible).To(BeTrue())

			By("Verifying comment indicator for cells with comments")
			commentIndicator := page.Locator("[data-testid='matrix-comment-e2e_matrix_s1-mission']")
			commentCount, _ := commentIndicator.Count()
			Expect(commentCount).To(Equal(1))

			By("Verifying no comment indicator for cells without comments")
			noComment := page.Locator("[data-testid='matrix-comment-e2e_matrix_s1-value']")
			noCommentCount, _ := noComment.Count()
			Expect(noCommentCount).To(Equal(0))
		})
	})
})
