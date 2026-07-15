# AI Report Contract

Status: **planning**.

This document defines how Yauli should evolve from deterministic report data
into AI-assisted reports for on-demand product views and scheduled email
delivery. It is intentionally written before implementation so the data shape,
ownership boundaries, AI rules, and rollout plan are agreed before code
changes begin.

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

The first AI consumers should be:

* an on-demand daily report in the web app;
* scheduled daily email reports;
* scheduled weekly email reports;
* later MCP tools that retrieve the same backend-owned report output.

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
  -> report data
  -> baby analytics
  -> recent baseline
  -> AI input
  -> cached AI insight output
  -> web app / scheduled email / MCP
```

The normal daily report and timeline must remain fast without calling OpenAI.

## Report Types

AI reports should share one contract and vary by selected range.

Initial report types:

* `daily`: one local calendar day;
* `weekly`: seven complete local calendar days.

Likely later report types:

* `last_three_days`;
* `custom_range`, if the product needs it.

Daily reports may be generated for:

* today so far, when explicitly requested in the app or by MCP;
* yesterday or another complete day, especially for scheduled email.

Weekly scheduled email reports should use complete windows, such as the last
seven local calendar days ending yesterday. They should not use a partial
current day unless the email copy clearly says the range is incomplete.

Report type must be part of the AI cache key and output metadata.

## Daily Report vs Report Data

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

### Report Data

Report data is the complete factual input for one selected local date range.
For a one-day report, `start_date` and `end_date` are the same date. It should
include range-level totals and analytics, daily reports, daily totals, daily
analytics, and ordered raw events.

Current endpoint:

```http
GET /api/v1/babies/current/reports/data?start_date=2026-07-13&end_date=2026-07-13
```

This endpoint should be backend-owned and can later be reused by MCP tools.

## Report Data Shape

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
  "analytics": {
    "timeline": {},
    "chronology": {},
    "intervals": {},
    "relationships": [],
    "comparison": {}
  },
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
    "totals": {},
    "analytics": {}
  },
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
      "analytics": {},
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

Report data should include notes on each event:

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
documented in the report-data contract.

## Baby Analytics

Baby analytics turn events into a small set of deterministic, parent-useful
facts. They should answer parent questions, not compute everything. Reports
are the first consumer, not the owner.

The analytics contract lives in [docs/baby-analytics.md](baby-analytics.md).

Current analytics:

* `timeline`;
* `chronology`;
* `intervals`;
* `relationships`;
* selected-range `comparison`.

Later candidates:

* wake windows;
* activity periods;
* notable intervals.

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

The baseline should return factual totals and baby analytics. Selected-range
analytics may include comparison values derived from selected totals and
baseline daily averages.

## AI Input Contract

The AI input should be a structured envelope around `/reports/data`. The AI
must receive already-calculated facts; it must not calculate totals, averages,
durations, gaps, or comparisons from raw events.

Recommended envelope:

```json
{
  "schema_version": "ai_report_input.v1",
  "report_type": "daily",
  "audience": "parent",
  "delivery": "on_demand",
  "locale": "en",
  "report_data": {}
}
```

`delivery` describes the intended renderer or scheduler context. In v1 it
should not change the generated report content. The same channel-neutral AI
report JSON should be renderable in the web app, email, and later MCP. If a
future version intentionally changes tone or length by delivery channel, that
style profile must be explicit and included in the cache identity.

`report_data` should be the canonical response from:

```http
GET /api/v1/babies/current/reports/data?start_date=YYYY-MM-DD&end_date=YYYY-MM-DD
```

It should include:

* baby context;
* selected range metadata;
* deterministic daily report;
* totals;
* baby analytics;
* selected-range analytics comparison, when present;
* recent baseline;
* ordered event list, including notes and labels;
* note coverage signals, such as how many events have notes and which event
  types they belong to.

It should not include:

* secrets;
* family member data;
* auth/session data;
* unrelated historical raw events outside the baseline calculation.

For scheduled email delivery, the envelope should also include email-safe
delivery metadata that is not shown to the model as user facts:

```json
{
  "delivery": "scheduled_email",
  "email_report": {
    "schedule": "daily",
    "selected_window_complete": true
  }
}
```

Do not include recipient names, email addresses, access tokens, or session
data in the AI input.

### Canonical Input Hash

The input hash for AI caching should be computed from canonical deterministic
input:

* selected report data, including baseline, after canonicalization;
* `report_type`;
* `locale`;
* prompt version;
* input schema version;
* output schema version.

Canonicalization should use stable JSON encoding and remove volatile generated
timestamps that describe when report data was assembled, not what happened.
Examples:

* `range.generated_at`;
* `baseline.range.generated_at`;
* `days[*].report.generated_at`, if present.

Do not remove event timestamps, selected range boundaries, or baby timezone.
Those are factual input.

Do not include the current generation timestamp in the input hash, otherwise
the cache will miss every time. Do not include `delivery` in the semantic
input hash unless delivery intentionally changes generated content.

## AI Output Contract

AI output should be structured JSON. The backend can render that JSON into the
web app or an email template. The model should not return HTML.

Suggested first output shape:

```json
{
  "schema_version": "ai_report_output.v1",
  "title": "YauYau's day so far",
  "summary": "Today's logged timeline shows regular feeds, several nappies, and one longer sleep.",
  "highlights": [
    "Feeds were logged steadily through the morning.",
    "The longest recorded sleep was 2 hours 50 minutes."
  ],
  "patterns": [
    "Several nappies were logged within 30 minutes after feeds."
  ],
  "comparison": [
    "So far today, feed count is tracking close to the recent daily average."
  ],
  "caveats": [
    "Today's timeline is still partial, so the pattern may change as more events are logged."
  ],
  "questions_for_parent": [
    "How does today's sleep compare with the last week?",
    "What usually happens after the evening bath?"
  ]
}
```

Field rules:

* `title`: short, parent-facing title.
* `summary`: one concise paragraph.
* `highlights`: 0-5 items; the most useful concrete facts, not every total.
* `patterns`: 0-3 items; cautious observations from backend analytics.
* `comparison`: 0-3 items; use only when backend comparison data exists.
* `caveats`: 0-2 items; required only when deterministic backend facts require
  them.
* `questions_for_parent`: 0-3 items; optional, practical follow-up questions.

Caveats in v1 should be triggered by backend-owned facts, not by the model
independently judging the timeline. Required caveat triggers include:

* `range.is_partial` is true;
* comparison is unavailable where the report type normally expects a
  comparison;
* an ongoing sleep is present and relevant to the wording;
* a future backend coverage flag explicitly says the range is incomplete or
  sparse.

Scheduled email rendering may omit `questions_for_parent` if the email needs
to stay short.

## AI Interpretation Rules

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

Additional rules:

* If `range.is_partial` is true, use wording such as "so far today", "at this
  point in the day", or "based on the logs so far".
* If `range.is_partial` is true, do not present comparison deltas as final
  daily differences.
* If comparison data is absent, do not invent a comparison.
* If event notes are used, attribute them to the parent, for example "you
  noted" or "the notes mention".
* Do not infer logging coverage quality unless backend report data provides a
  deterministic flag or caveat trigger for it.
* Do not mention model limitations, prompts, schemas, or backend mechanics in
  parent-facing output.
* Do not produce diagnosis, treatment advice, urgency assessments, or safety
  claims.

## Scheduled Email Reports

Scheduled email reports should use the same AI report output contract as the
web app. Email delivery is a renderer and scheduler concern, not a separate AI
reporting brain.

Daily email reports:

* should usually cover yesterday as a complete local calendar day;
* should include baseline comparison when available;
* should avoid "today so far" unless the schedule explicitly sends an
  in-progress daytime digest.

Weekly email reports:

* should cover seven complete local calendar days;
* should compare selected daily averages against the previous baseline when
  available;
* should summarize the week at a high level and avoid listing every event.

Email output should be calm and compact:

* one title;
* one summary paragraph;
* three to five highlights;
* one caveat only when needed;
* optional follow-up questions.

Email output must not include raw event IDs, internal schema names, tokens,
family membership data, or debugging metadata.

## API Plan

### Deterministic Data

```http
GET /api/v1/babies/current/reports/data?start_date=2026-07-13&end_date=2026-07-13
```

Returns the canonical report-data payload.

### On-Demand AI

```http
POST /api/v1/babies/current/reports/ai
```

Request:

```json
{
  "report_type": "daily",
  "start_date": "2026-07-13",
  "end_date": "2026-07-13"
}
```

Generates or returns cached AI insights for the selected range.

Rules:

* called only when the user asks for AI;
* never called automatically after event changes;
* uses deterministic report data and baseline as input;
* returns cached output if the input hash matches;
* regenerates only when input changes or cache policy says to refresh.

Scheduled email jobs should call the same backend AI report generation path
with a complete selected range. For example, a weekly scheduled email can use
`report_type=weekly` and a seven-day local date range ending yesterday.

## Caching

Use an `ai_report_cache` table keyed by:

* `family_id`;
* `baby_id`;
* `report_type`;
* `range_start`;
* `range_end`;
* `input_hash`.

Store:

* model;
* prompt/schema version;
* input schema version;
* output schema version;
* generated content JSONB;
* `created_at`;
* optional delivery/rendering metadata for audit only.

The cache protects UX and cost. It should not make event creation slower.
It is not canonical baby history; events remain the source of truth, and AI
reports are regenerable from deterministic report data.

Scheduled email jobs should reuse cached channel-neutral reports when the
deterministic input hash matches. They should not regenerate the same report
repeatedly for each recipient.

The first AI backend PR should add `created_at` so cache entries are ready for
future retention cleanup. A later scheduler or maintenance job should delete
old cache rows after the agreed retention window, for example 90 days. Do not
add that cleanup job before cached AI reports are actually being generated.

## Evals

Do not add an eval framework before the first AI behavior exists.

When the AI backend is implemented, add a small `evals/` directory with
representative cases for:

* complete daily report;
* today-so-far partial report;
* weekly report;
* sparse logging;
* notes-heavy day;
* comparison present;
* comparison absent;
* no medical advice;
* no invented facts;
* correct use of baby's timezone;
* partial comparison phrasing, using "so far" style language.

The first eval suite should check:

* output is valid `ai_report_output.v1` JSON;
* output does not contain facts absent from input;
* output does not perform arithmetic not supplied by backend facts;
* output does not diagnose or advise treatment;
* partial ranges are described as partial;
* scheduled weekly output stays concise.
* canonical input hashing is stable when only generated timestamps change.

Document the eval command in the eval README when the suite is added.

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
   * Status: implemented for `/reports/data`.

5. **Analytics comparison**
   * Add selected range versus baseline daily-average comparisons.
   * Compare like with like; do not compare a one-day value with a seven-day
     total.
   * Status: implemented for selected-range analytics in `/reports/data`.

6. **AI backend**
   * Add AI input/output contract types.
   * Add `ai_report_cache` migration and store methods.
   * Add on-demand AI endpoint.
   * Cache by deterministic input hash and schema version.

7. **AI generation**
   * Add OpenAI client.
   * Generate `ai_report_output.v1` JSON on cache misses.
   * Validate model output before caching it.

8. **Scheduled report delivery**
   * Add daily and weekly scheduled report jobs.
   * Use complete selected windows by default.
   * Render cached AI report JSON into email templates.

9. **Frontend AI interaction**
   * Add explicit AI button/toggle.
   * Show loading/error states.
   * Keep AI hidden by default.
   * Do not call AI during normal timeline refresh.

10. **MCP exposure**
   * Expose deterministic report data first.
   * Expose AI insight retrieval only after backend behavior is stable.

## Open Questions

* What note length cap should be used before AI input, if any?
* What local time should scheduled daily and weekly emails be generated?
* What is the minimum data threshold before AI should produce comparison
  insights?
* Should AI output be editable/dismissible by parents?
* Should AI insights be stored permanently or treated as regenerable cache?
* What model should be the default OpenAI model for this feature?
* Should the first AI version support past days, today only, or every range
  supported by the timeline?
* Will a future version need explicit delivery-specific style profiles, or is
  channel-neutral content enough?

## Decision to Confirm Before Implementation

Before writing implementation code, confirm this product/technical decision:

**The AI report is an interpretation of backend-derived facts, not a raw-event
summarizer or calculator.**

If we keep that boundary, the implementation should stay reliable, testable,
and easier to evolve.
