package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Session Timeout", Label("e2e"), func() {
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
	})

	AfterEach(func() {
		_, _ = db.Exec(`DELETE FROM users WHERE id = 'e2e_timeout_user'`)
		if page != nil {
			page.Close()
		}
		if ctx != nil {
			ctx.Close()
		}
	})

	Describe("Session expired banner on login page", func() {
		Context("when navigating to /login?expired=true", func() {
			It("should display the session expired banner", func() {
				By("Navigating to login page with expired query param")
				_, err := page.Goto(frontendURL + "/login?expired=true")
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for the login form to load")
				err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the session expired banner is visible")
				banner := page.Locator("[data-testid='session-expired-banner']")
				Eventually(func() bool {
					visible, _ := banner.IsVisible()
					return visible
				}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying the banner text")
				text, err := banner.TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(text).To(ContainSubstring("session has expired"))
			})
		})

		Context("when navigating to /login without expired param", func() {
			It("should NOT display the session expired banner", func() {
				By("Navigating to clean login page")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for the login form to load")
				err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying the session expired banner is NOT visible")
				banner := page.Locator("[data-testid='session-expired-banner']")
				Consistently(func() bool {
					visible, _ := banner.IsVisible()
					return visible
				}, 2*time.Second, 500*time.Millisecond).Should(BeFalse())
			})
		})
	})

	Describe("Auth data cleared redirects to login", func() {
		Context("when user clears auth data and navigates to protected page", func() {
			It("should redirect to login page", func() {
				By("Creating a test user for this scenario")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_timeout_user', 'e2e_timeout', 'e2e_timeout@test.com', 'Timeout Test User', 'level-5', $1)
					ON CONFLICT (id) DO UPDATE SET password_hash = EXCLUDED.password_hash
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				By("Logging in")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_timeout")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for redirect to home page")
				Eventually(func() string {
					return page.URL()
				}, 15*time.Second, 500*time.Millisecond).Should(ContainSubstring("/home"))

				By("Simulating session expiry by clearing auth data")
				_, err = page.Evaluate(`() => {
					localStorage.removeItem('accessToken');
					localStorage.removeItem('refreshToken');
					document.cookie = 'user=; path=/; max-age=0';
				}`)
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to a protected page")
				_, err = page.Goto(frontendURL + "/home")
				Expect(err).NotTo(HaveOccurred())

				By("Verifying redirect to login page")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/login"))
			})
		})
	})
})
