// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"database/sql"
	"fmt"
)

// DemoPasswordHash is the bcrypt hash of "demo" password used across all test users
// Generated with: bcrypt.GenerateFromPassword([]byte("demo"), bcrypt.DefaultCost)
const DemoPasswordHash = "$2a$10$OFyj3qtGv0zgv3r3kn9h/OvqyNxNgh7vOCvrF56HyBMcU73QU4LtG"

// CleanupTestData removes all test data matching the given prefix from all tables
// Tables are cleaned in the correct order to respect foreign key constraints
func CleanupTestData(db *sql.DB, prefix string) error {
	// Order matters due to foreign key constraints
	tables := []struct {
		table  string
		column string
	}{
		{"health_check_responses", "session_id"},
		{"health_check_sessions", "id"},
		{"team_supervisors", "team_id"},
		{"team_members", "team_id"},
		{"teams", "id"},
		{"users", "id"},
	}

	for _, t := range tables {
		var query string
		if t.table == "health_check_responses" {
			// health_check_responses references session_id, not id directly
			query = fmt.Sprintf(
				"DELETE FROM %s WHERE session_id IN (SELECT id FROM health_check_sessions WHERE id LIKE $1)",
				t.table,
			)
		} else {
			query = fmt.Sprintf("DELETE FROM %s WHERE %s LIKE $1", t.table, t.column)
		}

		_, err := db.Exec(query, prefix+"%")
		if err != nil {
			return fmt.Errorf("failed to cleanup %s: %w", t.table, err)
		}
	}

	return nil
}

// CleanupAllTestData removes all data from test tables (use with caution)
// Tables are cleaned in the correct order to respect foreign key constraints
func CleanupAllTestData(db *sql.DB) error {
	tables := []string{
		"health_check_responses",
		"health_check_sessions",
		"team_supervisors",
		"team_members",
		"teams",
		"users",
	}

	for _, table := range tables {
		_, err := db.Exec(fmt.Sprintf("DELETE FROM %s", table))
		if err != nil {
			return fmt.Errorf("failed to cleanup %s: %w", table, err)
		}
	}

	return nil
}
