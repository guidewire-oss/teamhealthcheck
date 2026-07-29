// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Admin Branding Settings", Label("e2e", "admin", "branding"), func() {
	var page playwright.Page

	BeforeEach(func() {
		var err error
		page, err = browser.NewPage()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Reset branding to defaults after each test
		_, err := db.Exec("UPDATE app_settings SET company_name = 'My Company', logo_url = NULL WHERE id = 1")
		Expect(err).NotTo(HaveOccurred())
		if page != nil {
			page.Close()
		}
	})

	loginAsAdmin := func() {
		By("Admin logging in")
		_, err := page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='username']").Fill("admin")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='password']").Fill("admin")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("button[type='submit']").Click()
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/admin"))
	}

	navigateToSettings := func() {
		By("Navigating to Settings tab")
		settingsTab := page.Locator("[data-testid='settings-tab']").Or(page.Locator("button:has-text('Settings')"))
		Eventually(func() bool {
			visible, _ := settingsTab.IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

		err := settingsTab.Click()
		Expect(err).NotTo(HaveOccurred())
		time.Sleep(1 * time.Second)
	}

	Describe("Company Branding Configuration", func() {
		Context("when admin sets a company name", func() {
			It("should save the company name and display it on the dashboard", func() {
				loginAsAdmin()
				navigateToSettings()

				By("Waiting for branding section to load")
				brandingSection := page.Locator("[data-testid='branding-settings']")
				Eventually(func() bool {
					visible, _ := brandingSection.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Entering a custom company name")
				companyNameInput := page.Locator("[data-testid='branding-company-name']")
				Eventually(func() bool {
					visible, _ := companyNameInput.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err := companyNameInput.Fill("")
				Expect(err).NotTo(HaveOccurred())
				err = companyNameInput.Fill("Acme Corporation")
				Expect(err).NotTo(HaveOccurred())

				By("Saving settings")
				saveButton := page.Locator("[data-testid='save-settings-btn']").Or(page.Locator("button:has-text('Save Settings')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for save to complete
				time.Sleep(2 * time.Second)

				By("Verifying company name saved in database")
				var dbCompanyName string
				Eventually(func() bool {
					err := db.QueryRow("SELECT company_name FROM app_settings WHERE id = 1").Scan(&dbCompanyName)
					return err == nil && dbCompanyName == "Acme Corporation"
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Company name saved to database: %s\n", dbCompanyName)

				By("Navigating to dashboard to verify branding appears")
				// Log in as a team lead to access the dashboard
				page2, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page2.Close()

				_, err = page2.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page2.Locator("input[name='username']").Fill("e2e_lead1")
				Expect(err).NotTo(HaveOccurred())

				err = page2.Locator("input[name='password']").Fill("demo")
				Expect(err).NotTo(HaveOccurred())

				err = page2.Locator("button[type='submit']").Click()
				Expect(err).NotTo(HaveOccurred())

				Eventually(func() string {
					return page2.URL()
				}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/dashboard"))

				// Verify the custom company name appears on the dashboard
				By("Verifying company name appears on dashboard")
				acmeText := page2.Locator("text=Acme Corporation").First()
				Eventually(func() bool {
					visible, _ := acmeText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Company name 'Acme Corporation' appears on dashboard\n")
			})
		})

		Context("when admin returns to settings after saving", func() {
			It("should show the previously saved company name", func() {
				// Pre-set company name in database
				_, err := db.Exec("INSERT INTO app_settings (id, company_name) VALUES (1, 'Persisted Corp') ON CONFLICT (id) DO UPDATE SET company_name = 'Persisted Corp'")
				Expect(err).NotTo(HaveOccurred())

				loginAsAdmin()
				navigateToSettings()

				By("Verifying company name input is pre-filled with saved value")
				companyNameInput := page.Locator("[data-testid='branding-company-name']")
				Eventually(func() string {
					val, _ := companyNameInput.InputValue()
					return val
				}, 10*time.Second, 500*time.Millisecond).Should(Equal("Persisted Corp"))

				GinkgoWriter.Printf("✅ Saved company name persists across admin visits\n")
			})
		})
	})
})
