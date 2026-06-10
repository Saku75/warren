// Package fault is Warren's error taxonomy. Services return *fault.Error
// so transports (REST, UI) can map outcomes to status codes and user
// messages without inspecting database errors themselves.
package fault

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Kind classifies an error for transport mapping.
type Kind int

const (
	// Internal is an unexpected failure; details are logged, not shown.
	Internal Kind = iota
	// NotFound: the referenced object does not exist.
	NotFound
	// Conflict: uniqueness or referential integrity prevents the change
	// (slug taken, object still in use).
	Conflict
	// Invalid: the input is malformed or violates a business rule.
	Invalid
)

// Error is a classified error with a user-presentable message.
type Error struct {
	Kind    Kind
	Message string
	wrapped error
}

func (e *Error) Error() string {
	if e.wrapped != nil {
		return e.Message + ": " + e.wrapped.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.wrapped }

// New creates a classified error.
func New(kind Kind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap classifies an underlying error, keeping it in the chain.
func Wrap(kind Kind, err error, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...), wrapped: err}
}

// KindOf extracts the Kind from an error chain, defaulting to Internal.
func KindOf(err error) Kind {
	var fe *Error
	if errors.As(err, &fe) {
		return fe.Kind
	}
	return Internal
}

// HTTPStatus maps a Kind to its HTTP status code.
func HTTPStatus(k Kind) int {
	switch k {
	case NotFound:
		return 404
	case Conflict:
		return 409
	case Invalid:
		return 400
	default:
		return 500
	}
}

// Message returns the user-presentable message of a classified error, or a
// generic one for unclassified (internal) errors.
func Message(err error) string {
	var fe *Error
	if errors.As(err, &fe) && fe.Kind != Internal {
		return fe.Message
	}
	return "internal error"
}

// FromDB classifies common database outcomes: no rows becomes NotFound and
// constraint violations become Conflict, both with messages built from
// label (e.g. "site cph-dc1").
func FromDB(err error, label string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return Wrap(NotFound, err, "%s not found", label)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return Wrap(Conflict, err, "%s conflicts with an existing object (duplicate slug or key in this scope)", label)
		case "23503": // foreign_key_violation
			return Wrap(Conflict, err, "%s is referenced by other objects or references a missing one", label)
		case "23514": // check_violation
			return Wrap(Invalid, err, "%s has a value outside the allowed set", label)
		}
	}
	return Wrap(Internal, err, "%s: operation failed", label)
}
