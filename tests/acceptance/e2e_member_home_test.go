package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Team Member Home Page", Label("e2e"), func() {
	var (
		page playwright.Page
		ctx  playwright.BrowserContext
		// Use pre-seeded test user from suite_test.go
		testUserID = "e2e_demo"
	)

	BeforeEach(func() {
		var err error
		ctx, err = browser.NewContext()
		Expect(err).NotTo(HaveOccurred())

		page, err = ctx.NewPage()
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

	Describe("Member Home Dashboard", func() {
		Context("when team member logs in", func() {
			It("should redirect to member home page instead of survey", func() {
				By("Logging in as team member")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying redirect to /home instead of /survey")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying home page displays welcome message")
				welcomeText := page.Locator("[data-testid='welcome-message']")
				err = welcomeText.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Team member successfully redirected to /home\n")
			})
		})

		// Note: These tests rely on e2e_demo_session1 and e2e_demo_session2
		// seeded in SynchronizedBeforeSuite (suite_test.go)
		Context("when member has survey history", func() {
			It("should display user's own survey history", func() {
				By("Logging in as team member")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for home page to load")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
					State: playwright.LoadStateNetworkidle,
				})
				Expect(err).NotTo(HaveOccurred())

				// Extra wait for React to render after API response
				time.Sleep(2 * time.Second)

				By("Verifying survey history section is displayed")
				historySection := page.Locator("[data-testid='survey-history']")
				err = historySection.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(20000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying historical entries are shown")
				historyEntries := page.Locator("[data-testid='history-entry']")
				count, err := historyEntries.Count()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">=", 1), "Should display at least one history entry")

				GinkgoWriter.Printf("Survey history displayed successfully with %d entries\n", count)
			})

			It("should show visualization of user's own health scores", func() {
				By("Logging in as team member")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for home page to load")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				// Wait for page to fully load including API calls
				err = page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
					State: playwright.LoadStateNetworkidle,
				})
				Expect(err).NotTo(HaveOccurred())

				time.Sleep(2 * time.Second) // Allow charts to render

				By("Verifying radar chart visualization exists")
				chartElement := page.Locator("[data-testid='health-chart']")
				err = chartElement.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(15000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Health visualization displayed successfully\n")
			})
		})

		Context("Take Survey button", func() {
			It("should display a prominent 'Take Survey' button", func() {
				By("Logging in as team member")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for home page to load")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying Take Survey button is prominent and visible")
				surveyButton := page.Locator("[data-testid='take-survey-btn']")
				err = surveyButton.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking Take Survey button navigates to survey page")
				err = surveyButton.Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/survey", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Take Survey button works correctly\n")
			})

			It("should show current assessment period", func() {
				By("Logging in as team member")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(testUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for home page to load")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying assessment period context is shown")
				periodText := page.Locator("[data-testid='current-period']")
				err = periodText.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				// Verify it shows the period label (cadence is team-specific, so home shows generic label)
				text, err := periodText.TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(text).To(ContainSubstring("Current Period"))

				GinkgoWriter.Printf("Current assessment period displayed: %s\n", text)
			})
		})

		Context("when member has no survey history", func() {
			It("should show empty state with call to action", func() {
				// Use e2e_fresh_member who has no survey history in seed data
				// Note: e2e_fresh_member is hierarchy level-5 (Team Member) so gets redirected to /home
				freshUserID := "e2e_fresh_member"

				By("Logging in as fresh member with no history")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill(freshUserID)
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for home page to load")
				err = page.WaitForURL("**/home", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				dismissOnboardingIfVisible(page)

				By("Verifying empty state is displayed")
				emptyState := page.Locator("[data-testid='empty-state']")
				err = emptyState.WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Empty state displayed for new member\n")
			})
		})
	})
})
