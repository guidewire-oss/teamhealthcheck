package postgres

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// golang-migrate applies files by numeric version and records only the
// highest one applied, so two files sharing a version, or a shipped version
// renumbered, silently leaves a migration unapplied on upgraded databases.
// These checks need no database, so they run everywhere.

var migrationFile = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.(up|down)\.sql$`)

type migration struct {
	version  int
	name     string
	up, down bool
}

func loadMigrations(t *testing.T) map[int]*migration {
	t.Helper()
	entries, err := os.ReadDir("migrations")
	if err != nil {
		t.Fatalf("read migrations dir: %v", err)
	}
	byVersion := map[int]*migration{}
	for _, e := range entries {
		m := migrationFile.FindStringSubmatch(e.Name())
		if m == nil {
			t.Errorf("%s does not match <version>_<name>.(up|down).sql", e.Name())
			continue
		}
		version, _ := strconv.Atoi(m[1])
		cur, ok := byVersion[version]
		if !ok {
			cur = &migration{version: version, name: m[2]}
			byVersion[version] = cur
		}
		if cur.name != m[2] {
			t.Errorf("version %06d is used by two different migrations: %q and %q", version, cur.name, m[2])
		}
		if m[3] == "up" {
			cur.up = true
		} else {
			cur.down = true
		}
	}
	return byVersion
}

func TestMigrations_EveryVersionIsUniqueAndHasUpAndDown(t *testing.T) {
	for v, m := range loadMigrations(t) {
		if !m.up || !m.down {
			t.Errorf("version %06d (%s): up=%v down=%v, want both", v, m.name, m.up, m.down)
		}
	}
}

// Versions that have shipped must keep their number and name: databases that
// already recorded them would otherwise skip the renumbered file.
func TestMigrations_ShippedVersionsKeepTheirNames(t *testing.T) {
	shipped := map[int]string{
		19: "add_team_distribution_list_email",
		20: "create_action_items",
		21: "add_health_check_enabled",
		22: "add_org_sync_delete_threshold",
	}
	got := loadMigrations(t)
	for v, name := range shipped {
		if m, ok := got[v]; !ok || m.name != name {
			t.Errorf("version %06d must remain %q (shipped), got %+v", v, name, m)
		}
	}
}

// display_order sorts after every version that already shipped on main (the
// highest there is 000025), so upgraded databases still apply it.
func TestMigrations_DisplayOrderIsAfterShippedVersions(t *testing.T) {
	var version int
	var versions []int
	for v, m := range loadMigrations(t) {
		versions = append(versions, v)
		if m.name == "add_dimension_display_order" {
			version = v
		}
	}
	sort.Ints(versions)
	if version < 26 {
		t.Fatalf("add_dimension_display_order is version %d, want >= 26 (above main's last shipped migration)", version)
	}
}

func TestMigrations_HealthCheckEnabledColumnAndCommentAreDefined(t *testing.T) {
	up, err := os.ReadFile(filepath.Join("migrations", "000021_add_health_check_enabled.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(up))
	for _, want := range []string{
		"alter table teams add column if not exists health_check_enabled boolean not null default true",
		"comment on column teams.health_check_enabled",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("000021 up migration is missing %q", want)
		}
	}
}

func TestMigrations_DisplayOrderBackfillsAndAssignsPositions(t *testing.T) {
	up, err := os.ReadFile(filepath.Join("migrations", "000026_add_dimension_display_order.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(up))
	for _, want := range []string{
		"add column if not exists display_order",
		"where display_order = 0",
		"create trigger trg_health_dimensions_display_order",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("000026 up migration is missing %q", want)
		}
	}
}
