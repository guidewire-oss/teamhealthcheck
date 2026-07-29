// SPDX-License-Identifier: Apache-2.0

package integration_test

import (
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Database Constraints", func() {
	var (
		db      *sql.DB
		cleanup func()
	)

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("User table constraints", func() {
		Context("email format validation", func() {
			It("should accept valid email formats", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-valid-email', 'validuser', 'valid@example.com', 'Valid User', 'level-5', 'hash')
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject invalid email formats", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-invalid-email', 'invaliduser', 'not-an-email', 'Invalid User', 'level-5', 'hash')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_users_email_format"))
			})
		})

		Context("username format validation", func() {
			It("should accept valid username formats", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-valid-username', 'valid_user-123', 'validuser@example.com', 'Valid User', 'level-5', 'hash')
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject usernames with invalid characters", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-invalid-username', 'invalid@user', 'invaliduser@example.com', 'Invalid User', 'level-5', 'hash')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_users_username_format"))
			})

			It("should reject usernames that are too short", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-short-username', 'a', 'shortuser@example.com', 'Short User', 'level-5', 'hash')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_users_username_format"))
			})
		})

		Context("unique constraints", func() {
			BeforeEach(func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('existing-user', 'existinguser', 'existing@example.com', 'Existing User', 'level-5', 'hash')
				`)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject duplicate usernames", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-dup-username', 'existinguser', 'different@example.com', 'Dup User', 'level-5', 'hash')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate key"))
			})

			It("should reject duplicate emails", func() {
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('test-dup-email', 'differentuser', 'existing@example.com', 'Dup User', 'level-5', 'hash')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("duplicate key"))
			})
		})
	})

	Describe("Health check responses constraints", func() {
		BeforeEach(func() {
			// Create a user, team, and session for testing
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('constraint-test-user', 'constraintuser', 'constraint@example.com', 'Constraint User', 'level-5', 'hash')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO teams (id, name, team_lead_id)
				VALUES ('constraint-test-team', 'Constraint Team', 'constraint-test-user')
			`)
			Expect(err).NotTo(HaveOccurred())

			_, err = db.Exec(`
				INSERT INTO health_check_sessions (id, team_id, user_id, date, completed)
				VALUES ('constraint-test-session', 'constraint-test-team', 'constraint-test-user', '2024-01-15', false)
			`)
			Expect(err).NotTo(HaveOccurred())

			// Create health dimensions needed for response tests
			_, err = db.Exec(`
				INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight)
				VALUES
					('dim-mission', 'Mission', 'Test dimension', 'Good', 'Bad', true, 1.0),
					('dim-improving', 'Improving', 'Test dimension', 'Good', 'Bad', true, 1.0),
					('dim-stable', 'Stable', 'Test dimension', 'Good', 'Bad', true, 1.0),
					('dim-declining', 'Declining', 'Test dimension', 'Good', 'Bad', true, 1.0),
					('dim-comment-ok', 'Comment OK', 'Test dimension', 'Good', 'Bad', true, 1.0),
					('dim-comment-long', 'Comment Long', 'Test dimension', 'Good', 'Bad', true, 1.0)
				ON CONFLICT (id) DO NOTHING
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		Context("score range validation", func() {
			It("should accept valid score values (1, 2, 3)", func() {
				for _, score := range []int{1, 2, 3} {
					_, err := db.Exec(`
						INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
						VALUES ('constraint-test-session', $1, $2, 'stable')
					`, "dim-mission", score)
					// Clean up for next iteration
					if err == nil {
						db.Exec(`DELETE FROM health_check_responses WHERE session_id = 'constraint-test-session' AND dimension_id = $1`, "dim-mission")
					}
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should reject score values below 1", func() {
				_, err := db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES ('constraint-test-session', 'dim-test-0', 0, 'stable')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_responses_score_range"))
			})

			It("should reject score values above 3", func() {
				_, err := db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES ('constraint-test-session', 'dim-test-4', 4, 'stable')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_responses_score_range"))
			})
		})

		Context("trend value validation", func() {
			It("should accept valid trend values", func() {
				for _, trend := range []string{"improving", "stable", "declining"} {
					_, err := db.Exec(`
						INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
						VALUES ('constraint-test-session', $1, 2, $2)
					`, "dim-"+trend, trend)
					Expect(err).NotTo(HaveOccurred())
				}
			})

			It("should reject invalid trend values", func() {
				_, err := db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES ('constraint-test-session', 'dim-invalid-trend', 2, 'unknown')
				`)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_responses_trend_values"))
			})
		})

		Context("comment length validation", func() {
			It("should accept comments within length limit", func() {
				comment := ""
				for i := 0; i < 500; i++ {
					comment += "a"
				}
				_, err := db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES ('constraint-test-session', 'dim-comment-ok', 2, 'stable', $1)
				`, comment)
				Expect(err).NotTo(HaveOccurred())
			})

			It("should reject comments exceeding 1000 characters", func() {
				comment := ""
				for i := 0; i < 1001; i++ {
					comment += "a"
				}
				_, err := db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend, comment)
					VALUES ('constraint-test-session', 'dim-comment-long', 2, 'stable', $1)
				`, comment)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("chk_responses_comment_length"))
			})
		})
	})

	Describe("Team cadence constraints", func() {
		BeforeEach(func() {
			// Create a user for team lead
			_, err := db.Exec(`
				INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
				VALUES ('cadence-test-user', 'cadenceuser', 'cadence@example.com', 'Cadence User', 'level-4', 'hash')
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should accept valid cadence values", func() {
			for i, cadence := range []string{"monthly", "quarterly", "half-yearly", "yearly"} {
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, $2, 'cadence-test-user', $3)
				`, "cadence-test-team-"+cadence, "Team "+cadence, cadence)
				Expect(err).NotTo(HaveOccurred(), "Failed for cadence %s at index %d", cadence, i)
			}
		})

		It("should accept NULL cadence", func() {
			_, err := db.Exec(`
				INSERT INTO teams (id, name, team_lead_id, cadence)
				VALUES ('cadence-test-null', 'Team Null Cadence', 'cadence-test-user', NULL)
			`)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject invalid cadence values", func() {
			_, err := db.Exec(`
				INSERT INTO teams (id, name, team_lead_id, cadence)
				VALUES ('cadence-test-invalid', 'Team Invalid', 'cadence-test-user', 'daily')
			`)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("chk_teams_cadence_values"))
		})

		It("should reject legacy cadence values (weekly, biweekly)", func() {
			for _, cadence := range []string{"weekly", "biweekly"} {
				_, err := db.Exec(`
					INSERT INTO teams (id, name, team_lead_id, cadence)
					VALUES ($1, $2, 'cadence-test-user', $3)
				`, "cadence-test-legacy-"+cadence, "Team "+cadence, cadence)
				Expect(err).To(HaveOccurred(), "Should reject legacy cadence %s", cadence)
				Expect(err.Error()).To(ContainSubstring("chk_teams_cadence_values"))
			}
		})
	})

	Describe("Foreign key constraints", func() {
		Context("health_check_responses foreign keys", func() {
			It("should cascade delete responses when session is deleted", func() {
				// Create user, team, session, dimension, and response
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('fk-delete-user', 'fkdeleteuser', 'fkdelete@example.com', 'FK Delete User', 'level-5', 'hash')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('fk-delete-team', 'FK Delete Team', 'fk-delete-user')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, completed)
					VALUES ('fk-delete-session', 'fk-delete-team', 'fk-delete-user', '2024-01-15', false)
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight)
					VALUES ('dim-fk-test', 'FK Test Dimension', 'Test', 'Good', 'Bad', true, 1.0)
					ON CONFLICT (id) DO NOTHING
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES ('fk-delete-session', 'dim-fk-test', 2, 'stable')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Verify response exists
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE session_id = 'fk-delete-session'").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(1))

				// Delete the session - this should cascade to responses
				_, err = db.Exec("DELETE FROM health_check_sessions WHERE id = 'fk-delete-session'")
				Expect(err).NotTo(HaveOccurred())

				// Verify response was deleted (FK constraint with ON DELETE CASCADE)
				err = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE session_id = 'fk-delete-session'").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(0))
			})

			It("should prevent deleting dimensions that have responses (soft delete required)", func() {
				// Dimensions should be soft-deleted (is_active = false) rather than hard-deleted
				// since historical responses reference them

				// Create user, team, session, dimension, and response
				_, err := db.Exec(`
					INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash)
					VALUES ('fk-dim-delete-user', 'fkdimdel', 'fkdimdel@example.com', 'FK Dim Delete User', 'level-5', 'hash')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO teams (id, name, team_lead_id)
					VALUES ('fk-dim-delete-team', 'FK Dim Delete Team', 'fk-dim-delete-user')
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_sessions (id, team_id, user_id, date, completed)
					VALUES ('fk-dim-delete-session', 'fk-dim-delete-team', 'fk-dim-delete-user', '2024-01-15', false)
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_dimensions (id, name, description, good_description, bad_description, is_active, weight)
					VALUES ('dim-to-delete', 'Dimension To Delete', 'Test', 'Good', 'Bad', true, 1.0)
				`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`
					INSERT INTO health_check_responses (session_id, dimension_id, score, trend)
					VALUES ('fk-dim-delete-session', 'dim-to-delete', 2, 'stable')
				`)
				Expect(err).NotTo(HaveOccurred())

				// Attempting to delete a dimension with responses should fail (RESTRICT)
				_, err = db.Exec("DELETE FROM health_dimensions WHERE id = 'dim-to-delete'")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("violates foreign key constraint"))

				// Soft delete should work instead - set is_active = false
				_, err = db.Exec("UPDATE health_dimensions SET is_active = false WHERE id = 'dim-to-delete'")
				Expect(err).NotTo(HaveOccurred())

				// Verify dimension is now inactive
				var isActive bool
				err = db.QueryRow("SELECT is_active FROM health_dimensions WHERE id = 'dim-to-delete'").Scan(&isActive)
				Expect(err).NotTo(HaveOccurred())
				Expect(isActive).To(BeFalse())

				// Historical response should still exist
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE dimension_id = 'dim-to-delete'").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(1))
			})
		})

		// Note: FK constraints for health_check_sessions to users/teams are NOT added
		// because demo seed data may reference non-existent users.
		// Application-level validation ensures referential integrity.
		// These constraints should be added after fixing the seed data in a future migration.
	})
})
