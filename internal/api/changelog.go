package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/saku75/warren/internal/core/fault"
	"github.com/saku75/warren/internal/core/id"
	"github.com/saku75/warren/internal/db/gen"
)

type changelogRep struct {
	ID          string          `json:"id"`
	EventAt     time.Time       `json:"event_at"`
	Actor       string          `json:"actor"`
	Action      string          `json:"action"`
	ObjectType  string          `json:"object_type"`
	ObjectID    string          `json:"object_id"`
	ObjectLabel string          `json:"object_label"`
	DataBefore  json.RawMessage `json:"data_before"`
	DataAfter   json.RawMessage `json:"data_after"`
}

func changelogToRep(e gen.Changelog) changelogRep {
	return changelogRep{
		ID:          e.ID.String(),
		EventAt:     e.EventAt,
		Actor:       e.Actor,
		Action:      e.Action,
		ObjectType:  e.ObjectType,
		ObjectID:    e.ObjectID.String(),
		ObjectLabel: e.ObjectLabel,
		DataBefore:  e.DataBefore,
		DataAfter:   e.DataAfter,
	}
}

// listChangelog returns recent events, optionally filtered to one object
// with ?object_type=dcim.site&object_id=<uuid>.
func (h *Handler) listChangelog(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	objectType := r.URL.Query().Get("object_type")
	objectID := r.URL.Query().Get("object_id")

	if (objectType == "") != (objectID == "") {
		h.writeError(w, r, fault.New(fault.Invalid, "object_type and object_id must be provided together"))
		return
	}

	if objectType != "" {
		oid, err := id.Parse(objectID)
		if err != nil {
			h.writeError(w, r, fault.Wrap(fault.Invalid, err, "invalid object_id"))
			return
		}
		items, err := h.changelog.ListForObject(r.Context(), objectType, oid, limit, offset)
		if err != nil {
			h.writeError(w, r, err)
			return
		}
		reps := make([]changelogRep, len(items))
		for i, e := range items {
			reps[i] = changelogToRep(e)
		}
		h.writeJSON(w, http.StatusOK, map[string]any{"items": reps})
		return
	}

	items, total, err := h.changelog.List(r.Context(), limit, offset)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	reps := make([]changelogRep, len(items))
	for i, e := range items {
		reps[i] = changelogToRep(e)
	}
	h.writeJSON(w, http.StatusOK, listEnvelope{Items: reps, Total: total, Limit: limit, Offset: offset})
}
