# Current User API

## Context

The frontend needs to show which account is signed in. Auth-service owns
sessions and token minting, but user profile data such as email already lives
in backend-api's users table and is keyed by the authenticated user id in the
JWT.

## Decision

Add `GET /api/v1/users/me` and `PATCH /api/v1/users/me` to backend-api. The
routes are protected by the same JWT middleware as the baby routes. The read
returns the authenticated user's id, email, and optional display name; the
write updates optional account profile fields owned by that user.

## Alternatives Considered

The frontend could decode the JWT, but the token does not contain email and
frontend code should not grow identity logic.

Auth-service could include email in session/token responses, but that would
duplicate user-profile reads across service boundaries.

## Consequences

The frontend can render account UI consistently without widening timeline
member permissions or teaching auth-service baby-domain concerns. API clients
now have a small authenticated identity endpoint to consume when they need the
current user's display email or nickname.
