package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypeNappy = "nappy"

// NappyKind is the set of valid nappy change kinds.
type NappyKind string

const (
	NappyKindWet  NappyKind = "wet"
	NappyKindPoo  NappyKind = "poo"
	NappyKindBoth NappyKind = "both"
)

func (k NappyKind) Valid() bool {
	switch k {
	case NappyKindWet, NappyKindPoo, NappyKindBoth:
		return true
	default:
		return false
	}
}

type PooSize string

const (
	PooSizeSmear   PooSize = "smear"
	PooSizeSmall   PooSize = "small"
	PooSizeMedium  PooSize = "medium"
	PooSizeLarge   PooSize = "large"
	PooSizeBlowout PooSize = "blowout"
)

func (s PooSize) Valid() bool {
	switch s {
	case PooSizeSmear, PooSizeSmall, PooSizeMedium, PooSizeLarge, PooSizeBlowout:
		return true
	default:
		return false
	}
}

type createNappyRequest struct {
	PooSize    string   `json:"poo_size"`
	Labels     []string `json:"labels"`
	Kind       string   `json:"kind"`
	Notes      string   `json:"notes"`
	OccurredAt string   `json:"occurred_at"`
}

// nappyResponse is a nappy-change event as returned to API consumers.
type nappyResponse struct {
	ID         uuid.UUID `json:"id"`
	BabyID     uuid.UUID `json:"baby_id"`
	Kind       NappyKind `json:"kind"`
	PooSize    PooSize   `json:"poo_size,omitempty"`
	Labels     []string  `json:"labels,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	OccurredAt time.Time `json:"occurred_at"`
	CreatedAt  time.Time `json:"created_at"`
}

func (h *Handlers) CreateNappy(w http.ResponseWriter, r *http.Request) {
	var req createNappyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	kind := NappyKind(req.Kind)
	if !kind.Valid() {
		writeError(w, http.StatusBadRequest, "kind must be one of: wet, poo, both")
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	attributes := map[string]any{"kind": string(kind)}
	if kind == NappyKindPoo || kind == NappyKindBoth {
		pooSize := PooSizeMedium
		if req.PooSize != "" {
			pooSize = PooSize(req.PooSize)
		}
		if !pooSize.Valid() {
			writeError(w, http.StatusBadRequest, "poo_size must be one of: smear, small, medium, large, blowout")
			return
		}
		attributes["poo_size"] = string(pooSize)
	}
	labels, ok := normalizeNappyLabels(req.Labels)
	if !ok {
		writeError(w, http.StatusBadRequest, "labels include an unsupported nappy label")
		return
	}
	if len(labels) > 0 {
		attributes["labels"] = labels
	}
	if req.Notes != "" {
		attributes["notes"] = req.Notes
	}

	createAndRespond(w, r, h, eventTypeNappy, attributes, occurredAt, nappyFromEvent)
}

func nappyFromEvent(ev store.Event) nappyResponse {
	resp := nappyResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if kind, ok := ev.Attributes["kind"].(string); ok {
		resp.Kind = NappyKind(kind)
	}
	if pooSize, ok := ev.Attributes["poo_size"].(string); ok {
		resp.PooSize = PooSize(pooSize)
	}
	if labels, ok := nappyLabelsFromAttribute(ev.Attributes["labels"]); ok {
		resp.Labels = labels
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	} else if colour, ok := ev.Attributes["colour"].(string); ok {
		resp.Notes = colour
	}
	return resp
}

func normalizeNappyLabels(raw []string) ([]string, bool) {
	seen := map[string]bool{}
	labels := make([]string, 0, len(raw))
	for _, label := range raw {
		if !validNappyLabel(label) {
			return nil, false
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	return labels, true
}

func nappyLabelsFromAttribute(raw any) ([]string, bool) {
	switch labels := raw.(type) {
	case nil:
		return nil, true
	case []string:
		return normalizeNappyLabels(labels)
	case []any:
		values := make([]string, 0, len(labels))
		for _, label := range labels {
			value, ok := label.(string)
			if !ok {
				return nil, false
			}
			values = append(values, value)
		}
		return normalizeNappyLabels(values)
	default:
		return nil, false
	}
}

func validNappyLabel(label string) bool {
	switch label {
	case "mustard_yellow", "green", "brown", "black", "red_blood", "pale_white",
		"seedy", "runny", "sticky", "hard", "mucus", "smelly", "rash":
		return true
	default:
		return false
	}
}
