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


# How Agents Should Work

This file is both project documentation and an operating contract for coding
agents working in the repository.

Producing more code is not the goal. Producing the smallest safe change that
fits the existing system is the goal.

When principles conflict, resolve the trade-off in this order:

1. Correctness
2. Security
3. Simplicity
4. Maintainability
5. Readability
6. Performance
7. Extensibility

Performance and extensibility sit last on purpose: do not change code solely
to reduce allocations, add caching, introduce concurrency, or generalize an
API without measured evidence it's needed. Measure first, optimize second.

## Before Changing Code

Before editing:

1. Read this file.
2. Inspect the affected service, package, tests, routes, and nearby code.
3. Read any relevant documents under `docs/`.
4. Confirm where the business logic belongs.
5. Write a short implementation plan for any change that touches more than
   one file or crosses a service boundary.
6. Identify risks before implementation — see Risk-Based Review below for
   the areas that need the closest scrutiny.

Do not invent routes, fields, database columns, configuration, abstractions,
or conventions without first checking the existing implementation.

If the task is ambiguous but can be completed safely, choose the smallest
reasonable interpretation and state the assumption in the final summary.

## Implementation Rules

While implementing:

* Prefer modifying an existing path over creating a parallel abstraction.
* Keep changes narrowly scoped to the task.
* Follow surrounding code before introducing a new pattern.
* Prefer explicit, boring code over clever code.
* Do not add interfaces, abstraction layers, or configuration for
  hypothetical needs (see Code Style).
* Do not refactor unrelated code unless the task cannot be completed safely
  without it.
* Do not rename identifiers for style alone.
* Once the task is complete, stop — do not add optional improvements,
  modernize surrounding code, or keep polishing unless asked.
* Preserve backward compatibility unless the task explicitly changes a
  contract.
* Treat generated migrations, auth changes, session changes, and destructive
  operations as high-risk.

## Self-Review

Before declaring a task complete, review the diff and ask:

* Could this be simpler?
* Did I duplicate logic that already exists?
* Are errors handled with useful context?
* Did I touch unrelated files?
* Would another engineer understand the change without asking why it exists?
* Re-check the boundaries and risk areas above (business logic ownership,
  abstractions, auth/authorization, API and contract changes) for anything
  this diff crosses.

## Definition of Done

A change is complete only when all applicable items are satisfied:

* The code builds.
* `gofmt` and `goimports` have been run on changed Go files (see Code
  Style).
* Relevant tests pass.
* New or changed behavior has tests where practical.
* Static checks configured by the repository pass.
* API, route, schema, configuration, and behavior changes are documented.
* `AGENTS.md` remains accurate.
* No unexplained TODOs, dead code, temporary debugging, or commented-out code
  were introduced.
* No secrets, tokens, personal data, or production credentials appear in the
  diff or logs.
* The final response includes:
  * what changed;
  * why it changed;
  * tests and checks run;
  * risks, assumptions, or follow-up work.

If a check cannot be run, say so explicitly. Never imply that an unrun check
passed.

## Commit Attribution

Keep the human operator as the primary Git author. Use co-author trailers to
make AI assistance visible in GitHub history.

When an AI assistant creates a commit, include that assistant's
`Co-authored-by` trailer in the commit message. For Codex, use:

```text
Co-authored-by: Codex <codex@openai.com>
```

If more than one assistant contributed materially to the same commit, include
one `Co-authored-by` trailer per assistant.

## Risk-Based Review

Not every line deserves equal review effort.

Review these areas especially carefully:

* authentication and session handling;
* authorization and family membership;
* database migrations and destructive queries;
* baby and family data isolation;
* event update and delete paths;
* timezones and calendar-day boundaries;
* token, cookie, and magic-link handling;
* externally visible API contracts;
* Railway networking and service exposure;
* anything that could lose, expose, or misattribute family data.

Low-risk presentation changes may rely more heavily on tests and focused
diff review. High-risk changes require direct inspection of the relevant
code paths and failure modes.

---

# Architectural Decisions

Record architectural decisions as ADRs under `docs/decisions/` — see
[docs/decisions/README.md](docs/decisions/README.md) for the format. Write
one when changing: service ownership boundaries, auth/session architecture,
event storage strategy, family membership rules, public API contracts,
deployment topology, or major dependencies/frameworks. Do not reverse an
existing decision accidentally — read the relevant record first.

---

# Future AI Behaviour and Evals

Yauli does not currently rely on AI-generated user-facing behavior, so there
is no eval suite yet.

When AI-generated reports, natural-language interpretation, or autonomous MCP
workflows are introduced, add an `evals/` directory and document:

* the behavior being evaluated;
* representative input cases;
* expected structured output;
* deterministic checks;
* rubric-based checks where exact matching is unsuitable;
* safety constraints;
* the command used to run the eval suite.

Likely first eval targets:

* natural language to MCP tool selection;
* extraction of feed, nappy, sleep, pump, and note attributes;
* daily report generation;
* refusal to invent missing facts;
* valid schema-constrained output;
* no diagnosis or medical advice;
* correct use of the baby's timezone;
* regression checks when prompts, models, or tools change.

Do not add an eval framework before AI behavior exists. Start with a small,
version-controlled set of representative cases when the first AI feature is
implemented.

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

Current top-level directory layout lives in
[docs/reference/repository-layout.md](docs/reference/repository-layout.md).
Core entities and the event model are documented in
[docs/data-model.md](docs/data-model.md). Current routes, handlers, and the
pattern for adding a new event type live in
[docs/reference/api-routes.md](docs/reference/api-routes.md).

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
does not read or decide family membership. Authentication methods are
documented in [docs/authentication.md](docs/authentication.md).

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

# Testing Strategy

Tests should protect behavior and boundaries, not mirror implementation
details.

Prefer:

* table-driven unit tests for validation and mapping logic;
* handler tests for status codes, payloads, and authorization behavior;
* store integration tests for SQL and transaction behavior;
* contract-style tests for service clients;
* focused end-to-end tests for critical user journeys.

Critical journeys include:

* sign in by magic link;
* OAuth authorization with PKCE;
* create or join a family;
* create, update, list, and delete an event;
* family member invitation and removal;
* daily timeline and report generation;
* session revocation after membership changes.

A bug fix should include a regression test whenever practical.

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

The main app view should default to today's timeline. A quick range nav for
the rolling last 7 days (Today, Yesterday, then weekday names) should sit
beside the timeline itself rather than behind settings or a secondary page.

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
* Prefer small pull requests and focused commits that are easy to review and
  revert.
* Generated code is not trusted merely because it compiles. Validate behavior,
  boundaries, and failure modes (see Definition of Done).
