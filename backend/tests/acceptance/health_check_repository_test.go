// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"context"
	"database/sql"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agopalakrishnan/teams360/backend/domain/healthcheck"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

var _ = Describe("HealthCheckRepository", func() {
	var (
		db         *sql.DB
		repository healthcheck.Repository
		ctx        context.Context
		err        error
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Connect to test database
		databaseURL := os.Getenv("TEST_DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
		}

		db, err = sql.Open("postgres", databaseURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		// Clean and run migrations
		_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
		Expect(err).NotTo(HaveOccurred())

		driver, err := migratePostgres.WithInstance(db, &migratePostgres.Config{})
		Expect(err).NotTo(HaveOccurred())

		migrationEngine, err := migrate.NewWithDatabaseInstance(
			"file://../../infrastructure/persistence/postgres/migrations",
			"postgres",
			driver,
		)
		Expect(err).NotTo(HaveOccurred())

		err = migrationEngine.Up()
		Expect(err).NotTo(HaveOccurred())

		// Initialize repository
		repository = postgres.NewHealthCheckRepository(db)
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
	})

	Describe("Save and FindByID", func() {
		Context("when saving a complete health check session", func() {
			It("should persist the session and all responses", func() {
				// Given: A valid health check session with multiple responses
				session := &healthcheck.HealthCheckSession{
					ID:               "test-session-001",
					TeamID:           "team1",
					UserID:           "user123",
					Date:             time.Now().Format("2006-01-02"),
					AssessmentPeriod: "2024 - 2nd Half",
					Responses: []healthcheck.HealthCheckResponse{
						{
							DimensionID: "mission",
							Score:       3,
							Trend:       "improving",
							Comment:     "Great clarity on our mission",
						},
						{
							DimensionID: "value",
							Score:       2,
							Trend:       "stable",
							Comment:     "Some improvements needed",
						},
						{
							DimensionID: "speed",
							Score:       1,
							Trend:       "declining",
							Comment:     "Too many blockers",
						},
					},
					Completed: true,
				}

				// When: Saving the session
				err := repository.Save(ctx, session)

				// Then: Should save without error
				Expect(err).NotTo(HaveOccurred())

				// And: Should be retrievable by ID
				retrieved, err := repository.FindByID(ctx, "test-session-001")
				Expect(err).NotTo(HaveOccurred())
				Expect(retrieved).NotTo(BeNil())
				Expect(retrieved.ID).To(Equal("test-session-001"))
				Expect(retrieved.TeamID).To(Equal("team1"))
				Expect(retrieved.UserID).To(Equal("user123"))
				Expect(retrieved.AssessmentPeriod).To(Equal("2024 - 2nd Half"))
				Expect(retrieved.Completed).To(BeTrue())

				// And: Should have all responses
				Expect(retrieved.Responses).To(HaveLen(3))

				// And: Responses should have correct data
				missionResponse := findResponse(retrieved.Responses, "mission")
				Expect(missionResponse).NotTo(BeNil())
				Expect(missionResponse.Score).To(Equal(3))
				Expect(missionResponse.Trend).To(Equal("improving"))
				Expect(missionResponse.Comment).To(Equal("Great clarity on our mission"))

				speedResponse := findResponse(retrieved.Responses, "speed")
				Expect(speedResponse).NotTo(BeNil())
				Expect(speedResponse.Score).To(Equal(1))
				Expect(speedResponse.Trend).To(Equal("declining"))
			})

			It("should enforce score constraints (1-3)", func() {
				// Given: A session with invalid score
				session := &healthcheck.HealthCheckSession{
					ID:     "test-session-002",
					TeamID: "team1",
					UserID: "user123",
					Date:   time.Now().Format("2006-01-02"),
					Responses: []healthcheck.HealthCheckResponse{
						{
							DimensionID: "mission",
							Score:       5, // Invalid score
							Trend:       "improving",
						},
					},
					Completed: true,
				}

				// When: Attempting to save
				err := repository.Save(ctx, session)

				// Then: Should return an error
				Expect(err).To(HaveOccurred())
			})

			It("should enforce trend constraints", func() {
				// Given: A session with invalid trend
				session := &healthcheck.HealthCheckSession{
					ID:     "test-session-003",
					TeamID: "team1",
					UserID: "user123",
					Date:   time.Now().Format("2006-01-02"),
					Responses: []healthcheck.HealthCheckResponse{
						{
							DimensionID: "mission",
							Score:       3,
							Trend:       "invalid-trend", // Invalid trend
						},
					},
					Completed: true,
				}

				// When: Attempting to save
				err := repository.Save(ctx, session)

				// Then: Should return an error
				Expect(err).To(HaveOccurred())
			})

			It("should prevent duplicate responses for same dimension", func() {
				// Given: A session with duplicate dimension responses
				session := &healthcheck.HealthCheckSession{
					ID:     "test-session-004",
					TeamID: "team1",
					UserID: "user123",
					Date:   time.Now().Format("2006-01-02"),
					Responses: []healthcheck.HealthCheckResponse{
						{
							DimensionID: "mission",
							Score:       3,
							Trend:       "improving",
						},
						{
							DimensionID: "mission", // Duplicate
							Score:       2,
							Trend:       "stable",
						},
					},
					Completed: true,
				}

				// When: Attempting to save
				err := repository.Save(ctx, session)

				// Then: Should return an error
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when session doesn't exist", func() {
			It("should return error for non-existent ID", func() {
				// When: Finding a non-existent session
				session, err := repository.FindByID(ctx, "non-existent-id")

				// Then: Should return error
				Expect(err).To(HaveOccurred())
				Expect(session).To(BeNil())
			})
		})
	})

	Describe("FindByTeamID", func() {
		Context("when team has multiple sessions", func() {
			It("should return all sessions for the team", func() {
				// Given: Multiple sessions for a team
				session1 := &healthcheck.HealthCheckSession{
					ID:        "session-team1-001",
					TeamID:    "team1",
					UserID:    "user123",
					Date:      "2024-01-15",
					Completed: true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "mission", Score: 3, Trend: "improving"},
					},
				}

				session2 := &healthcheck.HealthCheckSession{
					ID:        "session-team1-002",
					TeamID:    "team1",
					UserID:    "user456",
					Date:      "2024-01-16",
					Completed: true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "value", Score: 2, Trend: "stable"},
					},
				}

				Expect(repository.Save(ctx, session1)).To(Succeed())
				Expect(repository.Save(ctx, session2)).To(Succeed())

				// When: Finding sessions by team ID
				sessions, err := repository.FindByTeamID(ctx, "team1")

				// Then: Should return all sessions
				Expect(err).NotTo(HaveOccurred())
				Expect(sessions).To(HaveLen(2))

				// And: Should be ordered by date descending (most recent first)
				Expect(sessions[0].Date).To(Equal("2024-01-16"))
				Expect(sessions[1].Date).To(Equal("2024-01-15"))
			})
		})

		Context("when team has no sessions", func() {
			It("should return empty slice", func() {
				// When: Finding sessions for team with no data
				sessions, err := repository.FindByTeamID(ctx, "empty-team")

				// Then: Should return empty slice
				Expect(err).NotTo(HaveOccurred())
				Expect(sessions).To(BeEmpty())
			})
		})
	})

	Describe("FindByUserID", func() {
		Context("when user has submitted multiple health checks", func() {
			It("should return all sessions for the user", func() {
				// Given: Multiple sessions from same user
				session1 := &healthcheck.HealthCheckSession{
					ID:        "session-user-001",
					TeamID:    "team1",
					UserID:    "user123",
					Date:      "2024-01-10",
					Completed: true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "mission", Score: 3, Trend: "improving"},
					},
				}

				session2 := &healthcheck.HealthCheckSession{
					ID:        "session-user-002",
					TeamID:    "team2",
					UserID:    "user123",
					Date:      "2024-02-15",
					Completed: true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "value", Score: 2, Trend: "stable"},
					},
				}

				Expect(repository.Save(ctx, session1)).To(Succeed())
				Expect(repository.Save(ctx, session2)).To(Succeed())

				// When: Finding sessions by user ID
				sessions, err := repository.FindByUserID(ctx, "user123")

				// Then: Should return all user sessions
				Expect(err).NotTo(HaveOccurred())
				Expect(sessions).To(HaveLen(2))

				// And: Should be ordered by date descending
				Expect(sessions[0].Date).To(Equal("2024-02-15"))
				Expect(sessions[1].Date).To(Equal("2024-01-10"))
			})
		})
	})

	Describe("FindByAssessmentPeriod", func() {
		Context("when sessions exist for a specific assessment period", func() {
			It("should return only sessions from that period", func() {
				// Given: Sessions from different assessment periods
				session1 := &healthcheck.HealthCheckSession{
					ID:               "session-period-001",
					TeamID:           "team1",
					UserID:           "user123",
					Date:             "2024-01-15",
					AssessmentPeriod: "2023 - 2nd Half",
					Completed:        true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "mission", Score: 3, Trend: "improving"},
					},
				}

				session2 := &healthcheck.HealthCheckSession{
					ID:               "session-period-002",
					TeamID:           "team1",
					UserID:           "user456",
					Date:             "2024-07-20",
					AssessmentPeriod: "2024 - 1st Half",
					Completed:        true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "value", Score: 2, Trend: "stable"},
					},
				}

				Expect(repository.Save(ctx, session1)).To(Succeed())
				Expect(repository.Save(ctx, session2)).To(Succeed())

				// When: Finding sessions by assessment period
				sessions, err := repository.FindByAssessmentPeriod(ctx, "2024 - 1st Half")

				// Then: Should return only matching sessions
				Expect(err).NotTo(HaveOccurred())
				Expect(sessions).To(HaveLen(1))
				Expect(sessions[0].AssessmentPeriod).To(Equal("2024 - 1st Half"))
				Expect(sessions[0].ID).To(Equal("session-period-002"))
			})
		})
	})

	Describe("Delete", func() {
		Context("when deleting an existing session", func() {
			It("should remove the session and all its responses (cascade)", func() {
				// Given: An existing session with responses
				session := &healthcheck.HealthCheckSession{
					ID:        "session-delete-001",
					TeamID:    "team1",
					UserID:    "user123",
					Date:      time.Now().Format("2006-01-02"),
					Completed: true,
					Responses: []healthcheck.HealthCheckResponse{
						{DimensionID: "mission", Score: 3, Trend: "improving"},
						{DimensionID: "value", Score: 2, Trend: "stable"},
					},
				}

				Expect(repository.Save(ctx, session)).To(Succeed())

				// When: Deleting the session
				err := repository.Delete(ctx, "session-delete-001")

				// Then: Should delete without error
				Expect(err).NotTo(HaveOccurred())

				// And: Session should no longer exist
				retrieved, err := repository.FindByID(ctx, "session-delete-001")
				Expect(err).To(HaveOccurred())
				Expect(retrieved).To(BeNil())

				// And: Responses should also be deleted (cascade)
				var responseCount int
				err = db.QueryRow("SELECT COUNT(*) FROM health_check_responses WHERE session_id = $1", "session-delete-001").Scan(&responseCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(responseCount).To(Equal(0))
			})
		})
	})
})

// Helper function to find a response by dimension ID
func findResponse(responses []healthcheck.HealthCheckResponse, dimensionID string) *healthcheck.HealthCheckResponse {
	for _, r := range responses {
		if r.DimensionID == dimensionID {
			return &r
		}
	}
	return nil
}
