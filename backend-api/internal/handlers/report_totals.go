package handlers

import "github.com/andreistefanciprian/yauli/backend-api/internal/store"

type reportTotalsResponse struct {
	EventCount   int                     `json:"event_count"`
	Feeds        reportFeedTotals        `json:"feeds"`
	Nappies      reportNappyTotals       `json:"nappies"`
	Sleeps       reportSleepTotals       `json:"sleeps"`
	Pumps        reportPumpTotals        `json:"pumps"`
	Baths        reportBathTotals        `json:"baths"`
	Observations reportObservationTotals `json:"observations"`
	Temperatures reportTemperatureTotals `json:"temperatures"`
	Growth       reportGrowthTotals      `json:"growth"`
	Notes        reportNoteTotals        `json:"notes"`
}

type reportFeedTotals struct {
	Count                int `json:"count"`
	BreastCount          int `json:"breast_count"`
	FormulaCount         int `json:"formula_count"`
	ExpressedCount       int `json:"expressed_count"`
	TotalMl              int `json:"total_ml"`
	BreastMl             int `json:"breast_ml"`
	FormulaMl            int `json:"formula_ml"`
	ExpressedMl          int `json:"expressed_ml"`
	TotalDurationMinutes int `json:"total_duration_minutes"`
}

type reportNappyTotals struct {
	Count        int            `json:"count"`
	WetOnlyCount int            `json:"wet_only_count"`
	PooOnlyCount int            `json:"poo_only_count"`
	MixedCount   int            `json:"mixed_count"`
	PooSizes     map[string]int `json:"poo_sizes,omitempty"`
	Labels       map[string]int `json:"labels,omitempty"`
}

type reportSleepTotals struct {
	Count                int `json:"count"`
	CompletedCount       int `json:"completed_count"`
	OngoingCount         int `json:"ongoing_count"`
	TotalDurationMinutes int `json:"total_duration_minutes"`
}

type reportPumpTotals struct {
	Count   int `json:"count"`
	TotalMl int `json:"total_ml"`
}

type reportBathTotals struct {
	Count                int `json:"count"`
	WholeBodyCount       int `json:"whole_body_count"`
	BottomPartCount      int `json:"bottom_part_count"`
	TotalDurationMinutes int `json:"total_duration_minutes"`
}

type reportObservationTotals struct {
	Count      int            `json:"count"`
	Categories map[string]int `json:"categories,omitempty"`
}

type reportTemperatureTotals struct {
	Count   int            `json:"count"`
	MinC    *float64       `json:"min_c,omitempty"`
	MaxC    *float64       `json:"max_c,omitempty"`
	LatestC *float64       `json:"latest_c,omitempty"`
	Methods map[string]int `json:"methods,omitempty"`
}

type reportGrowthTotals struct {
	Count                     int      `json:"count"`
	LatestWeightGrams         *int     `json:"latest_weight_grams,omitempty"`
	LatestLengthCM            *float64 `json:"latest_length_cm,omitempty"`
	LatestHeadCircumferenceCM *float64 `json:"latest_head_circumference_cm,omitempty"`
}

type reportNoteTotals struct {
	EventsWithNotesCount int            `json:"events_with_notes_count"`
	ByEventType          map[string]int `json:"by_event_type,omitempty"`
}

func buildReportTotals(events []store.Event) reportTotalsResponse {
	totals := reportTotalsResponse{EventCount: len(events)}
	for _, ev := range events {
		totals.addEvent(ev)
	}
	return totals
}

func (t *reportTotalsResponse) addEvent(ev store.Event) {
	if reportEventNotes(ev) != "" {
		t.Notes.EventsWithNotesCount++
		incrementStringCount(&t.Notes.ByEventType, ev.EventType)
	}

	switch ev.EventType {
	case eventTypeFeed:
		t.addFeed(ev)
	case eventTypeNappy:
		t.addNappy(ev)
	case eventTypeSleep:
		t.addSleep(ev)
	case eventTypePump:
		t.addPump(ev)
	case eventTypeBath:
		t.addBath(ev)
	case eventTypeObservation:
		t.addObservation(ev)
	case eventTypeTemperature:
		t.addTemperature(ev)
	case eventTypeGrowthMeasurement:
		t.addGrowthMeasurement(ev)
	}
}

func (t *reportTotalsResponse) addFeed(ev store.Event) {
	t.Feeds.Count++
	feedType, _ := ev.Attributes["type"].(string)
	amountMl, hasAmount := attributeOptionalInt(ev.Attributes, "amount_ml")
	if hasAmount {
		t.Feeds.TotalMl += amountMl
	}
	switch FeedType(feedType) {
	case FeedTypeBreast:
		t.Feeds.BreastCount++
		if hasAmount {
			t.Feeds.BreastMl += amountMl
		}
	case FeedTypeFormula:
		t.Feeds.FormulaCount++
		if hasAmount {
			t.Feeds.FormulaMl += amountMl
		}
	case FeedTypeExpressed:
		t.Feeds.ExpressedCount++
		if hasAmount {
			t.Feeds.ExpressedMl += amountMl
		}
	}
	if durationMinutes, ok := attributeOptionalInt(ev.Attributes, "duration_minutes"); ok {
		t.Feeds.TotalDurationMinutes += durationMinutes
	}
}

func (t *reportTotalsResponse) addNappy(ev store.Event) {
	t.Nappies.Count++
	if kind, ok := ev.Attributes["kind"].(string); ok {
		switch NappyKind(kind) {
		case NappyKindWet:
			t.Nappies.WetOnlyCount++
		case NappyKindPoo:
			t.Nappies.PooOnlyCount++
		case NappyKindBoth:
			t.Nappies.MixedCount++
		}
	}
	if pooSize, ok := ev.Attributes["poo_size"].(string); ok {
		incrementStringCount(&t.Nappies.PooSizes, pooSize)
	}
	for _, label := range reportEventLabels(ev) {
		incrementStringCount(&t.Nappies.Labels, label)
	}
}

func (t *reportTotalsResponse) addSleep(ev store.Event) {
	t.Sleeps.Count++
	durationMinutes, ok := attributeOptionalInt(ev.Attributes, "duration_minutes")
	if !ok {
		t.Sleeps.OngoingCount++
		return
	}
	t.Sleeps.CompletedCount++
	t.Sleeps.TotalDurationMinutes += durationMinutes
}

func (t *reportTotalsResponse) addPump(ev store.Event) {
	t.Pumps.Count++
	if amountMl, ok := attributeOptionalInt(ev.Attributes, "amount_ml"); ok {
		t.Pumps.TotalMl += amountMl
	}
}

func (t *reportTotalsResponse) addBath(ev store.Event) {
	t.Baths.Count++
	if bathType, ok := ev.Attributes["type"].(string); ok {
		switch BathType(bathType) {
		case BathTypeWholeBody:
			t.Baths.WholeBodyCount++
		case BathTypeBottomPart:
			t.Baths.BottomPartCount++
		}
	}
	if durationMinutes, ok := attributeOptionalInt(ev.Attributes, "duration_minutes"); ok {
		t.Baths.TotalDurationMinutes += durationMinutes
	}
}

func (t *reportTotalsResponse) addObservation(ev store.Event) {
	t.Observations.Count++
	if category, ok := ev.Attributes["category"].(string); ok && category != "" {
		incrementStringCount(&t.Observations.Categories, category)
	}
}

func (t *reportTotalsResponse) addTemperature(ev store.Event) {
	temperatureC, ok := attributeFloat(ev.Attributes, "temperature_c")
	if !ok {
		return
	}
	t.Temperatures.Count++
	if t.Temperatures.MinC == nil || temperatureC < *t.Temperatures.MinC {
		value := temperatureC
		t.Temperatures.MinC = &value
	}
	if t.Temperatures.MaxC == nil || temperatureC > *t.Temperatures.MaxC {
		value := temperatureC
		t.Temperatures.MaxC = &value
	}
	latestValue := temperatureC
	t.Temperatures.LatestC = &latestValue
	if method, ok := ev.Attributes["method"].(string); ok && method != "" {
		incrementStringCount(&t.Temperatures.Methods, method)
	}
}

func (t *reportTotalsResponse) addGrowthMeasurement(ev store.Event) {
	weightGrams, hasWeight := attributeOptionalInt(ev.Attributes, "weight_grams")
	lengthCM, hasLength := attributeFloat(ev.Attributes, "length_cm")
	headCircumferenceCM, hasHeadCircumference := attributeFloat(ev.Attributes, "head_circumference_cm")
	if !hasWeight && !hasLength && !hasHeadCircumference {
		return
	}
	t.Growth.Count++
	if hasWeight {
		value := weightGrams
		t.Growth.LatestWeightGrams = &value
	}
	if hasLength {
		value := lengthCM
		t.Growth.LatestLengthCM = &value
	}
	if hasHeadCircumference {
		value := headCircumferenceCM
		t.Growth.LatestHeadCircumferenceCM = &value
	}
}

func incrementStringCount(counts *map[string]int, key string) {
	if key == "" {
		return
	}
	if *counts == nil {
		*counts = map[string]int{}
	}
	(*counts)[key]++
}
