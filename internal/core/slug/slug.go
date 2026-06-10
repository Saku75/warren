// Package slug implements Warren's single, uniform human-readable identifier.
//
// Warren has exactly two ways to identify an object: its ID (UUIDv7) and,
// for types that are human-addressable, its slug. There is one slug format,
// one validator, and one generator for the whole application; object types
// differ only in the scope within which their slugs must be unique (see the
// foundations design doc).
//
// Format: one or more groups of [a-z0-9] separated by single hyphens, at
// most 64 characters. A slug must not be a canonically formatted UUID, so a
// reference segment in a URL or API payload is always unambiguous: if it
// parses as a UUID it is an ID, otherwise it is a slug.
package slug

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// MaxLen is the maximum slug length in bytes.
const MaxLen = 64

var (
	// pattern: alphanumeric groups separated by single hyphens. This shape
	// rules out leading, trailing, and consecutive hyphens by construction.
	pattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

	// uuidShape matches the canonical UUID text form, which is otherwise
	// valid slug syntax and therefore must be rejected explicitly.
	uuidShape = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

var (
	ErrEmpty     = errors.New("slug is empty")
	ErrTooLong   = fmt.Errorf("slug exceeds %d characters", MaxLen)
	ErrBadFormat = errors.New("slug must be lowercase letters, digits, and single hyphens between groups")
	ErrUUIDShape = errors.New("slug must not be formatted like a UUID")
	ErrReserved  = errors.New("slug is reserved")
)

// reserved are slugs that would collide with fixed UI/API routes
// (e.g. /tenancy/tenants/new).
var reserved = map[string]bool{"new": true}

// Validate reports whether s is a well-formed slug.
func Validate(s string) error {
	switch {
	case s == "":
		return ErrEmpty
	case len(s) > MaxLen:
		return ErrTooLong
	case !pattern.MatchString(s):
		return ErrBadFormat
	case uuidShape.MatchString(s):
		return ErrUUIDShape
	case reserved[s]:
		return ErrReserved
	}
	return nil
}

// translit maps letters that do not decompose to ASCII under NFKD onto
// conventional ASCII spellings. Letters not listed here are handled by
// decomposition (é → e) or dropped.
var translit = map[rune]string{
	'æ': "ae", 'Æ': "ae",
	'ø': "o", 'Ø': "o",
	'å': "a", 'Å': "a", // also decomposes, listed for clarity
	'ß': "ss",
	'œ': "oe", 'Œ': "oe",
	'đ': "d", 'Đ': "d",
	'ð': "d", 'Ð': "d",
	'þ': "th", 'Þ': "th",
	'ł': "l", 'Ł': "l",
	'ı': "i",
}

// stripMarks removes combining marks left behind by NFKD decomposition,
// turning "é" into "e".
var stripMarks = transform.Chain(
	norm.NFKD,
	runes.Remove(runes.In(unicode.Mn)),
)

// Make derives a slug from an arbitrary display name, for use as a default
// that users may edit. It transliterates and folds to ASCII, lowercases,
// and collapses every other character run into single hyphens. It returns
// an error if nothing usable remains.
func Make(name string) (string, error) {
	var b strings.Builder
	for _, r := range name {
		if rep, ok := translit[r]; ok {
			b.WriteString(rep)
			continue
		}
		b.WriteRune(r)
	}

	folded, _, err := transform.String(stripMarks, b.String())
	if err != nil {
		// Fall back to the unfolded input; non-ASCII is dropped below.
		folded = b.String()
	}

	var out strings.Builder
	lastHyphen := true // suppress leading hyphen
	for _, r := range strings.ToLower(folded) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			out.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				out.WriteByte('-')
				lastHyphen = true
			}
		}
	}

	s := strings.TrimRight(out.String(), "-")
	if len(s) > MaxLen {
		s = strings.TrimRight(s[:MaxLen], "-")
	}

	if err := Validate(s); err != nil {
		return "", fmt.Errorf("cannot derive slug from %q: %w", name, err)
	}
	return s, nil
}
