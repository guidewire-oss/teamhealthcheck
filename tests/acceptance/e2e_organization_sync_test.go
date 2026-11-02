package acceptance_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Organization Snapshot Contract v1.0 fixes the endpoint, the credential
// header and the two environment variable names for EVERY conforming provider
// -- see docs/organization-snapshot-contract.md. They are spelled out
// literally here because the acceptance suite is its own Go module and does
// not depend on the backend module.
const (
	providerBaseURLEnv   = "DATA_PROVIDER_BASE_URL"
	providerTokenEnv     = "DATA_PROVIDER_API_TOKEN"
	providerSnapshotPath = "/org-snapshot"

	// providerFixtureToken is the only credential the fixture accepts. The
	// backend process is started with exactly this value, so asserting the
	// fixture received it proves the running server read its configuration and
	// sent the credential the contract requires.
	providerFixtureToken = "e2e-provider-fixture-token"
)

// The records providerFixtureSnapshot imports. These are deliberately generic
// contract data, not a copy of any particular vendor's payload: the point of a
// provider-neutral contract is that this shape is all Team Health Check ever
// sees. Keep these in step with the JSON below.
const (
	syncFixtureTeamID   = "e2e-sync-team"
	syncFixtureTeamName = "Provider Imported Team"
	syncFixtureLeadID   = "e2e-sync-lead"
	syncFixtureMemberID = "e2e-sync-member"
)

// providerFixtureSnapshot is a minimal but complete v1.0 snapshot: two users on
// levels this deployment configures, one team, and the membership rows that
// place both users on it. It intentionally mirrors the example in
// docs/organization-snapshot-contract.md so the two can be read side by side.
const providerFixtureSnapshot = `{
  "contractVersion": "1.0",
  "generatedAt": "2026-01-15T10:00:00Z",
  "users": [
    {
      "id": "e2e-sync-lead",
      "username": "e2esynclead",
      "displayName": "Sync Fixture Lead",
      "email": "e2esynclead@example.com",
      "hierarchyLevelId": "level-4"
    },
    {
      "id": "e2e-sync-member",
      "username": "e2esyncmember",
      "displayName": "Sync Fixture Member",
      "email": "e2esyncmember@example.com",
      "hierarchyLevelId": "level-5",
      "reportsToId": "e2e-sync-lead"
    }
  ],
  "teams": [
    {
      "id": "e2e-sync-team",
      "name": "Provider Imported Team",
      "healthCheckEnabled": true,
      "teamLeadId": "e2e-sync-lead"
    }
  ],
  "memberships": [
    {"userId": "e2e-sync-lead", "teamId": "e2e-sync-team"},
    {"userId": "e2e-sync-member", "teamId": "e2e-sync-team"}
  ]
}`

var (
	providerFixtureServer *httptest.Server

	// providerFixtureMu guards the recorded request details: the fixture is
	// written by its own HTTP goroutines and read from specs.
	providerFixtureMu     sync.Mutex
	providerFixtureAPIKey string
	providerFixturePath   string
	providerFixtureHits   int
)

// startOrgProviderFixture boots a stand-in organization data provider and
// returns its base URL.
//
// This MUST be called before the backend process starts: cmd/api/main.go reads
// the provider configuration once at startup (dataprovider.LoadConfig), so a
// provider that appears later is invisible to the running server.
func startOrgProviderFixture() string {
	providerFixtureServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		providerFixtureMu.Lock()
		providerFixtureAPIKey = r.Header.Get("x-api-key")
		providerFixturePath = r.URL.Path
		providerFixtureHits++
		providerFixtureMu.Unlock()

		// A conforming provider serves the snapshot at exactly one path and
		// accepts exactly one credential scheme. Behaving strictly here is what
		// makes the spec below meaningful: a backend that called the wrong path
		// or sent the wrong header would get an error, not a snapshot.
		if r.URL.Path != providerSnapshotPath {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("x-api-key") != providerFixtureToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(providerFixtureSnapshot))
	}))

	return providerFixtureServer.URL
}

// stopOrgProviderFixture shuts the stand-in provider down.
func stopOrgProviderFixture() {
	if providerFixtureServer != nil {
		providerFixtureServer.Close()
	}
}

// lastProviderRequest reports what the fixture last received.
func lastProviderRequest() (apiKey, path string, hits int) {
	providerFixtureMu.Lock()
	defer providerFixtureMu.Unlock()
	return providerFixtureAPIKey, providerFixturePath, providerFixtureHits
}

// Serial, because a sync is authoritative and destructive: every user and team
// that is neither protected (orgprovider.protectedUserIDs/protectedTeamIDs) nor
// present in the snapshot is hard-deleted. Running this alongside another spec
// could delete the fixtures that spec is in the middle of using.
//
// Ordered, because these specs share one expensive action: the first spec
// performs the sync through the UI and the rest assert on what it did.
var _ = Describe("E2E: Organization Provider Sync", Serial, Ordered, Label("e2e", "admin", "sync"), func() {
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

		Expect(page.Locator("input[name='username']").Fill("admin")).To(Succeed())
		Expect(page.Locator("input[name='password']").Fill("admin")).To(Succeed())
		Expect(page.Locator("button[type='submit']").Click()).To(Succeed())

		Eventually(func() string {
			return page.URL()
		}, 10*time.Second, 500*time.Millisecond).Should(ContainSubstring("/admin"))
	}

	countRowsForSync := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	It("imports the provider's users, teams and memberships when an admin clicks Sync Now", func() {
		loginAsAdmin()

		By("Opening the Settings tab")
		settingsTab := page.Locator("[data-testid='settings-tab']")
		Eventually(func() bool {
			visible, _ := settingsTab.IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		Expect(settingsTab.Click()).To(Succeed())

		By("Waiting for the data provider panel to load its settings")
		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='data-provider-settings']").IsVisible()
			return visible
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())

		// The button is disabled until the backend reports readyToSync, which it
		// only does when BOTH provider environment variables were present at
		// startup. An enabled button is therefore end-to-end proof that the
		// running server picked the configuration up.
		By("Verifying the provider is reported as ready to sync")
		syncButton := page.Locator("[data-testid='sync-now-btn']")
		Eventually(func() bool {
			enabled, _ := syncButton.IsEnabled()
			return enabled
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"Sync Now should be enabled once DATA_PROVIDER_BASE_URL and DATA_PROVIDER_API_TOKEN are configured")

		By("Clicking Sync Now")
		Expect(syncButton.Click()).To(Succeed())

		By("Waiting for the sync result panel")
		result := page.Locator("[data-testid='sync-result']")
		Eventually(func() bool {
			visible, _ := result.IsVisible()
			return visible
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"a successful sync should report its counts rather than an error")

		By("Verifying the reported counts match the snapshot")
		counts, err := page.Locator("[data-testid='sync-counts']").TextContent()
		Expect(err).NotTo(HaveOccurred())
		Expect(counts).To(ContainSubstring("Users: 2 added"))
		// The membership total also counts removals of rows other specs left in the
		// shared database, so only its presence is pinned here; the exact rows are
		// verified against team_members below.
		Expect(counts).To(MatchRegexp(`Membership changes: \d+`))

		// A safe sync must be exactly as it always was: no hold warning, no
		// Sync Anyway affordance, and no override reported on the result. The
		// held path itself cannot be provoked here -- this suite starts the
		// backend with ORG_SYNC_MAX_DELETE_PERCENT=100 (see suite_test.go) and
		// the threshold is process-level configuration, so a held sync would
		// need a second backend. That path is covered in the backend
		// integration suite; what this spec pins down is that adding the
		// override left the ordinary sync untouched.
		By("Verifying a safe sync shows no mass-deletion hold and no override")
		holdVisible, _ := page.Locator("[data-testid='sync-mass-deletion-hold']").IsVisible()
		Expect(holdVisible).To(BeFalse(), "a sync under the threshold must not be held")
		anywayVisible, _ := page.Locator("[data-testid='sync-anyway-btn']").IsVisible()
		Expect(anywayVisible).To(BeFalse(), "Sync Anyway is only offered for a held sync")
		overrideVisible, _ := page.Locator("[data-testid='sync-override-applied']").IsVisible()
		Expect(overrideVisible).To(BeFalse(), "a normal sync is not an override")

		By("Verifying the users landed in the database")
		Expect(countRowsForSync(
			`SELECT COUNT(*) FROM users WHERE id IN ($1, $2)`, syncFixtureLeadID, syncFixtureMemberID,
		)).To(Equal(2))

		var reportsTo sql.NullString
		Expect(db.QueryRow(
			`SELECT reports_to FROM users WHERE id = $1`, syncFixtureMemberID,
		).Scan(&reportsTo)).To(Succeed())
		Expect(reportsTo.Valid).To(BeTrue(), "the reporting line from the snapshot should be applied")
		Expect(reportsTo.String).To(Equal(syncFixtureLeadID))

		By("Verifying the team landed with its name, lead and health-check flag")
		var name string
		var teamLead sql.NullString
		var healthCheckEnabled bool
		Expect(db.QueryRow(
			`SELECT name, team_lead_id, health_check_enabled FROM teams WHERE id = $1`, syncFixtureTeamID,
		).Scan(&name, &teamLead, &healthCheckEnabled)).To(Succeed())
		Expect(name).To(Equal(syncFixtureTeamName))
		Expect(teamLead.String).To(Equal(syncFixtureLeadID))
		Expect(healthCheckEnabled).To(BeTrue())

		By("Verifying both memberships landed")
		Expect(countRowsForSync(
			`SELECT COUNT(*) FROM team_members WHERE team_id = $1`, syncFixtureTeamID,
		)).To(Equal(2))
	})

	It("sends the configured credential to the contract's snapshot endpoint", func() {
		// The fixture rejects any other path or credential, so the sync above
		// could not have succeeded otherwise -- but asserting it explicitly is
		// what pins the outbound half of the contract. This is only observable
		// from a true end-to-end run: an in-process test router never exercises
		// the configuration the real server loads at startup.
		apiKey, path, hits := lastProviderRequest()

		Expect(hits).To(BeNumerically(">", 0), "the backend should have called the provider")
		Expect(path).To(Equal(providerSnapshotPath))
		Expect(apiKey).To(Equal(providerFixtureToken))
	})

	It("leaves the protected demo and E2E fixture records untouched", func() {
		// A sync hard-deletes every non-protected record missing from the
		// snapshot, and the snapshot above names none of these. They survive
		// only because orgprovider's protected allowlists cover them -- if that
		// protection regresses, the rest of this acceptance suite loses the
		// users and teams it is seeded with, so guard it here explicitly.
		By("Verifying the seeded E2E users survived")
		// Named individually rather than counted by prefix: other specs create
		// their own e2e_-prefixed users at runtime, and those are NOT protected,
		// so a sync legitimately removes them. What must hold is that the nine
		// users this suite seeds are all still here.
		for _, userID := range []string{
			"e2e_manager1", "e2e_testmanager1", "e2e_lead1", "e2e_lead2",
			"e2e_demo", "e2e_member1", "e2e_member2", "e2e_member3", "e2e_fresh_member",
		} {
			Expect(countRowsForSync(`SELECT COUNT(*) FROM users WHERE id = $1`, userID)).To(Equal(1),
				userID+" must survive a sync that never mentions it")
		}

		By("Verifying the seeded E2E teams survived")
		for _, teamID := range []string{"e2e_team1", "e2e_team2", "e2e_team3"} {
			Expect(countRowsForSync(`SELECT COUNT(*) FROM teams WHERE id = $1`, teamID)).To(Equal(1),
				teamID+" must survive a sync that never mentions it")
		}

		By("Verifying their memberships survived the membership reconciliation")
		Expect(countRowsForSync(
			`SELECT COUNT(*) FROM team_members WHERE team_id IN ('e2e_team1', 'e2e_team2', 'e2e_team3')`,
		)).To(BeNumerically(">", 0), "protected teams must keep the memberships they were seeded with")

		By("Verifying the permanent admin account survived")
		Expect(countRowsForSync(`SELECT COUNT(*) FROM users WHERE id = 'admin'`)).To(Equal(1))
	})

	It("shows the imported team to an admin in the Teams tab", func() {
		loginAsAdmin()

		By("Opening the Teams tab")
		teamsTab := page.Locator("[data-testid='teams-tab']")
		Eventually(func() bool {
			visible, _ := teamsTab.IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		Expect(teamsTab.Click()).To(Succeed())

		By("Verifying the synced team is rendered")
		importedTeam := page.Locator("text=" + syncFixtureTeamName).First()
		Eventually(func() bool {
			visible, _ := importedTeam.IsVisible()
			return visible
		}, 20*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"a team imported from the provider should be visible to an admin, not just present in the database")
	})
})
