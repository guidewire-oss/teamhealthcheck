package postgres

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRunesKeepsValidUTF8(t *testing.T) {
	// "é" is two bytes; a byte cut at an odd offset would split it.
	long := strings.Repeat("é", maxErrorMessage+50)
	got := truncateRunes(long, maxErrorMessage)
	if !utf8.ValidString(got) {
		t.Fatal("truncated message is not valid UTF-8")
	}
	if n := utf8.RuneCountInString(got); n != maxErrorMessage {
		t.Fatalf("expected %d characters, got %d", maxErrorMessage, n)
	}
}

func TestTruncateRunesLeavesShortMessagesAlone(t *testing.T) {
	msg := "The synchronization failed while saving changes."
	if got := truncateRunes(msg, maxErrorMessage); got != msg {
		t.Fatalf("short message changed: %q", got)
	}
}
