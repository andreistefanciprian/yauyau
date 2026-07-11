# Timeline Access and Permissions

Status: **living design doc**. This document captures the intended direction
for managing who can see and contribute to a baby's timeline. Keep it updated
as access management, family membership, relationships, and permissions grow
as one coherent part of the platform rather than as one-off screens.

## Product language

Users should mostly see **baby timeline**, not **family**.

`family` remains a backend tenancy boundary: the thing that owns babies,
memberships, and events. In the product, the central object is the baby's
timeline: "who can see Ada's timeline", "invite someone to Ada's timeline",
"manage timeline access".

This keeps the UI focused on what parents care about while preserving the
existing backend model.

## Concepts

### User

A person who can authenticate by email. Users live in `users` and are shared
across all future access models.

### Family

A backend-owned tenant. Today a family is created implicitly when a user adds
their first baby. It is not currently named or shown to users.

Long term, "family" may become more visible if Yauli supports multiple babies,
multiple households, or switching between groups, but that is not needed for
the next access-management slice.

### Baby timeline

The user-facing access surface. A baby timeline contains:

* the baby profile
* timeline events
* the people who can access it
* relationship labels for those people
* later, permission levels for those people

Today each baby belongs to one family and event routes resolve the current
baby from the session's `family_id`. The main app defaults to the baby's
current calendar day and lets parents quickly switch to any of the 6 days
before it (Yesterday, then each day by weekday name) without leaving the
timeline.

### Membership

A row in `family_members`. This answers "does this user have access to this
timeline's underlying family?"

Existing technical fields:

* `role`: authorization role, currently `owner` or `member`
* `status`: lifecycle state, currently `invited` or `active`

Proposed user/profile fields:

* `relationship`: how this person relates to the baby, e.g. `Mum`, `Dad`,
  `Grandpa`, `Auntie`, `Carer`, `Night nanny`
* later, possibly `display_name`: how this person should be shown in the UI

Relationship is not a permission. "Grandpa" should not imply read-only, and
"Mum" should not be the same thing as owner. Keep relationship labels separate
from authorization roles.

## Current behavior

Owners open People from the baby timeline to manage who has access. The
backing route is `/settings/timeline`, but the UI should present this as
part of the baby workspace rather than a separate settings area. backend-api
creates a pending `family_members` row, and auth-service sends an invite
magic link. When the invitee verifies the link, backend-api activates the
pending membership.

Current behavior:

* The timeline defaults to Today, with quick access to the 6 days before it
  (Yesterday, then each day by weekday name).
* Owners can open People from the baby header to see who has access.
* Owners can invite people to the timeline from the People view.
* Owners can set optional relationship labels such as Mum, Dad, Grandpa, or
  Carer.
* Owners can cancel pending invites.
* Owners can remove active non-owner members. backend-api revokes the
  member's auth-service sessions for the family before deleting the
  membership row.

Current limitations:

* `owner` vs `member` is the only authorization split.
* All active members can use the timeline in the same way.
* Multi-timeline switching is intentionally deferred. If someone already
  created a baby in Yauli and needs to join another baby's timeline, they
  should delete/archive their current baby first and then use the invite
  link. The timeline shows a notice when backend-api reports that the current
  user has a pending invite outside their active baby family.
* Owners cannot yet transfer ownership or remove another owner.

## Design decisions

| Question | Decision |
|---|---|
| User-facing name | Use "timeline access" rather than "family members". |
| Where membership lives | Continue using `family_members` for now. |
| Relationship storage | Add nullable `relationship TEXT` to `family_members`. |
| Relationship validation | Store free text, but offer presets in the UI. |
| Relationship vs permissions | Keep them separate. Relationship is identity/context, not authority. |
| First access UI | Keep `/settings/timeline` as the authenticated route, but expose it from the baby header as `People`. |
| First permission model | Keep existing `owner` / `member` behavior; defer granular permissions. |
| Removing access | Owner-only; do not allow removing the last/only owner. |

## Relationship labels

The UI can offer common presets:

* Mum
* Dad
* Parent
* Grandma
* Grandpa
* Auntie
* Uncle
* Carer
* Nanny
* Other

The stored value should be plain text rather than an enum. Families use
different names, languages, and relationship structures. Presets help the
common path, but the model should not force everyone into a fixed list.

Relationship labels should be optional. An invited user can start with no
relationship set, and the owner can fill it in later.

## Roles and permissions

### Authorization role

`role` should remain a small, technical set.

Initial roles:

* `owner`: can manage access, invite people, remove people, and later manage
  baby settings
* `member`: can access the timeline

Near-term behavior:

* owner can invite
* owner can list members
* owner can update relationship labels
* owner can remove members
* member can view and add timeline events, matching today's behavior

### Future permission levels

Do not add these in the first settings PR unless product need forces it. They
are likely, but they need careful UX because parenting care is collaborative.

Possible future permissions:

* `view_timeline`
* `add_events`
* `edit_own_events`
* `edit_any_events`
* `manage_members`
* `manage_baby_profile`

When this becomes necessary, prefer a capability-based model over multiplying
roles like `grandparent_viewer` or `nanny_editor`. Relationships and
permissions should remain independent.

## Implemented slice: timeline access settings

The first implementation is intentionally narrow and useful.

### Backend schema

The schema adds:

```sql
ALTER TABLE family_members
  ADD COLUMN IF NOT EXISTS relationship TEXT;
```

`display_name` is intentionally not present yet. Email is enough to identify
people for the first settings page, and relationship is the higher-value
addition.

### Backend API

User-facing endpoints under the current baby route:

* `GET /api/v1/babies/current/members`
* `PATCH /api/v1/babies/current/members/{user_id}`
* `DELETE /api/v1/babies/current/members/{user_id}`

These routes should:

* require a valid JWT
* resolve the caller's current family from `family_id`
* require active owner membership for management actions
* list only members for the caller's current family
* prevent deleting the caller's own owner membership
* prevent deleting any owner membership for now
* revoke auth-service sessions before deleting an active non-owner member

Suggested response shape:

```json
{
  "members": [
    {
      "user_id": "uuid",
      "email": "parent@example.com",
      "role": "owner",
      "status": "active",
      "relationship": "Mum"
    }
  ]
}
```

Patch request:

```json
{
  "relationship": "Grandpa"
}
```

### Frontend

Frontend route: `/settings/timeline`, entered from the baby header as
`People`.

The page should show:

* baby name
* list of active and invited people
* email
* status
* role
* relationship
* controls for owner-only relationship editing
* controls for owner-only remove access
* invite form

The dashboard should link to this page with copy like `Timeline access`.

Recommendation: move the existing invite form from the dashboard into the
settings page in this PR, so all access management lives in one place. If that
makes the first PR too large, duplicate it briefly and remove the dashboard
form in a follow-up.

## Safety rules

Access-management code should protect these invariants:

* A non-owner cannot list/manage other members.
* A non-owner cannot invite, remove, or edit relationships.
* An owner cannot remove themselves.
* Owner removal/transfer is not exposed yet.
* Removing an invited user should be allowed; it cancels the invite.
* Removing an active member must revoke their still-valid auth-service
  sessions for that family before deleting the membership row. Existing
  short-lived access tokens may remain valid until expiry.
* Auth-service should not make membership decisions. It should continue to ask
  backend-api about family membership.

## Open questions

* Should a member be allowed to set their own relationship label, or only the
  owner?
* Should relationship be attached to the user-family membership or to a future
  user-baby membership? Current model says family membership, but multiple
  babies may make this more nuanced later.
* Should invited people appear in the list immediately? Recommendation: yes,
  with status `Invited`.
* Should removed users keep historical event attribution later? Today events
  do not attribute user IDs, so this is not yet a concern.
* What should happen when there are multiple babies in a family? Current
  behavior treats family membership as access to the family's current baby
  timeline. A future multi-baby model may need per-baby access.

## Long-term direction

The likely mature model is:

* families or households remain tenancy boundaries
* babies have timelines
* users have memberships
* memberships have relationship labels
* permissions are explicit capabilities
* relationships remain flexible, human labels

This lets Yauli support real-life care networks without confusing identity
("Grandpa") with authority ("can manage members").
