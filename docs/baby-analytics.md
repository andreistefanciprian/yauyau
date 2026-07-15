# Baby Analytics

Status: **planning**.

This document defines Yauli's backend-owned baby analytics layer.

Analytics are backend-owned facts calculated from stored events. They help AI
and product surfaces understand the shape of a selected period without asking
them to do arithmetic, infer timing patterns, or inspect every raw event
manually.

The goal is not to compute everything. The goal is to answer parent questions
with a small set of high-quality, carefully named metrics.

Reports are the first consumer of baby analytics, but not the owner. The same
analytics layer should eventually support:

* report data;
* AI summaries;
* MCP tools;
* dashboards;
* widgets;
* notifications;
* weekly or monthly summaries.

AI report input/output, scheduled daily and weekly email report behavior, and
AI interpretation rules are documented in
[docs/ai-report-contract.md](ai-report-contract.md).

## Core Idea

Analytics should answer questions, not produce a spreadsheet.

Every metric should help answer a parent question such as:

* "Has today been different?"
* "What's her rhythm?"
* "Did anything stand out?"
* "What happened after the bath?"
* "Are we forgetting to log something?"

If a metric does not help answer a likely parent question, do not add it.

## Principles

Analytics must be:

* deterministic;
* calculated only in `backend-api`;
* based only on events in the selected local time window;
* safe to show to AI and MCP clients;
* factual rather than interpretive;
* named clearly enough that another engineer can understand the calculation
  without reading the implementation.

Analytics must not:

* diagnose;
* classify a day as normal or abnormal;
* imply missing logs mean missing care;
* infer causation from event order;
* produce advice or recommendations;
* duplicate simple totals already present under `totals`.

## Placement

The first API consumer should be the existing report-data endpoint:

```http
GET /api/v1/babies/current/reports/data?start_date=2026-07-13&end_date=2026-07-13
```

They should appear at the same levels as totals:

```json
{
  "totals": {},
  "analytics": {},
  "baseline": {
    "totals": {},
    "analytics": {}
  },
  "days": [
    {
      "totals": {},
      "analytics": {},
      "events": []
    }
  ]
}
```

The same builder should be used for:

* the selected range;
* each local day in `days`;
* the previous-7-day baseline.

This does not require a new endpoint yet. Exposing analytics first through
`/reports/data` keeps the PR small while preserving the longer-term analytics
engine boundary.

Conceptual pipeline:

```text
events
  -> analytics window
  -> totals
  -> analytics
  -> reports / AI / MCP / dashboard
```

## Top-Level Shape

Use these analytics sections:

```json
{
  "analytics": {
    "timeline": {},
    "chronology": {},
    "intervals": {},
    "relationships": [],
    "comparison": {}
  }
}
```

Include `comparison` only on selected-range analytics when baseline comparison
has been calculated. Do not include `comparison` on individual day analytics or
baseline analytics.

## PR Sequence

### Analytics PR 1

Add high-confidence, low-interpretation metrics:

* timeline span;
* small chronology set;
* feed intervals;
* sleep durations;
* fixed relationship definitions.

No comparison yet.

### Analytics PR 2

Add baseline comparison:

* selected range versus baseline daily averages;
* clear selected and baseline values;
* clear deltas.

Comparison must compare like with like.

### Later Analytics PRs

Add metrics that require slightly more product care:

* wake windows;
* most active period;
* quietest period;
* notable intervals, only if they simplify consumers without duplicating too
  much interval data.

## Analytics PR 1 Shape

Proposed first implementation:

```json
{
  "analytics": {
    "timeline": {
      "first_event_at": "2026-07-15T06:42:00+09:30",
      "last_event_at": "2026-07-15T21:18:00+09:30",
      "span_minutes": 876
    },
    "chronology": {
      "first_feed_at": "2026-07-15T07:10:00+09:30",
      "last_feed_at": "2026-07-15T21:15:00+09:30",
      "last_nappy_at": "2026-07-15T20:55:00+09:30",
      "last_poo_at": "2026-07-15T18:20:00+09:30",
      "last_sleep_start_at": "2026-07-15T19:10:00+09:30"
    },
    "intervals": {
      "feeds": {
        "gap_count": 5,
        "average_gap_minutes": 170,
        "longest_gap_minutes": 230,
        "shortest_gap_minutes": 82
      },
      "sleeps": {
        "completed_count": 3,
        "ongoing_count": 1,
        "average_duration_minutes": 88,
        "longest_duration_minutes": 172,
        "shortest_duration_minutes": 35
      }
    },
    "relationships": [
      {
        "key": "feed_then_nappy",
        "from": "feed",
        "to": "nappy",
        "window_minutes": 30,
        "count": 5
      },
      {
        "key": "feed_then_sleep",
        "from": "feed",
        "to": "sleep",
        "window_minutes": 45,
        "count": 4
      },
      {
        "key": "bath_then_sleep",
        "from": "bath",
        "to": "sleep",
        "window_minutes": 60,
        "count": 1
      }
    ]
  }
}
```

Fields with no meaningful value should be omitted where possible. For example,
if there is only one feed, `gap_count` should be `0`, but average, longest,
and shortest feed gaps should be omitted.

## Timeline

Timeline metrics describe the logged window without interpreting it as the
baby's actual activity or wakefulness.

Parent questions:

* "What did the day look like?"
* "When did things start and end?"
* "Was there much logged today?"

### `first_event_at`

The earliest event timestamp in the selected window, as an RFC3339 timestamp
in the baby's timezone.

Omit when there are no events.

### `last_event_at`

The latest event timestamp in the selected window, as an RFC3339 timestamp in
the baby's timezone.

Omit when there are no events.

### `span_minutes`

The elapsed minutes between the first and last event.

For example, events from `06:20` to `20:44` have a timeline span of `864`
minutes.

Use only logged events. Do not stretch this value to midnight, `range_end`, or
the current time.

Omit when there are fewer than two events.

AI may say "the logged timeline spans around 14 hours." AI must not imply the
baby was awake or active for exactly that long.

## Chronology

Chronology captures a small set of first/last event times that answer common
parent questions.

Parent questions:

* "When was the first feed?"
* "When was the last feed?"
* "When was the last nappy?"
* "When was the last poo?"
* "When did the last sleep start?"

Initial fields:

* `first_feed_at`;
* `last_feed_at`;
* `last_nappy_at`;
* `last_poo_at`;
* `last_sleep_start_at`.

Rules:

* Values are RFC3339 timestamps in the baby's timezone.
* UI and AI layers may format these timestamps as `HH:MM` when the selected
  analytics window is one local day.
* Omit fields when the relevant event type is absent.
* `last_poo_at` includes nappies with `kind` of `poo` or `both`.
* Do not add first/last fields for every event type unless a real parent
  question needs them.

This section intentionally stays small. It should not become an exhaustive
event-type index.

## Intervals

Intervals describe spacing and duration.

Parent questions:

* "What's her rhythm?"
* "How far apart were feeds?"
* "How long were sleeps?"
* "Did anything stand out?"

### Feed Intervals

All feed types count as feeds:

* breast;
* formula;
* expressed.

Bottle volume stays in `totals`. Analytics should not calculate bottle amount
averages in PR 1.

Feed gaps are the minutes between consecutive feed start times, ordered by
`occurred_at`.

Fields:

* `gap_count`;
* `average_gap_minutes`;
* `longest_gap_minutes`;
* `shortest_gap_minutes`.

Rules:

* `gap_count` is `feed_count - 1`.
* If there are fewer than two feeds, `gap_count` is `0`.
* If `gap_count` is `0`, omit average, longest, and shortest gap fields.
* Use integer minutes.
* Round average to the nearest whole minute.
* Do not split by feed type in PR 1.
* Partition calculations by the baby's local date. Feeds on different local
  dates are never paired, even when the selected analytics window spans
  multiple days.

For multi-day selected ranges and baselines, range-level feed gap metrics are
calculated from the union of local-day feed gaps. This means overnight gaps
between the final feed on one local date and the first feed on the next local
date are not included.

AI may say "recorded feeds were spaced about X apart on average." AI must not
say the baby was hungry, satisfied, overfed, or underfed.

### Sleep Intervals

Sleep intervals describe recorded sleep durations. They do not estimate total
sleep if logs are incomplete.

Fields:

* `completed_count`;
* `ongoing_count`;
* `average_duration_minutes`;
* `longest_duration_minutes`;
* `shortest_duration_minutes`.

Rules:

* Completed sleeps have `duration_minutes`.
* Ongoing sleeps do not have `duration_minutes`.
* Duration fields use completed sleeps only.
* If there are no completed sleeps, omit duration fields.
* Use integer minutes.
* Round average to the nearest whole minute.

AI may say "the longest recorded sleep was X." AI must not imply this was the
baby's longest actual sleep if logging coverage is sparse.

## Relationships

Relationships count nearby event orderings that may be useful context. They do
not imply causation.

Parent questions:

* "What happened after the bath?"
* "Do nappies tend to happen after feeds?"
* "Do sleeps tend to follow feeds?"

First version:

* feed -> nappy within 30 minutes;
* feed -> sleep within 45 minutes;
* bath -> sleep within 60 minutes.

Response shape:

```json
[
  {
    "key": "feed_then_nappy",
    "from": "feed",
    "to": "nappy",
    "window_minutes": 30,
    "count": 4
  },
  {
    "key": "feed_then_sleep",
    "from": "feed",
    "to": "sleep",
    "window_minutes": 45,
    "count": 2
  },
  {
    "key": "bath_then_sleep",
    "from": "bath",
    "to": "sleep",
    "window_minutes": 60,
    "count": 1
  }
]
```

Relationships should use a backend-owned fixed registry, not dynamic
generation and not user configuration:

```go
var relationshipDefinitions = []relationshipDefinition{
    {Key: "feed_then_nappy", From: eventTypeFeed, To: eventTypeNappy, Window: 30 * time.Minute},
    {Key: "feed_then_sleep", From: eventTypeFeed, To: eventTypeSleep, Window: 45 * time.Minute},
    {Key: "bath_then_sleep", From: eventTypeBath, To: eventTypeSleep, Window: 60 * time.Minute},
}
```

The `key` is a stable semantic identifier for tests, prompts, frontend code,
MCP clients, and future API consumers.

Rules:

* Events are ordered oldest-first by `occurred_at`.
* For each source event, count at most one matching target event.
* The target event must occur after the source event.
* The target event must occur within the named window.
* If multiple matching target events exist, use the earliest one.
* Do not count target events before the source event.
* Partition calculations by the baby's local date. Events on different local
  dates are never matched, even when the selected analytics window spans
  multiple days.

Therefore, range-level relationship counts equal the sum of the corresponding
daily relationship counts. A feed at `23:50` and a nappy at `00:10` the next
local day do not count as a feed -> nappy relationship.

AI may say "X nappies were logged within 30 minutes after feeds." AI must not
say "feeds caused nappies" or "bath caused sleep."

## Comparison

Comparison helps answer:

* "Has today been different?"
* "How does this selected range compare with recent days?"

Comparison is included only on selected-range analytics. It compares the
selected range to the previous-7-day baseline using daily averages.

### Hard Rule

Comparison metrics must compare like with like.

Do not compare a one-day selected value to a seven-day baseline total:

```json
{
  "selected_feed_count": 7,
  "baseline_feed_count": 49
}
```

Instead, normalize the baseline to a daily average:

```json
{
  "selected_feed_count": 7,
  "baseline_average_daily_feed_count": 7,
  "feed_count_delta_from_baseline_daily_average": 0
}
```

For multi-day selected ranges, normalize the selected range too:

```json
{
  "selected_average_daily_feed_count": 8,
  "baseline_average_daily_feed_count": 7,
  "feed_count_delta_from_baseline_daily_average": 1
}
```

Comparison fields must use selected and baseline values normalized to the same
unit, usually per-day averages.

Initial fields:

```json
{
  "comparison": {
    "selected_days_included": 1,
    "baseline_days_included": 7,

    "selected_average_daily_feed_count": 7,
    "baseline_average_daily_feed_count": 7,
    "feed_count_delta_from_baseline_daily_average": 0,

    "selected_average_daily_nappy_count": 8,
    "baseline_average_daily_nappy_count": 6.9,
    "nappy_count_delta_from_baseline_daily_average": 1.1,

    "selected_average_daily_completed_sleep_count": 3,
    "baseline_average_daily_completed_sleep_count": 3.4,
    "completed_sleep_count_delta_from_baseline_daily_average": -0.4,

    "selected_average_daily_sleep_minutes": 260,
    "baseline_average_daily_sleep_minutes": 310,
    "sleep_minutes_delta_from_baseline_daily_average": -50
  }
}
```

Rules:

* Round averages and deltas to one decimal place.
* Calculate deltas as selected daily average minus baseline daily average.
* Do not compare selected values to baseline totals.
* Do not include comparison in `days[].analytics` or `baseline.analytics`.

### Partial Ranges

The selected range can be partial, most commonly when requesting today's
calendar day before it has finished. The response already marks this with
`range.is_partial: true`.

For partial selected ranges, the current comparison fields should be treated as
partial context, not as final daily deltas. A today-so-far selected range is
still divided by `selected_days_included`, while the baseline uses complete
previous local calendar days. This can help AI understand that the selected
window is still developing, but consumers must not present the deltas as if
the day were complete.

AI instructions should make this explicit. When `range.is_partial` is `true`,
AI must not describe comparison deltas as final daily differences. Prefer
phrases such as "so far today", "at this point in the day", or "based on the
logs so far", and mention that the pattern may change as more events are
recorded.

Scheduled reports should prefer complete selected windows, such as yesterday,
a completed last-three-days range, or a completed last-seven-days range. In
those cases, selected daily averages and baseline daily averages are directly
comparable and the comparison fields are suitable for email/report generation.

Future improvement: add an explicit comparison mode such as
`partial_elapsed_window` for today-so-far. That mode should compare today's
midnight-to-now window against each previous baseline day from midnight to the
same local cutoff time, rather than comparing a partial day to full-day
baseline averages.

## Later Candidates

These are useful, but need more care than PR 1.

### Wake Windows

Parent question:

* "How long was she awake between sleeps?"

Candidate fields:

```json
{
  "average_wake_window_minutes": 81,
  "longest_wake_window_minutes": 145
}
```

Rules need to define whether wake windows use:

* sleep end to next sleep start;
* sleep start plus `duration_minutes` to next sleep start;
* only completed sleeps.

### Activity Periods

Parent question:

* "Which part of the day was busiest?"

Candidate fields:

```json
{
  "most_active_period": "morning",
  "quietest_period": "afternoon"
}
```

This needs a fixed local-day partition:

* night;
* morning;
* afternoon;
* evening.

Define the exact time ranges before implementation.

### Notable Intervals

Parent question:

* "Did anything stand out?"

Candidate fields:

```json
{
  "notable_intervals": {
    "longest_feed_gap_minutes": 230,
    "longest_sleep_minutes": 172,
    "time_between_bath_and_sleep_minutes": 18
  }
}
```

This is mostly a curated view over existing interval and relationship metrics.
Add it only if it makes AI prompts simpler without duplicating too much data.

## Separate Features

These are useful, but they should not be part of the generic historical
analytics builder.

### Current State

Candidate shape:

```json
{
  "current_state": {
    "minutes_since_last_feed": 140,
    "minutes_since_last_sleep": 53,
    "minutes_since_last_nappy": 82
  }
}
```

This should be separate from analytics because it depends on the current clock,
not only on the selected event window.

Reasons to keep it separate:

* it changes even when no event changes;
* it only makes sense for today;
* it affects caching;
* it cannot be reproduced identically later;
* it conflicts with the rule that analytics are based only on the selected
  local window.

Implement current state through focused backend/MCP operations such as:

* `get_last_feed`;
* `get_last_nappy`;
* `get_current_sleep`;
* a future `GET /api/v1/babies/current/state`.

### Highlights

Candidate shape:

```json
{
  "highlights": {
    "longest_sleep_minutes": 182,
    "longest_feed_gap_minutes": 235
  }
}
```

Do not add this in PR 1.

Reason: highlights can duplicate facts already present under `intervals`.
Selecting which facts matter is a good job for AI and UI presentation layers.
Add deterministic highlights only if several consumers repeatedly need the
same curated subset.

## Empty Data

When a selected window has no events:

```json
{
  "analytics": {
    "timeline": {},
    "chronology": {},
    "intervals": {
      "feeds": {
        "gap_count": 0
      },
      "sleeps": {
        "completed_count": 0,
        "ongoing_count": 0
      }
    },
    "relationships": [
      {
        "key": "feed_then_nappy",
        "from": "feed",
        "to": "nappy",
        "window_minutes": 30,
        "count": 0
      },
      {
        "key": "feed_then_sleep",
        "from": "feed",
        "to": "sleep",
        "window_minutes": 45,
        "count": 0
      },
      {
        "key": "bath_then_sleep",
        "from": "bath",
        "to": "sleep",
        "window_minutes": 60,
        "count": 0
      }
    ]
  }
}
```

This keeps the shape predictable while avoiding fake timestamps or fake
duration metrics.

## Deferred On Purpose

These may be useful later, but should not be in the first analytics PR.

### Nappy Timing

Deferred:

* longest gap between nappies;
* time since last wet nappy;
* time since last poo nappy.

Reason: easy to over-interpret and more useful in a targeted "last nappy" or
"nappy trend" tool than in general AI day insight.

### Bottle Amount Averages

Deferred:

* average bottle amount;
* largest bottle amount;
* smallest bottle amount.

Reason: totals already capture volume. Averages are more useful after we know
how AI will phrase them without sounding clinical or judgmental.

### Data Quality Flags

Deferred:

* possibly incomplete day;
* sparse logging;
* missing sleep after evening;
* no events after a certain time.

Reason: these are useful, but they are interpretive enough to deserve a
separate data-quality document and PR.

### Complex Statistics

Do not add:

* variance;
* standard deviation;
* percentiles;
* z-scores;
* trendlines.

These do not help parents understand the day.

## Implementation Notes

Likely Go shape:

```go
type BabyAnalytics struct {
    Timeline      TimelineAnalytics       `json:"timeline"`
    Chronology    ChronologyAnalytics     `json:"chronology"`
    Intervals     IntervalAnalytics       `json:"intervals"`
    Relationships []RelationshipAnalytics `json:"relationships"`
}

func BuildBabyAnalytics(events []store.Event, loc *time.Location) BabyAnalytics
```

The report-data response can embed `BabyAnalytics`, but the analytics types and
builder should not be named as report-owned concepts.

The builder should expect events sorted oldest-first, or sort defensively using
the same ordering as report data:

1. `occurred_at` ascending;
2. event ID ascending for equal timestamps.

Tests should cover:

* no events;
* one feed;
* multiple feed gaps;
* feed gaps do not cross local dates;
* completed and ongoing sleeps;
* timeline and chronology timestamps are RFC3339 values in the baby's timezone;
* relationship counts inside and outside their time windows;
* relationships do not match events across local dates;
* at most one target event counted per source event;
* range, day, and baseline wiring.

## Open Questions

* Should `span_minutes` be omitted or set to `0` when there is exactly one
  event?
* Should average values be rounded half up or use Go's normal rounding?
* Should relationship windows be configurable constants or fixed names in the
  response contract?
* Should ongoing feed count include bottle feeds that have amount but no
  duration, or only feeds started through the "start feed" flow?

## Growth Context

Growth measurements should not be folded into timeline analytics for PR 1.
They answer a different question than daily rhythm, intervals, and
relationships.

Instead, reports should receive optional baby context from a
`baby_latest_growth` projection:

```json
{
  "baby": {
    "latest_growth": {
      "weight": {
        "grams": 7200,
        "measured_at": "2026-07-10T08:00:00+09:30",
        "age_days": 190
      },
      "length": {
        "cm": 66.5,
        "measured_at": "2026-07-01T08:00:00+09:30",
        "age_days": 181
      }
    }
  }
}
```

Growth measurement events remain the source of truth. The projection is updated
when growth measurement events are created, edited, or deleted, so reports do
not need to scan years of historical events just to provide the latest known
weight or length. Each measurement carries its own timestamp because families
may record weight, length, and head circumference on different dates.
