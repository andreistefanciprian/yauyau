# Yauli Design System

Source of truth for the values below is `frontend/static/style.css` (CSS
custom properties in `:root`). If this doc and the CSS ever disagree, the CSS
wins — update this file to match it, not the other way around.

Current palette/type/layout was adapted from a Claude Design mockup
("Timeline Line", 2026-07-21) into the app's real Go/htmx templates and
`style.css`; see git history around that date for the full set of follow-up
fixes (font self-hosting, connecting-line timeline, responsive sidebar,
htmx-powered day switching).

## Brand Personality

Yauli is a modern parenting companion.

The interface should always feel:

* calm
* warm
* trustworthy
* joyful
* uncluttered
* premium
* approachable

Avoid bright, saturated children's-app colors.

The design should feel closer to Apple Health, Headspace or Calm than to a toy store.

---

## Typography

Two families, loaded as self-hosted variable-font files
(`frontend/static/fonts/`) rather than from a font CDN — a cross-origin font
fetch on every full-page navigation made `font-display`'s swap/fallback
decision depend on that request's latency each time, causing an
intermittent layout shift. Both use `font-display: optional`, so a font that
isn't ready in time just doesn't apply for that page view rather than
swapping in later and causing a reflow.

**Sora** (`--font-display`) — headings, the KPI numbers on the daily report
card, dialog titles, the navbar wordmark. Weights 600–800, always bold
(700–800) in practice.

**Nunito Sans** (`--font-body`) — everything else. Weight 400 for body copy,
600–700 for emphasis and most UI labels.

Fall back stack for both: `-apple-system, BlinkMacSystemFont, "Segoe UI",
Roboto, sans-serif`.

---

## Brand Colors

### Primary — Teal

```
--color-teal: #5FBCB0
```

Represents trust, calm, care, growth. Use for: primary buttons, active
navigation, links, selected controls, focus rings, timeline connectors, and
— doubling as the nappy event color — nappy icons/markers.

Teal is the dominant brand color. Primary CTAs (the "+ Add Event" button,
dialog submit buttons) use a soft two-stop gradient toward this color
(`linear-gradient(155deg, color-mix(in srgb, var(--color-teal) 88%, white),
var(--color-teal) 70%)`) with a matching soft colored shadow and a subtle
inset highlight — see "Gradients" under Design Principles before assuming
gradients are banned outright.

### Secondary — Blue

```
--color-blue: #3D7A9C
--color-heading: #2C5C77   /* a darker, separate shade for headings */
```

Represents reliability, stability, professionalism. `--color-blue` is used
for links, the navbar wordmark's "Yau", and stat/label accents. `--color-heading`
is a distinct, darker blue used specifically for `h1`/`h2` and the daily
report's title — don't conflate the two; they're intentionally different
shades of the same hue.

Blue should rarely be used for call-to-action buttons.

### Accent — Coral

```
--color-coral: #E2694A
```

Represents warmth, love, feeding, positive attention. Also doubles as the
temperature/health event color and the navbar wordmark's "li". Use sparingly
— it should never dominate the interface.

---

## Neutral Colors

```
--color-bg:            #FAF6F1   /* default app background, never pure white */
--color-surface:       #FFFDFA   /* cards, dialogs, form fields */
--color-bg-secondary:  #F3EEE5   /* grouped/muted backgrounds */
--color-border:        #EDE2D6   /* subtle borders and dividers */
```

Prefer whitespace over borders; when a border is needed, it should be this
subtle warm neutral, not a cool gray.

---

## Text Colors

```
--color-text-primary:   #3A332C
--color-text-secondary: #6B7280
--color-text-muted:     #9C9184
```

Never pure black.

---

## Event Colors

| Event       | Token                     | Color     |
|-------------|---------------------------|-----------|
| Nappy       | `--color-event-nappy`     | `#5FBCB0` |
| Feed        | `--color-event-feed`      | `#E8A87C` |
| Pump        | `--color-event-pump`      | `#D9946A` |
| Bath        | `--color-event-bath`      | `#6FAAD1` |
| Sleep       | `--color-event-sleep`     | `#B99BD1` |
| Observation | `--color-event-observation` | `#9CAF88` |
| Growth/weight | `--color-event-weight`  | `#8FB8D6` |
| Temperature/health | `--color-event-health` | `#E2694A` |

Each has a soft tint derived via `color-mix()` (`--card-bg-*`, ~20% of the
event color mixed into `--color-surface` in light mode, ~28% mixed into the
dark background in dark mode) — used for the timeline's icon-marker
backgrounds and the add-event category picker cards. Never hardcode a
one-off tint; add a `--card-bg-*` token instead so light/dark stay in sync.

---

## Semantic Colors

```
--color-success: #4E6D3A   /* the "Ongoing" status pill */
--color-warning: #F2B950
--color-danger:  #B5432A   /* delete/destructive actions */
```

---

## Dark Mode

Every token above is redefined under `@media (prefers-color-scheme: dark)` —
background/surface flip to a warm near-black (`#1B1712` / `#26211A`), text
flips to warm off-white, and the `--card-bg-*` tints re-mix against the dark
base at a higher percentage (~28-30%) so they stay legible. Brand colors
(teal/blue/coral) themselves stay the same in both modes — only neutrals and
derived tints change. Always add new colors as tokens with both a light and
dark definition; never reference a raw hex value in a component rule.

---

## Layout

Mobile-first single column, `body { max-width: 480px }`, unchanged from
before this redesign. The one exception: the main timeline app view
(any page whose body contains `.app-layout`) switches to a sidebar + content
layout at `min-width: 900px` — day pills and the type filter become a
sticky left sidebar with full labels (icon + text, not icon-only chips), and
the "+ Add Event" button moves inline next to the content instead of
floating. Settings/login/onboarding/intro are untouched by this and stay a
single centered column at every width.

On mobile, the day/filter controls are collapsed by default behind a small
"Filters ▾" text toggle (styled like the existing `.settings-link` pattern,
not a heavy standalone button) to keep the fold-line clear for the daily
report and timeline.

Day-range switching goes through htmx (`hx-get` swapping `#timeline-workspace`)
rather than a full page load, so it feels as instant as the client-side type
filter. Browser back/forward is deliberately routed to a real full-page
reload instead (`htmx.config.historyCacheSize = 0` +
`refreshOnHistoryMiss = true`) — htmx's own history-snapshot restore
replaces the whole document from a cache that doesn't line up with
client-side state living outside the swapped region (the day pills), which
silently broke every other cached DOM reference on the page. Don't remove
that config without re-testing back/forward thoroughly.

---

## Components

**Primary Button** (`.fab`, `.event-form button[type="submit"]`)

* soft teal gradient background, white text, soft colored shadow + inset
  highlight (see Gradients, below)

**Secondary / text-link controls**

* transparent or `--color-surface` background, `--color-heading` or
  `--color-text-secondary` text, no heavy border

**Danger Button**

* `--color-danger` background or border, used for delete/destructive actions
  only

**Cards** (daily report, settings sections)

* `--color-surface` or a tinted mix of `--color-blue`/`--color-surface`
  background, soft warm-toned shadow (`rgba(58, 51, 38, …)`, never a cool
  gray shadow), rounded corners (`0.875–1rem`), minimal/no border

**Navbar**

* `--color-surface`-tinted translucent background with backdrop blur, sticky
  to top
* brand mark: a small square logo image + a two-tone "Yau"/"li" wordmark
  (Sora, 800 weight) — 40px mark / 20px wordmark on mobile, 56px / 28px at
  the desktop breakpoint, matching the source mockup exactly
* account menu: an initial-letter avatar (first letter of the account's
  display name/email, uppercase) on a neutral circle, not an icon

**Timeline** — a connecting-line activity feed, not stacked cards

* each event is a row: a right-aligned time column, an icon-marker +
  connecting-line column, and a content column
* the marker is a circle tinted with the event's `--card-bg-*`, containing
  the event's real multi-tone SVG icon (not a flattened single-tone icon —
  those partials draw with hardcoded fill/stroke colors, not `currentColor`,
  so they can't be recolored white-on-a-solid-dot the way a placeholder
  mockup icon can)
* the connecting line is `var(--color-border)`, 2px, and must span the
  row's *full* rendered height including any "Finish now" quick-action —
  that action lives inside the same content column as the Ongoing badge (on
  one row together), not as a sibling outside it, or the line visibly
  breaks/bends for in-progress events
* historical days show a "Jul 16, 11:00 AM"-style time label instead of a
  bare "11:00 AM" — the time column stacks the date above the clock within
  the same fixed width rather than widening for it, so mixing historical and
  current-day rows never bends the line

**Daily Report Card**

* deterministic title with the baby's name and selected day
* exactly four KPIs, always in this order: feeds, sleep, pump, nappies
* the four columns are **fixed equal width** (`flex: 1 1 0%`, not sized from
  content) — a day with much longer numbers/detail text (e.g. "540 ml · 45
  m") must render at the exact same column widths as a day with all zeros,
  so switching days never visibly resizes the card. Count/label/detail all
  truncate with an ellipsis rather than overflow if a value is ever too long
  for its column
* the count is Sora, bold, ~2.1rem; the label is a small bold uppercase
  event-colored word; the detail line (ml / duration / "changed") is muted
  and small
* no generated prose, icons, emoji, or model-dependent state — the daily
  report is fully deterministic
* a static "Events" heading (`h2`, same Sora/heading treatment) separates
  the report card from the event list below it
* same structure for today and historical days

**Add/Edit Event dialogs**

* bottom-sheet on mobile, warm-toned backdrop (`rgba(58, 51, 38, 0.35)`) and
  shadow (`rgba(58, 51, 38, 0.18)`) — not neutral black
* category picker cards: tinted `--card-bg-*` background, **no border** (the
  tint alone reads as the card boundary)
* form fields use `--color-surface`/`--color-text-primary` explicitly, not
  the browser's native `Field`/`FieldText` system colors — those vary by
  OS/browser theme and don't match the palette
* focus state on text/date/time/select inputs: `--color-teal` border + a
  soft `color-mix(in srgb, var(--color-teal) 18%, transparent)` ring,
  `outline: none` — not the browser default outline
* all radios and checkboxes use `accent-color: var(--color-teal)` globally
* keep Time and Date inputs on one responsive row in both create and edit
  flows; use the same grouped Date/Time treatment for Started and Fell
  asleep fields; preserve field order and labeling between create and edit
  forms

---

## Design Principles

Always prefer:

* whitespace over borders
* rounded corners
* soft, warm-toned shadows (`rgba(58, 51, 38, …)`, never a cool gray/black
  shadow)
* generous spacing
* simple layouts
* large tap targets
* minimal visual noise

Never use:

* neon colors
* saturated reds
* heavy shadows
* thick borders
* glossy effects
* skeuomorphic styling

**Gradients**: a plain flat fill is still the default for most controls.
The one deliberate exception is primary call-to-action buttons (the Add
Event FAB, a dialog's final submit button), which use a soft two-stop
gradient within the *same* hue (teal into teal, never two different colors)
plus a matching soft colored shadow and a faint inset highlight. That's a
considered accent for the single most important action on a screen, not a
general license for gradients — don't extend it to secondary buttons, cards,
or backgrounds.

The interface should always feel peaceful and reassuring for sleep-deprived parents.

When in doubt, choose simplicity.
