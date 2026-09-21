package acceptance_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// thresholdFixtureIDPrefix marks the throwaway users this spec seeds directly
// in the database to manufacture a mass-deletion hold. They are deliberately
// NOT part of providerFixtureSnapshot (e2e_organization_sync_test.go), so a
// sync proposes deleting every one of them, and deliberately use a dash
// (matching the sync fixture's own "e2e-sync-*" convention, not the
// underscore "e2e_*" convention the rest of this suite's protected/seeded
// cast uses) so cleanup and other specs' prefix-based assertions never
// confuse the two.
const thresholdFixtureIDPrefix = "e2e-threshold-user-"

// nonProtectedUserPredicate mirrors orgprovider.IsProtectedUser's rule as a SQL
// WHERE clause. This suite is its own Go module and does not depend on the
// backend module (see the comment on providerBaseURLEnv in
// e2e_organization_sync_test.go), so the rule is restated here rather than
// imported -- keep it in step with domain/orgprovider/orgprovider.go if that
// ever changes.
const nonProtectedUserPredicate = `
	id != 'admin' AND hierarchy_level_id != 'level-admin' AND id NOT IN (
		'vp', 'director1', 'director2', 'manager1', 'manager2', 'manager3',
		'teamlead1', 'teamlead2', 'teamlead3', 'teamlead4', 'teamlead5',
		'alice', 'bob', 'carol', 'david', 'eve', 'demo',
		'test-vp', 'test-director', 'test-manager', 'test-lead', 'test-member1', 'test-member2',
		'e2e_manager1', 'e2e_testmanager1', 'e2e_lead1', 'e2e_lead2',
		'e2e_demo', 'e2e_member1', 'e2e_member2', 'e2e_member3', 'e2e_fresh_member'
	)`

// thresholdEndpoint is the deletion-threshold settings route under test.
const thresholdEndpoint = "/api/v1/admin/settings/organization-provider/deletion-threshold"

// suiteBaselineDeletePercent restores what this whole acceptance suite
// actually relies on: suite_test.go starts the backend with
// ORG_SYNC_MAX_DELETE_PERCENT=100 specifically so every OTHER spec's ordinary
// syncs -- against a shared database that dozens of unrelated spec files add
// throwaway users to over a full run -- are never accidentally held. This
// spec deliberately lowers the threshold to provoke a hold, and unlike most
// per-file fixtures, the deletion threshold is a persisted, process-wide
// admin setting, not something scoped to this file or even this browser
// session. Leaving it lowered after this Describe block finishes would keep
// poisoning every later spec's real syncs for the rest of the process's
// life.
const suiteBaselineDeletePercent = 100

// resetDeletionThreshold restores suiteBaselineDeletePercent via a plain HTTP
// call, independent of any browser page (so it works from an AfterAll, after
// every page in this Describe block has already closed). It logs in itself
// rather than reusing a token from a closed page, and is written to never
// panic on failure -- a best-effort cleanup that swallows its own errors would
// hide a broken suite, but this one reports failures through Expect the same
// way every other assertion in this file does.
func resetDeletionThreshold() {
	loginBody, err := json.Marshal(map[string]string{"username": "admin", "password": "admin"})
	Expect(err).NotTo(HaveOccurred())

	loginResp, err := http.Post(backendURL+"/api/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	Expect(err).NotTo(HaveOccurred())
	defer loginResp.Body.Close()
	Expect(loginResp.StatusCode).To(Equal(http.StatusOK), "cleanup login must succeed to restore the shared threshold")

	var login struct {
		AccessToken string `json:"accessToken"`
	}
	Expect(json.NewDecoder(loginResp.Body).Decode(&login)).To(Succeed())

	resetBody, err := json.Marshal(map[string]int{"maxDeletePercent": suiteBaselineDeletePercent})
	Expect(err).NotTo(HaveOccurred())

	req, err := http.NewRequest(http.MethodPut, backendURL+thresholdEndpoint, bytes.NewReader(resetBody))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)

	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	// 409 means a sync is still running or held when cleanup runs -- which
	// this file's own specs should never leave behind, since the last one
	// resolves its own hold via Sync Anyway. Fail loudly rather than leaving
	// a lowered threshold in place for the rest of the suite.
	Expect(resp.StatusCode).To(Equal(http.StatusOK),
		"failed to restore the shared deletion threshold to its suite baseline -- later specs' ordinary syncs may now be held")
}

// loginAsAdminOnPage logs the given page in as admin. It takes the page
// explicitly (rather than closing over a package-level var, the pattern the
// rest of this suite's per-file loginAsAdmin closures use) because this spec
// needs two independent pages open at once to prove a second tab observes the
// same persisted value.
func loginAsAdminOnPage(p playwright.Page) {
	By("Admin logging in")
	_, err := p.Goto(frontendURL + "/login")
	Expect(err).NotTo(HaveOccurred())

	Expect(p.Locator("input[name='username']").Fill("admin")).To(Succeed())
	Expect(p.Locator("input[name='password']").Fill("admin")).To(Succeed())
	Expect(p.Locator("button[type='submit']").Click()).To(Succeed())

	// 30s, not the 10s the rest of this suite uses elsewhere: when this spec is
	// run in isolation (e.g. via -ginkgo.focus), it is the first request /admin
	// ever receives in this process, and Next.js dev mode compiles a route on
	// its first hit rather than ahead of time. A cold compile of this page has
	// been observed to take 15s+; a 10s budget is only safe once something
	// earlier in a full suite run has already warmed it.
	Eventually(func() string {
		return p.URL()
	}, 90*time.Second, 500*time.Millisecond).Should(ContainSubstring("/admin"))
}

// openThresholdSettingsOnPage navigates to Settings and waits for the
// deletion-threshold card to finish its initial load.
func openThresholdSettingsOnPage(p playwright.Page) {
	By("Opening the Settings tab")
	settingsTab := p.Locator("[data-testid='settings-tab']")
	Eventually(func() bool {
		visible, _ := settingsTab.IsVisible()
		return visible
	}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	Expect(settingsTab.Click()).To(Succeed())

	By("Waiting for the deletion-threshold card to load its current value")
	Eventually(func() bool {
		visible, _ := p.Locator("[data-testid='mass-deletion-threshold-settings']").IsVisible()
		return visible
	}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
	Eventually(func() bool {
		visible, _ := p.Locator("[data-testid='threshold-loading']").IsVisible()
		return visible
	}, 15*time.Second, 500*time.Millisecond).Should(BeFalse(),
		"the threshold card should not still be showing its initial loading state")
}

func thresholdInputValueOnPage(p playwright.Page) string {
	value, err := p.Locator("[data-testid='threshold-input']").InputValue()
	Expect(err).NotTo(HaveOccurred())
	return value
}

// Serial and Ordered for the same reason as the sync suite above it: a sync is
// destructive, and this spec's later steps depend on the hold the earlier ones
// provoke. It runs against the same provider fixture (providerFixtureSnapshot,
// providerFixtureToken) that e2e_organization_sync_test.go starts -- the
// backend process is shared across this whole test binary -- but exercises a
// different concern: the admin-configurable deletion threshold persisting
// across reload and freezing while a sync is running or held.
var _ = Describe("E2E: Organization Sync Deletion Threshold", Serial, Ordered, Label("e2e", "admin", "sync"), func() {
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

	// Runs once, after every spec in this Ordered container, regardless of
	// which ones passed or failed -- see resetDeletionThreshold's doc comment
	// for why this cross-spec-file cleanup is not optional.
	AfterAll(func() {
		resetDeletionThreshold()
	})

	countRowsForThreshold := func(query string, args ...any) int {
		var n int
		Expect(db.QueryRow(query, args...).Scan(&n)).To(Succeed())
		return n
	}

	// seedDeletableUsers inserts n throwaway, non-protected users that the
	// provider fixture never reports, so a sync proposes deleting every one of
	// them.
	seedDeletableUsers := func(n int) {
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("%s%d", thresholdFixtureIDPrefix, i)
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ($1, $1, $2, $1, 'level-5', $3)
				ON CONFLICT (id) DO NOTHING
			`, id, id+"@test.com", DemoPasswordHash)
			Expect(err).NotTo(HaveOccurred())
		}
	}

	It("saves a threshold that survives a reload, in this tab and in a second one", func() {
		loginAsAdminOnPage(page)
		openThresholdSettingsOnPage(page)

		By("Reading whatever threshold is currently in force")
		Expect(thresholdInputValueOnPage(page)).NotTo(BeEmpty())

		By("Setting the threshold to 10% and saving it")
		Expect(page.Locator("[data-testid='threshold-preset-10']").Click()).To(Succeed())
		Expect(page.Locator("[data-testid='threshold-save-btn']").Click()).To(Succeed())

		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='threshold-save-success']").IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		Expect(thresholdInputValueOnPage(page)).To(Equal("10"))

		By("Reloading the page")
		_, err := page.Reload()
		Expect(err).NotTo(HaveOccurred())
		openThresholdSettingsOnPage(page)

		By("Verifying the reloaded page still shows 10%, not the 20% default")
		Expect(thresholdInputValueOnPage(page)).To(Equal("10"),
			"a saved threshold must survive a reload rather than reverting to the built-in default")

		By("Verifying a second, independent page/tab agrees")
		secondPage, err := browser.NewPage()
		Expect(err).NotTo(HaveOccurred())
		defer secondPage.Close()
		loginAsAdminOnPage(secondPage)
		openThresholdSettingsOnPage(secondPage)
		Expect(thresholdInputValueOnPage(secondPage)).To(Equal("10"),
			"a second tab must read the same persisted value, not a locally-remembered one")
	})

	It("holds a sync that exceeds the saved threshold, locks the controls, and survives a reload", func() {
		loginAsAdminOnPage(page)
		openThresholdSettingsOnPage(page)

		// The threshold from the previous spec (10%) is still in force. Seed
		// enough deletable users that a sync unambiguously exceeds it: this
		// spec runs alongside many other specs that create their own
		// e2e_-prefixed users, so the exact current eligible population is not
		// something this spec controls. Doubling whatever is currently eligible
		// guarantees roughly 50% deleted -- comfortably over the 10% threshold
		// regardless of what else the suite has seeded by this point.
		existing := countRowsForThreshold(`SELECT COUNT(*) FROM users WHERE ` + nonProtectedUserPredicate)
		seedCount := existing
		if seedCount < 10 {
			seedCount = 10
		}
		seedDeletableUsers(seedCount)

		By("Clicking Sync Now")
		syncButton := page.Locator("[data-testid='sync-now-btn']")
		Eventually(func() bool {
			enabled, _ := syncButton.IsEnabled()
			return enabled
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		Expect(syncButton.Click()).To(Succeed())

		By("Waiting for the mass-deletion hold banner")
		holdBanner := page.Locator("[data-testid='sync-mass-deletion-hold']")
		Eventually(func() bool {
			visible, _ := holdBanner.IsVisible()
			return visible
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"a sync proposing to delete roughly half of the eligible users should be held, not applied")

		By("Verifying the threshold card is now locked")
		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='threshold-locked-notice']").IsVisible()
			return visible
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
		for _, testID := range []string{"threshold-slider", "threshold-input", "threshold-save-btn", "threshold-preset-50"} {
			disabled, err := page.Locator("[data-testid='" + testID + "']").IsDisabled()
			Expect(err).NotTo(HaveOccurred())
			Expect(disabled).To(BeTrue(), testID+" must be disabled while a sync is held for review")
		}

		By("Verifying the hold banner itself explains the lock")
		lockText, err := page.Locator("[data-testid='sync-hold-threshold-locked']").TextContent()
		Expect(err).NotTo(HaveOccurred())
		Expect(lockText).To(ContainSubstring("frozen"))

		By("Verifying nothing was actually deleted")
		Expect(countRowsForThreshold(
			`SELECT COUNT(*) FROM users WHERE id LIKE $1`, thresholdFixtureIDPrefix+"%",
		)).To(Equal(seedCount), "a held sync must write nothing")

		By("Reloading the page while the hold is unresolved")
		_, err = page.Reload()
		Expect(err).NotTo(HaveOccurred())
		openThresholdSettingsOnPage(page)

		By("Verifying the controls are STILL disabled after the reload")
		Eventually(func() bool {
			disabled, _ := page.Locator("[data-testid='threshold-input']").IsDisabled()
			return disabled
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"the lock lives on the server, so a reload must not re-enable editing")

		By("Attempting to raise the threshold to 100% while the hold stands, and confirming the server refuses it")
		// The controls being disabled already proves the UI's intent; this step
		// proves the backend enforces the lock too, not just the disabled
		// attribute -- a request sent by any means, not only this rendered
		// button, must be refused while the hold stands.
		//
		// This app authenticates API calls with a JWT the frontend stores in
		// localStorage (see frontend/lib/auth.ts's ACCESS_TOKEN_KEY) and attaches
		// itself as an Authorization header -- it is not a browser cookie, so
		// Playwright's APIRequestContext (which only forwards real cookies) would
		// otherwise send this request unauthenticated. Read it out of the page
		// and attach it explicitly.
		accessToken, err := page.Evaluate("() => localStorage.getItem('accessToken')")
		Expect(err).NotTo(HaveOccurred())
		Expect(accessToken).NotTo(BeNil(), "expected an access token to be stored after logging in")

		resp, err := page.Context().Request().Put(backendURL+thresholdEndpoint,
			playwright.APIRequestContextPutOptions{
				Data:    map[string]interface{}{"maxDeletePercent": 100},
				Headers: map[string]string{"Authorization": "Bearer " + fmt.Sprint(accessToken)},
			})
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(409),
			"raising the threshold must be refused server-side while a sync is held, regardless of what the UI sends")
	})

	It("lets Sync Anyway through and unlocks the threshold afterwards", func() {
		loginAsAdminOnPage(page)
		openThresholdSettingsOnPage(page)

		stillLocked, _ := page.Locator("[data-testid='threshold-locked-notice']").IsVisible()
		Expect(stillLocked).To(BeTrue(), "the hold from the previous spec should still be unresolved")

		// The previous spec ended by reloading the page, which starts this
		// spec's page with a fresh React tree. The hold itself is genuinely
		// still unresolved server-side (the threshold card above just proved
		// that by reading it from the server), but the Sync Anyway banner is
		// populated only as the direct, in-memory result of THIS component
		// instance's own POST /sync call receiving a 409 -- it is never
		// rehydrated from the server on mount. Re-clicking Sync Now surfaces
		// that banner again, exactly as it did the first time the hold was hit.
		By("Clicking Sync Now again to bring the hold banner (and Sync Anyway) back")
		syncButton := page.Locator("[data-testid='sync-now-btn']")
		Eventually(func() bool {
			enabled, _ := syncButton.IsEnabled()
			return enabled
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue())
		Expect(syncButton.Click()).To(Succeed())

		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='sync-anyway-btn']").IsVisible()
			return visible
		}, 15*time.Second, 500*time.Millisecond).Should(BeTrue(),
			"re-attempting a sync against an unresolved hold should surface it again")

		By("Confirming Sync Anyway")
		Expect(page.Locator("[data-testid='sync-anyway-btn']").Click()).To(Succeed())
		Expect(page.Locator("[data-testid='sync-anyway-confirm-btn']").Click()).To(Succeed())

		By("Waiting for the sync to complete with the override applied")
		Eventually(func() bool {
			visible, _ := page.Locator("[data-testid='sync-override-applied']").IsVisible()
			return visible
		}, 30*time.Second, 500*time.Millisecond).Should(BeTrue())

		By("Verifying the seeded throwaway users were actually deleted this time")
		Expect(countRowsForThreshold(
			`SELECT COUNT(*) FROM users WHERE id LIKE $1`, thresholdFixtureIDPrefix+"%",
		)).To(Equal(0), "an explicit override applies the sync, including its deletions")

		By("Verifying the threshold is editable again")
		Eventually(func() bool {
			disabled, _ := page.Locator("[data-testid='threshold-input']").IsDisabled()
			return disabled
		}, 10*time.Second, 500*time.Millisecond).Should(BeFalse(),
			"resolving the hold via override must unlock the threshold")

		By("Verifying the fixture team's lead and member survived (they are still in the snapshot)")
		Expect(countRowsForThreshold(
			`SELECT COUNT(*) FROM users WHERE id IN ('e2e-sync-lead', 'e2e-sync-member')`,
		)).To(Equal(2), "records the provider still reports must survive even an overridden sync")
	})
})
