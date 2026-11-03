package postgres

import (
	"math"
	"testing"
)

func TestPageOffset(t *testing.T) {
	cases := []struct {
		name           string
		page, pageSize int
		want           int
		ok             bool
	}{
		{"first page", 1, 25, 0, true},
		{"second page", 2, 25, 25, true},
		{"page 40 of size 100", 40, 100, 3900, true},
		{"page below 1 is the first page", 0, 25, 0, true},
		{"negative page is the first page", -7, 25, 0, true},
		{"a non-positive page size has no valid offset", 3, 0, 0, false},
		{"negative page size has no valid offset", 3, -1, 0, false},
		// Largest page whose offset still fits in an int, for page size 100.
		{"largest safe page", math.MaxInt/100 + 1, 100, (math.MaxInt / 100) * 100, true},
		{"one past the largest safe page overflows", math.MaxInt/100 + 2, 100, 0, false},
		{"absurd page overflows", math.MaxInt, 25, 0, false},
		{"page size 1 never overflows below MaxInt", math.MaxInt, 1, math.MaxInt - 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := pageOffset(c.page, c.pageSize)
			if ok != c.ok || (ok && got != c.want) {
				t.Fatalf("pageOffset(%d, %d) = (%d, %v), want (%d, %v)", c.page, c.pageSize, got, ok, c.want, c.ok)
			}
			if ok && got < 0 {
				t.Fatalf("offset must never be negative, got %d", got)
			}
		})
	}
}

// The naive product wraps negative for the absurd page, which is exactly what
// PostgreSQL rejects as a negative OFFSET.
func TestPageOffset_NaiveProductWouldWrapNegative(t *testing.T) {
	page, pageSize := math.MaxInt, 25
	if naive := (page - 1) * pageSize; naive >= 0 {
		t.Skipf("expected the naive product to wrap on this platform, got %d", naive)
	}
	if _, ok := pageOffset(page, pageSize); ok {
		t.Fatal("pageOffset must report overflow where the naive product wraps")
	}
}
