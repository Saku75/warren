package id

import (
	"testing"
)

func TestNewIsV7(t *testing.T) {
	got := New()
	if got.Version() != 7 {
		t.Fatalf("New() version = %d, want 7", got.Version())
	}
}

func TestNewIsTimeOrdered(t *testing.T) {
	// UUIDv7 encodes a millisecond timestamp in the most significant bits,
	// so IDs generated in sequence must never sort backwards.
	prev := New()
	for range 100 {
		next := New()
		if next.String() < prev.String() {
			t.Fatalf("IDs sorted backwards: %s before %s", next, prev)
		}
		prev = next
	}
}

func TestParseRoundTrip(t *testing.T) {
	want := New()
	got, err := Parse(want.String())
	if err != nil {
		t.Fatalf("Parse(%q): %v", want, err)
	}
	if got != want {
		t.Fatalf("Parse(%q) = %s", want, got)
	}
}

func TestParseRejectsNonCanonical(t *testing.T) {
	canonical := New().String()
	for _, s := range []string{
		"",
		"not-an-id",
		"urn:uuid:" + canonical,            // URN form
		"{" + canonical + "}",              // braced form
		"0192f3a17b2c7def8abc0123456789ab", // unhyphenated form
		canonical + "0",                    // too long
	} {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", s)
		}
		if Is(s) {
			t.Errorf("Is(%q) = true, want false", s)
		}
	}
	if !Is(canonical) {
		t.Errorf("Is(%q) = false, want true", canonical)
	}
}
