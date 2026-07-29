// SPDX-License-Identifier: Apache-2.0

package postgres

import (
	"database/sql"
	"os"

	"github.com/agopalakrishnan/teams360/backend/pkg/logger"
)

// EnsureAppConfig creates the app_config table (if it doesn't exist) and
// upserts the APP_ENV value from the environment. This runs BEFORE migrations
// so that migration scripts can conditionally execute based on the environment.
func EnsureAppConfig(db *sql.DB) error {
	log := logger.Get()

	appEnv := os.Getenv("APP_ENV")
	if appEnv == "" {
		appEnv = "dev"
	}

	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS app_config (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		INSERT INTO app_config (key, value) VALUES ('app_env', $1)
		ON CONFLICT (key) DO UPDATE SET value = $1
	`, appEnv)
	if err != nil {
		return err
	}

	log.WithField("APP_ENV", appEnv).Info("app_config environment set")
	return nil
}

// SeedDemoData inserts demo users, teams, supervisor chains, and health check
// sessions/responses. All statements use ON CONFLICT DO NOTHING so this is
// idempotent and safe to call on every startup when APP_ENV=demo.
func SeedDemoData(db *sql.DB) error {
	log := logger.Get()
	log.Info("seeding demo data")

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// --- Demo users ---
	_, err = tx.Exec(`
		INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
		('vp', 'vp', 'vp@teams360.demo', 'VP - Sarah Johnson', 'level-1', NULL, '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('director1', 'director1', 'director1@teams360.demo', 'Director - Mike Chen', 'level-2', 'vp', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('director2', 'director2', 'director2@teams360.demo', 'Director - Lisa Anderson', 'level-2', 'vp', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('manager1', 'manager1', 'manager1@teams360.demo', 'Manager - John Smith', 'level-3', 'director1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('manager2', 'manager2', 'manager2@teams360.demo', 'Manager - Emma Wilson', 'level-3', 'director1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('manager3', 'manager3', 'manager3@teams360.demo', 'Manager - David Brown', 'level-3', 'director2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('teamlead1', 'teamlead1', 'teamlead1@teams360.demo', 'Team Lead - Phoenix Squad', 'level-4', 'manager1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('teamlead2', 'teamlead2', 'teamlead2@teams360.demo', 'Team Lead - Dragon Squad', 'level-4', 'manager1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('teamlead3', 'teamlead3', 'teamlead3@teams360.demo', 'Team Lead - Titan Squad', 'level-4', 'manager2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('teamlead4', 'teamlead4', 'teamlead4@teams360.demo', 'Team Lead - Falcon Squad', 'level-4', 'manager2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('teamlead5', 'teamlead5', 'teamlead5@teams360.demo', 'Team Lead - Eagle Squad', 'level-4', 'manager3', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('alice', 'alice', 'alice@teams360.demo', 'Alice Cooper', 'level-5', 'teamlead1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('bob', 'bob', 'bob@teams360.demo', 'Bob Martinez', 'level-5', 'teamlead1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('carol', 'carol', 'carol@teams360.demo', 'Carol Davis', 'level-5', 'teamlead2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('david', 'david', 'david@teams360.demo', 'David Lee', 'level-5', 'teamlead2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('eve', 'eve', 'eve@teams360.demo', 'Eve Taylor', 'level-5', 'teamlead3', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('demo', 'demo', 'demo@teams360.demo', 'Demo User', 'level-5', 'teamlead1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG')
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return err
	}

	// Nova test team users
	_, err = tx.Exec(`
		INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
		('test-vp', 'test-vp', 'test-vp@teams360.demo', 'VP - Rachel Kim', 'level-1', NULL, '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('test-director', 'test-director', 'test-director@teams360.demo', 'Director - James Park', 'level-2', 'test-vp', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('test-manager', 'test-manager', 'test-manager@teams360.demo', 'Manager - Priya Patel', 'level-3', 'test-director', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('test-lead', 'test-lead', 'test-lead@teams360.demo', 'Team Lead - Nova Squad', 'level-4', 'test-manager', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('test-member1', 'test-member1', 'test-member1@teams360.demo', 'Nora Blake', 'level-5', 'test-lead', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('test-member2', 'test-member2', 'test-member2@teams360.demo', 'Leo Chang', 'level-5', 'test-lead', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG')
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return err
	}

	// E2E test users (for automated E2E acceptance tests)
	_, err = tx.Exec(`
		INSERT INTO users (id, username, email, full_name, hierarchy_level_id, reports_to, password_hash) VALUES
		('e2e_manager1', 'e2e_manager1', 'e2e_manager1@teams360.demo', 'E2E Manager One', 'level-3', NULL, '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_testmanager1', 'e2e_testmanager1', 'e2e_testmanager@teams360.demo', 'E2E Test Manager', 'level-3', NULL, '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_lead1', 'e2e_lead1', 'e2e_lead1@teams360.demo', 'E2E Lead One', 'level-4', 'e2e_manager1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_lead2', 'e2e_lead2', 'e2e_lead2@teams360.demo', 'E2E Lead Two', 'level-4', 'e2e_manager1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_demo', 'e2e_demo', 'e2e_demo@teams360.demo', 'E2E Demo User', 'level-5', 'e2e_lead1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_member1', 'e2e_member1', 'e2e_member1@teams360.demo', 'E2E Member One', 'level-5', 'e2e_lead1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_member2', 'e2e_member2', 'e2e_member2@teams360.demo', 'E2E Member Two', 'level-5', 'e2e_lead2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_member3', 'e2e_member3', 'e2e_member3@teams360.demo', 'E2E Member Three', 'level-5', 'e2e_lead2', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG'),
		('e2e_fresh_member', 'e2e_fresh_member', 'e2e_fresh_member@teams360.demo', 'E2E Fresh Member', 'level-5', 'e2e_lead1', '$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG')
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return err
	}

	// --- Demo teams ---
	_, err = tx.Exec(`
		INSERT INTO teams (id, name, team_lead_id) VALUES
		('team-phoenix', 'Phoenix Squad', 'teamlead1'),
		('team-dragon', 'Dragon Squad', 'teamlead2'),
		('team-titan', 'Titan Squad', 'teamlead3'),
		('team-falcon', 'Falcon Squad', 'teamlead4'),
		('team-eagle', 'Eagle Squad', 'teamlead5'),
		('team-nova', 'Nova Squad', 'test-lead')
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, team_lead_id = EXCLUDED.team_lead_id
	`)
	if err != nil {
		return err
	}

	// --- Team members ---
	// Note: demo and teamlead1 are in both Phoenix and Titan squads to demonstrate multi-team switching
	// We use Titan (not Dragon) because authorization tests assert demo cannot access team-dragon
	_, err = tx.Exec(`
		INSERT INTO team_members (team_id, user_id) VALUES
		('team-phoenix', 'teamlead1'), ('team-phoenix', 'alice'), ('team-phoenix', 'bob'), ('team-phoenix', 'demo'),
		('team-dragon', 'teamlead2'), ('team-dragon', 'carol'), ('team-dragon', 'david'),
		('team-titan', 'teamlead3'), ('team-titan', 'eve'), ('team-titan', 'demo'), ('team-titan', 'teamlead1'),
		('team-falcon', 'teamlead4'),
		('team-eagle', 'teamlead5'),
		('team-nova', 'test-lead'), ('team-nova', 'test-member1'), ('team-nova', 'test-member2')
		ON CONFLICT (team_id, user_id) DO NOTHING
	`)
	if err != nil {
		return err
	}

	// --- Supervisor chains ---
	_, err = tx.Exec(`
		INSERT INTO team_supervisors (team_id, user_id, hierarchy_level_id, position) VALUES
		('team-phoenix', 'teamlead1', 'level-4', 1), ('team-phoenix', 'manager1', 'level-3', 2), ('team-phoenix', 'director1', 'level-2', 3), ('team-phoenix', 'vp', 'level-1', 4),
		('team-dragon', 'teamlead2', 'level-4', 1), ('team-dragon', 'manager1', 'level-3', 2), ('team-dragon', 'director1', 'level-2', 3), ('team-dragon', 'vp', 'level-1', 4),
		('team-titan', 'teamlead3', 'level-4', 1), ('team-titan', 'manager2', 'level-3', 2), ('team-titan', 'director1', 'level-2', 3), ('team-titan', 'vp', 'level-1', 4),
		('team-falcon', 'teamlead4', 'level-4', 1), ('team-falcon', 'manager2', 'level-3', 2), ('team-falcon', 'director1', 'level-2', 3), ('team-falcon', 'vp', 'level-1', 4),
		('team-eagle', 'teamlead5', 'level-4', 1), ('team-eagle', 'manager3', 'level-3', 2), ('team-eagle', 'director2', 'level-2', 3), ('team-eagle', 'vp', 'level-1', 4),
		('team-nova', 'test-lead', 'level-4', 1), ('team-nova', 'test-manager', 'level-3', 2), ('team-nova', 'test-director', 'level-2', 3), ('team-nova', 'test-vp', 'level-1', 4)
		ON CONFLICT (team_id, user_id) DO UPDATE SET position = EXCLUDED.position
	`)
	if err != nil {
		return err
	}

	// --- Health check sessions ---
	_, err = tx.Exec(`
		INSERT INTO health_check_sessions (id, team_id, user_id, date, assessment_period, completed) VALUES
		('demo-session-phoenix-2024h1-1', 'team-phoenix', 'demo', '2024-03-15', '2024 - 1st Half', true),
		('demo-session-phoenix-2024h1-2', 'team-phoenix', 'alice', '2024-04-01', '2024 - 1st Half', true),
		('demo-session-phoenix-2024h2-1', 'team-phoenix', 'demo', '2024-09-15', '2024 - 2nd Half', true),
		('demo-session-phoenix-2024h2-2', 'team-phoenix', 'alice', '2024-10-01', '2024 - 2nd Half', true),
		('demo-session-phoenix-2025h1-1', 'team-phoenix', 'demo', '2025-02-15', '2025 - 1st Half', true),
		('demo-session-dragon-2024h1-1', 'team-dragon', 'bob', '2024-03-20', '2024 - 1st Half', true),
		('demo-session-dragon-2024h2-1', 'team-dragon', 'bob', '2024-09-20', '2024 - 2nd Half', true),
		('demo-session-dragon-2024h2-2', 'team-dragon', 'carol', '2024-10-05', '2024 - 2nd Half', true),
		('demo-session-dragon-2025h1-1', 'team-dragon', 'bob', '2025-02-20', '2025 - 1st Half', true),
		('demo-session-titan-2024h1-1', 'team-titan', 'david', '2024-04-10', '2024 - 1st Half', true),
		('demo-session-titan-2024h2-1', 'team-titan', 'david', '2024-10-10', '2024 - 2nd Half', true),
		('demo-session-titan-2025h1-1', 'team-titan', 'david', '2025-03-01', '2025 - 1st Half', true),
		('demo-session-falcon-2024h1-1', 'team-falcon', 'eve', '2024-03-25', '2024 - 1st Half', true),
		('demo-session-falcon-2024h2-1', 'team-falcon', 'eve', '2024-09-25', '2024 - 2nd Half', true),
		('demo-session-falcon-2025h1-1', 'team-falcon', 'eve', '2025-02-25', '2025 - 1st Half', true),
		('demo-session-eagle-2024h1-1', 'team-eagle', 'frank', '2024-04-05', '2024 - 1st Half', true),
		('demo-session-eagle-2024h2-1', 'team-eagle', 'frank', '2024-10-15', '2024 - 2nd Half', true),
		('demo-session-eagle-2025h1-1', 'team-eagle', 'frank', '2025-03-05', '2025 - 1st Half', true)
		ON CONFLICT (id) DO NOTHING
	`)
	if err != nil {
		return err
	}

	// --- Health check responses ---
	_, err = tx.Exec(`
		INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment) VALUES
		-- Phoenix 2024 H1 Session 1
		('demo-session-phoenix-2024h1-1', 'mission', 3, 'stable', 'Clear direction'),
		('demo-session-phoenix-2024h1-1', 'value', 3, 'improving', 'Delivering great value'),
		('demo-session-phoenix-2024h1-1', 'speed', 2, 'stable', 'Could be faster'),
		('demo-session-phoenix-2024h1-1', 'fun', 3, 'stable', 'Great team atmosphere'),
		('demo-session-phoenix-2024h1-1', 'health', 2, 'improving', 'Working on tech debt'),
		('demo-session-phoenix-2024h1-1', 'learning', 3, 'stable', 'Always learning'),
		('demo-session-phoenix-2024h1-1', 'support', 3, 'stable', 'Good support structure'),
		('demo-session-phoenix-2024h1-1', 'pawns', 3, 'stable', 'We make decisions'),
		('demo-session-phoenix-2024h1-1', 'release', 2, 'improving', 'Getting easier'),
		('demo-session-phoenix-2024h1-1', 'process', 3, 'stable', 'Process works well'),
		('demo-session-phoenix-2024h1-1', 'teamwork', 3, 'stable', 'Great collaboration'),
		-- Phoenix 2024 H1 Session 2
		('demo-session-phoenix-2024h1-2', 'mission', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'value', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'speed', 3, 'improving', 'Better velocity'),
		('demo-session-phoenix-2024h1-2', 'fun', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'health', 2, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'learning', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'support', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'pawns', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'release', 2, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'process', 3, 'stable', NULL),
		('demo-session-phoenix-2024h1-2', 'teamwork', 3, 'stable', NULL),
		-- Phoenix 2024 H2 Session 1
		('demo-session-phoenix-2024h2-1', 'mission', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'value', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'speed', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'fun', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'health', 3, 'improving', 'Paid off tech debt'),
		('demo-session-phoenix-2024h2-1', 'learning', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'support', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'pawns', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'release', 3, 'improving', 'CI/CD improved'),
		('demo-session-phoenix-2024h2-1', 'process', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-1', 'teamwork', 3, 'stable', NULL),
		-- Phoenix 2024 H2 Session 2
		('demo-session-phoenix-2024h2-2', 'mission', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'value', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'speed', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'fun', 2, 'declining', 'Crunch time'),
		('demo-session-phoenix-2024h2-2', 'health', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'learning', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'support', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'pawns', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'release', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'process', 3, 'stable', NULL),
		('demo-session-phoenix-2024h2-2', 'teamwork', 3, 'stable', NULL),
		-- Phoenix 2025 H1
		('demo-session-phoenix-2025h1-1', 'mission', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'value', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'speed', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'fun', 3, 'improving', 'Recovered'),
		('demo-session-phoenix-2025h1-1', 'health', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'learning', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'support', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'pawns', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'release', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'process', 3, 'stable', NULL),
		('demo-session-phoenix-2025h1-1', 'teamwork', 3, 'stable', NULL),
		-- Dragon 2024 H1 (Struggling)
		('demo-session-dragon-2024h1-1', 'mission', 2, 'declining', 'Unclear priorities'),
		('demo-session-dragon-2024h1-1', 'value', 2, 'declining', 'Value unclear'),
		('demo-session-dragon-2024h1-1', 'speed', 1, 'declining', 'Very slow'),
		('demo-session-dragon-2024h1-1', 'fun', 1, 'declining', 'Stressful'),
		('demo-session-dragon-2024h1-1', 'health', 1, 'declining', 'Tech debt mountain'),
		('demo-session-dragon-2024h1-1', 'learning', 2, 'stable', NULL),
		('demo-session-dragon-2024h1-1', 'support', 2, 'stable', NULL),
		('demo-session-dragon-2024h1-1', 'pawns', 1, 'declining', 'No autonomy'),
		('demo-session-dragon-2024h1-1', 'release', 1, 'declining', 'Releases scary'),
		('demo-session-dragon-2024h1-1', 'process', 2, 'stable', NULL),
		('demo-session-dragon-2024h1-1', 'teamwork', 2, 'stable', NULL),
		-- Dragon 2024 H2 Session 1 (Improving)
		('demo-session-dragon-2024h2-1', 'mission', 2, 'improving', 'Getting clearer'),
		('demo-session-dragon-2024h2-1', 'value', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-1', 'speed', 2, 'improving', 'Better'),
		('demo-session-dragon-2024h2-1', 'fun', 2, 'improving', 'Less stress'),
		('demo-session-dragon-2024h2-1', 'health', 2, 'improving', 'Tackling debt'),
		('demo-session-dragon-2024h2-1', 'learning', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-1', 'support', 3, 'improving', 'Got help'),
		('demo-session-dragon-2024h2-1', 'pawns', 2, 'improving', 'More voice'),
		('demo-session-dragon-2024h2-1', 'release', 2, 'improving', 'Better process'),
		('demo-session-dragon-2024h2-1', 'process', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-1', 'teamwork', 3, 'improving', 'Bonding'),
		-- Dragon 2024 H2 Session 2
		('demo-session-dragon-2024h2-2', 'mission', 3, 'improving', NULL),
		('demo-session-dragon-2024h2-2', 'value', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'speed', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'fun', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'health', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'learning', 3, 'improving', NULL),
		('demo-session-dragon-2024h2-2', 'support', 3, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'pawns', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'release', 2, 'stable', NULL),
		('demo-session-dragon-2024h2-2', 'process', 3, 'improving', NULL),
		('demo-session-dragon-2024h2-2', 'teamwork', 3, 'stable', NULL),
		-- Dragon 2025 H1 (Much better)
		('demo-session-dragon-2025h1-1', 'mission', 3, 'stable', NULL),
		('demo-session-dragon-2025h1-1', 'value', 3, 'improving', NULL),
		('demo-session-dragon-2025h1-1', 'speed', 3, 'improving', NULL),
		('demo-session-dragon-2025h1-1', 'fun', 3, 'improving', NULL),
		('demo-session-dragon-2025h1-1', 'health', 3, 'improving', 'Debt cleared'),
		('demo-session-dragon-2025h1-1', 'learning', 3, 'stable', NULL),
		('demo-session-dragon-2025h1-1', 'support', 3, 'stable', NULL),
		('demo-session-dragon-2025h1-1', 'pawns', 3, 'improving', NULL),
		('demo-session-dragon-2025h1-1', 'release', 3, 'improving', NULL),
		('demo-session-dragon-2025h1-1', 'process', 3, 'stable', NULL),
		('demo-session-dragon-2025h1-1', 'teamwork', 3, 'stable', NULL),
		-- Titan 2024 H1 (Average)
		('demo-session-titan-2024h1-1', 'mission', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'value', 3, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'speed', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'fun', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'health', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'learning', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'support', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'pawns', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'release', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'process', 2, 'stable', NULL),
		('demo-session-titan-2024h1-1', 'teamwork', 2, 'stable', NULL),
		-- Titan 2024 H2
		('demo-session-titan-2024h2-1', 'mission', 3, 'improving', NULL),
		('demo-session-titan-2024h2-1', 'value', 3, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'speed', 2, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'fun', 2, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'health', 2, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'learning', 3, 'improving', NULL),
		('demo-session-titan-2024h2-1', 'support', 2, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'pawns', 2, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'release', 2, 'stable', NULL),
		('demo-session-titan-2024h2-1', 'process', 3, 'improving', NULL),
		('demo-session-titan-2024h2-1', 'teamwork', 3, 'improving', NULL),
		-- Titan 2025 H1
		('demo-session-titan-2025h1-1', 'mission', 3, 'stable', NULL),
		('demo-session-titan-2025h1-1', 'value', 3, 'stable', NULL),
		('demo-session-titan-2025h1-1', 'speed', 3, 'improving', NULL),
		('demo-session-titan-2025h1-1', 'fun', 3, 'improving', NULL),
		('demo-session-titan-2025h1-1', 'health', 2, 'stable', NULL),
		('demo-session-titan-2025h1-1', 'learning', 3, 'stable', NULL),
		('demo-session-titan-2025h1-1', 'support', 3, 'improving', NULL),
		('demo-session-titan-2025h1-1', 'pawns', 3, 'improving', NULL),
		('demo-session-titan-2025h1-1', 'release', 3, 'improving', NULL),
		('demo-session-titan-2025h1-1', 'process', 3, 'stable', NULL),
		('demo-session-titan-2025h1-1', 'teamwork', 3, 'stable', NULL),
		-- Falcon 2024 H1 (New team)
		('demo-session-falcon-2024h1-1', 'mission', 2, 'improving', 'Getting aligned'),
		('demo-session-falcon-2024h1-1', 'value', 2, 'stable', NULL),
		('demo-session-falcon-2024h1-1', 'speed', 2, 'improving', NULL),
		('demo-session-falcon-2024h1-1', 'fun', 3, 'stable', 'Fresh energy'),
		('demo-session-falcon-2024h1-1', 'health', 3, 'stable', 'Fresh codebase'),
		('demo-session-falcon-2024h1-1', 'learning', 3, 'stable', 'Learning a lot'),
		('demo-session-falcon-2024h1-1', 'support', 2, 'stable', NULL),
		('demo-session-falcon-2024h1-1', 'pawns', 3, 'stable', NULL),
		('demo-session-falcon-2024h1-1', 'release', 2, 'improving', NULL),
		('demo-session-falcon-2024h1-1', 'process', 2, 'improving', 'Forming'),
		('demo-session-falcon-2024h1-1', 'teamwork', 3, 'stable', NULL),
		-- Falcon 2024 H2
		('demo-session-falcon-2024h2-1', 'mission', 3, 'improving', NULL),
		('demo-session-falcon-2024h2-1', 'value', 3, 'improving', NULL),
		('demo-session-falcon-2024h2-1', 'speed', 3, 'improving', NULL),
		('demo-session-falcon-2024h2-1', 'fun', 3, 'stable', NULL),
		('demo-session-falcon-2024h2-1', 'health', 3, 'stable', NULL),
		('demo-session-falcon-2024h2-1', 'learning', 3, 'stable', NULL),
		('demo-session-falcon-2024h2-1', 'support', 3, 'improving', NULL),
		('demo-session-falcon-2024h2-1', 'pawns', 3, 'stable', NULL),
		('demo-session-falcon-2024h2-1', 'release', 3, 'improving', NULL),
		('demo-session-falcon-2024h2-1', 'process', 3, 'improving', NULL),
		('demo-session-falcon-2024h2-1', 'teamwork', 3, 'stable', NULL),
		-- Falcon 2025 H1
		('demo-session-falcon-2025h1-1', 'mission', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'value', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'speed', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'fun', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'health', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'learning', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'support', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'pawns', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'release', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'process', 3, 'stable', NULL),
		('demo-session-falcon-2025h1-1', 'teamwork', 3, 'stable', NULL),
		-- Eagle 2024 H1 (Mixed)
		('demo-session-eagle-2024h1-1', 'mission', 3, 'stable', NULL),
		('demo-session-eagle-2024h1-1', 'value', 2, 'stable', NULL),
		('demo-session-eagle-2024h1-1', 'speed', 2, 'declining', 'Blockers'),
		('demo-session-eagle-2024h1-1', 'fun', 2, 'stable', NULL),
		('demo-session-eagle-2024h1-1', 'health', 2, 'declining', 'Growing debt'),
		('demo-session-eagle-2024h1-1', 'learning', 3, 'stable', NULL),
		('demo-session-eagle-2024h1-1', 'support', 2, 'stable', NULL),
		('demo-session-eagle-2024h1-1', 'pawns', 2, 'declining', NULL),
		('demo-session-eagle-2024h1-1', 'release', 1, 'declining', 'Painful'),
		('demo-session-eagle-2024h1-1', 'process', 2, 'stable', NULL),
		('demo-session-eagle-2024h1-1', 'teamwork', 3, 'stable', NULL),
		-- Eagle 2024 H2 (Recovery)
		('demo-session-eagle-2024h2-1', 'mission', 3, 'stable', NULL),
		('demo-session-eagle-2024h2-1', 'value', 3, 'improving', NULL),
		('demo-session-eagle-2024h2-1', 'speed', 2, 'improving', NULL),
		('demo-session-eagle-2024h2-1', 'fun', 2, 'stable', NULL),
		('demo-session-eagle-2024h2-1', 'health', 2, 'improving', 'Focus on debt'),
		('demo-session-eagle-2024h2-1', 'learning', 3, 'stable', NULL),
		('demo-session-eagle-2024h2-1', 'support', 3, 'improving', NULL),
		('demo-session-eagle-2024h2-1', 'pawns', 2, 'improving', NULL),
		('demo-session-eagle-2024h2-1', 'release', 2, 'improving', NULL),
		('demo-session-eagle-2024h2-1', 'process', 3, 'improving', NULL),
		('demo-session-eagle-2024h2-1', 'teamwork', 3, 'stable', NULL),
		-- Eagle 2025 H1 (Recovered)
		('demo-session-eagle-2025h1-1', 'mission', 3, 'stable', NULL),
		('demo-session-eagle-2025h1-1', 'value', 3, 'stable', NULL),
		('demo-session-eagle-2025h1-1', 'speed', 3, 'improving', NULL),
		('demo-session-eagle-2025h1-1', 'fun', 3, 'improving', NULL),
		('demo-session-eagle-2025h1-1', 'health', 3, 'improving', NULL),
		('demo-session-eagle-2025h1-1', 'learning', 3, 'stable', NULL),
		('demo-session-eagle-2025h1-1', 'support', 3, 'stable', NULL),
		('demo-session-eagle-2025h1-1', 'pawns', 3, 'improving', NULL),
		('demo-session-eagle-2025h1-1', 'release', 3, 'improving', NULL),
		('demo-session-eagle-2025h1-1', 'process', 3, 'stable', NULL),
		('demo-session-eagle-2025h1-1', 'teamwork', 3, 'stable', NULL)
		ON CONFLICT DO NOTHING
	`)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Info("demo data seeded successfully")
	return nil
}
