package slug

import (
	"errors"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	valid := []string{
		"a",
		"cph-dc1",
		"row-1",
		"a1-b2-c3",
		"0192", // digits alone are fine; only full UUID shape is reserved
		strings.Repeat("a", MaxLen),
	}
	for _, s := range valid {
		if err := Validate(s); err != nil {
			t.Errorf("Validate(%q) = %v, want nil", s, err)
		}
	}

	invalid := []struct {
		s    string
		want error
	}{
		{"", ErrEmpty},
		{strings.Repeat("a", MaxLen+1), ErrTooLong},
		{"-leading", ErrBadFormat},
		{"trailing-", ErrBadFormat},
		{"double--hyphen", ErrBadFormat},
		{"UPPER", ErrBadFormat},
		{"under_score", ErrBadFormat},
		{"spa ce", ErrBadFormat},
		{"dot.ted", ErrBadFormat},
		{"røde", ErrBadFormat},
		{"0192f3a1-7b2c-7def-8abc-0123456789ab", ErrUUIDShape},
	}
	for _, tc := range invalid {
		if err := Validate(tc.s); !errors.Is(err, tc.want) {
			t.Errorf("Validate(%q) = %v, want %v", tc.s, err, tc.want)
		}
	}
}

func TestMake(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Copenhagen DC 1", "copenhagen-dc-1"},
		{"  Rack -- Row #4  ", "rack-row-4"},
		{"Crème Brûlée", "creme-brulee"},
		{"Røde Æbler på Loftet", "rode-aebler-pa-loftet"},
		{"Straße 12", "strasse-12"},
		{"already-a-slug", "already-a-slug"},
		{"UPPER case", "upper-case"},
		{"core/agg–switch", "core-agg-switch"},
	}
	for _, tc := range cases {
		got, err := Make(tc.in)
		if err != nil {
			t.Errorf("Make(%q): %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Make(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if err := Validate(got); err != nil {
			t.Errorf("Make(%q) produced invalid slug %q: %v", tc.in, got, err)
		}
	}
}

func TestMakeTruncates(t *testing.T) {
	got, err := Make(strings.Repeat("word ", 40))
	if err != nil {
		t.Fatalf("Make: %v", err)
	}
	if len(got) > MaxLen {
		t.Fatalf("Make produced %d bytes, max %d", len(got), MaxLen)
	}
	if err := Validate(got); err != nil {
		t.Fatalf("truncated slug %q invalid: %v", got, err)
	}
}

func TestMakeErrorsWhenNothingUsable(t *testing.T) {
	for _, in := range []string{"", "!!!", "---", "日本語"} {
		if got, err := Make(in); err == nil {
			t.Errorf("Make(%q) = %q, want error", in, got)
		}
	}
}
