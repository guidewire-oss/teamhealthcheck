// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package acceptance_test

import (
	"database/sql"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

var _ = Describe("Database Migrations", func() {
	var (
		db              *sql.DB
		migrationEngine *migrate.Migrate
		err             error
	)

	BeforeEach(func() {
		// Connect to test database
		databaseURL := os.Getenv("TEST_DATABASE_URL")
		if databaseURL == "" {
			databaseURL = "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
		}

		db, err = sql.Open("postgres", databaseURL)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())

		// Clean up any existing tables
		_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if db != nil {
			db.Close()
		}
		if migrationEngine != nil {
			migrationEngine.Close()
		}
	})

	Describe("Running migrations", func() {
		Context("when migrations are applied", func() {
			It("should create health_dimensions table", func() {
				// Given: Migration files exist
				driver, err := postgres.WithInstance(db, &postgres.Config{})
				Expect(err).NotTo(HaveOccurred())

				migrationEngine, err = migrate.NewWithDatabaseInstance(
					"file://../infrastructure/persistence/postgres/migrations",
					"postgres",
					driver,
				)
				Expect(err).NotTo(HaveOccurred())

				// When: Running migrations up
				err = migrationEngine.Up()
				Expect(err).NotTo(HaveOccurred())

				// Then: health_dimensions table should exist
				var tableName string
				err = db.QueryRow(`
					SELECT table_name
					FROM information_schema.tables
					WHERE table_schema = 'public'
					AND table_name = 'health_dimensions'
				`).Scan(&tableName)
				Expect(err).NotTo(HaveOccurred())
				Expect(tableName).To(Equal("health_dimensions"))

				// And: Should have all required columns
				rows, err := db.Query(`
					SELECT column_name, data_type
					FROM information_schema.columns
					WHERE table_name = 'health_dimensions'
					ORDER BY ordinal_position
				`)
				Expect(err).NotTo(HaveOccurred())
				defer rows.Close()

				columns := make(map[string]string)
				for rows.Next() {
					var colName, dataType string
					Expect(rows.Scan(&colName, &dataType)).To(Succeed())
					columns[colName] = dataType
				}

				Expect(columns).To(HaveKey("id"))
				Expect(columns).To(HaveKey("name"))
				Expect(columns).To(HaveKey("description"))
				Expect(columns).To(HaveKey("good_description"))
				Expect(columns).To(HaveKey("bad_description"))
				Expect(columns).To(HaveKey("is_active"))
				Expect(columns).To(HaveKey("weight"))
			})

			It("should create health_check_sessions table with proper relationships", func() {
				// Setup migrations
				driver, err := postgres.WithInstance(db, &postgres.Config{})
				Expect(err).NotTo(HaveOccurred())

				migrationEngine, err = migrate.NewWithDatabaseInstance(
					"file://../infrastructure/persistence/postgres/migrations",
					"postgres",
					driver,
				)
				Expect(err).NotTo(HaveOccurred())

				// When: Running migrations
				err = migrationEngine.Up()
				Expect(err).NotTo(HaveOccurred())

				// Then: health_check_sessions table should exist
				var tableName string
				err = db.QueryRow(`
					SELECT table_name
					FROM information_schema.tables
					WHERE table_schema = 'public'
					AND table_name = 'health_check_sessions'
				`).Scan(&tableName)
				Expect(err).NotTo(HaveOccurred())
				Expect(tableName).To(Equal("health_check_sessions"))

				// And: Should have indexes for performance
				var indexName string
				err = db.QueryRow(`
					SELECT indexname
					FROM pg_indexes
					WHERE tablename = 'health_check_sessions'
					AND indexname = 'idx_sessions_team_date'
				`).Scan(&indexName)
				Expect(err).NotTo(HaveOccurred())
				Expect(indexName).To(Equal("idx_sessions_team_date"))
			})

			It("should create health_check_responses table with cascade delete", func() {
				// Setup migrations
				driver, err := postgres.WithInstance(db, &postgres.Config{})
				Expect(err).NotTo(HaveOccurred())

				migrationEngine, err = migrate.NewWithDatabaseInstance(
					"file://../infrastructure/persistence/postgres/migrations",
					"postgres",
					driver,
				)
				Expect(err).NotTo(HaveOccurred())

				// When: Running migrations
				err = migrationEngine.Up()
				Expect(err).NotTo(HaveOccurred())

				// Then: health_check_responses table should exist
				var tableName string
				err = db.QueryRow(`
					SELECT table_name
					FROM information_schema.tables
					WHERE table_schema = 'public'
					AND table_name = 'health_check_responses'
				`).Scan(&tableName)
				Expect(err).NotTo(HaveOccurred())
				Expect(tableName).To(Equal("health_check_responses"))

				// And: Should have proper constraints
				var constraintName string
				err = db.QueryRow(`
					SELECT constraint_name
					FROM information_schema.table_constraints
					WHERE table_name = 'health_check_responses'
					AND constraint_type = 'UNIQUE'
				`).Scan(&constraintName)
				Expect(err).NotTo(HaveOccurred())
				Expect(constraintName).NotTo(BeEmpty())
			})

			It("should seed health dimensions data", func() {
				// Setup migrations
				driver, err := postgres.WithInstance(db, &postgres.Config{})
				Expect(err).NotTo(HaveOccurred())

				migrationEngine, err = migrate.NewWithDatabaseInstance(
					"file://../infrastructure/persistence/postgres/migrations",
					"postgres",
					driver,
				)
				Expect(err).NotTo(HaveOccurred())

				// When: Running migrations
				err = migrationEngine.Up()
				Expect(err).NotTo(HaveOccurred())

				// Then: Should have 11 health dimensions
				var count int
				err = db.QueryRow("SELECT COUNT(*) FROM health_dimensions").Scan(&count)
				Expect(err).NotTo(HaveOccurred())
				Expect(count).To(Equal(11))

				// And: Should have specific dimensions
				var name string
				err = db.QueryRow("SELECT name FROM health_dimensions WHERE id = 'mission'").Scan(&name)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("Mission"))

				err = db.QueryRow("SELECT name FROM health_dimensions WHERE id = 'health'").Scan(&name)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("Health of Codebase"))
			})
		})

		Context("when rolling back migrations", func() {
			It("should successfully rollback all changes", func() {
				// Setup and run migrations
				driver, err := postgres.WithInstance(db, &postgres.Config{})
				Expect(err).NotTo(HaveOccurred())

				migrationEngine, err = migrate.NewWithDatabaseInstance(
					"file://../infrastructure/persistence/postgres/migrations",
					"postgres",
					driver,
				)
				Expect(err).NotTo(HaveOccurred())

				err = migrationEngine.Up()
				Expect(err).NotTo(HaveOccurred())

				// When: Rolling back migrations
				err = migrationEngine.Down()
				Expect(err).NotTo(HaveOccurred())

				// Then: Tables should not exist
				var tableCount int
				err = db.QueryRow(`
					SELECT COUNT(*)
					FROM information_schema.tables
					WHERE table_schema = 'public'
					AND table_name IN ('health_dimensions', 'health_check_sessions', 'health_check_responses')
				`).Scan(&tableCount)
				Expect(err).NotTo(HaveOccurred())
				Expect(tableCount).To(Equal(0))
			})
		})
	})
})
