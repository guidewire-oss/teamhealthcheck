package orgsnapshot_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agopalakrishnan/teams360/backend/pkg/orgsnapshot"
)

func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }

func validSnapshot() orgsnapshot.Snapshot {
	return orgsnapshot.Snapshot{
		ContractVersion: orgsnapshot.ContractVersion,
		GeneratedAt:     time.Unix(1, 0).UTC(),
		Teams: []orgsnapshot.Team{
			{ID: "team-1", Name: "Platform"},
			{ID: "team-2", Name: "Growth", HealthCheckEnabled: boolPtr(true)},
			{ID: "team-3", Name: "Data", HealthCheckEnabled: boolPtr(false)},
			{ID: "team-4", Name: "Infra"}, // HealthCheckEnabled omitted (nil)
		},
		Users: []orgsnapshot.User{
			{ID: "user-1", Username: "alice", DisplayName: "Alice", Email: "alice@example.com", HierarchyLevelID: "level-3"},
			{ID: "user-2", Username: "bob", DisplayName: "Bob", Email: "bob@example.com", ReportsToID: stringPtr("user-1"), HierarchyLevelID: "level-5"},
		},
		Memberships: []orgsnapshot.Membership{
			{UserID: "user-1", TeamID: "team-1"},
			{UserID: "user-2", TeamID: "team-2"},
		},
	}
}

func TestValidate_HappyPath(t *testing.T) {
	errs := validSnapshot().Validate()
	if len(errs) != 0 {
		t.Fatalf("expected no validation errors, got %+v", errs)
	}
}

func TestValidate_HealthCheckEnabledTriState(t *testing.T) {
	data, err := json.Marshal(validSnapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var snap orgsnapshot.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	var enabled, disabled, omitted *bool
	for i := range snap.Teams {
		switch snap.Teams[i].ID {
		case "team-2":
			enabled = snap.Teams[i].HealthCheckEnabled
		case "team-3":
			disabled = snap.Teams[i].HealthCheckEnabled
		case "team-4":
			omitted = snap.Teams[i].HealthCheckEnabled
		}
	}

	if enabled == nil || *enabled != true {
		t.Fatalf("expected team-2 HealthCheckEnabled to be explicit true, got %v", enabled)
	}
	if disabled == nil || *disabled != false {
		t.Fatalf("expected team-3 HealthCheckEnabled to be explicit false, got %v", disabled)
	}
	if omitted != nil {
		t.Fatalf("expected team-4 HealthCheckEnabled to be nil (omitted), got %v", *omitted)
	}
}

func TestUserJSON_AllowsMissingOrNullReportsToID(t *testing.T) {
	var user orgsnapshot.User
	if err := json.Unmarshal([]byte(`{"id":"user-1","username":"alice","displayName":"Alice","email":"alice@example.com","hierarchyLevelId":"level-3"}`), &user); err != nil {
		t.Fatalf("expected missing reportsToId to identify a root user: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"id":"user-1","username":"alice","displayName":"Alice","email":"alice@example.com","hierarchyLevelId":"level-3","reportsToId":null}`), &user); err != nil {
		t.Fatalf("expected null reportsToId to identify a root user: %v", err)
	}
}

func TestValidate_MissingUserID(t *testing.T) {
	snap := validSnapshot()
	snap.Users = append(snap.Users, orgsnapshot.User{DisplayName: "No ID"})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityUser, "id", "required") {
		t.Fatalf("expected a missing-id user error, got %+v", errs)
	}
}

func TestValidate_MissingTeamID(t *testing.T) {
	snap := validSnapshot()
	snap.Teams = append(snap.Teams, orgsnapshot.Team{Name: "No ID"})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityTeam, "id", "required") {
		t.Fatalf("expected a missing-id team error, got %+v", errs)
	}
}

func TestValidate_DuplicateUserID(t *testing.T) {
	snap := validSnapshot()
	snap.Users = append(snap.Users, orgsnapshot.User{ID: "user-1", DisplayName: "Duplicate Alice"})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityUser, "id", "duplicate") {
		t.Fatalf("expected a duplicate user id error, got %+v", errs)
	}
}

func TestValidate_DuplicateTeamID(t *testing.T) {
	snap := validSnapshot()
	snap.Teams = append(snap.Teams, orgsnapshot.Team{ID: "team-1", Name: "Duplicate Platform"})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityTeam, "id", "duplicate") {
		t.Fatalf("expected a duplicate team id error, got %+v", errs)
	}
}

func TestValidate_MembershipUnknownUser(t *testing.T) {
	snap := validSnapshot()
	snap.Memberships = append(snap.Memberships, orgsnapshot.Membership{UserID: "ghost-user", TeamID: "team-1"})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityMembership, "userId", "unknown user") {
		t.Fatalf("expected a membership unknown-user error, got %+v", errs)
	}
}

func TestValidate_MembershipUnknownTeam(t *testing.T) {
	snap := validSnapshot()
	snap.Memberships = append(snap.Memberships, orgsnapshot.Membership{UserID: "user-1", TeamID: "ghost-team"})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityMembership, "teamId", "unknown team") {
		t.Fatalf("expected a membership unknown-team error, got %+v", errs)
	}
}

func TestValidate_ReportsToUnknownManager(t *testing.T) {
	snap := validSnapshot()
	snap.Users = append(snap.Users, orgsnapshot.User{ID: "user-3", Username: "cara", DisplayName: "Cara", Email: "cara@example.com", HierarchyLevelID: "level-5", ReportsToID: stringPtr("ghost-manager")})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityUser, "reportsToId", "unknown user") {
		t.Fatalf("expected a reportsTo unknown-manager error, got %+v", errs)
	}
}

func TestValidate_ReportsToSelf(t *testing.T) {
	snap := validSnapshot()
	snap.Users = append(snap.Users, orgsnapshot.User{ID: "user-3", Username: "cara", DisplayName: "Cara", Email: "cara@example.com", HierarchyLevelID: "level-5", ReportsToID: stringPtr("user-3")})

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityUser, "reportsToId", "own manager") {
		t.Fatalf("expected a self-manager error, got %+v", errs)
	}
}

func TestValidate_UnsupportedContractVersion(t *testing.T) {
	snap := validSnapshot()
	snap.ContractVersion = "2.0"

	if !hasError(snap.Validate(), orgsnapshot.EntitySnapshot, "contractVersion", "unsupported") {
		t.Fatal("expected unsupported contract version error")
	}
}

func TestValidate_RejectsEmptyUsers(t *testing.T) {
	snap := validSnapshot()
	snap.Users = []orgsnapshot.User{}

	if !hasError(snap.Validate(), orgsnapshot.EntitySnapshot, "users", "at least one") {
		t.Fatal("expected empty users error")
	}
}

func TestValidate_RejectsEmptyTeams(t *testing.T) {
	snap := validSnapshot()
	snap.Teams = []orgsnapshot.Team{}

	if !hasError(snap.Validate(), orgsnapshot.EntitySnapshot, "teams", "at least one") {
		t.Fatal("expected empty teams error")
	}
}

func TestValidate_AllowsEmptyMemberships(t *testing.T) {
	snap := validSnapshot()
	snap.Memberships = []orgsnapshot.Membership{}

	if errs := snap.Validate(); len(errs) != 0 {
		t.Fatalf("expected empty memberships to be valid, got %+v", errs)
	}
}

func TestValidate_RejectsNilMemberships(t *testing.T) {
	snap := validSnapshot()
	snap.Memberships = nil

	if !hasError(snap.Validate(), orgsnapshot.EntitySnapshot, "memberships", "must not be null") {
		t.Fatal("expected nil memberships to be rejected")
	}
}

func TestValidate_RejectsMissingMembershipsField(t *testing.T) {
	data, err := json.Marshal(validSnapshot())
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal to map: %v", err)
	}
	delete(raw, "memberships")
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatalf("remarshal without memberships: %v", err)
	}

	var snap orgsnapshot.Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	if !hasError(snap.Validate(), orgsnapshot.EntitySnapshot, "memberships", "must not be null") {
		t.Fatal("expected a missing memberships field to be rejected")
	}
}

func TestValidate_DuplicateMembership(t *testing.T) {
	snap := validSnapshot()
	snap.Memberships = append(snap.Memberships, snap.Memberships[0])

	if !hasError(snap.Validate(), orgsnapshot.EntityMembership, "userId,teamId", "duplicate") {
		t.Fatal("expected duplicate membership error")
	}
}

func TestValidate_UnknownTeamLead(t *testing.T) {
	snap := validSnapshot()
	snap.Teams[0].TeamLeadID = stringPtr("ghost-user")

	if !hasError(snap.Validate(), orgsnapshot.EntityTeam, "teamLeadId", "unknown user") {
		t.Fatal("expected unknown team lead error")
	}
}

func TestValidate_MissingHierarchyLevelID(t *testing.T) {
	snap := validSnapshot()
	snap.Users[0].HierarchyLevelID = ""

	errs := snap.Validate()
	if !hasError(errs, orgsnapshot.EntityUser, "hierarchyLevelId", "required") {
		t.Fatalf("expected a missing hierarchy-level ID error, got %+v", errs)
	}
}

func TestValidate_ReportingCycle(t *testing.T) {
	snap := validSnapshot()
	snap.Users[0].ReportsToID = stringPtr("user-2")

	if !hasError(snap.Validate(), orgsnapshot.EntityUser, "reportsToId", "cycle") {
		t.Fatal("expected reporting cycle error")
	}
}

func TestValidate_ReportingCycleOnlyIdentifiesCycleMembers(t *testing.T) {
	snap := validSnapshot()
	snap.Users = []orgsnapshot.User{
		{ID: "b", Username: "user_b", DisplayName: "B", Email: "b@example.com", HierarchyLevelID: "level-3", ReportsToID: stringPtr("c")},
		{ID: "c", Username: "user_c", DisplayName: "C", Email: "c@example.com", HierarchyLevelID: "level-3", ReportsToID: stringPtr("b")},
		{ID: "a", Username: "user_a", DisplayName: "A", Email: "a@example.com", HierarchyLevelID: "level-5", ReportsToID: stringPtr("b")},
		{ID: "a2", Username: "user_a2", DisplayName: "A2", Email: "a2@example.com", HierarchyLevelID: "level-5", ReportsToID: stringPtr("a")},
	}

	errIDs := make([]string, 0)
	for _, err := range snap.Validate() {
		if err.Field == "reportsToId" && strings.Contains(err.Message, "cycle") {
			errIDs = append(errIDs, err.Identifier)
		}
	}
	if strings.Join(errIDs, ",") != "b,c" {
		t.Fatalf("expected only cycle members in snapshot order, got %v", errIDs)
	}
}

func TestValidate_ReportsToSelfHasOneError(t *testing.T) {
	snap := validSnapshot()
	snap.Users[0].ReportsToID = stringPtr("user-1")
	count := 0
	for _, err := range snap.Validate() {
		if err.Identifier == "user-1" && err.Field == "reportsToId" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one self-manager error, got %d", count)
	}
}

func TestValidate_TeamLeadMustBeMember(t *testing.T) {
	snap := validSnapshot()
	snap.Teams[0].TeamLeadID = stringPtr("user-2")

	if !hasError(snap.Validate(), orgsnapshot.EntityTeam, "teamLeadId", "must be a member") {
		t.Fatal("expected team lead membership error")
	}
}

func TestValidate_DistinctMembershipIDsContainingArrow(t *testing.T) {
	snap := validSnapshot()
	snap.Users = append(snap.Users,
		orgsnapshot.User{ID: "a->b", Username: "arrow", DisplayName: "Arrow", Email: "arrow@example.com", HierarchyLevelID: "level-5"},
		orgsnapshot.User{ID: "a", Username: "short", DisplayName: "Short", Email: "short@example.com", HierarchyLevelID: "level-5"},
	)
	snap.Teams = append(snap.Teams,
		orgsnapshot.Team{ID: "c", Name: "C"},
		orgsnapshot.Team{ID: "b->c", Name: "BC"},
	)
	snap.Memberships = append(snap.Memberships,
		orgsnapshot.Membership{UserID: "a->b", TeamID: "c"},
		orgsnapshot.Membership{UserID: "a", TeamID: "b->c"},
	)

	if hasError(snap.Validate(), orgsnapshot.EntityMembership, "userId,teamId", "duplicate") {
		t.Fatal("distinct membership pairs must not be treated as duplicates")
	}
}

func TestValidate_RequiredUserFields(t *testing.T) {
	tests := []struct {
		name            string
		field           string
		messageContains string
		clear           func(*orgsnapshot.User)
	}{
		{name: "username", field: "username", messageContains: "2-50", clear: func(u *orgsnapshot.User) { u.Username = "" }},
		{name: "display name", field: "displayName", messageContains: "required", clear: func(u *orgsnapshot.User) { u.DisplayName = "" }},
		{name: "email", field: "email", messageContains: "valid", clear: func(u *orgsnapshot.User) { u.Email = "" }},
		{name: "hierarchy level", field: "hierarchyLevelId", messageContains: "required", clear: func(u *orgsnapshot.User) { u.HierarchyLevelID = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snap := validSnapshot()
			tt.clear(&snap.Users[0])
			if !hasError(snap.Validate(), orgsnapshot.EntityUser, tt.field, tt.messageContains) {
				t.Fatalf("expected required %s error", tt.field)
			}
		})
	}
}

func TestValidate_UserFormats(t *testing.T) {
	snap := validSnapshot()
	snap.Users[0].Username = "invalid username!"
	snap.Users[0].Email = "not-an-email"
	snap.Users[0].HierarchyLevelID = strings.Repeat("x", 51)
	errs := snap.Validate()
	for _, field := range []string{"username", "email", "hierarchyLevelId"} {
		if !hasError(errs, orgsnapshot.EntityUser, field, "") {
			t.Fatalf("expected %s format error", field)
		}
	}
}

// TestValidate_RejectsValuesLongerThan255Characters guards against values
// that pass validation but exceed the VARCHAR(255) database columns backing
// displayName, email, and team name -- previously this only failed at sync
// time during the database insert.
func TestValidate_RejectsValuesLongerThan255Characters(t *testing.T) {
	t.Run("displayName", func(t *testing.T) {
		snap := validSnapshot()
		snap.Users[0].DisplayName = strings.Repeat("x", 256)
		if !hasError(snap.Validate(), orgsnapshot.EntityUser, "displayName", "255") {
			t.Fatal("expected displayName length error")
		}
	})

	t.Run("email", func(t *testing.T) {
		snap := validSnapshot()
		snap.Users[0].Email = strings.Repeat("x", 250) + "@example.com"
		if !hasError(snap.Validate(), orgsnapshot.EntityUser, "email", "255") {
			t.Fatal("expected email length error")
		}
	})

	t.Run("team name", func(t *testing.T) {
		snap := validSnapshot()
		snap.Teams[0].Name = strings.Repeat("x", 256)
		if !hasError(snap.Validate(), orgsnapshot.EntityTeam, "name", "255") {
			t.Fatal("expected team name length error")
		}
	})
}

func TestValidate_RejectsAddressesTheDatabaseConstraintRejects(t *testing.T) {
	// Each of these is a syntactically valid RFC 5322 address that
	// net/mail.ParseAddress used to accept, but chk_users_email_format
	// (migration 000013) and middleware.IsValidEmail both reject. Validate
	// must reject them too, or the sync would pass contract validation and
	// only fail later, mid-transaction, on the database CHECK constraint.
	addresses := []string{
		`"quoted local part"@example.com`,
		"user@localhost",
		"user@[192.168.1.1]",
		"user@example.com (a comment)",
	}
	for _, email := range addresses {
		snap := validSnapshot()
		snap.Users[0].Email = email
		errs := snap.Validate()
		if !hasError(errs, orgsnapshot.EntityUser, "email", "") {
			t.Errorf("email %q: expected a format error, got none", email)
		}
	}
}

func TestValidate_IDLessRecordsUseIndexes(t *testing.T) {
	snap := validSnapshot()
	snap.Users = []orgsnapshot.User{{}, {}}
	for _, err := range snap.Validate() {
		if err.Entity == orgsnapshot.EntityUser && err.Identifier == "" {
			t.Fatalf("expected indexed user error, got %+v", err)
		}
	}
}

func TestValidate_IDLessTeamUsesIndexOnEveryField(t *testing.T) {
	// A team missing both id and name must still let every one of its errors
	// -- including "name" -- be tied back to it via the indexed identifier,
	// since the empty id itself can't identify the record.
	snap := validSnapshot()
	snap.Teams = []orgsnapshot.Team{{}}
	found := false
	for _, err := range snap.Validate() {
		if err.Entity == orgsnapshot.EntityTeam && err.Field == "name" {
			found = true
			if err.Identifier == "" {
				t.Fatalf("expected an indexed identifier on the name error, got %+v", err)
			}
		}
	}
	if !found {
		t.Fatal("expected a name validation error for the id-less team")
	}
}

func TestValidationErrors_ComposeAsError(t *testing.T) {
	errs := orgsnapshot.ValidationErrors{{Entity: orgsnapshot.EntityUser, Field: "id", Message: "required"}}
	if errs.Error() == "" || len(errs.Unwrap()) != 1 {
		t.Fatalf("expected aggregate error behavior, got %q", errs.Error())
	}
}

func TestValidationError_ReturnsNilForValidSnapshot(t *testing.T) {
	if err := validSnapshot().ValidationError(); err != nil {
		t.Fatalf("expected nil error for valid snapshot, got %v", err)
	}
	snap := validSnapshot()
	snap.Users = nil
	if err := snap.ValidationError(); err == nil {
		t.Fatal("expected aggregate error for invalid snapshot")
	}
}

func TestValidate_RecordLevelErrorsIdentifyEntityAndField(t *testing.T) {
	snap := validSnapshot()
	snap.Users = append(snap.Users, orgsnapshot.User{})

	errs := snap.Validate()
	for _, err := range errs {
		if err.Entity == orgsnapshot.EntityUser && err.Identifier == "index 2" {
			if err.Field == "" || err.Message == "" {
				t.Fatalf("expected user error to identify field and message, got %+v", err)
			}
			return
		}
	}
	t.Fatal("expected an indexed user validation error")
}

func hasError(errs orgsnapshot.ValidationErrors, entity orgsnapshot.EntityType, field, messageContains string) bool {
	for _, e := range errs {
		if e.Entity == entity && e.Field == field && strings.Contains(e.Message, messageContains) {
			return true
		}
	}
	return false
}

// PostgreSQL VARCHAR(255) counts characters, so the limit is 255 code points:
// a value of exactly 255 passes, 256 fails, and multi-byte characters are
// counted once each (a byte-length check would wrongly reject 200 of them).
func TestValidate_LengthLimitsCountCharactersNotBytes(t *testing.T) {
	const emailSuffix = "@example.com" // 12 characters
	type fieldCase struct {
		name   string
		entity orgsnapshot.EntityType
		field  string
		set    func(*orgsnapshot.Snapshot, string)
		value  func(n int) string
	}
	ascii := func(n int) string { return strings.Repeat("x", n) }
	cjk := func(n int) string { return strings.Repeat("é", n) } // 2 bytes per character
	cases := []fieldCase{
		{"user id", orgsnapshot.EntityUser, "id", func(s *orgsnapshot.Snapshot, v string) {
			s.Users[1].ID = v
			s.Memberships[1].UserID = v
		}, ascii},
		{"team id", orgsnapshot.EntityTeam, "id", func(s *orgsnapshot.Snapshot, v string) {
			s.Teams[3].ID = v
		}, ascii},
		{"display name", orgsnapshot.EntityUser, "displayName", func(s *orgsnapshot.Snapshot, v string) { s.Users[0].DisplayName = v }, ascii},
		{"team name", orgsnapshot.EntityTeam, "name", func(s *orgsnapshot.Snapshot, v string) { s.Teams[0].Name = v }, ascii},
		{"email", orgsnapshot.EntityUser, "email", func(s *orgsnapshot.Snapshot, v string) { s.Users[0].Email = v + emailSuffix },
			func(n int) string { return ascii(n - len(emailSuffix)) }},
	}
	for _, c := range cases {
		t.Run(c.name+" at 255 is accepted", func(t *testing.T) {
			snap := validSnapshot()
			c.set(&snap, c.value(255))
			if hasError(snap.Validate(), c.entity, c.field, "255") {
				t.Fatalf("255 characters must be accepted: %v", snap.Validate())
			}
		})
		t.Run(c.name+" at 256 is rejected with a field-specific error", func(t *testing.T) {
			snap := validSnapshot()
			c.set(&snap, c.value(256))
			if !hasError(snap.Validate(), c.entity, c.field, "255") {
				t.Fatalf("256 characters must be rejected: %v", snap.Validate())
			}
		})
	}

	// Unicode: 255 two-byte characters is 510 bytes but exactly at the
	// character limit; 256 is over it.
	t.Run("display name: 255 multi-byte characters accepted", func(t *testing.T) {
		snap := validSnapshot()
		snap.Users[0].DisplayName = cjk(255)
		if hasError(snap.Validate(), orgsnapshot.EntityUser, "displayName", "255") {
			t.Fatal("255 two-byte characters fit VARCHAR(255) and must be accepted")
		}
	})
	t.Run("display name: 256 multi-byte characters rejected", func(t *testing.T) {
		snap := validSnapshot()
		snap.Users[0].DisplayName = cjk(256)
		if !hasError(snap.Validate(), orgsnapshot.EntityUser, "displayName", "255") {
			t.Fatal("256 characters must be rejected")
		}
	})
	t.Run("team name: 255 multi-byte characters accepted", func(t *testing.T) {
		snap := validSnapshot()
		snap.Teams[0].Name = strings.Repeat("日", 255) // 3 bytes each
		if hasError(snap.Validate(), orgsnapshot.EntityTeam, "name", "255") {
			t.Fatal("255 three-byte characters fit VARCHAR(255) and must be accepted")
		}
	})
	t.Run("user id: 255 multi-byte characters accepted", func(t *testing.T) {
		snap := validSnapshot()
		id := strings.Repeat("é", 255)
		snap.Users[1].ID = id
		snap.Memberships[1].UserID = id
		if hasError(snap.Validate(), orgsnapshot.EntityUser, "id", "255") {
			t.Fatal("255 characters must be accepted")
		}
	})
}

// An over-long id must not be echoed back in full as the record identifier.
func TestValidate_TooLongIDIsIdentifiedByPositionNotEchoedInFull(t *testing.T) {
	snap := validSnapshot()
	long := strings.Repeat("x", 300)
	snap.Teams[3].ID = long

	for _, e := range snap.Validate() {
		if e.Field == "id" && strings.Contains(e.Message, "255") {
			if strings.Contains(e.Identifier, long) {
				t.Fatalf("identifier echoed the whole oversized id: %q", e.Identifier)
			}
			if e.Identifier != "index 3" {
				t.Fatalf("want identifier %q, got %q", "index 3", e.Identifier)
			}
			return
		}
	}
	t.Fatal("expected an id length error")
}

func TestValidate_ContractVersions(t *testing.T) {
	for _, version := range []string{orgsnapshot.ContractVersion, orgsnapshot.ContractVersionV11} {
		snap := validSnapshot()
		snap.ContractVersion = version
		if errs := snap.Validate(); len(errs) != 0 {
			t.Errorf("version %q: expected no validation errors, got %+v", version, errs)
		}
	}
	snap := validSnapshot()
	snap.ContractVersion = "1.2"
	if errs := snap.Validate(); len(errs) != 1 || errs[0].Field != "contractVersion" {
		t.Fatalf("expected one contractVersion error for 1.2, got %+v", errs)
	}
}

func TestValidate_PodIQRole(t *testing.T) {
	withRole := func(version string, role *string) orgsnapshot.Snapshot {
		snap := validSnapshot()
		snap.ContractVersion = version
		snap.Users[0].PodIQRole = role
		return snap
	}

	for _, role := range []string{"L2", "L1", "Developer"} {
		if errs := withRole(orgsnapshot.ContractVersionV11, stringPtr(role)).Validate(); len(errs) != 0 {
			t.Errorf("role %q on v1.1: expected no errors, got %+v", role, errs)
		}
	}
	if errs := withRole(orgsnapshot.ContractVersionV11, nil).Validate(); len(errs) != 0 {
		t.Errorf("v1.1 without podiqRole: expected no errors, got %+v", errs)
	}

	for _, role := range []string{"", "l2", "L3", "developer", "Manager", " L1"} {
		errs := withRole(orgsnapshot.ContractVersionV11, stringPtr(role)).Validate()
		if len(errs) != 1 || errs[0].Field != "podiqRole" || errs[0].Entity != orgsnapshot.EntityUser {
			t.Errorf("role %q on v1.1: expected one user podiqRole error, got %+v", role, errs)
		}
	}

	errs := withRole(orgsnapshot.ContractVersion, stringPtr("L2")).Validate()
	if len(errs) != 1 || errs[0].Field != "podiqRole" {
		t.Errorf("podiqRole on v1.0: expected one podiqRole error, got %+v", errs)
	}
}

func TestSnapshotJSON_PodIQRole(t *testing.T) {
	var snap orgsnapshot.Snapshot
	payload := `{"users":[{"id":"a","podiqRole":"L1"},{"id":"b"},{"id":"c","podiqRole":null}]}`
	if err := json.Unmarshal([]byte(payload), &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.Users[0].PodIQRole == nil || *snap.Users[0].PodIQRole != "L1" {
		t.Errorf("expected L1, got %v", snap.Users[0].PodIQRole)
	}
	if snap.Users[1].PodIQRole != nil || snap.Users[2].PodIQRole != nil {
		t.Errorf("absent and null podiqRole must be nil")
	}
}
