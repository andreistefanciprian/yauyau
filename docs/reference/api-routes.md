# API Endpoint Structure (current implementation)

This section documents how the code is actually laid out today, so the
pattern is easy to follow when adding the next event type. See
[docs/data-model.md](../data-model.md) for the Event Model, the long-term
vision (feed/nappy/sleep/pump/etc. as a generalized model) that this
section's backend-api + frontend wiring implements.

## Public discovery routes

The frontend serves three static discovery files at the site root:

* `GET /robots.txt` allows the public landing page, excludes private
  family-timeline and token-bearing workflows from crawling, and points
  crawlers to the sitemap.
* `GET /sitemap.xml` lists only the canonical public landing page at
  `https://getyauli.com/`.
* `GET /llms.txt` provides a concise, factual product overview and links to
  the public landing page and source repository. It explicitly distinguishes
  current web-app functionality from the planned ChatGPT/MCP experience.

Authenticated and authentication-related HTML pages also emit a `noindex`
robots meta tag. The public sign-in page remains crawlable so crawlers can see
and honour that tag. `robots.txt` is crawler guidance, not an access-control
boundary; sessions and authorization continue to protect all private data.

## backend-api routes

Authenticated app routes are mounted under `/api/v1/users` and
`/api/v1/babies` in `backend-api/cmd/server/main.go`, behind
`authctx.Middleware` (verifies the `Authorization: Bearer` JWT's
signature/expiry and decodes the caller's identity into context — see
`internal/authctx`):

* `GET /healthz` — unauthenticated.
* `GET /api/v1/users/me` → `GetCurrentUser`; returns the authenticated
  user's id, email, and display name for account/settings UI.
* `PATCH /api/v1/users/me` → `UpdateCurrentUser`; updates optional account
  profile fields such as `display_name`.
* `POST /api/v1/babies` → `CreateBaby`. A caller with no existing family
  membership gets a family created implicitly (auto-named, never shown to
  the user) and becomes its owner in the same call; a caller who already
  belongs to a family just gets a sibling baby added to it.
* `GET /api/v1/babies/current` → `GetCurrentBaby`, family-scoped (the
  caller's family's first-created baby, or 404 meaning "no baby yet").
  Includes `has_pending_invite` so the frontend can warn one-active-timeline
  users that they must archive the current baby before joining another
  timeline.
* `PATCH /api/v1/babies/current` → `UpdateCurrentBaby`, owner-only; updates
  current baby profile fields such as name, timezone, birth date, birth
  weight, birth length, and sex.
* `DELETE /api/v1/babies/current` → `ArchiveCurrentBaby`, owner-only; requires
  the caller to confirm the exact baby name and soft-deletes the active baby
  by setting `babies.archived_at`. Archiving also removes the owner's
  membership for that family and revokes their sessions, keeping the current
  product model to one active timeline per user.
* `POST /api/v1/babies/{id}/invite` → `InviteHelper`, baby-scoped and
  owner-only; creates a pending helper invite for the supplied email.
* `GET /api/v1/babies/current/members` → `ListTimelineMembers`, owner-only;
  returns active and invited users with access to the current baby's
  timeline.
* `PATCH /api/v1/babies/current/members/{userID}` →
  `UpdateTimelineMember`, owner-only; updates the member's relationship label
  (profile context such as "Dad", not an authorization role).
* `PATCH /api/v1/babies/current/members/{userID}/report-preferences` →
  `UpdateTimelineMemberReportPreferences`, owner-only; enables or disables
  scheduled report email delivery for an active member of the current baby's
  family timeline.
* `DELETE /api/v1/babies/current/members/{userID}` →
  `RemoveTimelineMember`, owner-only; cancels pending invites or removes
  active non-owner access. Active removal first asks auth-service to revoke
  the member's still-valid sessions for the family, then deletes the
  `family_members` row.
* `GET /api/v1/babies/current/events` → `ListAllEvents`, the combined feed
  behind the frontend timeline: every event type, merged and ordered
  newest-first (`store.ListAllEvents`, capped at `allEventsLimit`). Supports
  `?date=YYYY-MM-DD`; an omitted date defaults to today. Dates select a
  single calendar day in the baby's timezone.
* `GET /api/v1/babies/current/events/stream` →
  `StreamTimelineEvents`, an authenticated Server-Sent Events invalidation
  stream for the current baby. It emits `ready` and `timeline_changed`
  events, plus heartbeat comments. Messages contain no event or family data;
  consumers re-fetch the combined events/report APIs after a change. The
  stream closes when its short-lived bearer token expires so callers
  reconnect and revalidate the session.
* `GET /api/v1/babies/current/reports/daily` → `GetDailyReport`, a
  deterministic calendar-day report for the current baby. Supports
  `?date=YYYY-MM-DD`; an omitted date defaults to today. Past dates cover the
  full local calendar day, while today's report runs from midnight to now in
  the baby's timezone. The first version lives in
  `backend-api/internal/handlers/report.go` and summarizes the merged event
  stream into a structured response (`title`, legacy `summary` and
  `highlights`, deterministic `card`, `generated_at`, `range_start`, and
  `range_end`). `card.metrics` always contains feed count, total recorded feed
  volume, and total recorded feed duration; sleep count and duration; pump
  count, recorded volume, and recorded duration; and nappy count.
  The title uses the baby's name and selected day. The card contains no AI
  prose or model-dependent fields.
* `GET /api/v1/babies/current/reports/data` → `GetReportData`, the canonical
  factual report-data payload for one to 31 local calendar days. Supports
  inclusive `?start_date=YYYY-MM-DD&end_date=YYYY-MM-DD`; omitting both
  dates defaults to today. The response includes minimal baby context, range
  metadata, factual totals and baby analytics for the whole range, including
  selected-range comparison against previous-7-day baseline daily averages.
  It returns one deterministic daily report plus factual totals, baby
  analytics, and normalized oldest-first events per day. It also includes
  previous-7-day baseline range metadata, totals, and baby analytics. It
  intentionally does not include AI output yet.
* `POST /api/v1/babies/current/reports/ai` → `CreateAIReport`, the existing
  cached AI generation path for selected daily and weekly range reports and
  scheduled email. It remains on `ai_report_output.v1`. Every report type and
  locale may occasionally use at most one subtle, everyday Australian English
  expression when it fits naturally; locale still controls terminology and
  units.
* `PATCH /api/v1/babies/current/events/{id}` → `UpdateEvent`, type-checked
  generic edit for an existing current-baby event.
* `DELETE /api/v1/babies/current/events/{id}` → `DeleteEvent`, removes one
  current-baby event regardless of type.
* Per event type, nested under its plural resource name (`/nappies`,
  `/feeds`, `/pumps`, `/baths`, `/observations`,
  `/growth-measurements`, ...):
  * `POST /api/v1/babies/current/<resource>` → `Create<Type>`
  * Feed, pump, and sleep events without `duration_minutes` are ongoing.
    Adding a duration completes the event.
  * Sleep `type` may be omitted on create or generic update. The backend then
    classifies the sleep from its start time: starts from 18:00 through 05:59
    are `night`, and starts from 06:00 through 17:59 are `nap`. Explicit
    `nap` or `night` values remain supported for corrections and older
    clients.

## The generic event store

There is one `events` table (`event_type TEXT`, `attributes JSONB`,
`occurred_at`, plus id/baby_id/created_at). `store.PostgresStore` exposes
generic event methods, shared by every event type:

* `CreateEvent(ctx, eventType string, attributes map[string]any, occurredAt time.Time) (Event, error)`
* `UpdateEvent(ctx, familyID, babyID, id uuid.UUID, eventType string, attributes map[string]any, occurredAt time.Time) (Event, error)`
* `DeleteEvent(ctx, familyID, babyID, id uuid.UUID) error`
* `ListAllEvents(ctx, familyID, babyID uuid.UUID, from, to time.Time, limit int) ([]Event, error)`

No event-type-specific SQL exists anywhere — a new event type never touches
`store/postgres.go`.

## Per-event-type handler file (backend-api)

Each event type is one file in `backend-api/internal/handlers/` (`nappy.go`,
`feed.go`, `pump.go`, `bath.go`, `observation.go`,
`growth_measurement.go`) containing, and nothing else:

1. A `const eventType<X> = "<x>"` string.
2. Any enum-like type for constrained fields (e.g. `NappyKind`, `FeedType`)
   with a `Valid()` method — only where the field genuinely has a fixed set
   of values. Free-text fields (like observation's `category`) skip this.
3. `create<X>Request` — the JSON body shape.
4. `<x>Response` — the JSON response shape.
5. `<x>FromEvent(ev store.Event) <x>Response` — maps `ev.Attributes` back to
   the typed response, doing any type coercion (see `attributeInt` in
   `feed.go`, reused by `bath.go`, for the JSONB int/float64 quirk).
6. `Create<X>` handler: decode request → validate/trim → build
   `map[string]any` attributes → call the shared
   `createAndRespond(w, r, h, eventType<X>, attributes, occurredAt, <x>FromEvent)`.
7. Combined reads go through `ListAllEvents`; per-type files do not define
   list handlers.

`createAndRespond` (a generic helper in `handlers.go`) owns the actual
`Store.CreateEvent` call, error logging, and JSON response — per-type create
handlers never call the store directly.

**To add a new event type on the backend:** create the new handler file
following the steps above, register its create route in `cmd/server/main.go`,
and add a migration only if the event needs no schema
changes beyond `attributes` (usually it doesn't — JSONB absorbs new fields
without a migration).

## Frontend wiring

`frontend/internal/backendclient` has no per-event-type methods — just
generic `ListEvents(ctx, resource string, date string, out any)`,
`CreateEvent(ctx, resource string, payload map[string]any)`, and
`UpdateEvent(ctx, id string, payload map[string]any)` against
`/api/v1/babies/current/<resource>`. Reads go through the combined
`ListEvents(ctx, "events", selectedDate, &events)` (backend-api's `/events`
endpoint, already merged, date-filtered, and sorted newest-first across
every event type); creates still go through `CreateEvent(ctx, "<resource>", payload)` per type
(`"nappies"`, `"feeds"`, `"pumps"`, `"baths"`, `"observations"`,
`"growth-measurements"`), while edits go through the combined `UpdateEvent`
route. The only shape
`backendclient.go` decodes is the generic `Event` struct (`EventType` plus
an `Attributes map[string]any`) — no per-event-type typed view structs.

The UI is a single merged, chronological timeline (not one list per event
type) fed by a single "Add Event" dialog (not one form per event type).
`frontend/internal/handlers/handlers.go`:

* Every event type is flattened into one presentation shape,
  `TimelineEvent` (`CSSClass`, `TypeLabel`, `Kind`, `Detail`, `Time`). Event
  icons are shared inline SVG templates selected by `EventType` in the
  frontend templates. Nappy and sleep discriminators are rendered directly
  as specific `TypeLabel` values (for example `Wee Poo` and `Nap`). `Kind` is
  used for feed/bath's type and observation's category; pump intentionally
  leaves it empty. It is rendered next to `TypeLabel`.
  A `<x>TimelineEvent(ev, loc, now)` function builds one from a generic
  `backendclient.Event`, reading its `Attributes` map — this is where
  per-type display text (e.g. feed's "70 ml · 10 min") is decided.
  `timelineEvent(ev, loc, now)` dispatches to the right builder by
  `ev.EventType`, skipping (and logging) any type the frontend doesn't
  recognize.
* `loadTimeline(ctx, loc, selectedDate)` makes one
  `ListEvents(ctx, "events", selectedDate, ...)` call and converts each item to
  a `TimelineEvent` — no client-side merging or sorting; the backend already
  returns one merged, ordered list for the selected date.
* `Index` calls `loadTimeline` and renders the full page.
* `Index` calls `Backend.GetDailyReport` for the selected date, then renders
  `templates/timeline.html`'s `timeline-workspace` partial. That workspace
  always contains both the daily KPI card and `#timeline`, so HTMX event
  mutations can refresh both together and avoid stale report counts. The
  Timeline filter controls event types only; it does not hide the KPI card.
* The frontend-only `GET /timeline/events/stream` route proxies backend-api's
  private SSE response through the browser's cookie-authenticated frontend
  session. `frontend/static/app.js` opens that same-origin stream and treats
  `ready`/`timeline_changed` as invalidation signals. It re-fetches the
  selected date through the existing HTMX `/app` path, which refreshes the
  full `timeline-workspace` (daily KPI card plus events). Signals are
  debounced, hidden tabs defer their refresh until visible, and every
  successful SSE connection reconciles canonical state. Native `EventSource`
  reconnects automatically.
* PostgreSQL migration `0012_timeline_event_notifications.sql` installs the
  commit-aware event-table trigger. Each backend-api instance owns one
  dedicated `LISTEN timeline_events_changed` connection and fans opaque
  `baby_id` signals only to matching authenticated subscribers. Notifications
  are non-durable and coalescing; connection/reconnection refreshes reconcile
  canonical state. See
  [How Timeline SSE Updates Work](../sse-timeline-updates.md) for the complete
  connection and mutation sequence.
* Each `Create<X>` handler parses the HTML form, builds a `map[string]any`
  payload (plus `occurred_at` via `parseEventTime`), calls
  `Backend.CreateEvent(ctx, "<resource>", payload)`, then calls the shared
  `renderTimeline` (itself `loadTimeline` + selected-date daily-report load
  + render), so every form's htmx response is the same re-sorted, all-types
  `timeline-workspace` partial (`templates/timeline.html`) swapped over
  `#timeline-workspace` with `outerHTML` — never a per-type partial. The
  selected date is carried in each form/delete request so HTMX refreshes
  preserve the parent's current view.
* `UpdateEvent` uses the same `renderTimeline` tail after patching the
  combined `/events/{id}` route. Timeline quick actions for ongoing feeds,
  pumps, and sleeps post to frontend-only finish routes
  (`/events/{id}/finish-feed`, `/events/{id}/finish-pump`, and
  `/events/{id}/finish-sleep`), which preserve the original event start time
  and call the same backend update route with a calculated duration.

On the client, `frontend/static/app.js` drives the "Add Event" dialog: a
type-picker step (one button per event type) followed by a
form-fields step showing only the chosen type's `<form class="event-form"
data-type="...">` block from `templates/index.html`. Each form still posts
straight to its own existing endpoint (`/nappies`, `/feeds`, `/pumps`, ...); the
dialog only changes what's *shown*, not the request shape.

Timeline cards carry the event's editable values in `data-*` attributes and
open the shared edit dialog when clicked or activated with Enter/Space. The
edit dialog keeps Save disabled until its populated form differs from the
original event and contains the event's immediate Delete action; deletion does
not add a separate confirmation step.

Frontend templates load CSS and JavaScript through content-fingerprinted
`/static/...?...` URLs generated once at server startup. A changed asset gets a
new URL on deployment, so CDN or browser caching cannot combine fresh HTML with
stale client behavior.

**To add a new event type on the frontend:** add a `<x>TimelineEvent`
builder in `handlers.go` (reading from `ev.Attributes`) and a case for it
in `timelineEvent`'s switch; add a `Create<X>` handler ending in
`h.renderTimeline(...)`; add a `<x>` type-choice button and its `.event-form
data-type="<x>"` block in `index.html`; add a `<x>: "Log a <x>"` entry to
`typeLabels` in `app.js`; wire the create/list routes for the new resource
in `cmd/server/main.go` (`/events` already returns every type, no change
needed there); and give the new card colour a light/dark pair in
`style.css`.
