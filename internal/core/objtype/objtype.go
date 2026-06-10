// Package objtype is Warren's registry of object types.
//
// Every model in Warren registers a stable type key of the form
// "domain.name", where both parts follow slug syntax (for example
// "dcim.site" or "dcim.device-type"). Cross-cutting features — change
// logging, tags, custom fields, webhooks, journaling, permissions — refer
// to objects with a (type key, ID) pair instead of ad-hoc per-feature
// conventions, so there is exactly one way to point at any object.
package objtype

import (
	"fmt"
	"regexp"
	"sort"
	"sync"
)

// keyPattern: a slug, a dot, a slug. Hyphens separate words within each part.
var keyPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*\.[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Type describes a registered object type.
type Type struct {
	// Key is the stable identifier, e.g. "dcim.site". It never changes
	// once released; persisted data depends on it.
	Key string
	// Name is the human singular display name, e.g. "Site".
	Name string
	// Plural is the human plural display name, e.g. "Sites".
	Plural string
}

var (
	mu       sync.RWMutex
	registry = map[string]Type{}
)

// Register adds a type to the registry. It is intended to be called from
// package init of each domain and panics on an invalid or duplicate key,
// since either is a programming error.
func Register(t Type) Type {
	if !keyPattern.MatchString(t.Key) {
		panic(fmt.Sprintf("objtype: invalid key %q (want \"domain.name\" in slug syntax)", t.Key))
	}
	if t.Name == "" || t.Plural == "" {
		panic(fmt.Sprintf("objtype: %q must have Name and Plural", t.Key))
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[t.Key]; dup {
		panic(fmt.Sprintf("objtype: duplicate registration of %q", t.Key))
	}
	registry[t.Key] = t
	return t
}

// Get looks up a type by key.
func Get(key string) (Type, bool) {
	mu.RLock()
	defer mu.RUnlock()
	t, ok := registry[key]
	return t, ok
}

// All returns every registered type, sorted by key.
func All() []Type {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Type, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
