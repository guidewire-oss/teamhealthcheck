package integration_test

import (
	"context"
	"database/sql"
	"os"
	"time"

	"github.com/agopalakrishnan/teams360/backend/application/services"
	"github.com/agopalakrishnan/teams360/backend/infrastructure/persistence/postgres"
	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
	"github.com/agopalakrishnan/teams360/backend/tests/testhelpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// These specs exercise the contract v1.1 podiqRole mapping (L2 -> level-3,
// L1 -> level-4, Developer -> level-5) through the real sync service and the
// real repository, because it is the last write inside the apply transaction.
var _ = Describe("Integration: PodIQ role level mapping", func() {
	var (
		db      *sql.DB
		cleanup func()
	)

	BeforeEach(func() {
		os.Setenv(services.EnvMaxDeletePercent, "100")
		db, cleanup = testhelpers.SetupTestDatabase()
	})

	AfterEach(func() {
		cleanup()
		os.Unsetenv(services.EnvMaxDeletePercent)
	})

	ptr := func(s string) *string { return &s }

	snapUser := func(id, level string, role *string) orgsnapshot.User {
		return orgsnapshot.User{
			ID: id, Username: id, DisplayName: id,
			Email: id + "@test.com", HierarchyLevelID: level, PodIQRole: role,
		}
	}
	syncOnce := func(version string, users []orgsnapshot.User, teams []orgsnapshot.Team, memberships []orgsnapshot.Membership) (*services.SyncResult, error) {
		snap := &orgsnapshot.Snapshot{
			ContractVersion: version,
			SnapshotType:    orgsnapshot.TypePartial,
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
		return service.Sync(context.Background())
	}
	runSync := func(version string, users []orgsnapshot.User, teams []orgsnapshot.Team, memberships []orgsnapshot.Membership) *services.SyncResult {
		result, err := syncOnce(version, users, teams, memberships)
		Expect(err).NotTo(HaveOccurred())
		return result
	}
	levelOf := func(id string) string {
		var level string
		Expect(db.QueryRow(`SELECT hierarchy_level_id FROM users WHERE id = $1`, id).Scan(&level)).To(Succeed())
		return level
	}
	team := func(id string, lead *string) orgsnapshot.Team {
		return orgsnapshot.Team{ID: id, Name: id, TeamLeadID: lead}
	}
	member := func(user, team string) orgsnapshot.Membership {
		return orgsnapshot.Membership{UserID: user, TeamID: team}
	}

	It("accepts a v1.0 and a v1.1 snapshot", func() {
		users := []orgsnapshot.User{snapUser("pq-a", "level-2", nil)}
		teams := []orgsnapshot.Team{team("pq-t", nil)}
		runSync(orgsnapshot.ContractVersion, users, teams, []orgsnapshot.Membership{member("pq-a", "pq-t")})
		runSync(orgsnapshot.ContractVersionV11, users, teams, []orgsnapshot.Membership{member("pq-a", "pq-t")})
		Expect(levelOf("pq-a")).To(Equal("level-2"))
	})

	Describe("without a podiqRole", func() {
		It("keeps the legacy pod-lead promotion on a v1.0 snapshot", func() {
			runSync(orgsnapshot.ContractVersion,
				[]orgsnapshot.User{snapUser("pq-lead", "level-5", nil), snapUser("pq-dev", "level-5", nil)},
				[]orgsnapshot.Team{team("pq-t", ptr("pq-lead"))},
				[]orgsnapshot.Membership{member("pq-lead", "pq-t"), member("pq-dev", "pq-t")})

			Expect(levelOf("pq-lead")).To(Equal("level-4"))
			Expect(levelOf("pq-dev")).To(Equal("level-5"))
		})

		It("keeps the legacy provider level and pod-lead promotion on a v1.1 snapshot", func() {
			result := runSync(orgsnapshot.ContractVersionV11,
				[]orgsnapshot.User{
					snapUser("pq-lead", "level-5", nil),
					snapUser("pq-dev", "level-5", nil),
					snapUser("pq-mgr", "level-2", nil),
				},
				[]orgsnapshot.Team{team("pq-t", ptr("pq-lead"))},
				[]orgsnapshot.Membership{member("pq-lead", "pq-t"), member("pq-dev", "pq-t"), member("pq-mgr", "pq-t")})

			Expect(levelOf("pq-lead")).To(Equal("level-4"))
			Expect(levelOf("pq-dev")).To(Equal("level-5"))
			Expect(levelOf("pq-mgr")).To(Equal("level-2"))
			Expect(result.PodLeadsPromoted).To(Equal(1))
		})
	})

	DescribeTable("stores a role at its level whatever level the provider sent",
		func(role, stored string, providerLevels ...string) {
			var users []orgsnapshot.User
			var memberships []orgsnapshot.Membership
			for i, level := range providerLevels {
				id := "pq-u" + string(rune('a'+i))
				users = append(users, snapUser(id, level, ptr(role)))
				memberships = append(memberships, member(id, "pq-t"))
			}
			// The first user is the named teamLeadId, which must not matter.
			runSync(orgsnapshot.ContractVersionV11, users,
				[]orgsnapshot.Team{team("pq-t", ptr(users[0].ID))}, memberships)

			for _, u := range users {
				Expect(levelOf(u.ID)).To(Equal(stored), u.ID)
			}
		},
		Entry("L2 -> level-3", "L2", "level-3", "level-1", "level-3", "level-4", "level-5"),
		Entry("L1 -> level-4", "L1", "level-4", "level-5", "level-2", "level-3", "level-4"),
		Entry("Developer -> level-5", "Developer", "level-5", "level-4", "level-1", "level-3", "level-5"),
	)

	It("stores every member of a multi-L1 pod with no teamLeadId at level-4", func() {
		runSync(orgsnapshot.ContractVersionV11,
			[]orgsnapshot.User{
				snapUser("pq-l1a", "level-5", ptr("L1")),
				snapUser("pq-l1b", "level-3", ptr("L1")),
				snapUser("pq-dev", "level-5", ptr("Developer")),
			},
			[]orgsnapshot.Team{team("pq-t", nil)},
			[]orgsnapshot.Membership{member("pq-l1a", "pq-t"), member("pq-l1b", "pq-t"), member("pq-dev", "pq-t")})

		Expect(levelOf("pq-l1a")).To(Equal("level-4"))
		Expect(levelOf("pq-l1b")).To(Equal("level-4"))
		Expect(levelOf("pq-dev")).To(Equal("level-5"))
	})

	It("does not apply the pod-lead override to a role-mapped lead", func() {
		runSync(orgsnapshot.ContractVersionV11,
			[]orgsnapshot.User{snapUser("pq-lead", "level-5", ptr("Developer"))},
			[]orgsnapshot.Team{team("pq-t", ptr("pq-lead"))},
			[]orgsnapshot.Membership{member("pq-lead", "pq-t")})

		Expect(levelOf("pq-lead")).To(Equal("level-5"))
	})

	It("moves a stored level-4 user to level-5 when their role becomes Developer", func() {
		users := func(role string) []orgsnapshot.User {
			return []orgsnapshot.User{snapUser("pq-lead", "level-5", ptr(role))}
		}
		teams := []orgsnapshot.Team{team("pq-t", ptr("pq-lead"))}
		memberships := []orgsnapshot.Membership{member("pq-lead", "pq-t")}

		runSync(orgsnapshot.ContractVersionV11, users("L1"), teams, memberships)
		Expect(levelOf("pq-lead")).To(Equal("level-4"))
		runSync(orgsnapshot.ContractVersionV11, users("Developer"), teams, memberships)
		Expect(levelOf("pq-lead")).To(Equal("level-5"))
	})

	It("stores a role-mapped user whose provider level THC does not have", func() {
		runSync(orgsnapshot.ContractVersionV11,
			[]orgsnapshot.User{snapUser("pq-hi", "level-9", ptr("L2")), snapUser("pq-in", "level-3", ptr("L2"))},
			[]orgsnapshot.Team{team("pq-t", nil)},
			[]orgsnapshot.Membership{member("pq-hi", "pq-t"), member("pq-in", "pq-t")})

		Expect(levelOf("pq-hi")).To(Equal("level-3"))
	})

	It("never changes a protected user", func() {
		_, err := db.Exec(`
			INSERT INTO users (id, username, email, full_name, hierarchy_level_id, password_hash, auth_type)
			VALUES ('pq-admin', 'pqadmin', 'pqadmin@test.com', 'pqadmin', 'level-admin', '', 'sso')`)
		Expect(err).NotTo(HaveOccurred())

		runSync(orgsnapshot.ContractVersionV11,
			[]orgsnapshot.User{snapUser("pq-admin", "level-5", ptr("L1")), snapUser("pq-ok", "level-5", ptr("L1"))},
			[]orgsnapshot.Team{team("pq-t", nil)},
			[]orgsnapshot.Membership{member("pq-ok", "pq-t")})

		Expect(levelOf("pq-admin")).To(Equal("level-admin"))
		Expect(levelOf("pq-ok")).To(Equal("level-4"))
	})

	It("rejects an invalid podiqRole and changes nothing", func() {
		_, err := syncOnce(orgsnapshot.ContractVersionV11,
			[]orgsnapshot.User{snapUser("pq-bad", "level-5", ptr("Manager"))},
			[]orgsnapshot.Team{team("pq-t", nil)},
			[]orgsnapshot.Membership{member("pq-bad", "pq-t")})

		Expect(err).To(MatchError(services.ErrInvalidSnapshot))
		Expect(err.Error()).To(ContainSubstring("podiqRole"))
		var n int
		Expect(db.QueryRow(`SELECT COUNT(*) FROM users WHERE id = 'pq-bad'`).Scan(&n)).To(Succeed())
		Expect(n).To(Equal(0))
	})

	It("rejects an invalid podiqRole even when the provider level is unknown to THC", func() {
		_, err := syncOnce(orgsnapshot.ContractVersionV11,
			[]orgsnapshot.User{snapUser("pq-bad", "level-9", ptr("Manager")), snapUser("pq-ok", "level-3", nil)},
			[]orgsnapshot.Team{team("pq-t", nil)},
			[]orgsnapshot.Membership{member("pq-ok", "pq-t")})

		Expect(err).To(MatchError(services.ErrInvalidSnapshot))
		Expect(err.Error()).To(ContainSubstring("podiqRole"))
	})

	It("rejects podiqRole in a v1.0 snapshot", func() {
		_, err := syncOnce(orgsnapshot.ContractVersion,
			[]orgsnapshot.User{snapUser("pq-a", "level-3", ptr("L2"))},
			[]orgsnapshot.Team{team("pq-t", nil)},
			[]orgsnapshot.Membership{member("pq-a", "pq-t")})

		Expect(err).To(MatchError(services.ErrInvalidSnapshot))
		Expect(err.Error()).To(ContainSubstring("podiqRole"))
	})

	It("creates no level update on a second identical v1.1 sync", func() {
		users := []orgsnapshot.User{
			snapUser("pq-l2", "level-5", ptr("L2")),
			snapUser("pq-l1", "level-5", ptr("L1")),
			snapUser("pq-dev", "level-3", ptr("Developer")),
			snapUser("pq-legacy", "level-5", nil),
		}
		teams := []orgsnapshot.Team{team("pq-t", ptr("pq-dev"))}
		memberships := []orgsnapshot.Membership{member("pq-l2", "pq-t"), member("pq-l1", "pq-t"), member("pq-dev", "pq-t"), member("pq-legacy", "pq-t")}

		first := runSync(orgsnapshot.ContractVersionV11, users, teams, memberships)
		second := runSync(orgsnapshot.ContractVersionV11, users, teams, memberships)

		Expect(first.InsertedUsers).To(Equal(4))
		Expect(second.InsertedUsers).To(Equal(0))
		Expect(second.UpdatedUsers).To(Equal(0))
		Expect(levelOf("pq-l2")).To(Equal("level-3"))
		Expect(levelOf("pq-l1")).To(Equal("level-4"))
		Expect(levelOf("pq-dev")).To(Equal("level-5"))
	})
})
