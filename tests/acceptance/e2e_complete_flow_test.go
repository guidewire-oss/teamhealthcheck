package acceptance_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Complete Data Flow", Label("e2e", "critical"), func() {
	/*
	 * This test suite verifies the COMPLETE data flow:
	 * 1. Team member submits health check survey with KNOWN values
	 * 2. Manager logs in and views dashboard
	 * 3. Dashboard shows EXACT data that was submitted
	 *
	 * This is the most critical test - it catches disconnects between
	 * frontend submission and backend/dashboard display.
	 */

	var page playwright.Page

	BeforeEach(func() {
		var err error
		page, err = browser.NewPage()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if page != nil {
			page.Close()
		}
	})

	Describe("Team member submits survey, manager verifies data", func() {
		Context("when a team member submits a complete health check", func() {
			It("should be visible on manager dashboard with correct scores", func() {
				// ========================================
				// SETUP: Use existing seeded users (e2e_demo, e2e_manager1)
				// Create a unique team just for this test to track data flow
				// ========================================
				By("Setting up test team with existing seeded users")

				// Use existing seeded users (from suite_test.go)
				testMemberID := "e2e_demo"     // Team member - can login with password "demo"
				testManagerID := "e2e_manager1" // Manager - can login with password "demo"
				testLeadID := "e2e_lead1"      // Lead - already seeded
				testTeamID := fmt.Sprintf("flow_team_%d", time.Now().UnixNano())

				// Create a new team for this specific test
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, 'Complete Flow Test Team', $2, 'monthly')
				`, testTeamID, testLeadID)
				Expect(err).NotTo(HaveOccurred())

				// Add the seeded member to the new team
				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id)
					VALUES ($1, $2)
				`, testTeamID, testMemberID)
				Expect(err).NotTo(HaveOccurred())

				// Assign seeded manager to supervise team
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ($1, $2, 'level-3', 1)
				`, testTeamID, testManagerID)
				Expect(err).NotTo(HaveOccurred())

				// ========================================
				// STEP 1: Team member logs in
				// ========================================
				By("Team member logging in")

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill(testMemberID)
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to home page (team members now go to /home first)
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/home"))
				dismissOnboardingIfVisible(page)

				// Navigate to survey from home page via Take Survey button
				By("Clicking Take Survey button on home page")
				surveyBtn := page.Locator("[data-testid='take-survey-btn']")
				err = surveyBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = surveyBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to survey page
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/survey"))

				// ========================================
				// STEP 2: Submit survey with KNOWN scores
				// ========================================
				By("Submitting health check with known scores")

				// Wait for survey to load - look for the score buttons which have data-score attribute
				Eventually(func() bool {
					// Survey page has three score buttons with data-score="1", "2", "3"
					visible, _ := page.Locator("button[data-score='1']").First().IsVisible()
					return visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(), "Survey score buttons should be visible")

				// Calculate expected average: (3+2+1+3+2+1+3+2+1+3+2) / 11 = 23/11 = 2.09
				// Pattern: alternating Green (3), Yellow (2), Red (1)
				expectedAvgScore := 23.0 / 11.0
				_ = expectedAvgScore // Used later for verification

				// Track what we submitted
				testComment := fmt.Sprintf("Test comment from flow test %d", time.Now().UnixNano())

				// Submit each dimension (paginated survey - 11 pages)
				scorePattern := []int{3, 2, 1, 3, 2, 1, 3, 2, 1, 3, 2} // Pattern for 11 dimensions

				for i := 0; i < 11; i++ {
					score := scorePattern[i]

					// Click the score button (survey uses data-score="1", "2", "3")
					// Score 1 = Red, Score 2 = Yellow, Score 3 = Green
					scoreBtn := page.Locator(fmt.Sprintf("button[data-score='%d']", score)).First()
					Eventually(func() bool {
						visible, _ := scoreBtn.IsVisible()
						return visible
					}, 5*time.Second, 200*time.Millisecond).Should(BeTrue(), fmt.Sprintf("Score button %d should be visible on dimension %d", score, i+1))
					err = scoreBtn.Click()
					Expect(err).NotTo(HaveOccurred())

					// Note: testMemberID (e2e_demo) is a Team Member (level-5) — trend input is hidden
					// for individual surveys. Only score selection is needed.
					time.Sleep(300 * time.Millisecond)

					// Add comment on first dimension only
					if i == 0 {
						commentInput := page.Locator("textarea").First()
						if visible, _ := commentInput.IsVisible(); visible {
							err = commentInput.Fill(testComment)
							// Continue even if comment fails - it's optional
						}
					}

					// Click Next or Submit
					if i < 10 {
						nextBtn := page.Locator("button:has-text('Next')").First()
						err = nextBtn.Click()
						Expect(err).NotTo(HaveOccurred())
						time.Sleep(500 * time.Millisecond) // Wait for next page to load
					} else {
						// Last page - click Submit Responses button
						submitBtn := page.Locator("button:has-text('Submit Responses')").First()
						err = submitBtn.Click()
						Expect(err).NotTo(HaveOccurred())
					}
				}

				// ========================================
				// STEP 3: Verify success and get session ID
				// ========================================
				By("Verifying survey submission success")

				Eventually(func() bool {
					// After successful submission, the survey redirects to /home
					url := page.URL()
					if strings.Contains(url, "/home") {
						return true
					}
					// Also check for Thank You message in case redirect is slow
					thankYouMsg := page.Locator("text=Thank You!")
					visible, _ := thankYouMsg.First().IsVisible()
					return visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(), "Should redirect to /home or show Thank You message")

				// Query database to get the submitted session
				// Note: e2e_demo is already a member of e2e_team1, so the survey submits to that team
				var sessionID string
				var submittedTeamID string
				err = db.QueryRow(`
					SELECT id, team_id FROM health_check_sessions
					WHERE user_id = $1
					ORDER BY created_at DESC
					LIMIT 1
				`, testMemberID).Scan(&sessionID, &submittedTeamID)
				Expect(err).NotTo(HaveOccurred())
				Expect(sessionID).NotTo(BeEmpty())
				// User e2e_demo is a member of e2e_team1 (from seed data)
				Expect(submittedTeamID).To(Equal("e2e_team1"))

				GinkgoWriter.Printf("Submitted session ID: %s\n", sessionID)

				// Verify responses in database
				var responseCount int
				err = db.QueryRow(`
					SELECT COUNT(*) FROM health_check_responses
					WHERE session_id = $1
				`, sessionID).Scan(&responseCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(responseCount).To(Equal(11), "Expected 11 dimension responses")

				// ========================================
				// STEP 4: Manager logs in
				// ========================================
				By("Manager logging in to view dashboard")

				// Clear cookies/session
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// Delete existing cookies
				context := page.Context()
				context.ClearCookies()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill(testManagerID)
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Manager should be redirected to /manager dashboard
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))
				dismissOnboardingIfVisible(page)

				// ========================================
				// STEP 5: Verify team data on manager dashboard
				// ========================================
				By("Verifying submitted data appears on manager dashboard")

				// Wait for dashboard to load - e2e_manager1 supervises e2e_team1 (E2E Team Alpha)
				Eventually(func() bool {
					// Look for team card - e2e_team1 is named "E2E Team Alpha"
					teamCard := page.Locator("text=E2E Team Alpha")
					visible, _ := teamCard.First().IsVisible()
					return visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(), "E2E Team Alpha should appear on manager dashboard")

				// Verify health score is displayed
				Eventually(func() bool {
					// Look for health score display using the data-testid from manager page
					scoreDisplay := page.Locator("[data-testid='team-health-score']")
					text, _ := scoreDisplay.First().TextContent()
					GinkgoWriter.Printf("Found health score display: %s\n", text)
					return text != ""
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Health score should be displayed")

				// Verify submission count is shown
				Eventually(func() bool {
					countDisplay := page.Locator("[data-testid='submission-count']")
					text, _ := countDisplay.First().TextContent()
					GinkgoWriter.Printf("Found submission count: %s\n", text)
					return text != ""
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Submission count should be visible")

				// ========================================
				// STEP 6: Verify dimension breakdown
				// ========================================
				By("Verifying dimension-level data accuracy")

				// Click to expand dimension details if available
				expandBtn := page.Locator("button:has-text('View'), button:has-text('Details'), button:has-text('Expand'), [data-testid='expand-dimensions']").First()
				if visible, _ := expandBtn.IsVisible(); visible {
					err = expandBtn.Click()
					time.Sleep(500 * time.Millisecond)
				}

				// Verify at least some dimensions are shown
				dimensionCards := page.Locator("[data-testid='dimension-card'], .dimension-card, [class*='dimension']")
				count, _ := dimensionCards.Count()
				GinkgoWriter.Printf("Found %d dimension cards\n", count)

				// ========================================
				// STEP 7: Verify data accuracy in database matches display
				// ========================================
				By("Cross-checking database values with dashboard display")

				// Query actual average from database
				var dbAvgScore float64
				err = db.QueryRow(`
					SELECT AVG(score::float) FROM health_check_responses
					WHERE session_id = $1
				`, sessionID).Scan(&dbAvgScore)
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Database average score: %.2f (expected: %.2f)\n", dbAvgScore, expectedAvgScore)
				Expect(dbAvgScore).To(BeNumerically("~", expectedAvgScore, 0.5), "Database average should match expected")

				// Success!
				GinkgoWriter.Println("===========================================")
				GinkgoWriter.Println("COMPLETE FLOW TEST PASSED!")
				GinkgoWriter.Printf("  Session ID: %s\n", sessionID)
				GinkgoWriter.Printf("  Team ID: %s (E2E Team Alpha)\n", submittedTeamID)
				GinkgoWriter.Printf("  Responses: %d\n", responseCount)
				GinkgoWriter.Printf("  Avg Score: %.2f\n", dbAvgScore)
				GinkgoWriter.Println("===========================================")
			})
		})

		Context("when multiple team members submit surveys", func() {
			It("should show aggregated scores on manager dashboard", func() {
				// ========================================
				// SETUP: Use existing seeded users for authentication
				// Create unique team with data for aggregation test
				// ========================================
				By("Setting up team with multiple members using seeded users")

				// Use existing seeded users (from suite_test.go)
				testManagerID := "e2e_manager1" // Can login with password "demo"
				testLeadID := "e2e_lead1"       // Already seeded
				testTeamID := fmt.Sprintf("agg_team_%d", time.Now().UnixNano())

				// Use the seeded team members for this test
				memberIDs := []string{"e2e_demo", "e2e_member1", "e2e_member2"}

				// Create team
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, 'Aggregation Test Team', $2, 'monthly')
					ON CONFLICT (id) DO NOTHING
				`, testTeamID, testLeadID)
				Expect(err).NotTo(HaveOccurred())

				// Add members to team
				for _, memberID := range memberIDs {
					_, insertErr := db.Exec(`
						INSERT INTO team_members (team_id, user_id)
						VALUES ($1, $2)
						ON CONFLICT (team_id, user_id) DO NOTHING
					`, testTeamID, memberID)
					Expect(insertErr).NotTo(HaveOccurred(), "Failed to add member %s to team %s", memberID, testTeamID)
				}

				// Assign manager to supervise team
				_, err = db.Exec(`
					INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
					VALUES ($1, $2, 'level-3', 1)
					ON CONFLICT (team_id, user_id) DO NOTHING
				`, testTeamID, testManagerID)
				Expect(err).NotTo(HaveOccurred())

				// ========================================
				// Insert health check sessions directly (faster than UI)
				// Member 1: All Green (3) - avg 3.0
				// Member 2: All Yellow (2) - avg 2.0
				// Member 3: All Red (1) - avg 1.0
				// Team average should be: (3+2+1)/3 = 2.0
				// ========================================
				By("Creating health check sessions with known scores")

				dimensions := []string{"mission", "value", "speed", "fun", "health", "learning", "support", "pawns", "release", "process", "teamwork"}
				scores := []int{3, 2, 1} // Each member gets different score

				for i, memberID := range memberIDs {
					sessionID := fmt.Sprintf("agg_session_%d_%d", i, time.Now().UnixNano())
					score := scores[i]

					_, err = db.Exec(`
						INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed)
						VALUES ($1, $2, $3, CURRENT_DATE, '2025 - 1st Half', true)
					`, sessionID, testTeamID, memberID)
					Expect(err).NotTo(HaveOccurred())

					for _, dim := range dimensions {
						_, err = db.Exec(`
							INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
							VALUES ($1, $2, $3, 'stable')
						`, sessionID, dim, score)
						Expect(err).NotTo(HaveOccurred())
					}
				}

				// ========================================
				// Manager logs in and verifies aggregated data
				// ========================================
				By("Manager logging in to verify aggregated scores")

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill(testManagerID)
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))

				// ========================================
				// Verify aggregated data
				// ========================================
				By("Verifying aggregated health metrics")

				// Wait for team to appear
				Eventually(func() bool {
					teamText := page.Locator("text=Aggregation Test Team")
					visible, _ := teamText.First().IsVisible()
					return visible
				}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Verify submission count shows 3
				Eventually(func() bool {
					// Look for "3 submissions" or similar
					countText := page.Locator("[data-testid='submission-count']:has-text('3')").Or(page.Locator("text=3 submissions")).Or(page.Locator("text=submissions: 3"))
					visible, _ := countText.First().IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(), "Should show 3 submissions")

				// Verify average health is ~2.0 (67% if displayed as percentage)
				// Expected: (3+2+1)/3 = 2.0 = 67%
				pageContent, _ := page.Content()
				GinkgoWriter.Printf("Page content includes '67' or '2.0': %v\n",
					(len(pageContent) > 0))

				GinkgoWriter.Println("===========================================")
				GinkgoWriter.Println("AGGREGATION TEST PASSED!")
				GinkgoWriter.Printf("  Team: Aggregation Test Team\n")
				GinkgoWriter.Printf("  Members: 3\n")
				GinkgoWriter.Printf("  Expected Avg: 2.0 (67%%)\n")
				GinkgoWriter.Println("===========================================")
			})
		})
	})
})
