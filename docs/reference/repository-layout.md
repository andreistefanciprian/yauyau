# Repository Layout

```text
frontend/       Server-rendered web UI, HTMX handlers, templates, static assets
backend-api/    Baby and family domain logic, validation, events, reports
auth-service/   OAuth, PKCE, magic links, tokens, sessions
mcp-server/     MCP tools that call backend-api
docs/           Architecture, design system, decisions, operational notes
.github/        CI and repository automation
```

When adding a new top-level directory or changing a service boundary,
update this map.
