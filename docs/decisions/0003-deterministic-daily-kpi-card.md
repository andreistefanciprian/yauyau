# Deterministic Daily KPI Card

## Context

The AI daily report card became larger and more variable than the timeline
needed. Parents primarily need the day's core totals at a glance, and the card
should not depend on model latency, configuration, caching, or prose quality.

The generic AI range report still serves scheduled email and future report
consumers, so removing AI from the timeline must not remove that workflow.

## Decision

The timeline daily report is a deterministic KPI card owned by backend-api.
For every selected day it returns the baby's day-specific title and exactly
four metrics in a stable order:

1. feed count, total recorded volume, and total recorded duration;
2. completed sleep count and duration;
3. pump count and recorded volume;
4. nappy count.

The frontend only renders those values. It makes no secondary AI request and
displays no generated title, story, observation, or closing. The dedicated
daily-card prompt, schema, model method, handler, route, cache workflow, evals,
and frontend loading path are removed.

The generic `GenerateAIReport` workflow and its `/reports/ai` endpoint remain
unchanged for scheduled emails and other range-report consumers.

## Alternatives Considered

Keep the AI card behind the existing visibility toggle. This still retains an
unused product path, model cost, latency, and maintenance burden when the card
is shown.

Keep AI only for today's title or one short observation. This preserves the
same prompt, schema, cache, error handling, and client complexity for too little
timeline value.

Calculate the totals in the frontend. This would duplicate backend business
logic and violate the API-first boundary.

## Consequences

The timeline card is fast, predictable, and identical in structure across
today and historical days. It is always visible above the timeline and
continues to refresh after event mutations. The event-type filter does not
control the KPI card.

Feed volume and duration sum the recorded `amount_ml` and `duration_minutes`
across all feed types. Missing values are not estimated. Sleep duration
represents recorded completed duration, matching the existing deterministic
report calculations. Zero-value metrics are still displayed so the layout
never shifts.

The UI no longer gives a generated narrative interpretation. Scheduled email
reports continue to provide AI-generated summaries through the separate generic
report path.
