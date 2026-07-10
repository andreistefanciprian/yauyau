# Baby Management

Status: **living design doc**.

This document captures how Yauli manages the baby profile and the lifecycle
of a baby timeline.

## Product model

The baby timeline is the main workspace. Parents should not have to think in
terms of backend families or tenants when managing it.

Owner-facing actions live in **Baby settings**, entered from the baby header
as `Baby`.

## Current behavior

Owners can:

* rename the current baby
* update the current baby's timezone
* delete the current baby timeline after typing the exact baby name

Deleting is implemented as **archive**, not hard delete.

## Archive, Not Hard Delete

`DELETE /api/v1/babies/current` sets `babies.archived_at`.

Archived babies:

* no longer appear as the current baby
* no longer return from direct active baby lookups
* no longer have their events returned through `/api/v1/babies/current/...`
* keep their rows and events in Postgres for possible future recovery or audit
* remove the archiving owner's `family_members` row and revoke their sessions
  for that family, so an accidental starter baby no longer blocks accepting
  an invite to another baby's timeline

This is deliberate. A baby timeline contains family history; hard-deleting it
as the first implementation would be too easy to regret.

## Owner Rules

Only an active owner of the current baby's family can update or archive the
baby.

Archive requires typing the baby name exactly. The frontend checks this before
calling backend-api, and backend-api checks it again before setting
`archived_at`.

## Joining Another Baby's Timeline

Yauli does not support multi-timeline switching yet. For now, a user who
created a starter baby but needs to join another baby's timeline should delete
their starter timeline first, then use the invitation link.

After archive, the frontend clears the session and sends the user back to
sign-in. A normal sign-in with no remaining membership returns them to
onboarding; an invitation link can activate the pending invite because the
old active membership was removed.

Onboarding still allows:

* brand-new sessions with no family
* existing family sessions where backend-api returns 404 for the current baby

A session with an active baby still redirects away from onboarding to `/app`.

## Future Work

Possible follow-ups:

* restore an archived baby
* show archived timelines to owners
* hard-delete/export data after an explicit privacy flow
* support selecting between multiple active babies
* prevent accidental timezone strings with a curated timezone picker
