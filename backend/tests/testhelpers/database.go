// Copyright 2025 Guidewire Software, Inc.
// SPDX-License-Identifier: Apache-2.0

package testhelpers

import (
	"database/sql"
	"os"

	"github.com/golang-migrate/migrate/v4"
	migratePostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// GetTestDatabaseURL returns the test database URL from environment or default
func GetTestDatabaseURL() string {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres:postgres@localhost:5432/teams360_test?sslmode=disable"
	}
	return databaseURL
}

// SetupTestDatabase creates a fresh database connection and runs migrations
// Returns the database connection and a cleanup function
func SetupTestDatabase() (*sql.DB, func()) {
	databaseURL := GetTestDatabaseURL()

	// Open database connection
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		panic("failed to connect to test database: " + err.Error())
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		db.Close()
		panic("failed to ping test database: " + err.Error())
	}

	// Clean database schema
	_, err = db.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		db.Close()
		panic("failed to clean database schema: " + err.Error())
	}

	// Run migrations
	if err := RunMigrations(db); err != nil {
		db.Close()
		panic("failed to run migrations: " + err.Error())
	}

	// Return cleanup function
	cleanup := func() {
		if db != nil {
			db.Close()
		}
	}

	return db, cleanup
}

// RunMigrations runs database migrations from the migrations directory
// Migration path is relative to tests/integration/ directory
func RunMigrations(db *sql.DB) error {
	driver, err := migratePostgres.WithInstance(db, &migratePostgres.Config{})
	if err != nil {
		return err
	}

	migrationEngine, err := migrate.NewWithDatabaseInstance(
		"file://../../infrastructure/persistence/postgres/migrations",
		"postgres",
		driver,
	)
	if err != nil {
		return err
	}

	return migrationEngine.Up()
}
