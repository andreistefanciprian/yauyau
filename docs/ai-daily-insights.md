# AI Daily Insights Plan

Status: **planning**.

This document defines how Yauli should evolve from a deterministic daily
summary into an on-demand AI-assisted insight feature. It is intentionally
written before implementation so the data shape, ownership boundaries, and
rollout plan are agreed before code changes begin.

## Goal

Help parents understand the baby's day quickly, with useful context such as
rhythm, notable intervals, recent changes, and gentle follow-up questions.

The feature should feel like:

* "What happened today?"
* "Was today similar to recent days?"
* "What patterns are visible in the logged data?"
* "What might I want to ask next?"

The AI should make the day easier to understand, not replace the factual
timeline.

## Non-Goals

AI must not:

* diagnose;
* provide medical advice;
* imply something is unsafe;
* invent missing events;
* calculate averages, medians, durations, gaps, percentages, or comparisons;
* run automatically every time events change;
* block normal event creation, editing, or timeline loading.

All calculations belong in backend-api. AI only interprets backend-derived
facts.

## Core Principle

Backend Go calculates facts. AI explains facts.

That means:

* events are the source of truth;
* deterministic daily reports remain backend-owned;
* baby analytics are calculated in backend-api;
* recent baselines are calculated in backend-api;
* AI receives structured, already-calculated input;
* AI output is optional, cached, and generated only on demand.

## Data Flow

```text
events
  -> deterministic daily report
  -> full day data
  -> baby analytics
  -> recent baseline
  -> AI input
  -> cached AI insight output
```

The normal daily report and timeline must remain fast without calling OpenAI.

## Daily Report vs Day Data

Yauli should keep two separate concepts.

### Daily Report

The daily report is the compact deterministic summary shown in the UI today.
It answers "what was logged?" in a short, scannable way.

Current shape:

```json
{
  "title": "Today so far",
  "summary": "Today has feeding, nappies, and sleep logged so far.",
  "highlights": [
    "8 feeds with 610 ml recorded.",
    "11 nappy changes: 9 mixed, 1 wet only, 1 poo only.",
    "3 sleeps totalling 4 hours 20 minutes."
  ],
  "generated_at": "2026-07-13T09:30:00+09:30",
  "range_start": "2026-07-13T00:00:00+09:30",
  "range_end": "2026-07-13T09:30:00+09:30"
}
```

This is useful for the UI, but it is not enough context for AI insights.

### Day Data

Report data is the complete factual input for one selected local date range.
For a one-day report, `start_date` and `end_date` are the same date. It should
include range-level totals, daily reports, daily totals, and ordered raw
events. Baby analytics should be added separately after the factual contract
is stable.

Proposed endpoint:

```http
GET /api/v1/babies/current/reports/data?start_date=2026-07-13&end_date=2026-07-13
```

This endpoint should be backend-owned and can later be reused by MCP tools.

## Proposed Day Data Shape

```json
{
  "baby": {
    "id": "baby-id",
    "name": "YauYau",
    "timezone": "Australia/Adelaide",
    "birth_date": "2026-01-01",
    "age_days": 193
  },
  "range": {
    "start_date": "2026-07-13",
    "end_date": "2026-07-13",
    "days_included": 1,
    "includes_today": true,
    "is_partial": true,
    "range_start": "2026-07-13T00:00:00+09:30",
    "range_end": "2026-07-13T09:30:00+09:30",
    "generated_at": "2026-07-13T09:30:00+09:30"
  },
  "totals": {},
  "days": [
    {
      "local_date": "2026-07-13",
      "label": "Today",
      "range_start": "2026-07-13T00:00:00+09:30",
      "range_end": "2026-07-13T09:30:00+09:30",
      "is_today": true,
      "is_partial": true,
      "report": {
        "title": "Today so far",
        "summary": "Today has feeding, nappies, and sleep logged so far.",
        "highlights": []
      },
      "totals": {},
      "events": []
    }
  ]
}
```

Events should be grouped by local day and ordered oldest-first for narrative
analysis.

## Totals

Totals should remain factual counts and sums. They answer "how much was
logged?"

Feed totals should preserve type-specific meaning. Formula and expressed milk
have millilitre amounts, while breast feeds are counted separately and may have
duration only. Keep the overall `total_ml`, but limit per-type ml fields to
bottle feeds so AI can talk about bottle volume without implying breast-feed
volume was recorded.

Suggested categories:

```json
{
  "totals": {
    "event_count": 28,
    "feeds": {
      "count": 8,
      "breast_count": 0,
      "formula_count": 1,
      "expressed_count": 7,
      "total_ml": 610,
      "formula_ml": 80,
      "expressed_ml": 530,
      "total_duration_minutes": 80
    },
    "nappies": {
      "count": 11,
      "mixed_count": 9,
      "wet_only_count": 1,
      "poo_only_count": 1,
      "poo_sizes": {
        "small": 2,
        "medium": 6,
        "large": 1
      },
      "labels": {
        "mustard yellow": 4,
        "seedy": 2
      }
    },
    "sleeps": {
      "count": 3,
      "completed_count": 2,
      "ongoing_count": 1,
      "total_duration_minutes": 260
    },
    "pumps": {
      "count": 2,
      "total_ml": 160
    },
    "baths": {
      "count": 1,
      "whole_body_count": 0,
      "bottom_part_count": 1,
      "total_duration_minutes": 10
    },
    "temperatures": {
      "count": 1,
      "min_c": 37.1,
      "max_c": 37.1,
      "latest_c": 37.1,
      "methods": {
        "ear": 1
      }
    },
    "observations": {
      "count": 2,
      "categories": {
        "mood": 1,
        "skin": 1
      }
    },
    "notes": {
      "events_with_notes_count": 5,
      "by_event_type": {
        "feed": 2,
        "nappy": 1,
        "sleep": 2
      }
    }
  }
}
```

## Event Notes

Event notes should be first-class AI context. They capture the parent-entered
details that structured fields cannot fully represent, such as "fussy",
"needed top-up", "mustard yellow", "slept in pram", "after bath", or
"seemed unsettled".

Day data should include notes on each event:

```json
{
  "id": "event-id",
  "type": "feed",
  "occurred_at": "2026-07-13T08:20:00+09:30",
  "local_time": "08:20",
  "notes": "needed a small top-up after waking",
  "attributes": {
    "type": "expressed",
    "amount_ml": 80,
    "duration_minutes": 10
  }
}
```

AI may use notes as parent-entered context, but must not treat them as
clinical observations or overstate them. Prefer phrasing such as "you noted"
or "the notes mention" when using note content.

Very long notes may need truncation or a per-event character cap before being
sent to the AI input. If truncation is added, it should be deterministic and
documented in the day-data contract.

## Baby Analytics

Baby analytics turn events into a small set of deterministic, parent-useful
facts. They should answer parent questions, not compute everything. Reports
are the first consumer, not the owner.

The analytics contract lives in [docs/baby-analytics.md](baby-analytics.md).

First version:

* `timeline`;
* `chronology`;
* `intervals`;
* `relationships`.

Later versions:

* wake windows;
* activity periods;
* notable intervals;
* baseline comparison.

Comparison must compare like with like. For example, a one-day selected range
must be compared with a baseline daily average, not the seven-day baseline
total.

## Recent Baseline

A single day can describe what happened. A baseline lets Yauli explain what
changed compared with recent patterns.

The first baseline should cover the previous 7 calendar days in the baby's
timezone before the selected range starts. It should not overlap the selected
range.

Example:

```json
{
  "baseline": {
    "range": {
      "start_date": "2026-07-06",
      "end_date": "2026-07-12",
      "days_included": 7,
      "includes_today": false,
      "is_partial": false,
      "range_start": "2026-07-06T00:00:00+09:30",
      "range_end": "2026-07-13T00:00:00+09:30",
      "generated_at": "2026-07-13T09:30:00+09:30"
    },
    "totals": {}
  }
}
```

The first baseline version should return factual totals only. Baseline
analytics and averages should be added after the analytics builder exists.

## AI Input

The AI input should be the selected day data plus baseline.

It should include:

* baby context;
* selected day metadata;
* deterministic daily report;
* totals;
* baby analytics;
* recent baseline;
* ordered event list, including notes and labels;
* note coverage signals, such as how many events have notes and which event
  types they belong to.

It should not include:

* secrets;
* family member data;
* auth/session data;
* unrelated historical raw events outside the baseline calculation.

The input hash for AI caching should be computed from deterministic input:

* selected day data;
* baseline;
* prompt/schema version.

Do not include the current generation timestamp in the input hash, otherwise
the cache will miss every time.

## AI Output

Suggested output shape:

```json
{
  "ai_summary": "Today's recorded rhythm was fairly steady, with regular feeds and most nappies appearing shortly afterwards.",
  "rhythm_insights": [
    "The median recorded gap between feeds was 2 hours 45 minutes.",
    "Four nappies were logged within 30 minutes of a feed."
  ],
  "comparison_insights": [
    "Feed spacing was close to the recent seven-day median."
  ],
  "notable_intervals": [
    "Longest recorded sleep: 2 hours 50 minutes.",
    "Longest recorded feed gap: 3 hours 50 minutes."
  ],
  "data_quality_note": "No sleep events were recorded after 6:10 PM, so the evening timeline may be incomplete.",
  "suggested_questions": [
    "How does today's sleep compare with the last week?",
    "What usually happens after the evening bath?"
  ]
}
```

AI should:

* select the most useful facts;
* explain them naturally;
* avoid repeating obvious totals;
* mention uncertainty;
* suggest useful follow-up questions.

AI should not:

* perform arithmetic;
* infer causation from sequences;
* provide medical advice;
* imply missing logs mean missing care;
* overstate weak patterns.

## API Plan

### Deterministic Data

```http
GET /api/v1/babies/current/reports/data?start_date=2026-07-13&end_date=2026-07-13
```

Returns the canonical day data payload.

### On-Demand AI

```http
POST /api/v1/babies/current/reports/daily/ai?date=2026-07-13
```

Generates or returns cached AI insights for the selected range.

Rules:

* called only when the user asks for AI;
* never called automatically after event changes;
* uses deterministic day data and baseline as input;
* returns cached output if the input hash matches;
* regenerates only when input changes or cache policy says to refresh.

## Caching

Use an `ai_reports` table keyed by:

* `family_id`;
* `baby_id`;
* `report_type`;
* `range_start`;
* `range_end`;
* `input_hash`.

Store:

* model;
* prompt/schema version;
* generated content JSONB;
* created timestamp.

The cache protects UX and cost. It should not make event creation slower.

## Frontend Plan

The normal day summary stays visible and fast.

AI insights should be:

* hidden by default;
* generated only after an explicit click;
* displayed via a separate AI report section or filter;
* removable from view by clicking the AI control again;
* refreshed only by asking again after underlying data changes.

The UI should not block event saves while AI is generating.

## Rollout PRs

Recommended sequence:

1. **Report data contract**
   * Add backend report-data builder and endpoint.
   * Include range metadata, per-day reports, and ordered events.
   * Add tests for selected date ranges.

2. **Report totals**
   * Add deterministic range-level and per-day factual totals.
   * Include feed, nappy, sleep, pump, bath, observation, temperature, and
     note totals.
   * Add focused unit tests for totals and endpoint wiring.

3. **Recent baseline**
   * Add previous-7-day baseline range and factual totals.
   * Add tests for partial history and timezone boundaries.

4. **Baby analytics**
   * Add deterministic timeline, chronology, interval, and relationship
     analytics.
   * Add focused unit tests for calculations.

5. **Analytics comparison**
   * Add selected range versus baseline daily-average comparisons.
   * Compare like with like; do not compare a one-day value with a seven-day
     total.

6. **AI backend**
   * Add OpenAI client.
   * Add `ai_reports` migration and store methods.
   * Add on-demand AI endpoint.
   * Cache by deterministic input hash and schema version.

7. **Frontend AI interaction**
   * Add explicit AI button/toggle.
   * Show loading/error states.
   * Keep AI hidden by default.
   * Do not call AI during normal timeline refresh.

8. **MCP exposure**
   * Expose deterministic day data first.
   * Expose AI insight retrieval only after backend behavior is stable.

## Open Questions

* What note length cap should be used before AI input, if any?
* Should baseline include the selected day for "today so far", or always
  exclude the selected day?
* What is the minimum data threshold before AI should produce comparison
  insights?
* Should AI output be editable/dismissible by parents?
* Should AI insights be stored permanently or treated as regenerable cache?
* What model should be the default OpenAI model for this feature?
* Should the first AI version support past days, today only, or every range
  supported by the timeline?

## Decision to Confirm Before Implementation

Before writing implementation code, confirm this product/technical decision:

**The AI report is an interpretation of backend-derived facts, not a raw-event
summarizer or calculator.**

If we keep that boundary, the implementation should stay reliable, testable,
and easier to evolve.
