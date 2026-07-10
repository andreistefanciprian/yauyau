# AGENTS.md

## Project

**Yauli**

An AI-first baby tracking platform where the primary interface is conversational through ChatGPT using MCP tools. The web application exists primarily as a dashboard, administration interface, and manual fallback.

The first users are Cip and Jenny, but the system is designed from day one to support many families.

---

# Vision

Build the simplest and fastest way for parents to record and retrieve everything about their baby's day.

Typical interactions should be conversational:

* "Record a wet nappy."
* "Log a poo nappy, mustard yellow."
* "Record a 70 ml bottle feed."
* "YauYau just fell asleep."
* "When was her last feed?"
* "How many wet nappies today?"

The web interface should complement ChatGPT, not replace it.

---

# Design Principles

## AI First

ChatGPT is the primary user interface.

Every feature should be designed assuming it will eventually be exposed through MCP.

---

## API First

All business logic belongs in the Backend API.

The frontend and MCP server must never implement business logic independently.

---

## Thin Clients

The Frontend and MCP server are thin clients.

Responsibilities:

* authentication
* request validation
* rendering (frontend)
* tool exposure (MCP)

They should delegate business operations to the Backend API.

---

## Single Source of Truth

Only the Backend API owns:

* business rules
* validation
* event creation
* querying
* summaries

---

## Scale Ready

Design for growth without overengineering.

The application should comfortably support thousands of families without major architectural changes.

---

# Architecture

Frontend

* Go
* HTML templates
* HTMX
* Alpine.js (minimal)
* Tailwind CSS

Backend API

* Go
* REST/HTTP JSON
* business logic
* PostgreSQL access

Authentication Service

* Go
* OAuth 2.1
* PKCE
* Magic Links
* Session management
* JWT issuance

MCP Server

* Go
* OAuth protected
* exposes MCP tools
* communicates only with Backend API

Database

* PostgreSQL

Deployment

* Railway
* Four services
* One PostgreSQL database

---

# Services

## frontend

Responsibilities

* render HTML
* user dashboard
* manual event entry
* account management
* OAuth login

No business logic.

---

## backend-api

Responsibilities

* babies
* families
* users
* events
* summaries
* reporting
* validation
* timeline access membership

Owns the business domain. When membership changes affect authentication,
backend-api makes the business decision and asks auth-service to update its
session rows; auth-service must not decide who belongs to a family.

---

## auth-service

Responsibilities

* OAuth 2.1
* PKCE
* Magic Links
* access tokens
* refresh tokens
* session management

No baby domain logic. `user_id` and `family_id` are opaque identifiers here;
auth-service can revoke sessions for those IDs when backend-api asks, but it
does not read or decide family membership.

---

## mcp-server

Responsibilities

Expose tools such as:

* log_feed
* log_nappy
* log_sleep_start
* log_sleep_end
* log_pump
* log_note
* get_today_summary
* get_last_feed
* get_timeline

Never writes directly to PostgreSQL.

Always calls Backend API.

---

# Database

PostgreSQL from day one.

Core entities:

* users
* families
* family_members
* babies
* events

Authentication:

* oauth_clients
* oauth_authorization_codes
* oauth_access_tokens
* oauth_refresh_tokens
* magic_links
* sessions

Operational:

* audit_logs

---

# Event Model

Events are append-only records.

Examples:

* Feed
* Nappy
* Sleep
* Pump
* Note
* Weight
* Temperature
* Medication
* Bath
* Vaccination

The model should be extensible without frequent schema changes.

Use PostgreSQL JSONB for event-specific attributes where appropriate.

---

# API Endpoint Structure (current implementation)

This section documents how the code is actually laid out today, so the
pattern is easy to follow when adding the next event type. The Event Model
section above is the long-term vision (feed/nappy/sleep/pump/etc. as a
generalized model); this section is the concrete backend-api + frontend
wiring that implements it.

## backend-api routes

All routes are mounted under `/api/v1/babies` in
`backend-api/cmd/server/main.go`, behind `authctx.Middleware` (verifies the
`Authorization: Bearer` JWT's signature/expiry and decodes the caller's
identity into context — see `internal/authctx`):

* `GET /healthz` — unauthenticated.
* `POST /api/v1/babies` → `CreateBaby`. A caller with no existing family
  membership gets a family created implicitly (auto-named, never shown to
  the user) and becomes its owner in the same call; a caller who already
  belongs to a family just gets a sibling baby added to it.
* `GET /api/v1/babies/current` → `GetCurrentBaby`, family-scoped (the
  caller's family's first-created baby, or 404 meaning "no baby yet").
* `POST /api/v1/babies/{id}/invite` → `InviteHelper`, baby-scoped and
  owner-only; creates a pending helper invite for the supplied email.
* `GET /api/v1/babies/current/members` → `ListTimelineMembers`, owner-only;
  returns active and invited users with access to the current baby's
  timeline.
* `PATCH /api/v1/babies/current/members/{userID}` →
  `UpdateTimelineMember`, owner-only; updates the member's relationship label
  (profile context such as "Dad", not an authorization role).
* `DELETE /api/v1/babies/current/members/{userID}` →
  `RemoveTimelineMember`, owner-only; cancels pending invites or removes
  active non-owner access. Active removal first asks auth-service to revoke
  the member's still-valid sessions for the family, then deletes the
  `family_members` row.
* `GET /api/v1/babies/current/events` → `ListAllEvents`, the combined feed
  behind the frontend timeline: every event type, merged and ordered
  newest-first (`store.ListAllEvents`, capped at `allEventsLimit`). Supports
  `?range=today` (default), `?range=yesterday`, `?range=24h`, and
  `?range=3d`; calendar ranges are calculated in the baby's timezone.
* Per event type, nested under its plural resource name (`/nappies`,
  `/feeds`, `/baths`, `/observations`, ...):
  * `POST /api/v1/babies/current/<resource>` → `Create<Type>`
  * `GET /api/v1/babies/current/<resource>` → `List<Type>`

## The generic event store

There is one `events` table (`event_type TEXT`, `attributes JSONB`,
`occurred_at`, plus id/baby_id/created_at). `store.PostgresStore` exposes
only two event methods, shared by every event type:

* `CreateEvent(ctx, eventType string, attributes map[string]any, occurredAt time.Time) (Event, error)`
* `ListEvents(ctx, eventType string, limit int) ([]Event, error)`

No event-type-specific SQL exists anywhere — a new event type never touches
`store/postgres.go`.

## Per-event-type handler file (backend-api)

Each event type is one file in `backend-api/internal/handlers/` (`nappy.go`,
`feed.go`, `bath.go`, `observation.go`) containing, and nothing else:

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
7. `List<X>` handler: one line, `listAndRespond(w, r, h, eventType<X>, <x>FromEvent)`.

`createAndRespond`/`listAndRespond` (generic helpers in `handlers.go`) own
the actual `Store.CreateEvent`/`ListEvents` call, error logging, and JSON
response — per-type handlers never call the store directly.

**To add a new event type on the backend:** create the new handler file
following the 7 steps above, register its two routes in
`cmd/server/main.go`, and add a migration only if the event needs no schema
changes beyond `attributes` (usually it doesn't — JSONB absorbs new fields
without a migration).

## Frontend wiring

`frontend/internal/backendclient` has no per-event-type methods — just
generic `ListEvents(ctx, resource string, rangeKey string, out any)` and
`CreateEvent(ctx, resource string, payload map[string]any)` against
`/api/v1/babies/current/<resource>`. Reads go through the combined
`ListEvents(ctx, "events", rangeKey, &events)` (backend-api's `/events`
endpoint, already merged, range-filtered, and sorted newest-first across
every event type); writes still go through `CreateEvent(ctx, "<resource>", payload)` per type
(`"nappies"`, `"feeds"`, `"baths"`, `"observations"`). The only shape
`backendclient.go` decodes is the generic `Event` struct (`EventType` plus
an `Attributes map[string]any`) — no per-event-type typed view structs.

The UI is a single merged, chronological timeline (not one list per event
type) fed by a single "Add Event" dialog (not one form per event type).
`frontend/internal/handlers/handlers.go`:

* Every event type is flattened into one presentation shape,
  `TimelineEvent` (`CSSClass`, `Icon`, `TypeLabel`, `Kind`, `Detail`,
  `Time`). `Kind` is the per-type discriminator (nappy's kind, feed/bath's
  type, observation's category), rendered as "(Kind)" next to `TypeLabel`.
  A `<x>TimelineEvent(ev, loc, now)` function builds one from a generic
  `backendclient.Event`, reading its `Attributes` map — this is where
  per-type display text (e.g. feed's "70 ml · 10 min") is decided.
  `timelineEvent(ev, loc, now)` dispatches to the right builder by
  `ev.EventType`, skipping (and logging) any type the frontend doesn't
  recognize.
* `loadTimeline(ctx, loc, rangeKey)` makes one
  `ListEvents(ctx, "events", rangeKey, ...)` call and converts each item to
  a `TimelineEvent` — no client-side merging or sorting; the backend already
  returns one merged, ordered list for the selected range.
* `Index` calls `loadTimeline` and renders the full page.
* Each `Create<X>` handler parses the HTML form, builds a `map[string]any`
  payload (plus `occurred_at` via `parseEventTime`), calls
  `Backend.CreateEvent(ctx, "<resource>", payload)`, then calls the shared
  `renderTimeline` (itself `loadTimeline` + render), so every form's htmx
  response is the same re-sorted, all-types timeline partial
  (`templates/timeline.html`) swapped into `#timeline` — never a per-type
  partial. The selected range is carried in each form/delete request so HTMX
  refreshes preserve the parent's current view.

On the client, `frontend/static/app.js` drives the "Add Event" dialog: a
type-picker step (four buttons, one per event type) followed by a
form-fields step showing only the chosen type's `<form class="event-form"
data-type="...">` block from `templates/index.html`. Each form still posts
straight to its own existing endpoint (`/nappies`, `/feeds`, ...); the
dialog only changes what's *shown*, not the request shape.

**To add a new event type on the frontend:** add a `<x>TimelineEvent`
builder in `handlers.go` (reading from `ev.Attributes`) and a case for it
in `timelineEvent`'s switch; add a `Create<X>` handler ending in
`h.renderTimeline(...)`; add a `<x>` type-choice button and its `.event-form
data-type="<x>"` block in `index.html`; add a `<x>: "Log a <x>"` entry to
`typeLabels` in `app.js`; wire the create/list routes for the new resource
in `cmd/server/main.go` (`/events` already returns every type, no change
needed there); and give the new card colour a light/dark pair in
`style.css`.

---

# Authentication

OAuth 2.1 Authorization Code Flow with PKCE.

Primary authentication methods:

* Magic Link
* ChatGPT OAuth

Future:

* Google Sign-In
* Apple Sign-In

---

# API Guidelines

* REST first
* JSON payloads
* Versioned endpoints
* Idempotent where appropriate
* Proper HTTP status codes

Avoid introducing gRPC until there is a demonstrated need.

---

# Go Service Conventions

Each Go service (Backend API, Auth Service, MCP Server) that talks to PostgreSQL, or another service over HTTP, should use a standard repository pattern:

* A `store` (or `<thing>client`) package owns the connection/HTTP client, migrations if applicable, and all query/request methods, exporting concrete types only.
* The consuming package (typically `handlers`) defines the interface it needs, sized to only the methods it actually calls, not in the producer package.
* Handlers depend only on that interface, never on the database driver or `net/http` directly.
* Prefer small, focused interfaces per domain over one large interface as the number of methods grows.

Interfaces belong at the consumer, not the producer — this keeps them minimal and testable with fakes, and keeps SQL/driver/HTTP details out of the handler layer.

---

# Code Style

Follow idiomatic Go, not just working Go:

* Run `gofmt`/`goimports` on everything; no unformatted code.
* Handle errors where they occur; wrap with `fmt.Errorf("...: %w", err)` to preserve context instead of discarding or logging-and-continuing.
* Don't introduce an interface, abstraction layer, or config option until there's a real second case that needs it — avoid designing for hypothetical futures.
* Accept interfaces, return concrete structs.
* Keep package names short and lowercase with no stutter (`store.New`, not `store.NewStore`).
* Use `context.Context` as the first parameter for functions that do I/O, and thread it through rather than storing it on a struct.
* Keep functions small and single-purpose; extract a helper only once logic is actually duplicated, not in anticipation of it.

---

# Design System

Brand personality, color palette (primary/secondary/accent, neutrals,
typography, borders, event and semantic colors), component styling, and
visual design principles live in [docs/design-system.md](docs/design-system.md).

Any UI work (frontend templates, CSS, new components) should follow that
palette rather than introducing new colors ad hoc.

---

# Frontend Philosophy

The frontend is intentionally lightweight.

Avoid unnecessary JavaScript frameworks.

Prefer:

* server rendering
* HTMX
* progressive enhancement

---

# Timeline Philosophy

The timeline is the heart of Yauli.

Everything should be optimized around helping parents answer one question quickly:

"What happened today?"

Users should understand their baby's day within 5 seconds.

The main app view should default to today's timeline. Quick range controls
for Yesterday, 24h, and 3 days should sit beside the timeline itself rather
than behind settings or a secondary page.

Every event should be immediately distinguishable using:

* icon
* color
* title
* timestamp

Avoid requiring users to open event details for common information.

The timeline should feel effortless to scan, even after months of recorded history.

---

# MCP Philosophy

Every user action should be possible through MCP.

Examples:

* log feed
* retrieve today's summary
* ask for the last sleep
* retrieve trends

The MCP experience should be considered the primary product.

---

# Deployment

Railway

Services:

* frontend
* backend-api
* auth-service
* mcp-server

Shared:

* PostgreSQL

Each service has its own Dockerfile and deployment pipeline.

Network exposure:

* frontend and mcp-server are public
* backend-api and auth-service are private (internal-only, reachable by other services but not exposed externally)

---

# Engineering Principles

* Keep services focused.
* Keep business logic inside Backend API.
* Prefer simplicity over cleverness.
* Design APIs before UI.
* Favor maintainability over premature optimization.
* Build for public use from day one.
* Prioritize reliability and correctness over feature count.
* Don't over-engineer for scale or theoretical edge cases. The app has a
  small user base — skip near-zero-probability race conditions, complex
  transaction schemes, and defensive code for attack vectors that require
  precise timing or adversarial clients. Fix things users actually hit.
