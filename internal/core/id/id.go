// Package id provides Warren's universal object identity.
//
// Every object in Warren is identified by exactly one UUIDv7, generated at
// creation time and immutable for the life of the object. UUIDv7 is
// time-sortable, safe to generate on any replica without coordination, and
// unique across Warren instances (which keeps import/export and
// staging-to-production moves collision-free).
package id

import (
	"fmt"

	"github.com/google/uuid"
)

// ID is the universal identifier for all Warren objects.
type ID = uuid.UUID

// Nil is the zero ID.
var Nil = uuid.Nil

// New generates a new UUIDv7.
func New() ID {
	id, err := uuid.NewV7()
	if err != nil {
		// NewV7 only fails if the entropy source fails, which is not
		// recoverable at this layer.
		panic("id: generating UUIDv7: " + err.Error())
	}
	return id
}

// Parse parses s as an ID. It accepts only the canonical 36-character
// hyphenated form, so values that pass slug validation can never also parse
// as an ID.
func Parse(s string) (ID, error) {
	if len(s) != 36 {
		return Nil, fmt.Errorf("id: invalid length %d, want 36", len(s))
	}
	return uuid.Parse(s)
}

// Is reports whether s is a canonically formatted ID. URL handlers use this
// to decide whether a reference segment is an ID or a slug path.
func Is(s string) bool {
	_, err := Parse(s)
	return err == nil
}
