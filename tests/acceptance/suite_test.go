package acceptance_test

import (
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/mxschmitt/playwright-go"

	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// Shared test resources accessible to all specs
var (
	db              *sql.DB
	backendSession  *gexec.Session
	frontendSession *gexec.Session
	pw              *playwright.Playwright
	browser         playwright.Browser
	embeddedPG      *embeddedpostgres.EmbeddedPostgres

	// Test environment URLs
	backendURL  = "http://localhost:8080"
	frontendURL = "http://localhost:3000"
)

// isPortOpen checks whether a TCP port is already accepting connections.
func isPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func TestAcceptance(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Acceptance Suite")
}

// SynchronizedBeforeSuite runs ONCE across all parallel nodes
// Phase 1: Runs on the first process only (setup shared resources)
// Phase 2: Runs on all processes (receives data from Phase 1)
var _ = SynchronizedBeforeSuite(
	// Phase 1: Runs on process #1 only - setup database and servers
	func() []byte {
		By("Phase 1: Setting up shared test infrastructure (database, servers)")

		// Kill any existing processes on ports 3000 and 8080
		By("Cleaning up existing backend and frontend processes")
		// Force kill any processes holding the ports
		exec.Command("bash", "-c", "lsof -ti:8080 | xargs kill -9 2>/dev/null").Run()
		exec.Command("bash", "-c", "lsof -ti:3000 | xargs kill -9 2>/dev/null").Run()
		// Also kill by process name as fallback
		exec.Command("pkill", "-9", "-f", "go run cmd/api/main.go").Run()
		exec.Command("pkill", "-9", "-f", "npm run dev").Run()
		exec.Command("pkill", "-9", "-f", "next dev").Run()
		time.Sleep(3 * time.Second) // Give processes time to die

		// Start embedded PostgreSQL if no external database is reachable
		databaseURL := os.Getenv("TEST_DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
		}
		if !isPortOpen(5432) {
			By("No PostgreSQL on port 5432 — starting embedded PostgreSQL")
			embeddedPG = embeddedpostgres.NewDatabase(
				embeddedpostgres.DefaultConfig().
					Port(5432).
					Database("teams360_test").
					Username("postgres").
					Password("postgres"),
			)
			err := embeddedPG.Start()
			Expect(err).NotTo(HaveOccurred(), "Failed to start embedded PostgreSQL")
			GinkgoWriter.Printf("✅ Embedded PostgreSQL started on port 5432\n")
		} else {
			GinkgoWriter.Printf("✅ Using existing PostgreSQL on port 5432\n")
			embeddedPG = nil
		}

		var err error
		db, err = sql.Open("postgres", databaseURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		// Clean and run migrations
		By("Cleaning and migrating test database")
		_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
		Expect(err).NotTo(HaveOccurred())

		driver, err := migratePostgres.WithInstance(db, &migratePostgres.Config{})
		Expect(err).NotTo(HaveOccurred())

		migrationEngine, err := migrate.NewWithDatabaseInstance(
			"file://../../backend/infrastructure/persistence/postgres/migrations",
			"postgres",
			driver,
		)
		Expect(err).NotTo(HaveOccurred())

		err = migrationEngine.Up()
		Expect(err).NotTo(HaveOccurred())

		// Seed test data
		By("Seeding test users, teams, and health check data")

		// Clean up any existing e2e test users from previous runs to ensure fresh state
		// Delete by ID, username, and email patterns to handle all potential conflicts
		_, _ = db.Exec(`DELETE FROM users WHERE id LIKE 'e2e_%' OR username LIKE 'e2e_%' OR email LIKE 'e2e_%@%'`)

		// Insert test-specific users to avoid conflicts with seed migration
		// Using test-specific IDs (e2e_*) to avoid conflicts with existing seed data
		// Schema: id, username, email, full_name, hierarchy_level_id, reports_to, password_hash
		// Insert users in order to respect reports_to foreign key constraints
		// First: managers (no reports_to)
		_, err = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
			('e2e_manager1', 'e2e_manager1', 'e2e_manager1@teams360.demo', 'E2E Manager One', 'level-3', NULL, $1),
			('e2e_testmanager1', 'e2e_testmanager1', 'e2e_testmanager@teams360.demo', 'E2E Test Manager', 'level-3', NULL, $1)
		`, DemoPasswordHash)
		Expect(err).NotTo(HaveOccurred(), "Failed to insert e2e managers")

		// Second: team leads (reports_to managers)
		_, err = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
			('e2e_lead1', 'e2e_lead1', 'e2e_lead1@teams360.demo', 'E2E Lead One', 'level-4', 'e2e_manager1', $1),
			('e2e_lead2', 'e2e_lead2', 'e2e_lead2@teams360.demo', 'E2E Lead Two', 'level-4', 'e2e_manager1', $1)
		`, DemoPasswordHash)
		Expect(err).NotTo(HaveOccurred(), "Failed to insert e2e team leads")

		// Third: team members (reports_to team leads)
		_, err = db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
			('e2e_demo', 'e2e_demo', 'e2e_demo@teams360.demo', 'E2E Demo User', 'level-5', 'e2e_lead1', $1),
			('e2e_member1', 'e2e_member1', 'e2e_member1@teams360.demo', 'E2E Member One', 'level-5', 'e2e_lead1', $1),
			('e2e_member2', 'e2e_member2', 'e2e_member2@teams360.demo', 'E2E Member Two', 'level-5', 'e2e_lead2', $1),
			('e2e_member3', 'e2e_member3', 'e2e_member3@teams360.demo', 'E2E Member Three', 'level-5', 'e2e_lead2', $1),
			('e2e_fresh_member', 'e2e_fresh_member', 'e2e_fresh_member@teams360.demo', 'E2E Fresh Member', 'level-5', 'e2e_lead1', $1)
		`, DemoPasswordHash)
		Expect(err).NotTo(HaveOccurred(), "Failed to insert e2e team members")

		// Verify that e2e users were actually inserted
		var userCount int
		err = db.QueryRow(`SELECT COUNT(*) FROM users WHERE id LIKE 'e2e_%'`).Scan(&userCount)
		Expect(err).NotTo(HaveOccurred())
		Expect(userCount).To(Equal(9), "Expected 9 e2e_ users to be inserted (2 managers + 2 leads + 5 members)")

		// Insert E2E test teams (schema: id, name, team_lead_id)
		// Note: No manager_id column - managers tracked via team_supervisors table
		_, err = db.Exec(`
			INSERT INTO teams (id, name, team_lead_id) VALUES
			('e2e_team1', 'E2E Team Alpha', 'e2e_lead1'),
			('e2e_team2', 'E2E Team Beta', 'e2e_lead2'),
			('e2e_team3', 'E2E Team Gamma', 'e2e_lead1')
			ON CONFLICT (id) DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		// Insert team supervisors for manager access
		// This is how managers are associated with teams
		_, err = db.Exec(`
			INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position) VALUES
			('e2e_team1', 'e2e_lead1', 'level-4', 1),
			('e2e_team1', 'e2e_manager1', 'level-3', 2),
			('e2e_team2', 'e2e_lead2', 'level-4', 1),
			('e2e_team2', 'e2e_manager1', 'level-3', 2),
			('e2e_team3', 'e2e_lead1', 'level-4', 1),
			('e2e_team3', 'e2e_testmanager1', 'level-3', 2)
			ON CONFLICT (team_id, user_id) DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		// Insert team members
		_, err = db.Exec(`
			INSERT INTO team_members (team_id, user_id) VALUES
			('e2e_team1', 'e2e_demo'),
			('e2e_team1', 'e2e_member1'),
			('e2e_team1', 'e2e_member2'),
			('e2e_team2', 'e2e_member2'),
			('e2e_team2', 'e2e_member3'),
			('e2e_team3', 'e2e_member1'),
			('e2e_team1', 'e2e_fresh_member')
			ON CONFLICT DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		// Insert sample health check sessions
		// IMPORTANT: Include sessions for e2e_demo to test survey history display on /home page
		_, err = db.Exec(`
			INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed) VALUES
			('e2e_session1', 'e2e_team1', 'e2e_member1', '2024-01-15', '2023 - 2nd Half', true),
			('e2e_session2', 'e2e_team1', 'e2e_member2', '2024-01-16', '2023 - 2nd Half', true),
			('e2e_session3', 'e2e_team2', 'e2e_member2', '2024-07-15', '2024 - 1st Half', true),
			('e2e_session4', 'e2e_team2', 'e2e_member3', '2024-07-16', '2024 - 1st Half', true),
			('e2e_demo_session1', 'e2e_team1', 'e2e_demo', '2024-03-15', '2023 - 2nd Half', true),
			('e2e_demo_session2', 'e2e_team1', 'e2e_demo', '2024-09-20', '2024 - 1st Half', true)
			ON CONFLICT (id) DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		// Insert health check responses
		// IMPORTANT: Include responses for e2e_demo sessions to test survey history on /home page
		_, err = db.Exec(`
			INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
			('e2e_session1', 'mission', 3, 'improving', 'Clear direction'),
			('e2e_session1', 'value', 2, 'stable', 'Good delivery'),
			('e2e_session1', 'speed', 2, 'stable', 'Reasonable pace'),
			('e2e_session2', 'mission', 3, 'improving', 'Well understood'),
			('e2e_session2', 'value', 3, 'improving', 'Great value'),
			('e2e_session2', 'speed', 2, 'declining', 'Some blockers'),
			('e2e_session3', 'mission', 1, 'declining', 'Unclear goals'),
			('e2e_session3', 'value', 2, 'stable', 'Moderate value'),
			('e2e_session3', 'speed', 3, 'improving', 'Fast delivery'),
			('e2e_session4', 'mission', 2, 'stable', 'Okay clarity'),
			('e2e_session4', 'value', 2, 'stable', 'Standard delivery'),
			('e2e_session4', 'speed', 2, 'stable', 'Normal pace'),
			('e2e_demo_session1', 'mission', 2, 'stable', 'Clear enough'),
			('e2e_demo_session1', 'value', 2, 'stable', 'Good delivery'),
			('e2e_demo_session1', 'speed', 1, 'declining', 'Too slow'),
			('e2e_demo_session2', 'mission', 3, 'improving', 'Very clear now'),
			('e2e_demo_session2', 'value', 3, 'improving', 'Great value'),
			('e2e_demo_session2', 'speed', 2, 'improving', 'Getting faster')
			ON CONFLICT DO NOTHING
		`)
		Expect(err).NotTo(HaveOccurred())

		GinkgoWriter.Printf("✅ Test data seeded successfully\n")

		// Start backend API server using gexec for proper process management
		By("Starting backend API server")
		backendCmd := exec.Command("go", "run", "cmd/api/main.go")
		backendCmd.Dir = "../../backend"
		backendCmd.Env = append(os.Environ(),
			"PORT=8080",
			"APP_ENV=demo",
			fmt.Sprintf("DATABASE_URL=%s", databaseURL),
		)
		// Set process group so we can kill all child processes
		backendCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		backendSession, err = gexec.Start(backendCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		// Wait for backend to be ready
		Eventually(func() error {
			resp, err := exec.Command("curl", "-s", "--max-time", "5", "-o", "/dev/null", "-w", "%{http_code}", backendURL+"/health").Output()
			if err != nil {
				return err
			}
			if string(resp) != "200" {
				return fmt.Errorf("backend not ready, status: %s", string(resp))
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed(), "Backend API should start successfully")

		// Start frontend Next.js server using gexec for proper process management
		By("Starting frontend Next.js server")
		frontendCmd := exec.Command("npm", "run", "dev")
		frontendCmd.Dir = "../../frontend"
		frontendCmd.Env = append(os.Environ(),
			fmt.Sprintf("NEXT_PUBLIC_API_URL=%s", backendURL),
		)
		// Set process group so we can kill all child processes (Next.js spawns multiple)
		frontendCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		frontendSession, err = gexec.Start(frontendCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		// Wait for frontend to be ready (use --max-time to prevent curl from hanging)
		Eventually(func() error {
			resp, err := exec.Command("curl", "-s", "--max-time", "5", "-o", "/dev/null", "-w", "%{http_code}", frontendURL).Output()
			if err != nil {
				return err
			}
			if string(resp) != "200" {
				return fmt.Errorf("frontend not ready, status: %s", string(resp))
			}
			return nil
		}, 300*time.Second, 5*time.Second).Should(Succeed(), "Frontend should start successfully")

		GinkgoWriter.Printf("✅ Test infrastructure ready\n")
		GinkgoWriter.Printf("   Backend:  %s\n", backendURL)
		GinkgoWriter.Printf("   Frontend: %s\n", frontendURL)

		// Return connection info to all processes
		return []byte(databaseURL)
	},

	// Phase 2: Runs on ALL processes - setup Playwright
	func(databaseURL []byte) {
		By("Phase 2: Setting up Playwright (per process)")

		// Each process gets its own Playwright instance
		var err error
		pw, err = playwright.Run()
		Expect(err).NotTo(HaveOccurred())

		browser, err = pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
			Headless: playwright.Bool(true), // Set to false for debugging
		})
		Expect(err).NotTo(HaveOccurred())

		// Reconnect to database for this process
		db, err = sql.Open("postgres", string(databaseURL))
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		GinkgoWriter.Printf("✅ Playwright initialized for process %d\n", GinkgoParallelProcess())
	},
)

// SynchronizedAfterSuite runs ONCE across all parallel nodes
// Phase 1: Runs on all processes
// Phase 2: Runs on process #1 only (cleanup shared resources)
var _ = SynchronizedAfterSuite(
	// Phase 1: Runs on ALL processes - cleanup Playwright
	func() {
		By("Phase 1: Cleaning up Playwright (per process)")

		if browser != nil {
			browser.Close()
		}

		if pw != nil {
			pw.Stop()
		}

		if db != nil {
			db.Close()
		}

		GinkgoWriter.Printf("✅ Playwright cleaned up for process %d\n", GinkgoParallelProcess())
	},

	// Phase 2: Runs on process #1 only - cleanup servers and database
	func() {
		By("Phase 2: Cleaning up shared test infrastructure (database, servers)")

		// Terminate sessions using gexec - this sends SIGTERM first
		// Then kill the entire process group to ensure child processes are killed
		if backendSession != nil {
			GinkgoWriter.Printf("Terminating backend server...\n")
			// Kill the process group (negative PID kills the group)
			if backendSession.Command.Process != nil {
				pgid, err := syscall.Getpgid(backendSession.Command.Process.Pid)
				if err == nil {
					syscall.Kill(-pgid, syscall.SIGKILL)
				}
			}
			backendSession.Kill()
			Eventually(backendSession, 10*time.Second).Should(gexec.Exit())
			GinkgoWriter.Printf("Backend server terminated\n")
		}

		if frontendSession != nil {
			GinkgoWriter.Printf("Terminating frontend server...\n")
			// Kill the process group (negative PID kills the group)
			if frontendSession.Command.Process != nil {
				pgid, err := syscall.Getpgid(frontendSession.Command.Process.Pid)
				if err == nil {
					syscall.Kill(-pgid, syscall.SIGKILL)
				}
			}
			frontendSession.Kill()
			Eventually(frontendSession, 10*time.Second).Should(gexec.Exit())
			GinkgoWriter.Printf("Frontend server terminated\n")
		}

		// Also cleanup any orphaned processes by port (belt and suspenders)
		exec.Command("bash", "-c", "lsof -ti:8080 | xargs kill -9 2>/dev/null").Run()
		exec.Command("bash", "-c", "lsof -ti:3000 | xargs kill -9 2>/dev/null").Run()

		// Clean up gexec build artifacts
		gexec.CleanupBuildArtifacts()

		// Stop embedded PostgreSQL if we started it
		if embeddedPG != nil {
			if err := embeddedPG.Stop(); err != nil {
				GinkgoWriter.Printf("Warning: failed to stop embedded PostgreSQL: %v\n", err)
			} else {
				GinkgoWriter.Printf("✅ Embedded PostgreSQL stopped\n")
			}
		}

		GinkgoWriter.Printf("✅ Test infrastructure shut down\n")
	},
)

// BeforeEach runs before EACH test spec
var _ = BeforeEach(func() {
	// Reset test data if needed
	// This runs before each It() block
})

// AfterEach runs after EACH test spec
var _ = AfterEach(func() {
	// Cleanup per-test resources if needed
	// This runs after each It() block
})

// dismissOnboardingIfVisible closes the onboarding modal if it is showing.
// Called after every login navigation so the modal never blocks test interactions.
// Waits up to 2 seconds for the modal to appear (React may not have rendered it yet).
func dismissOnboardingIfVisible(page playwright.Page) {
	modal := page.Locator("[data-testid='onboarding-modal']")
	// Wait briefly for the modal to appear — React renders it asynchronously after navigation.
	// If it doesn't appear within 2s it's not shown for this user/session, so proceed.
	err := modal.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(2000),
	})
	if err != nil {
		// Modal not shown — nothing to dismiss.
		return
	}
	btn := page.Locator("[data-testid='onboarding-dismiss-btn']")
	btnVisible, _ := btn.IsVisible()
	if btnVisible {
		_ = btn.Click()
	} else {
		// Modal is on an earlier slide — click the close X button
		_ = page.Locator("[data-testid='onboarding-close-btn']").Click()
	}
	// Wait for modal to disappear before returning
	_ = modal.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateHidden,
		Timeout: playwright.Float(3000),
	})
}
