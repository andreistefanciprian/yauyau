# AGENTS.md

## Project

**YauYau Tracker**

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

Owns the business domain.

---

## auth-service

Responsibilities

* OAuth 2.1
* PKCE
* Magic Links
* access tokens
* refresh tokens
* session management

No baby domain logic.

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

# Frontend Philosophy

The frontend is intentionally lightweight.

Avoid unnecessary JavaScript frameworks.

Prefer:

* server rendering
* HTMX
* progressive enhancement

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
