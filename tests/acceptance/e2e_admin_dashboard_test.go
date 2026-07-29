// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Admin Dashboard", func() {
	var testAdminID = "e2e_admin"

	BeforeEach(func() {
		// Ensure admin user exists
		_, _ = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
			($1, 'e2e_admin', 'e2e_admin@teams360.demo', 'E2E Admin User', 'level-admin', NULL, $2)
			ON CONFLICT (id) DO NOTHING
		`, testAdminID, DemoPasswordHash)
	})

	Describe("Hierarchy Tab", func() {
		Context("when Admin manages hierarchy levels", func() {
			It("should display list of hierarchy levels with up/down arrows for reordering", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				// Admin should be redirected to admin page
				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Hierarchy tab is active or clickable")
				hierarchyTab := page.Locator("[data-testid='hierarchy-tab'], button:has-text('Hierarchy')")
				err = hierarchyTab.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = hierarchyTab.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying hierarchy levels are displayed")
				levelRows := page.Locator("[data-testid='hierarchy-level-row'], tr:has(td)")
				err = levelRows.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				count, err := levelRows.Count()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">=", 4), "Should display at least 4 hierarchy levels (VP, Director, Manager, Team Lead)")

				By("Verifying up/down reorder arrows exist")
				upArrows := page.Locator("[data-testid='move-up'], button:has-text('↑'), [aria-label*='up'], svg[data-lucide='arrow-up']")
				count, _ = upArrows.Count()
				Expect(count).To(BeNumerically(">=", 1), "Should have up arrows for reordering")

				downArrows := page.Locator("[data-testid='move-down'], button:has-text('↓'), [aria-label*='down'], svg[data-lucide='arrow-down']")
				count, _ = downArrows.Count()
				Expect(count).To(BeNumerically(">=", 1), "Should have down arrows for reordering")

				By("Verifying permissions column exists")
				permissionsColumn := page.Locator("text=Permissions").Or(page.Locator("text=Access")).Or(page.Locator("text=Can View"))
				err = permissionsColumn.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Hierarchy tab displayed successfully\n")
			})

			It("should allow editing hierarchy level permissions", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to Hierarchy tab")
				hierarchyTab := page.Locator("[data-testid='hierarchy-tab'], button:has-text('Hierarchy')")
				_ = hierarchyTab.First().Click()
				time.Sleep(500 * time.Millisecond)

				By("Clicking edit on a hierarchy level")
				editButton := page.Locator("[data-testid='edit-level-btn'], [data-testid='edit-level'], button:has-text('Edit')")
				err = editButton.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = editButton.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying edit modal/form appears")
				editForm := page.Locator("[data-testid='edit-level-form'], [role='dialog'], form")
				err = editForm.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Hierarchy level editing works\n")
			})
		})
	})

	Describe("Teams Tab", func() {
		Context("when Admin manages teams", func() {
			It("should display list of teams with cadence and next check date", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking on Teams tab")
				teamsTab := page.Locator("[data-testid='teams-tab'], button:has-text('Teams')")
				err = teamsTab.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = teamsTab.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying teams are displayed in table/list")
				teamRows := page.Locator("[data-testid='team-row'], tr:has(td), [data-testid='team-card']")
				err = teamRows.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Cadence column exists")
				cadenceColumn := page.Locator("th:has-text('Cadence')").Or(page.Locator("text=Cadence")).Or(page.Locator("text=Frequency"))
				err = cadenceColumn.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying Team Lead column exists")
				teamLeadColumn := page.Locator("th:has-text('Team Lead')").Or(page.Locator("th:has-text('Lead')"))
				err = teamLeadColumn.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Teams tab displayed successfully\n")
			})

			It("should allow creating a new team", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to Teams tab")
				teamsTab := page.Locator("[data-testid='teams-tab'], button:has-text('Teams')")
				_ = teamsTab.First().Click()
				time.Sleep(500 * time.Millisecond)

				By("Clicking Add Team button")
				addButton := page.Locator("[data-testid='add-team-btn'], [data-testid='add-team'], button:has-text('Add Team'), button:has-text('Create Team'), button:has-text('New Team')")
				err = addButton.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = addButton.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying create team form appears")
				createForm := page.Locator("[data-testid='create-team-form'], [data-testid='team-create-modal'], [role='dialog'], form:has(input)")
				err = createForm.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying form has required fields")
				nameField := page.Locator("input[name='name'], input[placeholder*='name'], #team-name")
				err = nameField.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(3000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Team creation form works\n")
			})
		})
	})

	Describe("Users Tab", func() {
		Context("when Admin manages users", func() {
			It("should display list of users with role badges", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking on Users tab")
				usersTab := page.Locator("[data-testid='users-tab'], button:has-text('Users')")
				err = usersTab.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = usersTab.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying users are displayed")
				userRows := page.Locator("[data-testid='user-row'], tr:has(td), [data-testid='user-card']")
				err = userRows.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying role badges are displayed")
				roleBadges := page.Locator("[data-testid='role-badge']").Or(page.Locator(".badge")).Or(page.Locator("text=VP")).Or(page.Locator("text=Director")).Or(page.Locator("text=Manager"))
				count, err := roleBadges.Count()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">=", 1), "Should display role badges")

				GinkgoWriter.Printf("Users tab displayed successfully\n")
			})

			It("should allow creating a new user", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to Users tab")
				usersTab := page.Locator("[data-testid='users-tab'], button:has-text('Users')")
				_ = usersTab.First().Click()
				time.Sleep(500 * time.Millisecond)

				By("Clicking Add User button")
				addButton := page.Locator("[data-testid='add-user-btn'], [data-testid='add-user'], button:has-text('Add User'), button:has-text('Create User'), button:has-text('New User')")
				err = addButton.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = addButton.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying create user form appears")
				createForm := page.Locator("[data-testid='create-user-form'], [data-testid='user-create-modal'], [role='dialog'], form:has(input)")
				err = createForm.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("User creation form works\n")
			})

			It("should allow editing a user", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to Users tab")
				usersTab := page.Locator("[data-testid='users-tab'], button:has-text('Users')")
				_ = usersTab.First().Click()
				time.Sleep(500 * time.Millisecond)

				By("Clicking edit on a user")
				editButton := page.Locator("[data-testid='edit-user-btn'], [data-testid='edit-user'], button:has-text('Edit')")
				err = editButton.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = editButton.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying edit form appears")
				editForm := page.Locator("[data-testid='create-user-form'], [data-testid='user-edit-modal']").Filter(playwright.LocatorFilterOptions{HasText: "Edit User"})
				err = editForm.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(5000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("User editing works\n")
			})
		})
	})

	Describe("Settings Tab", func() {
		Context("when Admin configures system settings", func() {
			It("should display health dimensions configuration", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Clicking on Settings tab")
				settingsTab := page.Locator("[data-testid='settings-tab'], button:has-text('Settings')")
				err = settingsTab.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())
				err = settingsTab.First().Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Verifying Health Dimensions section exists")
				dimensionsSection := page.Locator("[data-testid='dimensions-settings']").Or(page.Locator("text=Health Dimensions")).Or(page.Locator("text=Dimensions Configuration"))
				err = dimensionsSection.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying dimension list is displayed")
				dimensionList := page.Locator("[data-testid='dimension-row']").Or(page.Locator("text=Mission")).Or(page.Locator("text=Value")).Or(page.Locator("text=Speed"))
				count, err := dimensionList.Count()
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">=", 1), "Should display health dimensions")

				GinkgoWriter.Printf("Settings - Dimensions displayed successfully\n")
			})

			It("should display notification settings", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to Settings tab")
				settingsTab := page.Locator("[data-testid='settings-tab'], button:has-text('Settings')")
				_ = settingsTab.First().Click()
				time.Sleep(500 * time.Millisecond)

				By("Verifying Notifications section exists")
				notificationsSection := page.Locator("[data-testid='notifications-settings']").Or(page.Locator("text=Notifications")).Or(page.Locator("text=Email Settings")).Or(page.Locator("text=Reminders"))
				err = notificationsSection.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Settings - Notifications displayed successfully\n")
			})

			It("should display data retention settings", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Navigating to Settings tab")
				settingsTab := page.Locator("[data-testid='settings-tab'], button:has-text('Settings')")
				_ = settingsTab.First().Click()
				time.Sleep(500 * time.Millisecond)

				By("Verifying Data Retention section exists")
				retentionSection := page.Locator("[data-testid='retention-settings']").Or(page.Locator("text=Data Retention")).Or(page.Locator("text=Retention Policy")).Or(page.Locator("text=Data Management"))
				err = retentionSection.First().WaitFor(playwright.LocatorWaitForOptions{
					State:   playwright.WaitForSelectorStateVisible,
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				GinkgoWriter.Printf("Settings - Data Retention displayed successfully\n")
			})
		})
	})

	Describe("Admin Cannot Take Surveys", func() {
		Context("when Admin tries to access survey page", func() {
			It("should redirect Admin away from survey page", func() {
				By("Logging in as Admin")
				page, err := browser.NewPage()
				Expect(err).NotTo(HaveOccurred())
				defer page.Close()

				_, err = page.Goto(frontendURL + "/login")
				Expect(err).NotTo(HaveOccurred())

				err = page.Locator("#username").Fill("e2e_admin")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("#password").Fill("demo")
				Expect(err).NotTo(HaveOccurred())
				err = page.Locator("button[type='submit']:has-text('Sign In')").Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for redirect to admin page
				err = page.WaitForURL("**/admin", playwright.PageWaitForURLOptions{
					Timeout: playwright.Float(10000),
				})
				Expect(err).NotTo(HaveOccurred())

				By("Attempting to navigate to survey page")
				_, err = page.Goto(frontendURL + "/survey")
				Expect(err).NotTo(HaveOccurred())

				// Wait a moment for potential redirect
				time.Sleep(2 * time.Second)

				By("Verifying Admin is NOT on survey page")
				currentURL := page.URL()
				// Admin should be redirected away from /survey
				// They should be on /admin or see an access denied message
				if currentURL == frontendURL+"/survey" {
					// If on survey page, check for access denied message
					accessDenied := page.Locator("text=Access Denied").Or(page.Locator("text=Not Authorized")).Or(page.Locator("text=Admin cannot"))
					count, _ := accessDenied.Count()
					Expect(count).To(BeNumerically(">=", 1), "Should show access denied message for admin on survey page")
				} else {
					Expect(currentURL).NotTo(ContainSubstring("/survey"), "Admin should be redirected away from survey")
				}

				GinkgoWriter.Printf("Admin survey restriction verified\n")
			})
		})
	})
})
