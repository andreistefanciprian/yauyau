<p align="center">
  <img src="logo_368_142px.png" alt="Yauli logo" width="368" height="142">
</p>

<h3 align="center">Your parenting companion, from day one.</h3>

<p align="center">
  <a href="#license"><img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License: MIT"></a>
  <img src="https://img.shields.io/badge/status-early%20development-orange.svg" alt="Status: early development">
  <img src="https://img.shields.io/badge/built%20with-Go-00ADD8.svg" alt="Built with Go">
</p>

---

**Yauli** is an AI-first parenting companion designed to help families effortlessly record, organize and understand their baby's daily life.

Instead of filling out endless forms, parents can simply talk naturally to ChatGPT or use the web application to record feeds, nappies, sleep, baths, observations and other important moments. Yauli builds a beautiful timeline of your child's journey while helping parents stay present and focus on what matters most.

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
* 💩 Nappy tracking
* 😴 Sleep tracking
* 🛁 Bath tracking
* 📝 Observations
* 📅 Chronological timeline

**Planned**

* ⚖️ Weight tracking
* 🌡 Temperature
* 💊 Medication
* 💉 Vaccinations
* ⭐ Milestones
* 📊 Daily and weekly summaries
* 📄 Pediatrician reports
* 👨‍👩‍👧 Family sharing
* 🤖 ChatGPT integration via MCP
* 🔐 OAuth 2.1 + PKCE authentication

---

## Architecture

Yauli is built as a collection of small Go services. `frontend` and `backend-api` exist today; `auth-service` and `mcp-server` are on the roadmap.

```text
                ChatGPT
                   │
             MCP Server           (planned)
                   │
            Backend API
           /           \
Frontend            Auth Service   (planned)
           \           /
             PostgreSQL
```

**Services**

| Service | Status | Description |
|---|---|---|
| [`backend-api`](backend-api) | ✅ Active | Owns all business logic, validation, event creation and querying. Single source of truth. |
| [`frontend`](frontend) | ✅ Active | Server-rendered dashboard and timeline. A thin client over the backend API. |
| `auth-service` | 🚧 Planned | OAuth 2.1 + PKCE and magic-link authentication. |
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
* Alpine.js
* Tailwind CSS

**Authentication** *(planned)*

* OAuth 2.1
* PKCE
* Magic Links

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

This starts PostgreSQL, `backend-api` and `frontend`.

* Frontend: [http://localhost:8080](http://localhost:8080)
* Backend API: [http://localhost:8081](http://localhost:8081)

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
