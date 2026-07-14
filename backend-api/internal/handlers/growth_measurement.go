package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/andreistefanciprian/yauli/backend-api/internal/store"
)

const eventTypeGrowthMeasurement = "growth_measurement"

type createGrowthMeasurementRequest struct {
	WeightGrams         *int     `json:"weight_grams"`
	LengthCM            *float64 `json:"length_cm"`
	HeadCircumferenceCM *float64 `json:"head_circumference_cm"`
	Notes               string   `json:"notes"`
	OccurredAt          string   `json:"occurred_at"`
}

type growthMeasurementResponse struct {
	ID                  uuid.UUID `json:"id"`
	BabyID              uuid.UUID `json:"baby_id"`
	WeightGrams         *int      `json:"weight_grams,omitempty"`
	LengthCM            *float64  `json:"length_cm,omitempty"`
	HeadCircumferenceCM *float64  `json:"head_circumference_cm,omitempty"`
	Notes               string    `json:"notes,omitempty"`
	OccurredAt          time.Time `json:"occurred_at"`
	CreatedAt           time.Time `json:"created_at"`
}

func (h *Handlers) CreateGrowthMeasurement(w http.ResponseWriter, r *http.Request) {
	var req createGrowthMeasurementRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	attributes, ok := growthMeasurementAttributes(w, req.WeightGrams, req.LengthCM, req.HeadCircumferenceCM, req.Notes)
	if !ok {
		return
	}

	occurredAt, ok := parseOccurredAt(w, req.OccurredAt)
	if !ok {
		return
	}

	createAndRespond(w, r, h, eventTypeGrowthMeasurement, attributes, occurredAt, growthMeasurementFromEvent)
}

func growthMeasurementFromEvent(ev store.Event) growthMeasurementResponse {
	resp := growthMeasurementResponse{ID: ev.ID, BabyID: ev.BabyID, OccurredAt: ev.OccurredAt, CreatedAt: ev.CreatedAt}
	if weightGrams, ok := attributeOptionalInt(ev.Attributes, "weight_grams"); ok {
		resp.WeightGrams = &weightGrams
	}
	if lengthCM, ok := attributeFloat(ev.Attributes, "length_cm"); ok {
		resp.LengthCM = &lengthCM
	}
	if headCircumferenceCM, ok := attributeFloat(ev.Attributes, "head_circumference_cm"); ok {
		resp.HeadCircumferenceCM = &headCircumferenceCM
	}
	if notes, ok := ev.Attributes["notes"].(string); ok {
		resp.Notes = notes
	}
	return resp
}

func growthMeasurementAttributes(w http.ResponseWriter, weightGrams *int, lengthCM, headCircumferenceCM *float64, notes string) (map[string]any, bool) {
	attributes := map[string]any{}
	if weightGrams != nil {
		if !validWeightGrams(*weightGrams) {
			writeError(w, http.StatusBadRequest, "weight_grams must be between 1 and 50000")
			return nil, false
		}
		attributes["weight_grams"] = *weightGrams
	}
	if lengthCM != nil {
		if !validLengthCM(*lengthCM) {
			writeError(w, http.StatusBadRequest, "length_cm must be between 1 and 300")
			return nil, false
		}
		attributes["length_cm"] = *lengthCM
	}
	if headCircumferenceCM != nil {
		if !validHeadCircumferenceCM(*headCircumferenceCM) {
			writeError(w, http.StatusBadRequest, "head_circumference_cm must be between 1 and 100")
			return nil, false
		}
		attributes["head_circumference_cm"] = *headCircumferenceCM
	}
	if len(attributes) == 0 {
		writeError(w, http.StatusBadRequest, "at least one growth measurement is required")
		return nil, false
	}
	if notes != "" {
		attributes["notes"] = notes
	}
	return attributes, true
}

func validWeightGrams(value int) bool {
	return value >= 1 && value <= 50000
}

func validLengthCM(value float64) bool {
	return value >= 1 && value <= 300
}

func validHeadCircumferenceCM(value float64) bool {
	return value >= 1 && value <= 100
}
