# Data Model

## Database

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

## Event Model

Events are append-only records.

Examples:

* Feed
* Nappy
* Sleep
* Pump
* Note
* Growth measurement
* Temperature
* Medication
* Bath
* Vaccination

The model should be extensible without frequent schema changes.

Use PostgreSQL JSONB for event-specific attributes where appropriate.

For how this maps onto current backend-api routes, the generic event
store, and the per-event-type handler pattern, see
[docs/reference/api-routes.md](reference/api-routes.md).
