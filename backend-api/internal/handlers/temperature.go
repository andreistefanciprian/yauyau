package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypeTemperature = "temperature"

type TemperatureMethod string

const (
	TemperatureMethodArmpit   TemperatureMethod = "armpit"
	TemperatureMethodForehead TemperatureMethod = "forehead"
	TemperatureMethodEar      TemperatureMethod = "ear"
	TemperatureMethodRectal   TemperatureMethod = "rectal"
	TemperatureMethodOther    TemperatureMethod = "other"
)

func (m TemperatureMethod) Valid() bool {
	switch m {
	case "", TemperatureMethodArmpit, TemperatureMethodForehead, TemperatureMethodEar, TemperatureMethodRectal, TemperatureMethodOther:
		return true
	default:
		return false
	}
}

type createTemperatureRequest struct {
	TemperatureC float64 `json:"temperature_c"`
	Method       string  `json:"method"`
	Notes        string  `json:"notes"`
	OccurredAt   string  `json:"occurred_at"`
}

type temperatureResponse struct {
	ID           uuid.UUID         `json:"id"`
	BabyID       uuid.UUID         `json:"baby_id"`
	TemperatureC float64           `json:"temperature_c"`
	Method       TemperatureMethod `json:"method,omitempty"`
	Notes        string            `json:"notes,omitempty"`
	OccurredAt   time.Time         `json:"occurred_at"`
	CreatedAt    time.Time         `json:"created_at"`
}

func (h *Handlers) CreateTemperature(w http.ResponseWriter, r *http.Request) {
	var req createTemperatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if !validTemperatureC(req.TemperatureC) {
		writeError(w, http.StatusBadRequest, "temperature_c must be between 30 and 45")
		return
	}

	method := TemperatureMethod(req.Method)
	if !method.Valid() {
		writeError(w, http.StatusBadRequest, "method must be one of: armpit, forehead, ear, rectal, other")
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	attributes := map[string]any{"temperature_c": req.TemperatureC}
	if method != "" {
		attributes["method"] = string(method)
	}
	if req.Notes != "" {
		attributes["notes"] = req.Notes
	}

	createAndRespond(w, r, h, eventTypeTemperature, attributes, occurredAt, temperatureFromEvent)
}

func temperatureFromEvent(ev store.Event) temperatureResponse {
	resp := temperatureResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if temperatureC, ok := attributeFloat(ev.Attributes, "temperature_c"); ok {
		resp.TemperatureC = temperatureC
	}
	if method, ok := ev.Attributes["method"].(string); ok {
		resp.Method = TemperatureMethod(method)
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	}
	return resp
}

func validTemperatureC(value float64) bool {
	return value >= 30 && value <= 45
}
