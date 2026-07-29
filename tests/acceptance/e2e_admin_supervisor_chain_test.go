// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/mxschmitt/playwright-go"
)

var _ = Describe("E2E: Admin Supervisor Chain Management", Label("e2e", "admin", "supervisor-chain"), func() {
	var page playwright.Page

	var testTeamID string
	var testTeamName string

	BeforeEach(func() {
		// Generate unique IDs per parallel process to avoid test collisions
		proc := GinkgoParallelProcess()
		testTeamID = fmt.Sprintf("sc_test_%d", proc)
		testTeamName = fmt.Sprintf("SC Test %d", proc)

		var err error
		page, err = browser.NewPage()
		Expect(err).NotTo(HaveOccurred())

		err = page.SetViewportSize(1280, 800)
		Expect(err).NotTo(HaveOccurred())

		// Create hierarchy users for this test: VP -> Director -> Manager -> Team Lead
		for _, q := range []string{
			fmt.Sprintf(`INSERT INTO users (id, username, full_name, email, hierarchy_level_id, password_hash)
				VALUES ('sc_vp_%d', 'sc_vp_%d', 'SC VP %d', 'sc_vp_%d@test.com', 'level-1', '$2a$10$dummyhashfortest000000000000000000000000000000000000')
				ON CONFLICT (id) DO NOTHING`, proc, proc, proc, proc),
			fmt.Sprintf(`INSERT INTO users (id, username, full_name, email, hierarchy_level_id, reports_to, password_hash)
				VALUES ('sc_dir_%d', 'sc_dir_%d', 'SC Director %d', 'sc_dir_%d@test.com', 'level-2', 'sc_vp_%d', '$2a$10$dummyhashfortest000000000000000000000000000000000000')
				ON CONFLICT (id) DO NOTHING`, proc, proc, proc, proc, proc),
			fmt.Sprintf(`INSERT INTO users (id, username, full_name, email, hierarchy_level_id, reports_to, password_hash)
				VALUES ('sc_mgr_%d', 'sc_mgr_%d', 'SC Manager %d', 'sc_mgr_%d@test.com', 'level-3', 'sc_dir_%d', '$2a$10$dummyhashfortest000000000000000000000000000000000000')
				ON CONFLICT (id) DO NOTHING`, proc, proc, proc, proc, proc),
			fmt.Sprintf(`INSERT INTO users (id, username, full_name, email, hierarchy_level_id, reports_to, password_hash)
				VALUES ('sc_lead_%d', 'sc_lead_%d', 'SC Lead %d', 'sc_lead_%d@test.com', 'level-4', 'sc_mgr_%d', '$2a$10$dummyhashfortest000000000000000000000000000000000000')
				ON CONFLICT (id) DO NOTHING`, proc, proc, proc, proc, proc),
		} {
			_, err = db.Exec(q)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		proc := GinkgoParallelProcess()
		// Clean up only this process's data in FK order
		db.Exec(fmt.Sprintf("DELETE FROM team_supervisors WHERE team_id = '%s'", testTeamID))
		db.Exec(fmt.Sprintf("DELETE FROM team_members WHERE team_id = '%s'", testTeamID))
		db.Exec(fmt.Sprintf("DELETE FROM teams WHERE id = '%s'", testTeamID))
		db.Exec(fmt.Sprintf("DELETE FROM users WHERE id IN ('sc_vp_%d', 'sc_dir_%d', 'sc_mgr_%d', 'sc_lead_%d')", proc, proc, proc, proc))

		if page != nil {
			page.Close()
		}
	})

	// Helper function to login as admin
	loginAsAdmin := func() {
		By("Logging in as admin")
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

	// Helper function to navigate to Teams tab
	navigateToTeamsTab := func() {
		By("Navigating to Teams tab")
		teamsTab := page.Locator("[data-testid='teams-tab']").Or(page.Locator("button:has-text('Teams')"))
		Eventually(func() bool {
			visible, _ := teamsTab.IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())

		err := teamsTab.Click()
		Expect(err).NotTo(HaveOccurred())

		// Wait for teams list to load
		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='teams-list']").IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	}

	// Helper function to open the supervisor chain modal for a given team name
	openSupervisorModal := func(teamName string) {
		By("Opening supervisor chain modal for " + teamName)
		teamRow := page.Locator("[data-testid='team-row']").Filter(playwright.LocatorFilterOptions{
			HasText: teamName,
		})
		Eventually(func() bool {
			visible, _ := teamRow.IsVisible()
			return visible
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

		manageBtn := teamRow.Locator("[data-testid='manage-hierarchy-btn']")
		err := manageBtn.Click()
		Expect(err).NotTo(HaveOccurred())

		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='supervisor-chain-modal']").IsVisible()
			return visible
		}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())
	}

	Context("when viewing the supervisor chain for a team with no chain", func() {
		It("should display modal with empty chain message", func() {
			// Create team with no team lead (so no derived chain)
			_, err := db.Exec(`
				INSERT INTO teams (id, name, cadence)
				VALUES ($1, $2, 'monthly')
				ON CONFLICT (id) DO UPDATE SET name = $2, team_lead_id = NULL
			`, testTeamID, testTeamName)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec("DELETE FROM team_supervisors WHERE team_id = $1", testTeamID)
			Expect(err).NotTo(HaveOccurred())

			loginAsAdmin()
			navigateToTeamsTab()
			openSupervisorModal(testTeamName)

			By("Verifying modal shows no chain message")
			noChainMsg := page.Locator("[data-testid='supervisor-chain-modal']").Locator("text=No supervisor chain found")
			Eventually(func() bool {
				visible, _ := noChainMsg.IsVisible()
				return visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			// No supervisor rows should appear
			rows, err := page.Locator("[data-testid='supervisor-row']").All()
			Expect(err).NotTo(HaveOccurred())
			Expect(len(rows)).To(Equal(0))
		})
	})

	Context("when viewing the supervisor chain for a team with a derived chain", func() {
		It("should display the auto-derived supervisor chain in read-only mode", func() {
			proc := GinkgoParallelProcess()
			// Create team with a team lead that has a reports_to chain
			teamLeadID := fmt.Sprintf("sc_lead_%d", proc)
			_, err := db.Exec(`
				INSERT INTO teams (id, name, team_lead_id, cadence)
				VALUES ($1, $2, $3, 'monthly')
				ON CONFLICT (id) DO UPDATE SET name = $2, team_lead_id = $3
			`, testTeamID, testTeamName, teamLeadID)
			Expect(err).NotTo(HaveOccurred())

			// Pre-populate supervisor chain (as the backend derive logic would)
			mgrID := fmt.Sprintf("sc_mgr_%d", proc)
			dirID := fmt.Sprintf("sc_dir_%d", proc)
			vpID := fmt.Sprintf("sc_vp_%d", proc)
			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ($1, $2, 'level-3', 1),
				       ($1, $3, 'level-2', 2),
				       ($1, $4, 'level-1', 3)
				ON CONFLICT DO NOTHING
			`, testTeamID, mgrID, dirID, vpID)
			Expect(err).NotTo(HaveOccurred())

			loginAsAdmin()
			navigateToTeamsTab()
			openSupervisorModal(testTeamName)

			By("Verifying modal displays 3 supervisor rows")
			Eventually(func() int {
				rows, _ := page.Locator("[data-testid='supervisor-row']").All()
				return len(rows)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(3))

			By("Verifying the modal is read-only (no save button, no add button)")
			saveBtn := page.Locator("[data-testid='save-supervisor-chain-btn']")
			saveBtnVisible, _ := saveBtn.IsVisible()
			Expect(saveBtnVisible).To(BeFalse(), "Save button should not appear in read-only modal")

			addBtn := page.Locator("[data-testid='add-supervisor-btn']")
			addBtnVisible, _ := addBtn.IsVisible()
			Expect(addBtnVisible).To(BeFalse(), "Add button should not appear in read-only modal")

			By("Verifying explanation text about auto-derivation")
			helpText := page.Locator("[data-testid='supervisor-chain-modal']").Locator("text=automatically derived")
			Eventually(func() bool {
				visible, _ := helpText.IsVisible()
				return visible
			}, 3*time.Second, 500*time.Millisecond).Should(BeTrue())
		})
	})

	Context("when closing the read-only supervisor chain modal", func() {
		It("should close without modifying the database", func() {
			proc := GinkgoParallelProcess()
			teamLeadID := fmt.Sprintf("sc_lead_%d", proc)
			mgrID := fmt.Sprintf("sc_mgr_%d", proc)

			// Create team with supervisor chain
			_, err := db.Exec(`
				INSERT INTO teams (id, name, team_lead_id, cadence)
				VALUES ($1, $2, $3, 'monthly')
				ON CONFLICT (id) DO UPDATE SET name = $2, team_lead_id = $3
			`, testTeamID, testTeamName, teamLeadID)
			Expect(err).NotTo(HaveOccurred())
			_, err = db.Exec(`
				INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position)
				VALUES ($1, $2, 'level-3', 1)
				ON CONFLICT DO NOTHING
			`, testTeamID, mgrID)
			Expect(err).NotTo(HaveOccurred())

			loginAsAdmin()
			navigateToTeamsTab()
			openSupervisorModal(testTeamName)

			By("Waiting for chain to load")
			Eventually(func() int {
				rows, _ := page.Locator("[data-testid='supervisor-row']").All()
				return len(rows)
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(1))

			By("Clicking the close button")
			err = page.Locator("[data-testid='close-supervisor-modal']").Click()
			Expect(err).NotTo(HaveOccurred())

			By("Verifying modal closes")
			Eventually(func() bool {
				visible, _ := page.Locator("[data-testid='supervisor-chain-modal']").IsVisible()
				return !visible
			}, 5*time.Second, 500*time.Millisecond).Should(BeTrue())

			By("Verifying database still has 1 supervisor")
			var count int
			err = db.QueryRow("SELECT COUNT(*) FROM team_supervisors WHERE team_id = $1", testTeamID).Scan(&count)
			Expect(err).NotTo(HaveOccurred())
			Expect(count).To(Equal(1))
		})
	})

	Context("when a team is created with a team lead via the API", func() {
		It("should auto-derive the supervisor chain from user hierarchy", func() {
			proc := GinkgoParallelProcess()
			teamLeadID := fmt.Sprintf("sc_lead_%d", proc)
			mgrID := fmt.Sprintf("sc_mgr_%d", proc)
			dirID := fmt.Sprintf("sc_dir_%d", proc)
			vpID := fmt.Sprintf("sc_vp_%d", proc)

			loginAsAdmin()

			By("Creating a team with team lead via API")
			// Use the admin API to create a team with a team lead
			apiTeamID := fmt.Sprintf("sc_api_%d_%d", proc, time.Now().UnixNano())
			createScript := fmt.Sprintf(`async () => {
				const token = localStorage.getItem('accessToken');
				const res = await fetch('/api/v1/admin/teams', {
					method: 'POST',
					headers: { 'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token },
					body: JSON.stringify({ id: '%s', name: 'SC API Team %d', teamLeadId: '%s', cadence: 'monthly' })
				});
				return res.status;
			}`, apiTeamID, proc, teamLeadID)

			result, err := page.Evaluate(createScript)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEquivalentTo(201))

			By("Verifying supervisor chain was auto-derived in database")
			Eventually(func() int {
				var count int
				db.QueryRow("SELECT COUNT(*) FROM team_supervisors WHERE team_id = $1", apiTeamID).Scan(&count)
				return count
			}, 5*time.Second, 500*time.Millisecond).Should(Equal(3), "Should have 3 supervisors: Manager, Director, VP")

			By("Verifying the chain contains correct users")
			var userIDs []string
			rows, err := db.Query("SELECT user_id FROM team_supervisors WHERE team_id = $1 ORDER BY position", apiTeamID)
			Expect(err).NotTo(HaveOccurred())
			defer rows.Close()
			for rows.Next() {
				var uid string
				Expect(rows.Scan(&uid)).To(Succeed())
				userIDs = append(userIDs, uid)
			}
			Expect(userIDs).To(Equal([]string{mgrID, dirID, vpID}),
				"Chain should be Manager -> Director -> VP (ordered by position)")

			// Clean up the API-created team
			db.Exec("DELETE FROM team_supervisors WHERE team_id = $1", apiTeamID)
			db.Exec("DELETE FROM teams WHERE id = $1", apiTeamID)
		})
	})
})
