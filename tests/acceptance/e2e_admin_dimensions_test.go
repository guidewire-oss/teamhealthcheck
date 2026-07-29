// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Admin Dimension Management", func() {
	var page playwright.Page

	BeforeEach(func() {
		var err error
		page, err = browser.NewPage()
		Expect(err).NotTo(HaveOccurred())

		// Set a reasonable viewport
		err = page.SetViewportSize(1280, 800)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if page != nil {
			page.Close()
		}
	})

	// Helper function to login as admin
	loginAsAdmin := func() {
		By("Logging in as admin")
		_, err := page.Goto(frontendURL + "/login")
		Expect(err).NotTo(HaveOccurred())

		// Fill in admin credentials
		err = page.Locator("input[name='username']").Fill("admin")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("input[name='password']").Fill("admin")
		Expect(err).NotTo(HaveOccurred())

		err = page.Locator("button[type='submit']").Click()
		Expect(err).NotTo(HaveOccurred())

		// Wait for redirect to admin page
		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/admin"))
	}

	// Helper function to navigate to Settings tab
	navigateToSettings := func() {
		By("Navigating to Settings tab")
		settingsTab := page.Locator("[data-testid='settings-tab']")
		err := settingsTab.Click()
		Expect(err).NotTo(HaveOccurred())

		// Wait for settings content to load
		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='dimensions-settings']").IsVisible()
			return visible
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
	}

	Context("when admin views dimensions", func() {
		It("should display all health dimensions from the database", func() {
			loginAsAdmin()
			navigateToSettings()

			By("Verifying dimensions are displayed")
			// Should see dimension rows
			Eventually(func() int {
				rows, _ := page.Locator("[data-testid='dimension-row']").All()
				return len(rows)
			}, 10*time.Second, 500*time.Millisecond).Should(BeNumerically(">=", 11))

			// Verify Mission dimension is visible
			missionDimension := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
				HasText: "Mission",
			})
			Eventually(func() bool {
				visible, _ := missionDimension.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})

	Context("when admin edits a dimension", func() {
		It("should allow editing dimension name and descriptions", func() {
			loginAsAdmin()
			navigateToSettings()

			By("Clicking edit on a dimension")
			// Find the Mission dimension and click edit
			missionRow := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
				HasText: "Mission",
			})
			editBtn := missionRow.Locator("[data-testid='edit-dimension-btn']")
			err := editBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for edit modal to appear")
			Eventually(func() bool {
				visible, _ := page.Locator("[data-testid='dimension-edit-modal']").IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Editing the dimension description")
			// Clear and fill new description
			descInput := page.Locator("[data-testid='dimension-description-input']")
			err = descInput.Fill("Updated description for Mission dimension")
			Expect(err).NotTo(HaveOccurred())

			By("Saving the changes")
			saveBtn := page.Locator("[data-testid='save-dimension-btn']")
			err = saveBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying modal closes and changes are saved")
			Eventually(func() bool {
				visible, _ := page.Locator("[data-testid='dimension-edit-modal']").IsVisible()
				return !visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			// Verify the change persists in the database
			var description string
			err = db.QueryRow("SELECT description FROM health_dimensions WHERE id = 'mission'").Scan(&description)
			Expect(err).NotTo(HaveOccurred())
			Expect(description).To(Equal("Updated description for Mission dimension"))
		})

		It("should allow toggling dimension active status", func() {
			loginAsAdmin()
			navigateToSettings()

			By("Finding a dimension and clicking toggle")
			// Use Fun dimension for this test
			funRow := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
				HasText: "Fun",
			})
			toggleBtn := funRow.Locator("[data-testid='toggle-dimension-btn']")
			err := toggleBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the dimension is now inactive in the database")
			Eventually(func() bool {
				var isActive bool
				err := db.QueryRow("SELECT is_active FROM health_dimensions WHERE id = 'fun'").Scan(&isActive)
				if err != nil {
					return true // Assume active if error
				}
				return !isActive
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Toggling back to active")
			err = toggleBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				var isActive bool
				err := db.QueryRow("SELECT is_active FROM health_dimensions WHERE id = 'fun'").Scan(&isActive)
				if err != nil {
					return false
				}
				return isActive
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})

	Context("when admin creates a new dimension", func() {
		It("should allow adding a new health dimension", func() {
			loginAsAdmin()
			navigateToSettings()

			By("Clicking Add Dimension button")
			addBtn := page.Locator("[data-testid='add-dimension-btn']")
			err := addBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for create modal to appear")
			Eventually(func() bool {
				visible, _ := page.Locator("[data-testid='dimension-create-modal']").IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Filling in the new dimension details")
			err = page.Locator("[data-testid='dimension-name-input']").Fill("E2E Test Dimension")
			Expect(err).NotTo(HaveOccurred())

			err = page.Locator("[data-testid='dimension-description-input']").Fill("A test dimension created by E2E test")
			Expect(err).NotTo(HaveOccurred())

			err = page.Locator("[data-testid='dimension-good-description-input']").Fill("Things are going great")
			Expect(err).NotTo(HaveOccurred())

			err = page.Locator("[data-testid='dimension-bad-description-input']").Fill("Things need improvement")
			Expect(err).NotTo(HaveOccurred())

			By("Saving the new dimension")
			saveBtn := page.Locator("[data-testid='save-dimension-btn']")
			err = saveBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying modal closes")
			Eventually(func() bool {
				visible, _ := page.Locator("[data-testid='dimension-create-modal']").IsVisible()
				return !visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying the dimension appears in the list")
			Eventually(func() bool {
				newDimension := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
					HasText: "E2E Test Dimension",
				})
				visible, _ := newDimension.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying the dimension exists in the database")
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM health_dimensions WHERE name = 'E2E Test Dimension'").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))
		})
	})

	Context("when admin deactivates a dimension", func() {
		BeforeEach(func() {
			// Create a dimension specifically for deactivation test
			_, err := db.Exec(`
				INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight)
				VALUES ('e2e_delete_test', 'E2E Delete Test', 'To be deactivated', 'Good', 'Bad', true, 1.0)
				ON CONFLICT (id) DO UPDATE SET is_active = true
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			// Clean up in case test failed
			db.Exec("DELETE FROM health_dimensions WHERE id = 'e2e_delete_test'")
		})

		It("should soft-delete a dimension by setting is_active to false", func() {
			loginAsAdmin()
			navigateToSettings()

			By("Finding the test dimension and clicking delete")
			deleteRow := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
				HasText: "E2E Delete Test",
			})

			// Wait for the row to be visible
			Eventually(func() bool {
				visible, _ := deleteRow.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			deleteBtn := deleteRow.Locator("[data-testid='delete-dimension-btn']")
			err := deleteBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Confirming deactivation in the dialog")
			confirmBtn := page.Locator("[data-testid='confirm-delete-btn']")
			Eventually(func() bool {
				visible, _ := confirmBtn.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			err = confirmBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the dimension now shows as Inactive in the list")
			// After soft delete, dimension should show as inactive but still be visible
			Eventually(func() bool {
				// Check that the row has the "Inactive" badge
				inactiveBadge := deleteRow.Locator("span:has-text('Inactive')")
				visible, _ := inactiveBadge.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying the dimension is soft-deleted in database (is_active = false)")
			var isActive bool
			err = db.QueryRow("SELECT is_active FROM health_dimensions WHERE id = 'e2e_delete_test'").Scan(&isActive)
			Expect(err).NotTo(HaveOccurred())
			Expect(isActive).To(BeFalse(), "Dimension should be soft-deleted (is_active = false)")

			By("Verifying historical data is preserved - dimension still exists")
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM health_dimensions WHERE id = 'e2e_delete_test'").Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1), "Dimension should still exist in database after soft delete")
		})

		It("should allow reactivating a soft-deleted dimension", func() {
			loginAsAdmin()
			navigateToSettings()

			// First, soft-delete the dimension
			By("Finding and deactivating the test dimension")
			deleteRow := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
				HasText: "E2E Delete Test",
			})

			Eventually(func() bool {
				visible, _ := deleteRow.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			deleteBtn := deleteRow.Locator("[data-testid='delete-dimension-btn']")
			err := deleteBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			confirmBtn := page.Locator("[data-testid='confirm-delete-btn']")
			Eventually(func() bool {
				visible, _ := confirmBtn.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			err = confirmBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			// Wait for deactivation to complete
			Eventually(func() bool {
				inactiveBadge := deleteRow.Locator("span:has-text('Inactive')")
				visible, _ := inactiveBadge.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Clicking the toggle button to reactivate")
			toggleBtn := deleteRow.Locator("[data-testid='toggle-dimension-btn']")
			err = toggleBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the Inactive badge disappears")
			Eventually(func() bool {
				inactiveBadge := deleteRow.Locator("span:has-text('Inactive')")
				visible, _ := inactiveBadge.IsVisible()
				return !visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying the dimension is active again in database")
			var isActive bool
			err = db.QueryRow("SELECT is_active FROM health_dimensions WHERE id = 'e2e_delete_test'").Scan(&isActive)
			Expect(err).NotTo(HaveOccurred())
			Expect(isActive).To(BeTrue(), "Dimension should be reactivated (is_active = true)")
		})
	})

	Context("when editing dimension weight", func() {
		It("should allow changing dimension weight for scoring", func() {
			loginAsAdmin()
			navigateToSettings()

			By("Clicking edit on Learning dimension")
			learningRow := page.Locator("[data-testid='dimension-row']").Filter(playwright.LocatorFilterOptions{
				HasText: "Learning",
			})
			editBtn := learningRow.Locator("[data-testid='edit-dimension-btn']")
			err := editBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Waiting for edit modal")
			Eventually(func() bool {
				visible, _ := page.Locator("[data-testid='dimension-edit-modal']").IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Changing the weight")
			weightInput := page.Locator("[data-testid='dimension-weight-input']")
			err = weightInput.Fill("")
			Expect(err).NotTo(HaveOccurred())
			err = weightInput.Fill("2.5")
			Expect(err).NotTo(HaveOccurred())

			By("Saving the changes")
			saveBtn := page.Locator("[data-testid='save-dimension-btn']")
			err = saveBtn.Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying weight is updated in database")
			Eventually(func() float64 {
				var weight float64
				db.QueryRow("SELECT weight FROM health_dimensions WHERE id = 'learning'").Scan(&weight)
				return weight
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(2.5))
		})
	})
})
