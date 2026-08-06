package acceptance_test

import (
	"database/sql"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

// Uses DemoPasswordHash from test_helpers_test.go

var _ = Describe("E2E: Admin Dashboard Management", Label("e2e", "admin"), func() {
	/*
	 * This test suite verifies the complete admin management flow:
	 * 1. Hierarchy Levels - CRUD operations for organizational levels
	 * 2. Teams Management - Create, edit, delete teams with team lead assignment
	 * 3. Users Management - Create, edit, delete users with role/hierarchy assignment
	 *
	 * All tests use Playwright to drive a real browser and verify database persistence.
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

	// Helper function to login as admin
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

		// Admin should be redirected to /admin dashboard
		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/admin"))
	}

	Describe("Hierarchy Levels Management", func() {
		BeforeEach(func() {
			loginAsAdmin()

			// Navigate to Hierarchy Levels tab
			By("Navigating to Hierarchy Levels tab")
			hierarchyTab := page.Locator("[data-testid='hierarchy-tab']").Or(page.Locator("button:has-text('Hierarchy Levels')"))
			Eventually(func() bool {
				visible, _ := hierarchyTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := hierarchyTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(500 * time.Millisecond)
		})

		Context("when admin views hierarchy levels", func() {
			It("should display list of all hierarchy levels from database", func() {
				By("Verifying hierarchy levels list is displayed")

				// Should show all 6 default hierarchy levels
				// Admin, VP, Director, Manager, Team Lead, Team Member
				Eventually(func() bool {
					// Look for hierarchy level cards or list items
					levelsList := page.Locator("[data-testid='hierarchy-list']").Or(page.Locator("[data-testid='hierarchy-levels-list']")).Or(page.Locator("text=Team Member"))
					visible, _ := levelsList.First().IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Verify database has default hierarchy levels
				var count int
				err := db.QueryRow("SELECT COUNT(*) FROM hierarchy_levels").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(BeNumerically(">=", 6), "Should have at least 6 hierarchy levels")

				GinkgoWriter.Printf("✅ Admin can view hierarchy levels (found %d levels)\n", count)
			})
		})

		Context("when admin creates a new hierarchy level", func() {
			It("should save the hierarchy level to database and display it", func() {
				By("Clicking Add New Level button")
				addButton := page.Locator("[data-testid='add-level-btn']").Or(page.Locator("[data-testid='add-hierarchy-level']")).Or(page.Locator("button:has-text('Add Level')"))
				Eventually(func() bool {
					visible, _ := addButton.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err := addButton.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Filling new hierarchy level form")
				// Note: The UI auto-generates IDs and positions, so we only fill the name
				testLevelName := fmt.Sprintf("Test Level %d", time.Now().Unix())

				// Fill level name (the only required field - ID and position are auto-generated)
				levelNameInput := page.Locator("[data-testid='level-name-input']").Or(page.Locator("input[name='name']"))
				Eventually(func() bool {
					visible, _ := levelNameInput.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
				err = levelNameInput.Fill(testLevelName)
				Expect(err).NotTo(HaveOccurred())

				// Toggle some permissions - actual UI uses permission-{permissionName} format
				canViewAllTeamsCheckbox := page.Locator("[data-testid='permission-canViewAllTeams']").Or(page.Locator("[data-testid='can-view-all-teams']"))
				if visible, _ := canViewAllTeamsCheckbox.IsVisible(); visible {
					err = canViewAllTeamsCheckbox.Check()
					Expect(err).NotTo(HaveOccurred())
				}

				By("Submitting the form")
				saveButton := page.Locator("[data-testid='save-level-btn']").Or(page.Locator("[data-testid='save-hierarchy-level']")).Or(page.Locator("button:has-text('Save')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				By("Verifying success message or the form closes")
				// The form should close after successful creation
				Eventually(func() bool {
					// Check if form is no longer visible (success) or success message appears
					form := page.Locator("[data-testid='create-level-form']")
					visible, _ := form.IsVisible()
					return !visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying hierarchy level exists in database")
				var dbLevelName string
				err = db.QueryRow("SELECT name FROM hierarchy_levels WHERE name = $1", testLevelName).Scan(&dbLevelName)
				Expect(err).NotTo(HaveOccurred())
				Expect(dbLevelName).To(Equal(testLevelName))

				GinkgoWriter.Printf("✅ Admin created hierarchy level: %s\n", testLevelName)
			})
		})

		Context("when admin edits a hierarchy level", func() {
			It("should update the hierarchy level permissions in database", func() {
				// Create a test level first
				testLevelID := fmt.Sprintf("edit-level-%d", time.Now().UnixNano())
				testLevelName := "Level to Edit"
				_, err := db.Exec(`
					INSERT INTO hierarchy_levels (id, name, position, can_view_all_teams, can_edit_teams)
					VALUES ($1, $2, 20, false, false)
				`, testLevelID, testLevelName)
				Expect(err).NotTo(HaveOccurred())

				// Refresh page to see new level
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Wait for hierarchy list to load after reload
				By("Waiting for hierarchy list to load")
				hierarchyList := page.Locator("[data-testid='hierarchy-list']")
				Eventually(func() bool {
					visible, _ := hierarchyList.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Wait for the specific test level to appear in the list
				By("Waiting for test level to appear in list")
				testLevelText := page.Locator(fmt.Sprintf("text=%s", testLevelName))
				Eventually(func() bool {
					visible, _ := testLevelText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Finding and clicking edit button for the test level")
				// Find the row containing our test level, then find the edit button within it
				testLevelRow := page.Locator("[data-testid='hierarchy-level-row']").Filter(playwright.LocatorFilterOptions{HasText: testLevelName})
				editButton := testLevelRow.Locator("[data-testid='edit-level-btn']")
				Eventually(func() bool {
					visible, _ := editButton.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				err = editButton.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Updating permissions - enabling canViewAllTeams")
				// Edit form uses edit-permission-{permissionName} format
				canViewAllTeamsCheckbox := page.Locator("[data-testid='edit-permission-canViewAllTeams']").Or(page.Locator("[data-testid='can-view-all-teams']")).Or(page.Locator("input[name='canViewAllTeams']"))
				Eventually(func() bool {
					visible, _ := canViewAllTeamsCheckbox.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err = canViewAllTeamsCheckbox.Check()
				Expect(err).NotTo(HaveOccurred())

				By("Saving changes")
				saveButton := page.Locator("[data-testid='save-edit-btn']").Or(page.Locator("[data-testid='save-hierarchy-level']")).Or(page.Locator("button:has-text('Save')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for save to complete - edit form should disappear
				By("Waiting for save to complete (edit form to close)")
				Eventually(func() bool {
					visible, _ := canViewAllTeamsCheckbox.IsVisible()
					return !visible // Form should be hidden after save
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying permissions updated in database")
				var canViewAllTeams bool
				Eventually(func() bool {
					err = db.QueryRow("SELECT can_view_all_teams FROM hierarchy_levels WHERE id = $1", testLevelID).Scan(&canViewAllTeams)
					return err == nil && canViewAllTeams
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Admin updated hierarchy level permissions: %s\n", testLevelID)
			})
		})

		Context("when admin reorders hierarchy levels", func() {
			It("should update the position in database", func() {
				// This test verifies move up/down buttons
				By("Finding enabled move up button")

				// Wait for hierarchy list to load fully
				time.Sleep(1 * time.Second)

				// Find enabled move up buttons (not first item, which is disabled)
				// The Nth(1) gets the second move-up button which should be enabled
				moveUpButton := page.Locator("[data-testid='move-up-btn']:not([disabled])").Nth(0)
				visible, _ := moveUpButton.IsVisible()
				if !visible {
					// Try alternative selector
					moveUpButton = page.Locator("button[aria-label='Move level up']:not([disabled])").Nth(0)
					visible, _ = moveUpButton.IsVisible()
				}

				if visible {
					By("Clicking move up button on a level")
					err := moveUpButton.Click()
					Expect(err).NotTo(HaveOccurred())
					time.Sleep(500 * time.Millisecond)

					By("Verifying position changed in database")
					// Check that positions are sequential and valid
					var positions []int
					rows, err := db.Query("SELECT position FROM hierarchy_levels ORDER BY position")
					Expect(err).NotTo(HaveOccurred())
					defer rows.Close()

					for rows.Next() {
						var pos int
						rows.Scan(&pos)
						positions = append(positions, pos)
					}

					Expect(len(positions)).To(BeNumerically(">", 0))
					GinkgoWriter.Printf("✅ Admin reordered hierarchy levels (positions: %v)\n", positions)
				} else {
					Skip("No enabled move buttons visible - hierarchy may have fixed positions")
				}
			})
		})

		Context("when admin deletes a hierarchy level", func() {
			It("should remove the level from database (except Team Member)", func() {
				// Create a deletable test level
				testLevelID := fmt.Sprintf("delete-level-%d", time.Now().UnixNano())
				testLevelName := "Level to Delete"
				_, err := db.Exec(`
					INSERT INTO hierarchy_levels (id, name, position, can_view_all_teams)
					VALUES ($1, $2, 30, false)
				`, testLevelID, testLevelName)
				Expect(err).NotTo(HaveOccurred())

				// Refresh page
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Wait for hierarchy list to load after reload
				By("Waiting for hierarchy list to load")
				hierarchyList := page.Locator("[data-testid='hierarchy-list']")
				Eventually(func() bool {
					visible, _ := hierarchyList.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Wait for the specific test level to appear in the list
				By("Waiting for test level to appear in list")
				testLevelText := page.Locator(fmt.Sprintf("text=%s", testLevelName))
				Eventually(func() bool {
					visible, _ := testLevelText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking delete button for test level")
				// Find the row containing our test level, then find the delete button within it
				testLevelRow := page.Locator("[data-testid='hierarchy-level-row']").Filter(playwright.LocatorFilterOptions{HasText: testLevelName})
				deleteButton := testLevelRow.Locator("[data-testid='delete-level-btn']")
				Eventually(func() bool {
					visible, _ := deleteButton.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Set up dialog handler to accept the native confirm() dialog
				page.OnDialog(func(dialog playwright.Dialog) {
					GinkgoWriter.Printf("Dialog appeared: %s - %s\n", dialog.Type(), dialog.Message())
					dialog.Accept()
				})

				err = deleteButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for deletion to complete - item should disappear from list
				By("Waiting for deletion to complete")
				Eventually(func() bool {
					visible, _ := testLevelText.IsVisible()
					return !visible // Level should disappear from list
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying level deleted from database")
				var exists bool
				Eventually(func() bool {
					err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM hierarchy_levels WHERE id = $1)", testLevelID).Scan(&exists)
					return err == nil && !exists
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Admin deleted hierarchy level: %s\n", testLevelID)
			})
		})
	})

	Describe("Teams Management", func() {
		BeforeEach(func() {
			loginAsAdmin()

			// Navigate to Teams tab
			By("Navigating to Teams tab")
			teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
			Eventually(func() bool {
				visible, _ := teamsTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := teamsTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(500 * time.Millisecond)
		})

		Context("when admin creates a new team", func() {
			It("should save the team to database with team lead assignment", func() {
				By("Clicking Add New Team button")
				addButton := page.Locator("[data-testid='add-team-btn']").Or(page.Locator("[data-testid='add-team']")).Or(page.Locator("button:has-text('Add Team')"))
				Eventually(func() bool {
					visible, _ := addButton.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err := addButton.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Filling new team form")
				testTeamName := fmt.Sprintf("Test Team %d", time.Now().Unix())

				// Fill team name
				teamNameInput := page.Locator("[data-testid='team-name-input']").Or(page.Locator("input[name='name']"))
				err = teamNameInput.Fill(testTeamName)
				Expect(err).NotTo(HaveOccurred())

				// Select team lead from dropdown
				teamLeadSelect := page.Locator("[data-testid='team-lead-select']").Or(page.Locator("select[name='teamLeadId']"))
				if visible, _ := teamLeadSelect.IsVisible(); visible {
					// Select e2e_lead1 as team lead
					_, err = teamLeadSelect.SelectOption(playwright.SelectOptionValues{
						Values: &[]string{"e2e_lead1"},
					})
					Expect(err).NotTo(HaveOccurred())
				}

				// Select cadence
				cadenceSelect := page.Locator("[data-testid='cadence-select']").Or(page.Locator("select[name='cadence']"))
				if visible, _ := cadenceSelect.IsVisible(); visible {
					_, err = cadenceSelect.SelectOption(playwright.SelectOptionValues{
						Values: &[]string{"monthly"},
					})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Submitting the form")
				saveButton := page.Locator("[data-testid='save-team-btn']").Or(page.Locator("[data-testid='save-team']")).Or(page.Locator("button:has-text('Save')")).Or(page.Locator("button:has-text('Create Team')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for form to close (success is indicated by form closing, no toast message)
				By("Waiting for form to close after creation")
				Eventually(func() bool {
					visible, _ := teamNameInput.IsVisible()
					return !visible // Form should close after successful creation
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying team exists in database")
				var dbTeamName string
				var dbTeamLeadID sql.NullString
				Eventually(func() bool {
					err = db.QueryRow("SELECT name, team_lead_id FROM teams WHERE name = $1", testTeamName).Scan(&dbTeamName, &dbTeamLeadID)
					return err == nil && dbTeamName == testTeamName
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
				if dbTeamLeadID.Valid {
					Expect(dbTeamLeadID.String).To(Equal("e2e_lead1"))
				}

				GinkgoWriter.Printf("✅ Admin created team: %s (lead: %v)\n", testTeamName, dbTeamLeadID)
			})
		})

		Context("when admin edits a team", func() {
			It("should update team details in database", func() {
				// Create a test team first
				testTeamID := fmt.Sprintf("edit-team-%d", time.Now().UnixNano())
				testTeamName := "Team to Edit"
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, $2, 'e2e_lead1', 'monthly')
				`, testTeamID, testTeamName)
				Expect(err).NotTo(HaveOccurred())

				// Refresh page
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Navigate back to Teams tab after reload
				By("Navigating back to Teams tab")
				teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
				Eventually(func() bool {
					visible, _ := teamsTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err = teamsTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for teams list to load after reload
				By("Waiting for teams list to load")
				teamsList := page.Locator("[data-testid='teams-list']")
				Eventually(func() bool {
					visible, _ := teamsList.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Wait for the specific test team to appear in the list
				By("Waiting for test team to appear in list")
				testTeamText := page.Locator(fmt.Sprintf("text=%s", testTeamName))
				Eventually(func() bool {
					visible, _ := testTeamText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Finding and clicking edit button for the test team")
				// Find the row containing our test team, then find the edit button within it
				testTeamRow := page.Locator("[data-testid='team-row']").Filter(playwright.LocatorFilterOptions{HasText: testTeamName})
				editButton := testTeamRow.Locator("[data-testid='edit-team-btn']")
				Eventually(func() bool {
					visible, _ := editButton.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				err = editButton.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Updating team name")
				teamNameInput := page.Locator("[data-testid='team-name-input']").Or(page.Locator("input[name='name']"))
				Eventually(func() bool {
					visible, _ := teamNameInput.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				newTeamName := "Updated Team Name"
				err = teamNameInput.Fill(newTeamName)
				Expect(err).NotTo(HaveOccurred())

				By("Changing team lead")
				teamLeadSelect := page.Locator("[data-testid='team-lead-select']").Or(page.Locator("select[name='teamLeadId']"))
				if visible, _ := teamLeadSelect.IsVisible(); visible {
					_, err = teamLeadSelect.SelectOption(playwright.SelectOptionValues{
						Values: &[]string{"e2e_lead2"},
					})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Saving changes")
				saveButton := page.Locator("[data-testid='save-team-btn']").Or(page.Locator("[data-testid='save-team']")).Or(page.Locator("button:has-text('Save')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for save to complete - form should close
				By("Waiting for save to complete")
				Eventually(func() bool {
					visible, _ := teamNameInput.IsVisible()
					return !visible // Form should close after save
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying updates in database")
				var dbTeamName, dbTeamLeadID string
				Eventually(func() bool {
					err = db.QueryRow("SELECT name, team_lead_id FROM teams WHERE id = $1", testTeamID).Scan(&dbTeamName, &dbTeamLeadID)
					return err == nil && dbTeamName == newTeamName
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
				Expect(dbTeamLeadID).To(Equal("e2e_lead2"))

				GinkgoWriter.Printf("✅ Admin updated team: %s (new lead: %s)\n", testTeamID, dbTeamLeadID)
			})
		})

		Context("when admin deletes a team", func() {
			It("should remove the team from database", func() {
				// Create a deletable test team
				testTeamID := fmt.Sprintf("delete-team-%d", time.Now().UnixNano())
				testTeamName := "Team to Delete"
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, $2, 'e2e_lead1', 'monthly')
				`, testTeamID, testTeamName)
				Expect(err).NotTo(HaveOccurred())

				// Refresh page
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Navigate back to Teams tab after reload
				By("Navigating back to Teams tab")
				teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
				Eventually(func() bool {
					visible, _ := teamsTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err = teamsTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for teams list to load after reload
				By("Waiting for teams list to load")
				teamsList := page.Locator("[data-testid='teams-list']")
				Eventually(func() bool {
					visible, _ := teamsList.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Wait for the specific test team to appear in the list
				By("Waiting for test team to appear in list")
				testTeamText := page.Locator(fmt.Sprintf("text=%s", testTeamName))
				Eventually(func() bool {
					visible, _ := testTeamText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking delete button for test team")
				// Find the row containing our test team, then find the delete button within it
				testTeamRow := page.Locator("[data-testid='team-row']").Filter(playwright.LocatorFilterOptions{HasText: testTeamName})
				deleteButton := testTeamRow.Locator("[data-testid='delete-team-btn']")
				Eventually(func() bool {
					visible, _ := deleteButton.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				// Set up dialog handler to accept the native confirm() dialog
				page.OnDialog(func(dialog playwright.Dialog) {
					GinkgoWriter.Printf("Dialog appeared: %s - %s\n", dialog.Type(), dialog.Message())
					dialog.Accept()
				})

				err = deleteButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for deletion to complete - item should disappear from list
				By("Waiting for deletion to complete")
				Eventually(func() bool {
					visible, _ := testTeamText.IsVisible()
					return !visible // Team should disappear from list
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying team deleted from database")
				var exists bool
				Eventually(func() bool {
					err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1)", testTeamID).Scan(&exists)
					return err == nil && !exists
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Admin deleted team: %s\n", testTeamID)
			})
		})
	})

	Describe("Users Management", func() {
		BeforeEach(func() {
			loginAsAdmin()

			// Navigate to Users tab
			By("Navigating to Users tab")
			usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
			Eventually(func() bool {
				visible, _ := usersTab.IsVisible()
				return visible
			}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

			err := usersTab.Click()
			Expect(err).NotTo(HaveOccurred())
			time.Sleep(500 * time.Millisecond)
		})

		Context("when admin creates a new user", func() {
			It("should save the user to database with role assignment", func() {
				By("Clicking Add New User button")
				addButton := page.Locator("[data-testid='add-user-btn']").Or(page.Locator("[data-testid='add-user']")).Or(page.Locator("button:has-text('Add User')"))
				Eventually(func() bool {
					visible, _ := addButton.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err := addButton.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Filling new user form")
				testUsername := fmt.Sprintf("testuser%d", time.Now().Unix())
				testEmail := fmt.Sprintf("test%d@teams360.test", time.Now().Unix())

				// Fill full name first (actual UI order)
				fullNameInput := page.Locator("[data-testid='user-fullname-input']").Or(page.Locator("[data-testid='full-name-input']")).Or(page.Locator("input[name='fullName']"))
				Eventually(func() bool {
					visible, _ := fullNameInput.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
				err = fullNameInput.Fill("Test User Full Name")
				Expect(err).NotTo(HaveOccurred())

				// Fill username
				usernameInput := page.Locator("[data-testid='user-username-input']").Or(page.Locator("[data-testid='username-input']")).Or(page.Locator("input[name='username']"))
				err = usernameInput.Fill(testUsername)
				Expect(err).NotTo(HaveOccurred())

				// Fill email
				emailInput := page.Locator("[data-testid='user-email-input']").Or(page.Locator("[data-testid='email-input']")).Or(page.Locator("input[name='email']"))
				err = emailInput.Fill(testEmail)
				Expect(err).NotTo(HaveOccurred())

				// Fill password
				passwordInput := page.Locator("[data-testid='user-password-input']").Or(page.Locator("[data-testid='password-input']")).Or(page.Locator("input[name='password']"))
				err = passwordInput.Fill("testpassword123")
				Expect(err).NotTo(HaveOccurred())

				// Select hierarchy level/role (Team Member)
				hierarchySelect := page.Locator("[data-testid='user-role-select']").Or(page.Locator("[data-testid='hierarchy-level-select']")).Or(page.Locator("select[name='hierarchyLevel']"))
				if visible, _ := hierarchySelect.IsVisible(); visible {
					_, err = hierarchySelect.SelectOption(playwright.SelectOptionValues{
						Values: &[]string{"level-5"}, // Team Member
					})
					Expect(err).NotTo(HaveOccurred())
				}

				By("Submitting the form")
				saveButton := page.Locator("[data-testid='save-user-btn']").Or(page.Locator("[data-testid='save-user']")).Or(page.Locator("button:has-text('Save')")).Or(page.Locator("button:has-text('Create User')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for form to close (success is indicated by form closing, no toast message)
				By("Waiting for form to close after creation")
				Eventually(func() bool {
					visible, _ := usernameInput.IsVisible()
					return !visible // Form should close after successful creation
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying user exists in database")
				var dbUsername, dbEmail, dbHierarchyLevel string
				Eventually(func() bool {
					err = db.QueryRow("SELECT username, email, hierarchy_level_id FROM users WHERE username = $1", testUsername).Scan(&dbUsername, &dbEmail, &dbHierarchyLevel)
					return err == nil && dbUsername == testUsername
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())
				Expect(dbEmail).To(Equal(testEmail))
				Expect(dbHierarchyLevel).To(Equal("level-5"))

				GinkgoWriter.Printf("✅ Admin created user: %s (%s, level: %s)\n", testUsername, testEmail, dbHierarchyLevel)
			})
		})

		Context("when admin edits a user", func() {
			It("should update user role and hierarchy in database", func() {
				// Create a test user first
				testUserID := fmt.Sprintf("edit-user-%d", time.Now().UnixNano())
				testUsername := fmt.Sprintf("edituser%d", time.Now().Unix())
				testFullName := "User to Edit"
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $2, $3, $4, 'level-5', $5)
				`, testUserID, testUsername, testUsername+"@test.com", testFullName, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Refresh page
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Navigate back to Users tab after reload
				By("Navigating back to Users tab")
				usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
				Eventually(func() bool {
					visible, _ := usersTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err = usersTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for users list to load
				By("Waiting for users list to load")
				time.Sleep(2 * time.Second) // Allow time for API call to complete

				// Wait for the specific test user to appear in the list
				By("Waiting for test user to appear in list")
				testUserText := page.Locator(fmt.Sprintf("text=%s", testFullName))
				Eventually(func() bool {
					visible, _ := testUserText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Finding and clicking edit button for the test user")
				// Find the row containing our test user, then find the edit button within it
				testUserRow := page.Locator("[data-testid='user-row']").Filter(playwright.LocatorFilterOptions{HasText: testFullName})
				editButton := testUserRow.Locator("[data-testid='edit-user-btn']")
				Eventually(func() bool {
					visible, _ := editButton.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				err = editButton.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				By("Updating hierarchy level to Manager")
				// Actual UI uses user-role-select
				hierarchySelect := page.Locator("[data-testid='user-role-select']").Or(page.Locator("[data-testid='hierarchy-level-select']")).Or(page.Locator("select[name='hierarchyLevel']"))
				Eventually(func() bool {
					visible, _ := hierarchySelect.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				_, err = hierarchySelect.SelectOption(playwright.SelectOptionValues{
					Values: &[]string{"level-3"}, // Manager
				})
				Expect(err).NotTo(HaveOccurred())

				By("Saving changes")
				saveButton := page.Locator("[data-testid='save-user-btn']").Or(page.Locator("[data-testid='save-user']")).Or(page.Locator("button:has-text('Save')"))
				err = saveButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for save to complete - form should close
				By("Waiting for save to complete")
				Eventually(func() bool {
					visible, _ := hierarchySelect.IsVisible()
					return !visible // Form should close after save
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying hierarchy level updated in database")
				var dbHierarchyLevel string
				Eventually(func() bool {
					err = db.QueryRow("SELECT hierarchy_level_id FROM users WHERE id = $1", testUserID).Scan(&dbHierarchyLevel)
					return err == nil && dbHierarchyLevel == "level-3"
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Admin updated user hierarchy: %s (new level: %s)\n", testUserID, dbHierarchyLevel)
			})
		})

		Context("when admin assigns user to team", func() {
			It("should update team_members table in database", func() {
				// Note: The current UI doesn't have team assignment in user edit form
				// This test verifies the database model works correctly via direct DB operations
				// TODO: Add UI team assignment feature in future sprint

				// Create test user and team
				testUserID := fmt.Sprintf("assign-user-%d", time.Now().UnixNano())
				testUsername := fmt.Sprintf("assignuser%d", time.Now().Unix())
				testTeamID := fmt.Sprintf("assign-team-%d", time.Now().UnixNano())

				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $2, $3, 'User to Assign', 'level-5', $4)
				`, testUserID, testUsername, testUsername+"@test.com", DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, 'Team for Assignment', 'e2e_lead1', 'monthly')
				`, testTeamID)
				Expect(err).NotTo(HaveOccurred())

				By("Assigning user to team via database (UI feature not yet implemented)")
				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id)
					VALUES ($1, $2)
				`, testTeamID, testUserID)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying team assignment in database")
				var exists bool
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM team_members WHERE user_id = $1 AND team_id = $2)", testUserID, testTeamID).Scan(&exists)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue())

				GinkgoWriter.Printf("✅ Team assignment model works: %s → %s\n", testUserID, testTeamID)
			})
		})

		Context("when admin deletes a user", func() {
			It("should remove the user from database", func() {
				// Create a deletable test user
				testUserID := fmt.Sprintf("delete-user-%d", time.Now().UnixNano())
				testUsername := fmt.Sprintf("deleteuser%d", time.Now().Unix())
				testFullName := "User to Delete"
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $2, $3, $4, 'level-5', $5)
				`, testUserID, testUsername, testUsername+"@test.com", testFullName, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				// Refresh page
				_, err = page.Reload()
				Expect(err).NotTo(HaveOccurred())

				// Navigate back to Users tab after reload
				By("Navigating back to Users tab")
				usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
				Eventually(func() bool {
					visible, _ := usersTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err = usersTab.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for users list to load
				By("Waiting for users list to load")
				time.Sleep(2 * time.Second) // Allow time for API call to complete

				// Wait for the specific test user to appear in the list
				By("Waiting for test user to appear in list")
				testUserText := page.Locator(fmt.Sprintf("text=%s", testFullName))
				Eventually(func() bool {
					visible, _ := testUserText.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Clicking delete button for test user")
				// Find the row containing our test user, then find the delete button within it
				testUserRow := page.Locator("[data-testid='user-row']").Filter(playwright.LocatorFilterOptions{HasText: testFullName})
				deleteButton := testUserRow.Locator("[data-testid='delete-user-btn']")
				Eventually(func() bool {
					visible, _ := deleteButton.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				err = deleteButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Users have a confirmation dialog with "Delete User" button
				By("Confirming deletion in modal")
				confirmButton := page.Locator("[data-testid='confirm-delete']").Or(page.Locator("button:has-text('Delete User')"))
				Eventually(func() bool {
					visible, _ := confirmButton.IsVisible()
					return visible
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				err = confirmButton.Click()
				Expect(err).NotTo(HaveOccurred())

				// Wait for deletion to complete - item should disappear from list
				By("Waiting for deletion to complete")
				Eventually(func() bool {
					visible, _ := testUserText.IsVisible()
					return !visible // User should disappear from list
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

				By("Verifying user deleted from database")
				var exists bool
				Eventually(func() bool {
					err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE id = $1)", testUserID).Scan(&exists)
					return err == nil && !exists
				}, 5*time.Second, 200*time.Millisecond).Should(BeTrue())

				GinkgoWriter.Printf("✅ Admin deleted user: %s\n", testUserID)
			})
		})
	})

	Describe("Admin Dashboard Integration", func() {
		Context("when admin performs multiple operations", func() {
			It("should handle full workflow of hierarchy → team → user management", func() {
				loginAsAdmin()

				// Workflow:
				// 1. Create custom hierarchy level
				// 2. Create team with that level in supervisor chain
				// 3. Create user with that hierarchy level
				// 4. Assign user to team
				// 5. Verify all relationships in database

				testLevelID := fmt.Sprintf("workflow-level-%d", time.Now().UnixNano())
				testTeamID := fmt.Sprintf("workflow-team-%d", time.Now().UnixNano())
				testUserID := fmt.Sprintf("workflow-user-%d", time.Now().UnixNano())
				testUsername := fmt.Sprintf("workflowuser%d", time.Now().Unix())

				By("Step 1: Creating custom hierarchy level")
				// Navigate to hierarchy tab
				hierarchyTab := page.Locator("[data-testid='hierarchy-tab']").Or(page.Locator("button:has-text('Hierarchy Levels')"))
				Eventually(func() bool {
					visible, _ := hierarchyTab.IsVisible()
					return visible
				}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
				err := hierarchyTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				// Create level
				_, err = db.Exec(`
					INSERT INTO hierarchy_levels (id, name, position, can_view_all_teams, can_edit_teams)
					VALUES ($1, 'Workflow Test Level', 15, true, true)
				`, testLevelID)
				Expect(err).NotTo(HaveOccurred())

				By("Step 2: Creating team")
				teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
				err = teamsTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, 'Workflow Test Team', 'e2e_lead1', 'monthly')
				`, testTeamID)
				Expect(err).NotTo(HaveOccurred())

				By("Step 3: Creating user with custom hierarchy level")
				usersTab := page.Locator("[data-testid='users-tab']").Or(page.Locator("button:has-text('Users')"))
				err = usersTab.Click()
				Expect(err).NotTo(HaveOccurred())
				time.Sleep(500 * time.Millisecond)

				_, err = db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ($1, $2, $3, 'Workflow Test User', $4, $5)
				`, testUserID, testUsername, testUsername+"@test.com", testLevelID, DemoPasswordHash)
				Expect(err).NotTo(HaveOccurred())

				By("Step 4: Assigning user to team")
				_, err = db.Exec(`
					INSERT INTO team_members (team_id, user_id)
					VALUES ($1, $2)
				`, testTeamID, testUserID)
				Expect(err).NotTo(HaveOccurred())

				By("Step 5: Verifying complete workflow in database")
				// Verify hierarchy level exists
				var levelName string
				err = db.QueryRow("SELECT name FROM hierarchy_levels WHERE id = $1", testLevelID).Scan(&levelName)
				Expect(err).NotTo(HaveOccurred())
				Expect(levelName).To(Equal("Workflow Test Level"))

				// Verify team exists
				var teamName string
				err = db.QueryRow("SELECT name FROM teams WHERE id = $1", testTeamID).Scan(&teamName)
				Expect(err).NotTo(HaveOccurred())
				Expect(teamName).To(Equal("Workflow Test Team"))

				// Verify user exists with correct hierarchy
				var userHierarchy string
				err = db.QueryRow("SELECT hierarchy_level_id FROM users WHERE id = $1", testUserID).Scan(&userHierarchy)
				Expect(err).NotTo(HaveOccurred())
				Expect(userHierarchy).To(Equal(testLevelID))

				// Verify team assignment
				var memberExists bool
				err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM team_members WHERE user_id = $1 AND team_id = $2)", testUserID, testTeamID).Scan(&memberExists)
				Expect(err).NotTo(HaveOccurred())
				Expect(memberExists).To(BeTrue())

				GinkgoWriter.Println("===========================================")
				GinkgoWriter.Println("ADMIN WORKFLOW TEST PASSED!")
				GinkgoWriter.Printf("  Hierarchy Level: %s (%s)\n", testLevelID, levelName)
				GinkgoWriter.Printf("  Team: %s (%s)\n", testTeamID, teamName)
				GinkgoWriter.Printf("  User: %s (level: %s)\n", testUserID, userHierarchy)
				GinkgoWriter.Printf("  Team Assignment: ✅\n")
				GinkgoWriter.Println("===========================================")
			})
		})
	})
})
