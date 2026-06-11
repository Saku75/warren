// Package changelog records every object mutation in an append-only log,
// written in the same transaction as the mutation itself (design doc §5.9).
// Entries reference objects by (type key, ID) and carry JSON snapshots, so
// they remain meaningful after the object is gone.
package changelog

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/gen"
)

// Actions recorded in the log.
const (
	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"
)

// SystemActor attributes mutations made outside a user context (CLI,
// migrations). Authenticated requests carry the username via WithActor.
const SystemActor = "system"

type actorKey struct{}

// WithActor attaches the acting username to the context; the auth
// middleware calls this for every authenticated request.
func WithActor(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, actorKey{}, username)
}

// ActorFrom returns the acting username, defaulting to SystemActor.
func ActorFrom(ctx context.Context) string {
	if v, ok := ctx.Value(actorKey{}).(string); ok && v != "" {
		return v
	}
	return SystemActor
}

// Record writes one entry using q, which must be bound to the same
// transaction as the mutation being recorded. before/after are object
// snapshots (nil for create/delete respectively).
func Record(ctx context.Context, q *gen.Queries, action, objectType string, objectID id.ID, label string, before, after any) error {
	encode := func(v any) ([]byte, error) {
		if v == nil {
			return nil, nil
		}
		return json.Marshal(v)
	}
	b, err := encode(before)
	if err != nil {
		return fmt.Errorf("changelog: encode before: %w", err)
	}
	a, err := encode(after)
	if err != nil {
		return fmt.Errorf("changelog: encode after: %w", err)
	}
	return q.InsertChangelogEntry(ctx, gen.InsertChangelogEntryParams{
		ID:          id.New(),
		Actor:       ActorFrom(ctx),
		Action:      action,
		ObjectType:  objectType,
		ObjectID:    objectID,
		ObjectLabel: label,
		DataBefore:  b,
		DataAfter:   a,
	})
}

// Service exposes read access to the log.
type Service struct {
	q *gen.Queries
}

func NewService(q *gen.Queries) *Service { return &Service{q: q} }

// List returns entries newest-first, with the total count for pagination.
func (s *Service) List(ctx context.Context, limit, offset int32) ([]gen.Changelog, int64, error) {
	total, err := s.q.CountChangelog(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("changelog: count: %w", err)
	}
	items, err := s.q.ListChangelog(ctx, gen.ListChangelogParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, fmt.Errorf("changelog: list: %w", err)
	}
	return items, total, nil
}

// ListForObject returns one object's history, newest-first.
func (s *Service) ListForObject(ctx context.Context, objectType string, objectID id.ID, limit, offset int32) ([]gen.Changelog, error) {
	items, err := s.q.ListChangelogForObject(ctx, gen.ListChangelogForObjectParams{
		ObjectType: objectType,
		ObjectID:   objectID,
		Limit:      limit,
		Offset:     offset,
	})
	if err != nil {
		return nil, fmt.Errorf("changelog: list for object: %w", err)
	}
	return items, nil
}
