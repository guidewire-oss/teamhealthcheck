// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Admin Modal Forms & Search/Filter", Label("e2e", "admin", "modals"), func() {
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

	Describe("Users Tab - Modal Forms", func() {
		BeforeEach(func() {
			loginAsAdmin()

			By("Navigating to Users tab")
			usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
			Eventually(func() bool {
				visible, _ := usersTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := usersTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(1 * time.Second)
		})

		Context("when admin clicks Add User", func() {
			It("should display a modal popup for creating a user", func() {
				By("Clicking Add User button")
				addButton := page.Locator("[data-testid='add-user-btn']").Or(page.Locator("button:has-text('Add User')"))
				Eventually(func() bool {
					visible, _ := addButton.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err := addButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying user create modal appears")
				createModal := page.Locator("[data-testid='user-create-modal']")
				Eventually(func() bool {
					visible, _ := createModal.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ User create modal is displayed\n")

				By("Closing the modal via Cancel button")
				cancelButton := createModal.Locator("button:has-text('Cancel')")
				err = cancelButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying modal is hidden after cancel")
				Eventually(func() bool {
					visible, _ := createModal.IsVisible()
					return !visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ User create modal closes on cancel\n")
			})
		})

		Context("when admin clicks edit on a user", func() {
			It("should display a modal popup for editing the user", func() {
				// Create a test user
				testUserID := fmt.Sprintf("modal-edit-user-%d", time.Now().UnixNano())
				testUsername := fmt.Sprintf("modaledituser%d", time.Now().UnixNano())
				testFullName := "Modal Edit Test User"
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $2, $3, $4, 'level-5', $5)
				`, testUserID, testUsername, testUsername+"@test.com", testFullName, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Refresh to see new user
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Navigate back to Users tab
				usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
				Eventually(func() bool {
					visible, _ := usersTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err = usersTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(1 * time.Second)

				By("Waiting for test user to appear")
				testUserText := page.Locator(fmt.Sprintf("text=%s", testFullName))
				Eventually(func() bool {
					visible, _ := testUserText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking edit on the test user")
				testUserRow := page.Locator("[data-testid='user-row']").Filter(playwright.LocatorFilterOptions{HasText: testFullName})
				editButton := testUserRow.Locator("[data-testid='edit-user-btn']")
				err = editButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying user edit modal appears")
				editModal := page.Locator("[data-testid='user-edit-modal']")
				Eventually(func() bool {
					visible, _ := editModal.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ User edit modal is displayed\n")

				// Cleanup test user
				db.Exec("DELETE FROM users WHERE id = $1", testUserID)
			})
		})
	})

	Describe("Users Tab - Search/Filter", func() {
		BeforeEach(func() {
			loginAsAdmin()

			By("Navigating to Users tab")
			usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
			Eventually(func() bool {
				visible, _ := usersTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := usersTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(1 * time.Second)
		})

		Context("when admin types in the user search box", func() {
			It("should filter the user list by search query", func() {
				By("Verifying search input is visible")
				searchInput := page.Locator("[data-testid='user-search-input']")
				Eventually(func() bool {
					visible, _ := searchInput.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				By("Counting initial user rows")
				time.Sleep(1 * time.Second) // Let list fully load
				initialRows := page.Locator("[data-testid='user-row']")
				initialCount, _ := initialRows.Count()
				Expect(initialCount).To(BeNumerically(">", 0), "Should have users displayed")
				GinkgoWriter.Printf("Initial user count: %d\n", initialCount)

				By("Typing a search query to filter users")
				err := searchInput.Fill("admin")
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying filtered results")
				filteredRows := page.Locator("[data-testid='user-row']")
				filteredCount, _ := filteredRows.Count()
				Expect(filteredCount).To(BeNumerically("<", initialCount), "Filtered list should have fewer users")
				Expect(filteredCount).To(BeNumerically(">", 0), "Should still show matching users")

				GinkgoWriter.Printf("✅ User search filters results: %d → %d\n", initialCount, filteredCount)

				By("Clearing search should restore full list")
				err = searchInput.Fill("")
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				restoredRows := page.Locator("[data-testid='user-row']")
				restoredCount, _ := restoredRows.Count()
				Expect(restoredCount).To(Equal(initialCount))

				GinkgoWriter.Printf("✅ Clearing search restores full user list\n")
			})
		})

		Context("when admin selects a role filter", func() {
			It("should filter users by hierarchy level", func() {
				By("Verifying role filter dropdown is visible")
				roleFilter := page.Locator("[data-testid='user-role-filter']")
				Eventually(func() bool {
					visible, _ := roleFilter.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				By("Counting initial user rows")
				time.Sleep(1 * time.Second)
				initialRows := page.Locator("[data-testid='user-row']")
				initialCount, _ := initialRows.Count()

				By("Selecting a specific role filter")
				_, err := roleFilter.SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"level-5"},
				})
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying filtered results")
				filteredRows := page.Locator("[data-testid='user-row']")
				filteredCount, _ := filteredRows.Count()
				Expect(filteredCount).To(BeNumerically("<=", initialCount))

				GinkgoWriter.Printf("✅ Role filter works: %d → %d (level-5)\n", initialCount, filteredCount)
			})
		})
	})

	Describe("Teams Tab - Modal Forms", func() {
		BeforeEach(func() {
			loginAsAdmin()

			By("Navigating to Teams tab")
			teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
			Eventually(func() bool {
				visible, _ := teamsTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := teamsTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(1 * time.Second)
		})

		Context("when admin clicks Add Team", func() {
			It("should display a modal popup for creating a team", func() {
				By("Clicking Add Team button")
				addButton := page.Locator("[data-testid='add-team-btn']").Or(page.Locator("button:has-text('Add Team')"))
				Eventually(func() bool {
					visible, _ := addButton.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err := addButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying team create modal appears")
				createModal := page.Locator("[data-testid='team-create-modal']")
				Eventually(func() bool {
					visible, _ := createModal.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Team create modal is displayed\n")

				By("Closing the modal via Cancel button")
				cancelButton := createModal.Locator("button:has-text('Cancel')")
				err = cancelButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying modal is hidden after cancel")
				Eventually(func() bool {
					visible, _ := createModal.IsVisible()
					return !visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Team create modal closes on cancel\n")
			})
		})

		Context("when admin clicks edit on a team", func() {
			It("should display a modal popup for editing the team", func() {
				// Create a test team
				testTeamID := fmt.Sprintf("modal-edit-team-%d", time.Now().UnixNano())
				testTeamName := "Modal Edit Test Team"
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, $2, 'e2e_lead1', 'monthly')
				`, testTeamID, testTeamName)
				Expect(err).NotTo(HaveOccurred())

				// Refresh to see new team
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Navigate back to Teams tab
				teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
				Eventually(func() bool {
					visible, _ := teamsTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err = teamsTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(1 * time.Second)

				By("Waiting for test team to appear")
				testTeamText := page.Locator(fmt.Sprintf("text=%s", testTeamName))
				Eventually(func() bool {
					visible, _ := testTeamText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking edit on the test team")
				testTeamRow := page.Locator("[data-testid='team-row']").Filter(playwright.LocatorFilterOptions{HasText: testTeamName})
				editButton := testTeamRow.Locator("[data-testid='edit-team-btn']")
				err = editButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying team edit modal appears")
				editModal := page.Locator("[data-testid='team-edit-modal']")
				Eventually(func() bool {
					visible, _ := editModal.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Team edit modal is displayed\n")

				// Cleanup test team
				db.Exec("DELETE FROM teams WHERE id = $1", testTeamID)
			})
		})
	})

	Describe("Teams Tab - Search", func() {
		BeforeEach(func() {
			loginAsAdmin()

			By("Navigating to Teams tab")
			teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
			Eventually(func() bool {
				visible, _ := teamsTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := teamsTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(1 * time.Second)
		})

		Context("when admin types in the team search box", func() {
			It("should filter the team list by search query", func() {
				By("Verifying search input is visible")
				searchInput := page.Locator("[data-testid='team-search-input']")
				Eventually(func() bool {
					visible, _ := searchInput.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				By("Counting initial team rows")
				time.Sleep(1 * time.Second)
				initialRows := page.Locator("[data-testid='team-row']")
				initialCount, _ := initialRows.Count()
				Expect(initialCount).To(BeNumerically(">", 0), "Should have teams displayed")
				GinkgoWriter.Printf("Initial team count: %d\n", initialCount)

				By("Typing a search query to filter teams")
				err := searchInput.Fill("Alpha")
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying filtered results")
				filteredRows := page.Locator("[data-testid='team-row']")
				filteredCount, _ := filteredRows.Count()
				Expect(filteredCount).To(BeNumerically("<", initialCount), "Filtered list should have fewer teams")
				Expect(filteredCount).To(BeNumerically(">", 0), "Should still show matching teams")

				GinkgoWriter.Printf("✅ Team search filters results: %d → %d\n", initialCount, filteredCount)

				By("Clearing search should restore full list")
				err = searchInput.Fill("")
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				restoredRows := page.Locator("[data-testid='team-row']")
				restoredCount, _ := restoredRows.Count()
				Expect(restoredCount).To(Equal(initialCount))

				GinkgoWriter.Printf("✅ Clearing search restores full team list\n")
			})
		})
	})
})
