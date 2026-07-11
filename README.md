<p align="center">
  <img src="frontend/static/logo.png" alt="Yauli logo" width="368" height="142">
</p>

<h3 align="center">Your parenting companion, from day one.</h3>

<p align="center">
  <a href="#license"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/status-early%20development-orange.svg" alt="Status: early development">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8.svg" alt="Built with Go">
</p>

---

**Yauli** is an AI-first parenting companion designed to help families effortlessly record, organize and understand their baby's daily life.

Instead of filling out endless forms, parents can use the web application today to record feeds, pumping, nappies, sleep, baths, observations and other important moments. Yauli builds a calm timeline of your child's day while helping parents stay present and focus on what matters most.

---

## Why Yauli?

Parenting is busy.

Trying to remember:

* When was the last feed?
* How many wet nappies today?
* How long has the baby been sleeping?
* What did the pediatrician recommend?
* When was the last bath?

shouldn't require searching through messages, notebooks or complicated apps.

Yauli remembers everything for you.

---

## Vision

Build the world's best AI-powered parenting companion.

ChatGPT is the primary interface.

The website is a beautiful dashboard and timeline for reviewing your baby's history.

---

## Features

**Current**

* 🍼 Feed tracking
* 🤱 Pump tracking
* 💩 Nappy tracking
* 😴 Sleep tracking
* 🛁 Bath tracking
* 📝 Observations
* 📅 Timeline ranges for today, yesterday, the last 24 hours and the last 3 days
* 🔐 Magic-link sign in with durable sessions and short-lived backend API JWTs
* 👥 Timeline access management, invites and relationship labels
* 👶 Baby profile settings, including birth date, birth weight, birth length and sex
* 🗑 Owner-controlled timeline archive/delete flow

**Planned**

* ⚖️ Weight tracking
* 🌡 Temperature
* 💊 Medication
* 💉 Vaccinations
* ⭐ Milestones
* 📊 Daily and weekly summaries
* 📄 Pediatrician reports
* 🤖 ChatGPT integration via MCP
* 🔐 OAuth 2.1 + PKCE for MCP/ChatGPT

---

## Architecture

Yauli is built as a collection of small Go services. The browser talks to the server-rendered `frontend`; `frontend` talks privately to `auth-service` for sessions and to `backend-api` for baby/timeline data. `backend-api` is the source of truth for business rules and can ask `auth-service` to revoke sessions when timeline access changes.

```text
Browser
  │
Frontend
  ├── Auth Service
  └── Backend API
        └── PostgreSQL

ChatGPT
  │
MCP Server (planned)
  └── Backend API
```

**Services**

| Service | Status | Description |
|---|---|---|
| [`backend-api`](backend-api) | ✅ Active | Owns business rules, users, baby profiles, timeline access, event creation and querying. |
| [`frontend`](frontend) | ✅ Active | Server-rendered app for sign in, onboarding, timeline review, event entry and settings. |
| [`auth-service`](auth-service) | ✅ Active | Magic links, sessions, JWT minting, logout, invite links and session revocation. |
| `mcp-server` | 🚧 Planned | Exposes Yauli as MCP tools so ChatGPT can record and query events directly. |

---

## Technology Stack

**Backend**

* Go
* Chi
* PostgreSQL

**Frontend**

* Go Templates
* HTMX
* Plain CSS

**Authentication**

* Magic Links
* Session cookies
* JWT access tokens for backend-api
* Mailgun for production email delivery

**Planned Authentication**

* OAuth 2.1
* PKCE

**Infrastructure**

* Docker
* Railway
* PostgreSQL

---

## Getting Started

### Prerequisites

* [Docker](https://www.docker.com/) and Docker Compose

### Run locally

```bash
git clone https://github.com/andreistefanciprian/yauli.git
cd yauli
cp .env.example .env
docker compose up --build
```

This starts PostgreSQL, `backend-api`, `auth-service` and `frontend`.

* Frontend: [http://localhost:8080](http://localhost:8080)
* Backend API: [http://localhost:8081](http://localhost:8081)
* Auth Service: [http://localhost:8082](http://localhost:8082)

In local development, magic links are logged to `auth-service` stdout instead of being emailed:

```bash
docker compose logs auth-service
```

To rebuild only the frontend after template or CSS changes:

```bash
docker compose up --build frontend
```

---

## Project Principles

* AI-first
* Conversation-first
* API-first
* Mobile-first
* Event-driven architecture
* Simple, maintainable Go services
* PostgreSQL from day one
* Build small, iterate quickly

---

## Example

Instead of opening an app and navigating multiple screens, simply tell ChatGPT:

> "Yauli, YauYau just had a mustard-yellow poo."

or

> "Log a 70 ml bottle feed."

or

> "I pumped 90 ml."

or

> "When was her last feed?"

Yauli records the event and keeps your family's timeline up to date.

---

## Project Status

🚧 **Early development**

The project is currently focused on building a solid event-driven foundation before expanding into AI-powered insights and richer parenting features.

---

## Contributing

Yauli is early and evolving quickly. Issues and pull requests are welcome — if you're planning something substantial, please open an issue first to discuss the approach.

---

## License

[MIT](LICENSE)
