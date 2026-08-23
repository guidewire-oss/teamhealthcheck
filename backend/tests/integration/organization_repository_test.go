package integration_test

import (
	"context"
	"database/sql"

	"github.com/agopalakrishnan/teams360/backend/domain/organization"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OrganizationRepository", func() {
	var (
		db      *sql.DB
		cleanup func()
		repo    organization.Repository
		ctx     context.Context
	)

	BeforeEach(func() {
		db, cleanup = testhelpers.SetupTestDatabase()
		repo = postgres.NewOrganizationRepository(db)
		ctx = context.Background()
	})

	AfterEach(func() {
		cleanup()
	})

	Describe("Get CompanyName fallback and configuration loading", func() {
		Context("when app_settings has no company_name configured or row is missing", func() {
			It("falls back to 'My Company'", func() {
				// Clear any app_settings row if present
				_, err := db.ExecContext(ctx, "DELETE FROM app_settings")
				Expect(err).NotTo(HaveOccurred())

				config, err := repo.Get(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.CompanyName).To(Equal("My Company"))
			})

			It("falls back to 'My Company' when GetAppSettings is called on an empty app_settings table", func() {
				_, err := db.ExecContext(ctx, "DELETE FROM app_settings")
				Expect(err).NotTo(HaveOccurred())

				settings, err := repo.GetAppSettings(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(settings.CompanyName).To(Equal("My Company"))
			})
		})

		Context("when app_settings contains a custom company_name", func() {
			It("loads the stored company_name in Get()", func() {
				err := repo.UpdateBrandingSettings(ctx, "Acme Corporation", "")
				Expect(err).NotTo(HaveOccurred())

				config, err := repo.Get(ctx)
				Expect(err).NotTo(HaveOccurred())
				Expect(config.CompanyName).To(Equal("Acme Corporation"))
			})
		})
	})
})
