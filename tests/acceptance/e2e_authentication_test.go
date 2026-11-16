package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Authentication", Label("e2e"), func() {
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

		// Clean up test data before each test.
		//
		// The underscores in this prefix must be escaped: in a SQL LIKE
		// pattern, '_' is a wildcard for any single character, not a literal
		// underscore. An unescaped 'e2e_test_%' happens to also match
		// 'e2e_testmanager1' -- a completely unrelated, permanently seeded
		// fixture from suite_test.go -- because "e2e" + (any char) + "test" +
		// (any char) + (anything) is satisfied by that name's exact spelling.
		// That silently deleted a shared fixture other specs depend on.
		_, err = db.Exec(`
			DELETE FROM users WHERE id LIKE 'e2e\_test\_%' ESCAPE '\';
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

	Describe("Cleanup query pattern", func() {
		// This is a read-only proof of the LIKE pattern's matching semantics
		// against two literal strings -- it never touches the users table, so
		// it can't collide with 'e2e_testmanager1' (a real, permanently seeded
		// fixture from suite_test.go) or with the 'e2e_test_mgr1' row a sibling
		// spec above may have already inserted. It exists to pin the escaping
		// itself as a regression guard: if someone "simplifies" the BeforeEach
		// query back to an unescaped 'e2e_test_%', this fails immediately
		// instead of silently reintroducing the cross-fixture deletion bug.
		matches := func(candidateID string) bool {
			var matched bool
			err := db.QueryRow(
				`SELECT $1::text LIKE 'e2e\_test\_%' ESCAPE '\'`, candidateID,
			).Scan(&matched)
			Expect(err).NotTo(HaveOccurred())
			return matched
		}

		It("matches this file's own fixture prefix", func() {
			Expect(matches("e2e_test_mgr1")).To(BeTrue(),
				"the cleanup query must still delete this file's own fixture")
		})

		It("does not match the unrelated shared fixture e2e_testmanager1", func() {
			Expect(matches("e2e_testmanager1")).To(BeFalse(),
				"the cleanup query must never match suite_test.go's shared e2e_testmanager1 fixture")
		})
	})

	Describe("Basic authentication flow", func() {
		Context("when user has valid credentials", func() {
			It("should successfully log in and redirect to appropriate dashboard", func() {
				// Given: A user exists in the database
				By("Creating a test manager user")
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('e2e_test_mgr1', 'e2e_test_mgr', 'e2e_test_mgr@test.com', 'E2E Test Manager', 'level-3', $1)
				`, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// When: User navigates to login page
				By("User navigating to login page")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// Then: Login form should be visible
				By("Verifying login form is displayed")
				_, err = page.WaitForSelector("input[name='username']", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				// When: User enters valid credentials
				By("User entering valid credentials")
				err = page.Locator("input[name='username']").Fill("e2e_test_mgr")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				// When: User submits the form
				By("User submitting login form")
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: User should be redirected to manager dashboard
				By("Verifying redirect to manager dashboard")
				Eventually(func() string {
					url := page.URL()
					return url
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/manager"))

				// And: Dashboard should be accessible
				By("Verifying dashboard loads successfully")
				_, err = page.WaitForSelector("text=Manager Dashboard", playwright.PageWaitForSelectorOptions{
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())
			})
		})

		Context("when user has invalid credentials", func() {
			It("should display error message and remain on login page", func() {
				// Given: User navigates to login page
				By("User navigating to login page")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				// When: User enters invalid credentials
				By("User entering invalid credentials")
				err = page.Locator("input[name='username']").Fill("invaliduser")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='password']").Fill("wrongpassword")
				Expect(err).NotTo(HaveOccurred())

				// When: User submits the form
				By("User submitting login form with invalid credentials")
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				// Then: Error message should be displayed
				By("Verifying error message is displayed")
				Eventually(func() bool {
					errorMsg := page.Locator("text=Invalid username or password").Or(page.Locator("text=Invalid credentials"))
					visible, _ := errorMsg.IsVisible()
					return visible
				}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

				// And: Should remain on login page
				Expect(page.URL()).To(ContainSubstring("/login"))
			})
		})

		Context("when user is not authenticated", func() {
			It("should redirect to login page when accessing protected route", func() {
				// Given: No authenticated user
				// When: User tries to access manager dashboard directly
				By("User navigating directly to protected route")
				_, err := page.Goto(frontendURL + "/manager")
				Expect(err).NotTo(HaveOccurred())

				// Then: Should be redirected to login page
				By("Verifying redirect to login page")
				Eventually(func() string {
					url := page.URL()
					return url
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/login"))
			})
		})
	})
})
