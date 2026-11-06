package integration_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/domain/orgprovider"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// podLeadFetcher hands the sync whatever snapshot a spec built, so each spec
// controls exactly what the provider "sent".
type podLeadFetcher struct {
	snapshot *orgsnapshot.Snapshot
}

func (f podLeadFetcher) FetchSnapshot(context.Context) (*orgsnapshot.Snapshot, error) {
	return f.snapshot, nil
}

// These specs exercise the THC-only pod-lead mapping (orgprovider.PodLeadLevelID)
// through the real sync service and the real repository, because the mapping
// runs inside the apply transaction and has to be checked against the rows the
// database actually ends up with.
var _ = Describe("Integration: Pod lead level mapping", func() {
	var (
		db      *sql.DB
		cleanup func()
	)

	BeforeEach(func() {
		// Complete snapshots in these specs remove or deactivate a record or two
		// out of a very small population, which the default guard would hold.
		os.Setenv(services.EnvMaxDeletePercent, "100")
		db, cleanup = testhelpers.SetupTestDatabase()
	})

	AfterEach(func() {
		cleanup()
		os.Unsetenv(services.EnvMaxDeletePercent)
	})

	ptr := func(s string) *string { return &s }

	// Stored rows, as THC already has them before the sync runs.
	insertUser := func(id, username, level string) {
		_, err := db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
			VALUES ($1, $2, $3, $2, $4, '', 'sso')
		`, id, username, username+"@test.com", level)
		Expect(err).NotTo(HaveOccurred())
	}
	insertTeam := func(id, name string, lead *string) {
		var leadID sql.NullString
		if lead != nil {
			leadID = sql.NullString{String: *lead, Valid: true}
		}
		_, err := db.Exec(`INSERT INTO teams (id, name, team_lead_id) VALUES ($1, $2, $3)`, id, name, leadID)
		Expect(err).NotTo(HaveOccurred())
	}
	addMember := func(teamID, userID string) {
		_, err := db.Exec(`INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, teamID, userID)
		Expect(err).NotTo(HaveOccurred())
	}
	deactivateTeam := func(id string) {
		_, err := db.Exec(`UPDATE teams SET is_active = false, deactivated_at = CURRENT_TIMESTAMP WHERE id = $1`, id)
		Expect(err).NotTo(HaveOccurred())
	}

	// What the provider sends.
	snapUser := func(id, username, level string) orgsnapshot.User {
		return orgsnapshot.User{
			ID: id, Username: username, DisplayName: username,
			Email: username + "@test.com", HierarchyLevelID: level,
		}
	}
	runSync := func(snapshotType string, users []orgsnapshot.User, teams []orgsnapshot.Team, memberships []orgsnapshot.Membership) *services.SyncResult {
		snap := &orgsnapshot.Snapshot{
			ContractVersion: orgsnapshot.ContractVersion,
			SnapshotType:    snapshotType,
			GeneratedAt:     time.Now().UTC(),
			Users:           users,
			Teams:           teams,
			Memberships:     memberships,
		}
		service := services.NewOrganizationSyncService(
			postgres.NewOrganizationProviderRepository(db),
			podLeadFetcher{snapshot: snap},
			postgres.NewUserRepository(db),
			postgres.NewTeamRepository(db),
		)
		result, err := service.Sync(context.Background())
		Expect(err).NotTo(HaveOccurred())
		return result
	}

	levelOf := func(id string) string {
		var level string
		Expect(db.QueryRow(`SELECT hierarchy_level_id FROM users WHERE id = $1`, id).Scan(&level)).To(Succeed())
		return level
	}

	// A team with a named lead and one other member, both sent at the given level.
	goaSnapshot := func(level string) ([]orgsnapshot.User, []orgsnapshot.Team, []orgsnapshot.Membership) {
		return []orgsnapshot.User{
				snapUser("pl-lead", "pllead", level),
				snapUser("pl-dev", "pldev", level),
			},
			[]orgsnapshot.Team{{ID: "pl-t1", Name: "Goa", TeamLeadID: ptr("pl-lead")}},
			[]orgsnapshot.Membership{{UserID: "pl-lead", TeamID: "pl-t1"}, {UserID: "pl-dev", TeamID: "pl-t1"}}
	}

	It("stores a pod lead sent as level-5 at level-4 and leaves a non-lead alone", func() {
		users, teams, memberships := goaSnapshot("level-5")

		runSync(orgsnapshot.TypePartial, users, teams, memberships)

		Expect(levelOf("pl-lead")).To(Equal("level-4"))
		Expect(levelOf("pl-dev")).To(Equal("level-5"))
	})

	DescribeTable("stores every grade at the documented level",
		func(grade int, leadStored, memberStored string) {
			// The provider's rule: level-(grade - 3).
			users, teams, memberships := goaSnapshot(fmt.Sprintf("level-%d", grade-3))

			runSync(orgsnapshot.TypePartial, users, teams, memberships)

			Expect(levelOf("pl-lead")).To(Equal(leadStored), "pod lead")
			Expect(levelOf("pl-dev")).To(Equal(memberStored), "non-lead")
		},
		Entry("grade 4 is level-1", 4, "level-1", "level-1"),
		Entry("grade 5 is level-2", 5, "level-2", "level-2"),
		Entry("grade 6 is level-3", 6, "level-3", "level-3"),
		Entry("grade 7 is level-4", 7, "level-4", "level-4"),
		Entry("grade 8 is level-5, and a lead is promoted", 8, "level-4", "level-5"),
	)

	It("does not promote a lead the provider names who is not a member of the team", func() {
		users, teams, _ := goaSnapshot("level-5")
		onlyDev := []orgsnapshot.Membership{{UserID: "pl-dev", TeamID: "pl-t1"}}

		runSync(orgsnapshot.TypePartial, users, teams, onlyDev)

		Expect(levelOf("pl-lead")).To(Equal("level-5"))
	})

	It("changes nothing on a second identical sync", func() {
		users, teams, memberships := goaSnapshot("level-5")

		runSync(orgsnapshot.TypePartial, users, teams, memberships)
		second := runSync(orgsnapshot.TypePartial, users, teams, memberships)

		Expect(levelOf("pl-lead")).To(Equal("level-4"))
		Expect(second.InsertedUsers).To(Equal(0))
		Expect(second.UpdatedUsers).To(Equal(0), "a stable pod lead must not show up as updated on every sync")
	})

	Describe("a lead THC already has", func() {
		BeforeEach(func() {
			insertUser("pl-lead", "pllead", "level-5")
			insertTeam("pl-t1", "Goa", ptr("pl-lead"))
			addMember("pl-t1", "pl-lead")
		})

		It("is promoted when the snapshot omits teamLeadId, because THC keeps the stored lead", func() {
			runSync(orgsnapshot.TypePartial,
				[]orgsnapshot.User{snapUser("pl-lead", "pllead", "level-5")},
				[]orgsnapshot.Team{{ID: "pl-t1", Name: "Goa"}},
				[]orgsnapshot.Membership{{UserID: "pl-lead", TeamID: "pl-t1"}})

			Expect(levelOf("pl-lead")).To(Equal("level-4"))
		})

		It("stays at level-4 when the snapshot omits teamLeadId", func() {
			_, err := db.Exec(`UPDATE users SET hierarchy_level_id = 'level-4' WHERE id = 'pl-lead'`)
			Expect(err).NotTo(HaveOccurred())
			users := []orgsnapshot.User{snapUser("pl-lead", "pllead", "level-5")}
			teams := []orgsnapshot.Team{{ID: "pl-t1", Name: "Goa"}}
			memberships := []orgsnapshot.Membership{{UserID: "pl-lead", TeamID: "pl-t1"}}

			result := runSync(orgsnapshot.TypePartial, users, teams, memberships)

			Expect(levelOf("pl-lead")).To(Equal("level-4"))
			Expect(result.UpdatedUsers).To(Equal(0), "keeping the level is not a change")
		})

		It("is promoted when a partial snapshot does not mention their team", func() {
			runSync(orgsnapshot.TypePartial,
				[]orgsnapshot.User{snapUser("pl-lead", "pllead", "level-5"), snapUser("pl-other", "plother", "level-5")},
				[]orgsnapshot.Team{{ID: "pl-t2", Name: "Mysuru"}},
				[]orgsnapshot.Membership{{UserID: "pl-other", TeamID: "pl-t2"}})

			Expect(levelOf("pl-lead")).To(Equal("level-4"))
		})

		It("is returned to level-5 when a complete snapshot removes their team", func() {
			_, err := db.Exec(`UPDATE users SET hierarchy_level_id = 'level-4' WHERE id = 'pl-lead'`)
			Expect(err).NotTo(HaveOccurred())

			runSync(orgsnapshot.TypeComplete,
				[]orgsnapshot.User{snapUser("pl-lead", "pllead", "level-5"), snapUser("pl-other", "plother", "level-5")},
				[]orgsnapshot.Team{{ID: "pl-t2", Name: "Mysuru"}},
				[]orgsnapshot.Membership{{UserID: "pl-other", TeamID: "pl-t2"}, {UserID: "pl-lead", TeamID: "pl-t2"}})

			Expect(levelOf("pl-lead")).To(Equal("level-5"))
		})

		It("is promoted when the snapshot brings their deactivated team back", func() {
			deactivateTeam("pl-t1")

			runSync(orgsnapshot.TypeComplete,
				[]orgsnapshot.User{snapUser("pl-lead", "pllead", "level-5")},
				[]orgsnapshot.Team{{ID: "pl-t1", Name: "Goa"}},
				[]orgsnapshot.Membership{{UserID: "pl-lead", TeamID: "pl-t1"}})

			var active bool
			Expect(db.QueryRow(`SELECT is_active FROM teams WHERE id = 'pl-t1'`).Scan(&active)).To(Succeed())
			Expect(active).To(BeTrue(), "the snapshot lists the team, so it is active again")
			Expect(levelOf("pl-lead")).To(Equal("level-4"))
		})
	})

	It("does not promote a stored lead whose membership of their team is inactive", func() {
		// An inactive membership row is history: the person moved on, and the
		// team still names them. They are not an active lead.
		insertUser("pl-lead", "pllead", "level-5")
		insertTeam("pl-t1", "Goa", ptr("pl-lead"))
		addMember("pl-t1", "pl-lead")
		_, err := db.Exec(`UPDATE team_members SET is_active = false, deactivated_at = CURRENT_TIMESTAMP WHERE team_id = 'pl-t1' AND user_id = 'pl-lead'`)
		Expect(err).NotTo(HaveOccurred())

		runSync(orgsnapshot.TypePartial,
			[]orgsnapshot.User{snapUser("pl-lead", "pllead", "level-5")},
			[]orgsnapshot.Team{{ID: "pl-t2", Name: "Mysuru"}},
			[]orgsnapshot.Membership{{UserID: "pl-lead", TeamID: "pl-t2"}})

		Expect(levelOf("pl-lead")).To(Equal("level-5"))
	})

	It("promotes a lead whose user id the provider has changed", func() {
		// THC knows the person under an old id; the provider now sends the same
		// email and username under a new id and omits teamLeadId. The sync
		// re-keys the row and repoints the team's lead to the new id.
		insertUser("pl-old", "plrekey", "level-5")
		insertTeam("pl-t1", "Goa", ptr("pl-old"))
		addMember("pl-t1", "pl-old")

		runSync(orgsnapshot.TypePartial,
			[]orgsnapshot.User{snapUser("pl-new", "plrekey", "level-5")},
			[]orgsnapshot.Team{{ID: "pl-t1", Name: "Goa"}},
			[]orgsnapshot.Membership{{UserID: "pl-new", TeamID: "pl-t1"}})

		var lead string
		Expect(db.QueryRow(`SELECT team_lead_id FROM teams WHERE id = 'pl-t1'`).Scan(&lead)).To(Succeed())
		Expect(lead).To(Equal("pl-new"))
		Expect(levelOf("pl-new")).To(Equal("level-4"))
	})

	It("moves the promotion when the provider names a different lead", func() {
		insertUser("pl-old", "plold", "level-4")
		insertUser("pl-new", "plnew", "level-5")
		insertTeam("pl-t1", "Goa", ptr("pl-old"))
		addMember("pl-t1", "pl-old")
		addMember("pl-t1", "pl-new")

		runSync(orgsnapshot.TypePartial,
			[]orgsnapshot.User{snapUser("pl-old", "plold", "level-5"), snapUser("pl-new", "plnew", "level-5")},
			[]orgsnapshot.Team{{ID: "pl-t1", Name: "Goa", TeamLeadID: ptr("pl-new")}},
			[]orgsnapshot.Membership{{UserID: "pl-old", TeamID: "pl-t1"}, {UserID: "pl-new", TeamID: "pl-t1"}})

		Expect(levelOf("pl-old")).To(Equal("level-5"), "the former lead returns to the provider's level")
		Expect(levelOf("pl-new")).To(Equal("level-4"), "the new lead is promoted")
	})

	It("promotes the lead a protected team already has, not the one the snapshot names", func() {
		// Persistence never writes a protected team, so the lead the snapshot
		// names is not applied and the stored lead stays the lead.
		insertUser("pl-old", "plold", "level-5")
		insertUser("pl-new", "plnew", "level-5")
		insertTeam("team-phoenix", "Phoenix", ptr("pl-old"))
		addMember("team-phoenix", "pl-old")

		runSync(orgsnapshot.TypePartial,
			[]orgsnapshot.User{snapUser("pl-old", "plold", "level-5"), snapUser("pl-new", "plnew", "level-5")},
			[]orgsnapshot.Team{{ID: "team-phoenix", Name: "Phoenix", TeamLeadID: ptr("pl-new")}},
			[]orgsnapshot.Membership{{UserID: "pl-old", TeamID: "team-phoenix"}, {UserID: "pl-new", TeamID: "team-phoenix"}})

		Expect(levelOf("pl-old")).To(Equal("level-4"))
		Expect(levelOf("pl-new")).To(Equal("level-5"))
	})

	It("leaves every level as the provider sent it when the mapping is off", func() {
		users, teams, memberships := goaSnapshot("level-5")
		snap := &orgsnapshot.Snapshot{
			ContractVersion: orgsnapshot.ContractVersion,
			GeneratedAt:     time.Now().UTC(),
			Users:           users,
			Teams:           teams,
			Memberships:     memberships,
		}

		_, err := postgres.NewOrganizationProviderRepository(db).ApplySnapshot(context.Background(), orgprovider.ApplyInput{
			Snapshot:         snap,
			SnapshotType:     orgprovider.SnapshotPartial,
			MaxDeletePercent: 100,
			// MapPodLeads left at its zero value.
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(levelOf("pl-lead")).To(Equal("level-5"))
	})
})
