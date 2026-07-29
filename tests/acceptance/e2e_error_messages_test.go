// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Error Messages", Label("e2e"), func() {
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
		if page != nil {
			page.Close()
		}
		if ctx != nil {
			ctx.Close()
		}
	})

	Describe("Dashboard error banner", func() {
		Context("when dashboard API returns an error", func() {
			It("should display an error banner", func() {
				By("Intercepting health-summary API to return 500")
				err := page.Route("**/api/v1/teams/*/dashboard/health-summary*", func(route playwright.Route) {
					err := route.Fulfill(playwright.RouteFulfillOptions{
						Status:      playwright.Int(500),
						ContentType: playwright.String("application/json"),
						Body:        []byte(`{"error":"Internal Server Error"}`),
					})
					Expect(err).NotTo(HaveOccurred())
				})
				Expect(err).NotTo(HaveOccurred())

				// Also intercept other dashboard endpoints to return 500
				err = page.Route("**/api/v1/teams/*/dashboard/response-distribution*", func(route playwright.Route) {
					route.Fulfill(playwright.RouteFulfillOptions{
						Status:      playwright.Int(500),
						ContentType: playwright.String("application/json"),
						Body:        []byte(`{"error":"Internal Server Error"}`),
					})
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.Route("**/api/v1/teams/*/dashboard/individual-responses*", func(route playwright.Route) {
					_ = route.Fulfill(playwright.RouteFulfillOptions{
						Status:      playwright.Int(500),
						ContentType: playwright.String("application/json"),
						Body:        []byte(`{"error":"Internal Server Error"}`),
					})
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.Route("**/api/v1/teams/*/dashboard/trends*", func(route playwright.Route) {
					_ = route.Fulfill(playwright.RouteFulfillOptions{
						Status:      playwright.Int(500),
						ContentType: playwright.String("application/json"),
						Body:        []byte(`{"error":"Internal Server Error"}`),
					})
				})
				Expect(err).NotTo(HaveOccurred())

				By("Logging in as team lead")
				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_lead1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for redirect to dashboard")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/dashboard"))

				By("Verifying error banner is visible")
				banner := page.Locator("[data-testid='dashboard-error-banner']")
				Eventually(func() bool {
					visible, _ := banner.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying error message is user-friendly")
				text, err := banner.TextContent()
				Expect(err).NotTo(HaveOccurred())
				Expect(text).To(ContainSubstring("Unable to load dashboard data"))
			})
		})

		Context("when dashboard loads successfully", func() {
			It("should NOT display an error banner", func() {
				By("Logging in as team lead")
				_, err := page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("input[name='username']").Fill("e2e_lead1")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				By("Waiting for redirect to dashboard")
				Eventually(func() string {
					return page.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/dashboard"))

				By("Waiting for dashboard data to load")
				err = page.Locator("[data-testid='radar-chart-section']").WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying error banner is NOT visible")
				banner := page.Locator("[data-testid='dashboard-error-banner']")
				Consistently(func() bool {
					visible, _ := banner.IsVisible()
					return visible
				}, 2*time.Second, 500*time.Millisecond).Should(BeFalse())
			})
		})
	})
})
