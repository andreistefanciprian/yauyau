# Magic-Link Auth — Remaining PRs

Tracks the PR-by-PR rollout of magic-link auth described in
[`auth-magic-link.md`](auth-magic-link.md). PR1–11 are merged; this file
covers what's left (PR12).

Renumbered from an earlier 14-PR breakdown: former PR7+PR8 merged into PR7,
and former PR9+PR10 merged into PR8 (see Sequencing notes below for why).
The former PR11–14 shift down to PR9–12 unchanged in content.

## Done

- **PR1** — backend-api: tenancy schema (`users`, `family_members`).
- **PR2** — backend-api: internal API (auth-service-facing).
- **PR3** — backend-api: JWT claims context (decode only, no verification yet).
- **PR4** — backend-api: baby creation, family created implicitly.
- **PR5** — auth-service: new service skeleton.
- **PR6** — auth-service: request + verify magic link.
- **PR7** — auth-service: JWT minting/logout/attach-family + frontend login pages.
- **PR8** — frontend: session gating + Bearer attachment + onboarding UI.
- **PR9** — backend-api: thread family scoping through event routes.
- **PR10** — backend-api: enforce JWT verification.
- **PR11** — invite someone to help with a baby.

## Remaining

### PR12 — auth-service: Mailgun integration for production email
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
  baby-creation endpoint, so onboarding (PR8) is one form, not two, and
  "family" never appears as a concept the user interacts with.
- An invited member's very first session already has a family_id (PR6
  resolves-and-activates in the same verify call) — they never see the
  onboarding UI at all. (PR4 also activates a pending invite defensively if
  `CreateBaby` is ever called before that happens.)
- Audit logging isn't a separate PR — folded into PR6 (login) and PR7
  (logout/revoke).
- PR12 (Mailgun) is last since nothing else depends on it and it keeps local
  dev on stdout logging.
- PR4 deliberately left the frontend broken for `GetCurrentBaby` (no Bearer
  token attached yet) and event routes still hardcoded to the seed
  family/baby — both are accepted, sequenced gaps closed by PR8 and PR9
  respectively, not oversights.
- PR7 bundles an auth-service half with the frontend half that consumes it
  in the same PR, rather than splitting along the service boundary the way
  PR1–PR6 did — the frontend half is unusable/untestable without its
  counterpart landing at the same time. PR8 is frontend-only but bundles
  two interdependent frontend features (session gating, onboarding) for
  the same reason: gating is the only thing that routes a session into the
  onboarding form, so shipping either alone leaves the other unreachable.
- PR8's onboarding form only collects a baby's name — `babies` has no
  birthdate column, and adding one wasn't in scope for a frontend-only PR.
  `CreateBaby`/`AttachFamily` are two independent calls with no
  compensation between them if the second fails after the first succeeds;
  documented as an accepted gap in `onboarding.go` rather than solved with
  an idempotency key, since a retry reuses the already-created family and
  just adds a duplicate baby row, not an orphaned one.

## Overall verification (after PR11)

Full loop (testable once PR10 lands): `docker compose up` → frontend →
`/login` → enter email → copy magic link from `docker compose logs
auth-service` → confirm on the verify page → "add your baby" → dashboard →
create a nappy/feed event → see it in the timeline → log out → confirm
dashboard now redirects to `/login`.

Separately (requires PR11): invite a second email to help with the baby,
confirm their first login skips the "add your baby" step entirely and
lands them straight on the shared dashboard.
