# Yauli Design System

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

## Brand Colors

### Primary

Teal

```
#74C7C3
```

Represents:

* trust
* calm
* care
* growth

Use for:

* primary buttons
* active navigation
* links
* selected controls
* icons
* focus states
* charts
* timeline connectors

Teal is the dominant brand color.

### Secondary

Blue

```
#56789D
```

Represents:

* reliability
* stability
* professionalism

Use for:

* headings
* navbar
* logo text
* important labels
* secondary buttons
* statistics

Blue should rarely be used for call-to-action buttons.

### Accent

Coral

```
#F28B72
```

Represents:

* warmth
* love
* feeding
* positive attention

Use sparingly.

Perfect for:

* primary call-to-action when emphasis is needed
* notification badges
* feed events
* hearts
* onboarding highlights

Coral should never dominate the interface.

---

## Neutral Colors

### Background

```
#FCFBF8
```

Default application background.

Avoid pure white pages.

### Surface

```
#FFFFFF
```

Cards, dialogs, forms, navigation surfaces.

### Secondary Background

```
#F3F8F8
```

Timeline sections, panels, grouped content.

---

## Typography

Primary text

```
#334155
```

Secondary text

```
#64748B
```

Muted text

```
#94A3B8
```

Never use pure black.

---

## Borders

```
#E6EEF0
```

Borders should be subtle.

Avoid strong outlines. Prefer whitespace over borders.

---

## Event Colors

| Event       | Color     |
|-------------|-----------|
| Feed        | `#F28B72` |
| Nappy       | `#74C7C3` |
| Sleep       | `#8A84E2` |
| Bath        | `#7EC8E3` |
| Observation | `#A3A380` |
| Weight      | `#7B8F4E` |
| Health      | `#D96C6C` |

---

## Semantic Colors

| Meaning | Color     |
|---------|-----------|
| Success | `#61B984` |
| Warning | `#F2B950` |
| Danger  | `#E46A6A` |

---

## Color Usage Rules

The interface should approximately follow this balance:

* 70% neutral backgrounds
* 20% teal
* 8% blue
* 2% coral

Coral should be reserved for moments that deserve attention.

If everything is orange, nothing stands out.

---

## Components

**Primary Button**

* teal background
* white text

**Secondary Button**

* white background
* blue text
* light border

**Danger Button**

* red background

**Cards**

* white
* subtle shadow
* rounded corners
* minimal borders

**Navbar**

* white background
* blue text
* teal active indicator

**Timeline**

* teal connector
* event-specific colored icons

**Daily Report Card**

* use a deterministic title with the baby's name and selected day
* show exactly four KPIs in this order: feeds, sleep, pump, nappies
* show the event count prominently, followed by a coloured uppercase label and
  compact detail such as volume, duration, or "changed"
* use event colours only for the labels; keep count and detail text neutral
* separate the four columns with subtle dividers
* do not add generated prose, icons, emoji, or model-dependent state
* use the same structure for today and historical timeline days
* keep the complete card glanceable in 5 seconds

---

## Design Principles

Always prefer:

* whitespace over borders
* rounded corners
* soft shadows
* generous spacing
* simple layouts
* large tap targets
* minimal visual noise

Never use:

* harsh gradients
* neon colors
* saturated reds
* heavy shadows
* thick borders
* glossy effects
* skeuomorphic styling

The interface should always feel peaceful and reassuring for sleep-deprived parents.

When in doubt, choose simplicity.
