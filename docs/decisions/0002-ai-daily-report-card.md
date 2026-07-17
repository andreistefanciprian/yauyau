# AI Daily Report Card

## Context

The deterministic daily report is accurate but reads like a statistics dump.
Parents need the feed and sleep facts within seconds, plus concise copy that
feels warm and acknowledges the work of keeping the timeline current.

Putting every word under model control would weaken factual guarantees and
would make the timeline depend on provider latency. Rendering model Markdown
or HTML would also add unnecessary presentation and security complexity.

## Decision

Backend API continues to own and format the primary feed and sleep metrics.
It also produces a complete deterministic card fallback from the current
baby, selected local day, events, and authenticated viewer relationship.

The existing AI report generation path and `internal/aiclient` are extended
with `ai_report_output.v2.daily_card`. The model returns four plain-text
fields: `intro`, `story`, `observation`, and `encouragement`. It never returns
the primary KPI lines. Generated output is constrained by strict JSON Schema,
then validated again before caching.

The web app renders the deterministic card immediately. An HTMX request loads
the cached or newly generated daily card prose and replaces only those four
fields. Any timeout, provider failure, refusal, or invalid output leaves the
fallback visible.

The semantic cache identity includes the current viewer relationship. No
other family-member profile data is sent to the model. The frontend escapes
all generated strings and applies bold emphasis only to deterministic primary
metrics.

## Alternatives Considered

Generate the whole card as Markdown. This was rejected because the model could
alter factual KPI values, raw Markdown would require another rendering and
sanitisation path, and formatting could vary between generations.

Generate synchronously during the normal daily report request. This was
rejected because model latency or availability would delay the timeline and
event mutation responses.

Use only rotating deterministic templates. This would be reliable, but it
would not provide the requested variation or future data-grounded observation
layer.

Create a separate provider client and cache for the UI card. This was rejected
because the existing AI report client, structured output support, report data,
and semantic cache already own this workflow.

## Consequences

The daily report stays accurate and available without OpenAI. Unchanged data
and relationship context reuse cached AI prose; changed events create a new
semantic input hash. Different viewers may receive separate cached closings.

The API adds a deterministic `card` object to the daily report and a
`daily_card` object to `ai_report_output.v2`. Legacy deterministic summary
fields remain available for existing report-data consumers.

The prompt and application validator now form a product contract. Changes to
tone, punctuation, emoji use, growth coverage, or relationship handling need
tests and prompt/schema version review.
