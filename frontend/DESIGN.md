---
name: "Xylona"
description: "Dark-only futuristic command-center design system for a self-hosted game server control panel."
colors:
  primary: "#3B82F6"
  secondary: "#6366F1"
  accent: "#1CB7CF"
  success: "#22C55E"
  danger: "#EF4444"
  warning: "#F59E0B"
  info: "#06B6D4"
  base: "#0D0E0F"
  surface0: "#141516"
  surface1: "#1A1C1D"
  surface2: "#222425"
  surface3: "#2B2E2F"
  surface4: "#383B3D"
  textPrimary: "#E0E4E6"
  textSecondary: "#979B9E"
  textMuted: "#858A8C"
  onColor: "#FFFFFF"
  purple: "#8B5CF6"
typography:
  brand:
    fontFamily: "Zen Dots, sans-serif"
    fontSize: "1.5rem"
    fontWeight: 400
    lineHeight: "1.15"
    letterSpacing: "0"
  heading:
    fontFamily: "Goldman, sans-serif"
    fontSize: "2rem"
    fontWeight: 700
    lineHeight: "1.2"
    letterSpacing: "0.02em"
  control:
    fontFamily: "Exo 2, -apple-system, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: "1.25"
    letterSpacing: "0"
  body:
    fontFamily: "Exo 2, -apple-system, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: "1.5"
    letterSpacing: "0"
  body-sm:
    fontFamily: "Exo 2, -apple-system, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: "1.4"
    letterSpacing: "0"
  caption:
    fontFamily: "Exo 2, -apple-system, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: "1.35"
    letterSpacing: "0"
  micro:
    fontFamily: "Exo 2, -apple-system, Helvetica Neue, Helvetica, Arial, sans-serif"
    fontSize: "0.6875rem"
    fontWeight: 600
    lineHeight: "1.3"
    letterSpacing: "0.04em"
  mono:
    fontFamily: "JetBrains Mono, Oxygen Mono, monospace"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: "1.45"
    letterSpacing: "0"
rounded:
  sm: "4px"
  md: "6px"
  lg: "8px"
  xl: "12px"
  pill: "999px"
spacing:
  "2xs": "2px"
  xs: "4px"
  sm: "8px"
  base: "12px"
  md: "16px"
  lg: "24px"
  xl: "32px"
  "2xl": "48px"
  "3xl": "64px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.onColor}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "4px 12px"
  button-secondary:
    backgroundColor: "{colors.surface2}"
    textColor: "{colors.textPrimary}"
    typography: "{typography.control}"
    rounded: "{rounded.md}"
    padding: "4px 12px"
  surface-card:
    backgroundColor: "{colors.surface1}"
    textColor: "{colors.textPrimary}"
    rounded: "{rounded.lg}"
    padding: "16px"
  toolbar:
    backgroundColor: "{colors.surface2}"
    textColor: "{colors.textPrimary}"
    typography: "{typography.control}"
    height: "50px"
  input:
    backgroundColor: "{colors.surface0}"
    textColor: "{colors.textPrimary}"
    typography: "{typography.body-sm}"
    rounded: "{rounded.md}"
    padding: "8px 12px"
  console:
    backgroundColor: "{colors.base}"
    textColor: "{colors.textPrimary}"
    typography: "{typography.mono}"
    rounded: "{rounded.md}"
    padding: "16px"
---

# Design System: Xylona

## Overview

Xylona is a dark-only product UI with a futuristic command-center feel. It should communicate operational control, server health, and technical confidence while staying approachable for self-hosters.

The visual language is layered, cool-tinted, and precise: near-black foundations, stacked surfaces, cyan and blue accents, readable status colors, and restrained sci-fi typography. The product should feel immersive without sacrificing speed, scanability, or workflow clarity.

The personality is **powerful, sleek, and futuristic** — a high-tech command center that is confident, technical, and in control. The tone can be gaming-native and energetic, but it must never become chaotic, toy-like, or cute. Xylona should feel at home beside Discord and Steam for audience familiarity, while matching the polish and clarity users expect from tools like Linear and Vercel.

**Anti-references.** Avoid generic admin-panel templates, cPanel-style clutter, flat lifeless dashboards, and sterile spreadsheet-like layouts. Avoid softening the identity into a plain SaaS theme. Do not imitate other game server control panels: study their domain patterns for console, file browser, server controls, backups, and configuration, then differentiate through stronger hierarchy, tighter interaction design, and a more distinctive command-center visual language.

**Key Characteristics:**

- Dark-only, cool-tinted, never pure black.
- Depth by stacked surfaces first, borders second, shadows last.
- Cyan and blue reserved for interactive and stateful meaning.
- Dense but legible; built for scanning and comparison.
- Display typefaces for identity and command, never for dense operational content.

## Colors

A cool, near-monochrome foundation carrying a narrow band of saturated signal color.

Use the existing token system instead of hardcoded color values. Extend `src/css/design-tokens.css` for custom properties, `src/css/overrides.css` for Quasar component overrides, and `src/css/quasar.variables.scss` only for Quasar theme primitives.

### Primary

- **Command Blue** (#3B82F6): primary actions, links, focus states, and high-confidence interactive emphasis.
- **Signal Cyan** (#1CB7CF): brand highlights, active navigation, technical emphasis, and high-signal UI details.

### Secondary

- **Deep Indigo** (#6366F1): secondary actions and supporting accent moments.
- **Telemetry Violet** (#8B5CF6): category and chart differentiation where blue and cyan are already spoken for.

### Tertiary

- **Running Green** (#22C55E): running servers and successful operations.
- **Fault Red** (#EF4444): errors, destructive actions, and stopped or failed server states.
- **Caution Amber** (#F59E0B): caution, pending risk, degraded state, and attention-needed alerts.
- **Cool Info Cyan** (#06B6D4): informational feedback and cool technical status.

### Neutral

- **Hull Black** (#0D0E0F): page foundation, and the darkest surface in the system.
- **Stacked Surfaces** (#141516 through #383B3D): five steps of UI depth, from primary app surfaces to hover and selected states.
- **Readout White** (#E0E4E6): primary content text.
- **Supporting Gray** (#979B9E): metadata and secondary text.
- **Muted Gray** (#858A8C): tertiary text, overlines, and fine print.

### Named Rules

**The Signal Reserve Rule.** Cyan and blue carry interactive or stateful meaning. A surface that spends accent color on decoration has spent the signal, and the next genuinely stateful element has nothing left to say.

**The Never-Pure-Black Rule.** The floor is `--xy-base` (#0D0E0F), never `#000`. Neutrals stay cool-tinted; a warm or fully desaturated gray reads as a different product.

**The Series-Is-Not-Status Rule.** Time-series data uses `--xy-series-*`, not `--xy-chart-3` / `--xy-chart-4`. Red and amber are reserved for real state — threshold breaches and configured-limit lines — so an ordinary series like storage growth never plots in alarm colors.

## Typography

**Brand Font:** Zen Dots (sans-serif fallback)
**Display Font:** Goldman (sans-serif fallback)
**Body Font:** Exo 2 (with `-apple-system`, Helvetica Neue, Helvetica, Arial fallbacks)
**Mono Font:** JetBrains Mono (with Oxygen Mono fallback)

**Character:** Two geometric sci-fi display faces carry identity and command, while a humanist body face and a technical monospace carry everything an operator actually reads. All four are self-hosted through `@fontsource`.

### Hierarchy

- **Brand** (Zen Dots, 400, 1.5rem, 1.15): identity moments and notification titles only.
- **Heading** (Goldman, 700, 2rem, 1.2, 0.02em): page titles and section headings.
- **Control** (Exo 2, 600, 1rem, 1.25): buttons, toolbar actions, and status badges. Note that controls render in Exo 2, not Goldman — `--xy-font-control` resolves to `--xy-font-body`.
- **Body** (Exo 2, 400, 1rem, 1.5): general product content. Cap prose line length around 65–75 characters.
- **Body Small** (Exo 2, 400, 0.875rem, 1.4): dense UI copy and helper text.
- **Caption** (Exo 2, 400, 0.75rem, 1.35): timestamps, helper text, and table metadata.
- **Micro** (Exo 2, 600, 0.6875rem, 1.3, 0.04em): dense metric labels, overlines, and fine print.
- **Mono** (JetBrains Mono, 400, 0.875rem, 1.45): console output, file paths, code, IDs, ports, versions, and metrics.

Type sizes come from the `--xy-font-size-*` tokens rather than ad-hoc rem values.

### Named Rules

**The Display Ceiling Rule.** Zen Dots and Goldman lose legibility below roughly 0.95rem. Caption and micro text is always Exo 2 or JetBrains Mono — never a display face.

**The Two-Steps-Below Rule.** Below `body-sm` (0.875rem) there are exactly two steps: caption (`--xy-font-size-xs`, 0.75rem) and micro (`--xy-font-size-2xs`, 0.6875rem). Do not freelance a size in between.

**The Comparable-Data Rule.** Anything an operator may need to compare character by character — ports, IDs, hashes, versions, paths, metric values — is JetBrains Mono. Proportional digits in a column of numbers are a defect, not a style choice.

## Layout

Design for repeated operational workflows: scanning server status, comparing values, controlling lifecycle, editing files, reading console output, managing backups, and navigating RBAC or settings.

Use dense but organized layouts with clear alignment, predictable actions, and durable responsive behavior. Page headers should expose the current object, status, and primary actions. Prefer progressive disclosure over cramming every control into the first view. Keep essential server health, player counts, resource usage, lifecycle state, and recent feedback visible without forcing users to hunt.

Page structure uses the shared utilities in `design-tokens.css`: `.xy-page-content` for the responsive content well (16px/24px padding, widening to 24px/32px above 1024px), `.xy-page-header` for the title-and-actions band, and `.xy-section-overline` for uppercase section labels.

Spacing comes from the `--xy-space-*` scale (2px through 64px, including a 12px `base` step). The toolbar height is fixed at 50px via `--xy-toolbar-height`.

**Breakpoints are currently ad-hoc.** The codebase contains more than twenty distinct `max-width` values, of which only 599px and 1023px align with Quasar's own scale. There is no breakpoint token. New work should prefer Quasar's breakpoints (599 / 1023 / 1439 / 1919) rather than adding another one-off value.

### Named Rules

**The No-Hero Rule.** Landing-page composition does not belong inside the app. Dashboards lead with state, not with a decorative hero band.

## Elevation & Depth

This system conveys depth primarily through **tonal layering**, with borders as the second instrument and shadows as the third. Surfaces stack rather than float.

- Page foundation: `--xy-base`.
- Primary app surfaces: `--xy-surface-0` and `--xy-surface-1`.
- Raised panels, toolbars, and active containers: `--xy-surface-2`.
- Hover, selected, or stronger separation states: `--xy-surface-3` and `--xy-surface-4`.

Borders come from `--xy-border` (10% white), `--xy-border-hover` (18% white), and `--xy-border-active` (40% primary blue), plus the semantic border tokens.

### Shadow Vocabulary

- **`--xy-shadow-sm`** (`0 1px 2px rgba(0,0,0,0.3)`): hairline separation for inline controls.
- **`--xy-shadow-md`** (`0 2px 8px rgba(0,0,0,0.4)`): raised panels and menus.
- **`--xy-shadow-lg`** (`0 4px 16px rgba(0,0,0,0.5)`): dialogs and popovers.
- **`--xy-shadow-xl`** (`0 10px 24px rgba(0,0,0,0.45)`): overlays above page content.
- **`--xy-shadow-2xl`** (`0 16px 34px rgba(0,0,0,0.5)`): fullscreen and modal layers.
- **`--xy-shadow-sticky-lg`** (`0 12px 30px rgba(0,0,0,0.28)`): sticky headers separating from scrolled content.

### Named Rules

**The Functional Shadow Rule.** Shadows communicate layering, never gloss. If a shadow is not separating one plane from another, remove it.

**The No-Glass Rule.** Glassmorphism and heavy backdrop blur are not a default treatment in this system.

## Shapes

The UI should feel technical and crisp. Radii are modest and always drawn from tokens:

- **4px** (`--xy-radius-sm`): small controls and compact technical elements.
- **6px** (`--xy-radius-md`): buttons and input controls.
- **8px** (`--xy-radius-lg`): cards, dialogs, repeated items, and larger panels.
- **12px** (`--xy-radius-xl`): hero panels, banners, and prominent framed sections.
- **Pill** (`--xy-radius-pill`): badges, chips, status dots, and segmented controls.

### Named Rules

**The Token Radius Rule.** Always use `--xy-radius-*`. A literal `1rem` or `999px` in a component is a defect.

**The Pill-Means-Badge Rule.** Pill radius is semantic, not stylistic. If the element is not a badge, chip, status dot, or segmented control, it does not get a pill.

**The No-Nested-Cards Rule.** Cards do not nest. A card inside a card means the outer container should have been an unframed layout band.

## Components

### Buttons

- **Shape:** modestly rounded (6px, `--xy-radius-md`).
- **Typography:** Exo 2 at weight 600, no text transform.
- **Primary:** Command Blue background with white text, 4px 12px padding. Reserved for the next most important action on the screen.
- **Secondary:** `--xy-surface-2` background with primary text.
- **Icons:** use a familiar symbol where one exists; icon-only controls require a tooltip.

Buttons are Quasar primitives styled through tokens and `overrides.css`, not bespoke elements.

### Cards / Containers

- **Corner Style:** 8px (`--xy-radius-lg`).
- **Background:** `--xy-surface-1`.
- **Border:** `--xy-border`, strengthening to `--xy-border-hover` on hover.
- **Internal Padding:** 16px (`--xy-space-md`).

Cards are for repeated items, modals, and genuinely framed tools. For dashboards, reach first for unframed layout bands, tables, compact lists, segmented controls, and status strips.

### Inputs / Fields

- **Style:** `--xy-surface-0` background, 6px radius, 8px 12px padding, body-small type.
- **Focus:** border shifts to `--xy-border-active`; the focus ring uses `--xy-accent-hover`.
- **Labels:** explicit and always present. Helper text explains consequences rather than repeating the label.

### Toolbar / Navigation

- **Height:** 50px (`--xy-toolbar-height`), background `--xy-surface-2`.
- **Type:** control role; active navigation carries Signal Cyan.

### Console

- **Background:** `--xy-base` — the console sits at the system's darkest level.
- **Type:** JetBrains Mono at 0.875rem.
- **Behavior:** stable dimensions, no layout shift, strong scroll behavior, quick access to frequent actions.

Console and file-browser surfaces should feel native to server administration. Notifications use the semantic tint classes (`.bg-xy-success-tint` and siblings) with readable iconography.

### Named Rules

**The Single Primary Rule.** One primary button per screen. If two actions both look primary, neither is.

**The Status-Never-Color-Alone Rule.** Every status pairs color with text, icon, shape, or position. A green dot alone is not a running state.

## Do's and Don'ts

### Do:

- **Do** use existing `--xy-*` tokens before adding new values.
- **Do** extend `design-tokens.css` for utilities and `overrides.css` for Quasar component styling.
- **Do** keep the app dark-only, layered, and cool-tinted.
- **Do** make status, feedback, and available actions obvious.
- **Do** use cyan and blue accents for interactive or stateful meaning.
- **Do** preserve the global reduced-motion block in `design-tokens.css`, and animate transform and opacity rather than layout properties.
- **Do** use `--xy-ease-standard` (`cubic-bezier(0.2, 0, 0, 1)`) and the `--xy-transition-fast` / `--xy-transition-base` tokens for state changes.

### Don't:

- **Don't** hardcode colors, radii, or type sizes in components.
- **Don't** create generic admin-template screens or cPanel-style clutter.
- **Don't** use decorative side-stripe borders, gradient text, default glassmorphism, or repeated icon-heading-text card grids.
- **Don't** hide server lifecycle actions, health, or errors behind vague menus.
- **Don't** set display faces (Zen Dots, Goldman) below roughly 0.95rem.
- **Don't** replace accepted `v-html` console and tooltip rendering unless the trust model changes.
- **Don't** add durable project documentation under the repo-root `/docs/`, which is intentionally local scratch space.
