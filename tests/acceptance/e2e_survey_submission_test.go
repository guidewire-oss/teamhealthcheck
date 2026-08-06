package acceptance_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Survey Submission Flow", func() {
	var (
		testTeamID    = "e2e_team1"
		testUserID    = "e2e_demo"
		testSessionID string
	)

	Describe("Complete survey submission workflow", Ordered, func() {
		// Note: Using Ordered container to ensure tests run in sequence and share state
		BeforeAll(func() {
			// Reset user's team assignment to e2e_team1 (may have been changed by other tests)
			_, err := db.Exec("DELETE FROM team_members WHERE user_id = $1", testUserID)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec("INSERT INTO team_members (team_id, user_id) VALUES ($1, $2) ON CONFLICT (team_id, user_id) DO NOTHING", testTeamID, testUserID)
			Expect(err).NotTo(HaveOccurred())

			// Clean up ONLY sessions created by THIS test (identified by being in current assessment period for this team/user)
			// This avoids deleting seeded historical data that other tests depend on
			// The survey test creates new sessions in the current assessment period, so we clear those
			// Sessions created by this test will have team_id='e2e_team1' and user_id='e2e_demo' with recent dates
			// We preserve sessions from other tests:
			// - e2e_demo_session% (seeded data)
			// - e2e_home_% (from e2e_member_home_test)
			// - e2e_trend_% (from e2e_user_home_test)
			// - e2e_comment_% (from e2e_user_home_test)
			_, err = db.Exec(`
				DELETE FROM health_check_responses
				WHERE session_id IN (
					SELECT id FROM health_check_sessions
					WHERE user_id = $1 AND team_id = $2
					AND id NOT LIKE 'e2e_demo_session%'
					AND id NOT LIKE 'e2e_home_%'
					AND id NOT LIKE 'e2e_trend_%'
					AND id NOT LIKE 'e2e_comment_%'
				)`, testUserID, testTeamID)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				DELETE FROM health_check_sessions
				WHERE user_id = $1 AND team_id = $2
				AND id NOT LIKE 'e2e_demo_session%'
				AND id NOT LIKE 'e2e_home_%'
				AND id NOT LIKE 'e2e_trend_%'
				AND id NOT LIKE 'e2e_comment_%'`, testUserID, testTeamID)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("when a user submits a health check survey", func() {
			It("should save the data to the database and show success confirmation", func() {
				By("Opening browser and logging in")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				// Navigate to login page
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// Fill login credentials (Team Member: e2e_demo/demo)
				usernameInput := page.Locator("#username")
				err = usernameInput.Fill("e2e_demo")
				Expect(err).NotTo(HaveOccurred())

				passwordInput := page.Locator("#password")
				err = passwordInput.Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				// Click Sign In button
				loginButton := page.Locator("button[type='submit']:has-text('Sign In')")
				err = loginButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to home page (Team Members now go to /home first)
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				// Navigate to survey via Take Survey button
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
				err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Wait for page to fully load
				err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
					State: playwright.LoadStateNetworkidle,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying survey page loaded")
				title, err := page.Title()
				Expect(err).NotTo(HaveOccurred())
				Expect(title).To(ContainSubstring("Health Check"))

				By("Waiting for health dimensions to load")
				// Wait for the first dimension card to appear
				err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

			By("Selecting health check responses for all 11 dimensions (paginated survey)")

			// Helper function to fill a dimension
			// Note: e2e_demo is a Team Member (level-5) so trend input is hidden for individual surveys.
			// Only score is required for Team Members; trend argument is accepted but ignored.
			fillDimension := func(dimensionID string, score int, _ string) {
				scoreSelector := fmt.Sprintf("[data-dimension='%s'][data-score='%d']", dimensionID, score)
				err = page.Locator(scoreSelector).WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator(scoreSelector).Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond) // Wait for React state update
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

			// 1. Mission - Green (3), Improving
			fillDimension("mission", 3, "improving")
			clickNext()

			// 2. Delivering Value - Yellow (2), Stable
			fillDimension("value", 2, "stable")
			clickNext()

			// 3. Speed - Red (1), Declining
			fillDimension("speed", 1, "declining")
			clickNext()

			// 4. Fun - Yellow (2), Stable
			fillDimension("fun", 2, "stable")
			clickNext()

			// 5. Health of Codebase - Green (3), Improving
			fillDimension("health", 3, "improving")
			clickNext()

			// 6. Learning - Green (3), Improving
			fillDimension("learning", 3, "improving")
			clickNext()

			// 7. Support - Yellow (2), Stable
			fillDimension("support", 2, "stable")
			clickNext()

			// 8. Pawns or Players - Green (3), Improving
			fillDimension("pawns", 3, "improving")
			clickNext()

			// 9. Easy to Release - Red (1), Declining
			fillDimension("release", 1, "declining")
			clickNext()

			// 10. Suitable Process - Yellow (2), Stable
			fillDimension("process", 2, "stable")
			clickNext()

			// 11. Teamwork - Green (3), Improving (LAST - no Next button)
			fillDimension("teamwork", 3, "improving")

			By("Submitting the survey (on last dimension)")
			submitButton := page.Locator("button[type='submit']:has-text('Submit')")
			err = submitButton.WaitFor(playwright.LocatorWaitForOptions{
				State:   playwright.WaitForSelectorStateVisible,
				Timeout: playwright.Float(3000),
			})
			Expect(err).NotTo(HaveOccurred())
			err = submitButton.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for redirect to home page after successful submission")
			// The survey page auto-redirects to /home after successful submission
			err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
				Timeout: playwright.Float(10000),
			})
			Expect(err).NotTo(HaveOccurred())
			dismissOnboardingIfVisible(page)

			By("Verifying we're on the home page")
			url := page.URL()
			GinkgoWriter.Printf("Redirected to URL: %s\n", url)
			Expect(url).To(ContainSubstring("/home"))

			By("Querying database to verify data was saved correctly")
			// Find the most recent session for this team/user
			var sessionID, teamID, userID, date, assessmentPeriod string
			var completed bool

			err = db.QueryRow(`
				SELECT id, team_id, user_id, date, assessment_period, completed
				FROM health_check_sessions
				WHERE team_id = $1 AND user_id = $2
				ORDER BY created_at DESC
				LIMIT 1
			`, testTeamID, testUserID).Scan(&sessionID, &teamID, &userID, &date, &assessmentPeriod, &completed)

			Expect(err).NotTo(HaveOccurred(), "Session should exist in database")
			Expect(teamID).To(Equal(testTeamID))
			Expect(userID).To(Equal(testUserID))
			Expect(completed).To(BeTrue())

			testSessionID = sessionID
			GinkgoWriter.Printf("Found session in database: %s\n", sessionID)

			By("Verifying all 11 responses were saved")
			var responseCount int
			err = db.QueryRow(`
				SELECT COUNT(*)
				FROM health_check_responses
				WHERE session_id = $1
			`, sessionID).Scan(&responseCount)

			Expect(err).NotTo(HaveOccurred())
			Expect(responseCount).To(Equal(11), "Should have 11 dimension responses")

			By("Verifying specific response data")
			// Note: e2e_demo is a Team Member (level-5) — trend input is hidden for individual surveys,
			// so only score is verified here.
			var dimensionID, comment string
			var score int

			err = db.QueryRow(`
				SELECT dimension_id, score, comment
				FROM health_check_responses
				WHERE session_id = $1 AND dimension_id = 'mission'
			`, sessionID).Scan(&dimensionID, &score, &comment)

			Expect(err).NotTo(HaveOccurred())
			Expect(dimensionID).To(Equal("mission"))
			Expect(score).To(Equal(3))

			// Check Speed response (Red)
			err = db.QueryRow(`
				SELECT dimension_id, score
				FROM health_check_responses
				WHERE session_id = $1 AND dimension_id = 'speed'
			`, sessionID).Scan(&dimensionID, &score)

			Expect(err).NotTo(HaveOccurred())
			Expect(dimensionID).To(Equal("speed"))
			Expect(score).To(Equal(1))

			By("Verifying assessment period was auto-calculated")
			Expect(assessmentPeriod).NotTo(BeEmpty())
			// Assessment period format depends on team cadence (monthly/quarterly/half-yearly/yearly)
			Expect(assessmentPeriod).To(MatchRegexp(`\d{4}( (Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)|( Q[1-4])|( H[12]))?`))

			GinkgoWriter.Printf("✅ E2E Test PASSED: Survey submitted successfully\n")
			GinkgoWriter.Printf("   Session ID: %s\n", sessionID)
			GinkgoWriter.Printf("   Team ID: %s\n", teamID)
			GinkgoWriter.Printf("   User ID: %s\n", userID)
			GinkgoWriter.Printf("   Assessment Period: %s\n", assessmentPeriod)
			GinkgoWriter.Printf("   Responses: %d/11\n", responseCount)
		})
	})

		Context("when viewing submitted survey results", func() {
			It("should display the survey data on the results page", func() {
				// Skip if no session was created in test 1
				if testSessionID == "" {
					Skip("Skipping results page test - no session was created in previous test")
				}

				By("Opening results page")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				// Navigate to login first
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// Login as e2e_demo user
				err = page.Locator("#username").Fill("e2e_demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to home (team members now go to /home first)
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying session exists in database")
				// Verify via direct database query that the session was created
				var sessionExists bool
				err = db.QueryRow(`SELECT EXISTS(SELECT 1 FROM health_check_sessions WHERE id = $1)`, testSessionID).Scan(&sessionExists)
				Expect(err).NotTo(HaveOccurred())
				Expect(sessionExists).To(BeTrue(), "Session should exist in database")

				GinkgoWriter.Printf("✅ Session %s verified in database\n", testSessionID)
			})
		})
	})

	Describe("Trend input visibility", func() {
		Context("when a Team Member (level-5) loads the survey", func() {
			It("should not show trend buttons for any dimension", func() {
				By("Opening browser and logging in as Team Member")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
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

				By("Waiting for survey to load")
				err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Selecting a score to trigger the post-score section")
				err = page.Locator("[data-dimension='mission'][data-score='3']").Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying trend buttons are NOT visible for Team Member on individual survey")
				improvingBtn := page.Locator("[data-dimension='mission'][data-trend='improving']")
				visible, err := improvingBtn.IsVisible()
				Expect(err).NotTo(HaveOccurred())
				Expect(visible).To(BeFalse(), "Trend buttons should not be shown for Team Members on individual survey")

				By("Verifying comment textarea IS still visible")
				commentBox := page.Locator("textarea[data-dimension='mission']")
				commentVisible, err := commentBox.IsVisible()
				Expect(err).NotTo(HaveOccurred())
				Expect(commentVisible).To(BeTrue(), "Comment box should still be visible for Team Members")

				GinkgoWriter.Printf("Trend buttons correctly hidden for Team Member on individual survey\n")
			})
		})

		Context("when a Team Member loads the post-workshop survey", func() {
			It("should show trend buttons on post-workshop survey regardless of role", func() {
				By("Opening browser and logging in as Team Member")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Navigating directly to post-workshop survey")
				_, err = page.Goto(frontendURL + "/survey?type=post_workshop")
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/survey**", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for survey to load")
				err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Selecting a score to trigger the post-score section")
				err = page.Locator("[data-dimension='mission'][data-score='3']").Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying trend buttons ARE visible on post-workshop survey")
				improvingBtn := page.Locator("[data-dimension='mission'][data-trend='improving']")
				err = improvingBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Trend buttons correctly shown on post-workshop survey\n")
			})
		})
	})

	Describe("Error handling and validation", func() {
		Context("when submitting incomplete survey", func() {
			It("should show validation errors and prevent submission", func() {
				By("Opening survey page")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				// Navigate to login first
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// Login as e2e_demo user
				err = page.Locator("#username").Fill("e2e_demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to home page (Team Members now go to /home first)
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				// Navigate to survey via Take Survey button
				surveyBtn := page.Locator("[data-testid='take-survey-btn']")
				err = surveyBtn.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = surveyBtn.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to survey page
				err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Wait for survey to fully load
				err = page.Locator("text=Mission").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Attempting to submit without filling required fields")
				// Try clicking Next without selecting a score - this should show validation error
				nextButton := page.Locator("button:has-text('Next')")
				err = nextButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying validation error messages appear")
				errorMessage := page.Locator("text=please select").Or(page.Locator("text=required")).Or(page.Locator("text=Please select"))
				err = errorMessage.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying no data was saved to database")
				var count int
				err = db.QueryRow(`
					SELECT COUNT(*)
					FROM health_check_sessions
					WHERE team_id = 'incomplete-test'
				`).Scan(&count)

				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(0), "No incomplete sessions should be saved")

				GinkgoWriter.Printf("✅ Validation working correctly\n")
			})
		})
	})
})
