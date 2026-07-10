# Magic-Link Auth — Remaining PRs

Tracks the PR-by-PR rollout of magic-link auth described in
[`auth-magic-link.md`](auth-magic-link.md). PR1–4 are merged; this file
covers what's left (PR5–14).

## Done

- **PR1** — backend-api: tenancy schema (`users`, `family_members`).
- **PR2** — backend-api: internal API (auth-service-facing).
- **PR3** — backend-api: JWT claims context (decode only, no verification yet).
- **PR4** — backend-api: baby creation, family created implicitly.
- **PR5** — auth-service: new service skeleton.
- **PR6** — auth-service: request + verify magic link.

## Remaining

### PR7 — auth-service: JWT minting + logout/revoke + attach-family
`POST /internal/auth/token` (session → `{access_token, family_id}` —
`family_id` surfaced as plain data so frontend never decodes the JWT
itself; JWT claims are `sub`=user_id + `family_id` if present, 10min TTL,
HMAC-SHA256 via `golang-jwt/jwt/v5`), `POST /internal/auth/logout`
(revoke + audit log), and `POST /internal/auth/session/{id}/attach-family`
(binds a null-family session to a newly created family_id — called once,
right after onboarding's "add your baby" step returns a family_id).

**Verify:** mint from a family-less session → `family_id: null` in the
response, decode the JWT and confirm no `family_id` claim; call
attach-family, mint again → `family_id` now present; mint from a
revoked/expired session → 401.

### PR8 — frontend: login + verify-confirmation pages
New `frontend/internal/authclient/http.go` (mirrors
`frontend/internal/backendclient/http.go`); new
`frontend/internal/handlers/auth.go` (`ShowLogin`, `RequestMagicLink`,
`ShowVerify`, `ConfirmVerify`, `Logout`); new `frontend/templates/login.html`
/ `auth-verify.html`. Sets `yauli_session` cookie (HttpOnly, Secure in
prod, SameSite=Lax). **Verify page must not consume the token on a bare
`GET`** — only an explicit confirm button/POST does (anti-prefetch
hardening). Dashboard stays open in this PR — only the login path is added.

Two additional hardening details land in this PR: the request logger must
not log the raw query string on `/auth/verify` (redact it, don't rely on
default logging middleware), and the confirmation page strips the token
from the visible URL via `history.replaceState` once loaded, so it doesn't
linger in browser history after being read.

**Verify:** manual browser click-through; confirm a bare `GET
/auth/verify?token=...` does not consume the token; confirm the URL bar no
longer shows the token after the page loads; confirm `docker compose logs
frontend` doesn't contain the raw token from this route.

**State explicitly in the PR description:** relies on `SameSite=Lax`, no
separate CSRF token — a documented trade-off, not an oversight.

### PR9 — frontend: session gating + Bearer attachment
Middleware (`frontend/internal/handlers/session.go`) requiring a valid
`yauli_session` cookie on protected routes; mints a fresh access token
from auth-service **on every request** (no caching — frontend stays fully
stateless). Branches on the mint response's plain `family_id` field: null
→ redirect to `/onboarding`, present → dashboard. Extend
`frontend/internal/backendclient/http.go`'s `do` to attach
`Authorization: Bearer <token>`.

**This PR is what makes the currently-frontend-breaking state of
`GetCurrentBaby` (since PR4) actually work end-to-end** — until this PR
lands, the frontend has no way to supply a Bearer token, and every
protected backend-api call 401s. That's the accepted, sequenced gap PR4
left behind.

**Verify:** no cookie → `/login`; fresh signup → `/onboarding`; a
membership with a family → dashboard. Restart the frontend container
mid-session and confirm the next request works with no re-auth (proves
statelessness).

### PR10 — frontend: onboarding UI ("add your baby" — one step)
New `frontend/internal/handlers/onboarding.go` + template: a single "add
your baby" form (no family step shown) → `POST /api/v1/babies` (via
backend-api client, PR4) → backend-api's response carries `family_id` →
frontend calls auth-service's attach-family with it → re-mint token →
redirect to dashboard.

**Verify:** full browser flow for a brand-new email: signup → "add your
baby" (name/birthdate) → land on dashboard showing that baby's (empty)
timeline. Nothing in the UI ever mentions "family."

### PR11 — backend-api: thread family scoping through event routes
Replace remaining reads of `store.FamilyID`/`store.BabyID` inside the
event-handler files (`nappy.go`, `feed.go`, `bath.go`, `sleep.go`,
`observation.go`) and their store queries with the `FamilyID` already
available via PR3's context — no new middleware needed, it's already
mounted. Delete the now-dead `store.FamilyID`/`store.BabyID` package vars
(currently in `backend-api/internal/store/store.go`, commented as
temporary as of PR4).

**Verify:** zero observable behavior change for an authenticated session
with a family — exercise the full existing app once end-to-end. Review is
primarily a diff read.

### PR12 — backend-api: enforce JWT verification (the one breaking PR)
Add real signature + expiry checking to PR3's decode path
(`authctx`), using a new `JWT_SIGNING_SECRET` shared with auth-service —
the only change is "verify, don't just decode." Mount enforcement on
`/api/v1/*` (`/internal/*` keeps PR2's shared-secret check; `/healthz`
stays open).

**This is the one PR that is not independently non-breaking** — after
merge, hand-built/unsigned bearer tokens (used for `curl` testing in
PR3/PR4) stop working; only real auth-service-issued tokens do. Plan to
merge/deploy this together with PR6–PR10 already live.

**Verify:** `curl` with an unsigned or tampered token → 401; full browser
flow (signup → onboarding → dashboard → create an event) → 200s
throughout.

### PR13 — invite someone to help with a baby
Backend-api only — no auth-service changes needed. `POST
/api/v1/babies/{id}/invite {email}` resolves the baby's `family_id`
internally, checks the caller is that family's owner (via PR3/PR12's
verified identity), and calls PR1's invite store method to create the
pending `family_members` row. Frontend: an "invite someone to help with
{baby name}" page/section (owner-only) — copy never says "family."
Confirms PR6's already-built behavior: an invitee who logs in lands
directly in the family, skipping onboarding entirely.

**Verify:** owner invites a second email via the baby-scoped invite; that
email signs up via its own magic link and lands directly on the shared
dashboard (no "add your baby" prompt, since a membership already exists);
a non-owner attempting to invite → 403.

**State explicitly in the PR description:** same `SameSite=Lax`-only CSRF
posture as PR8.

### PR14 — auth-service: Mailgun integration for production email
Replace the stdout-only email step from PR6 with a real Mailgun API call
in production (local dev keeps logging to stdout). New
`auth-service/internal/mailer/mailgun.go`, `MAILGUN_API_KEY` (+ domain)
env, a minimal magic-link email template. Updates `docs/auth-magic-link.md`'s
wording (currently says the provider is undecided) to name Mailgun.

**Verify:** send a real test email via Mailgun's sandbox/test domain, confirm
the received link round-trips through the normal verify flow.

## Sequencing notes

- Nullable `sessions.family_id` and the `attach-family` step (PR7) are the
  direct consequence of dropping any seeding/backfill approach — onboarding
  is a real state a session can be in, not glossed over.
- Family creation has no dedicated PR or UI step — it's folded into PR4's
  baby-creation endpoint, so onboarding (PR10) is one form, not two, and
  "family" never appears as a concept the user interacts with.
- An invited member's very first session already has a family_id (PR6
  resolves-and-activates in the same verify call) — they never see the
  onboarding UI at all. (PR4 also activates a pending invite defensively if
  `CreateBaby` is ever called before that happens.)
- Audit logging isn't a separate PR — folded into PR6 (login) and PR7
  (logout/revoke).
- PR14 (Mailgun) is last since nothing else depends on it and it doesn't
  affect local dev.
- PR4 deliberately left the frontend broken for `GetCurrentBaby` (no Bearer
  token attached yet) and event routes still hardcoded to the seed
  family/baby — both are accepted, sequenced gaps closed by PR9 and PR11
  respectively, not oversights.

## Overall verification (after PR12)

Full loop: `docker compose up` → frontend → `/login` → enter email → copy
magic link from `docker compose logs auth-service` → confirm on the verify
page → "add your baby" → dashboard → create a nappy/feed event → see it in
the timeline → log out → confirm dashboard now redirects to `/login`.
Separately: invite a second email to help with the baby, confirm their
first login skips the "add your baby" step entirely and lands them
straight on the shared dashboard.
