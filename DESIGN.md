---
version: "alpha"
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
    fontFamily: "Zen Dots"
    fontSize: "1.5rem"
    fontWeight: 400
    lineHeight: "1.15"
    letterSpacing: "0"
  heading:
    fontFamily: "Goldman"
    fontSize: "2rem"
    fontWeight: 700
    lineHeight: "1.2"
    letterSpacing: "0"
  control:
    fontFamily: "Goldman"
    fontSize: "1rem"
    fontWeight: 600
    lineHeight: "1.25"
    letterSpacing: "0"
  body:
    fontFamily: "Exo 2"
    fontSize: "1rem"
    fontWeight: 400
    lineHeight: "1.5"
    letterSpacing: "0"
  body-sm:
    fontFamily: "Exo 2"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: "1.4"
    letterSpacing: "0"
  caption:
    fontFamily: "Exo 2"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: "1.35"
    letterSpacing: "0"
  micro:
    fontFamily: "Exo 2"
    fontSize: "0.6875rem"
    fontWeight: 600
    lineHeight: "1.3"
    letterSpacing: "0.04em"
  mono:
    fontFamily: "JetBrains Mono"
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

## Overview

Xylona is a dark-only product UI with a futuristic command-center feel. It should communicate operational control, server health, and technical confidence while staying approachable for self-hosters.

The visual language is layered, cool-tinted, and precise: near-black foundations, stacked surfaces, cyan and blue accents, readable status colors, and restrained sci-fi typography. The product should feel immersive without sacrificing speed, scanability, or workflow clarity.

## Colors

Use the existing token system instead of hardcoded color values. Extend `frontend/src/css/design-tokens.css` for custom properties, `frontend/src/css/overrides.css` for Quasar component overrides, and `frontend/src/css/quasar.variables.scss` only for Quasar theme primitives.

- **Primary (#3B82F6):** primary actions, links, focus states, and high-confidence interactive emphasis.
- **Secondary (#6366F1):** secondary actions and supporting accent moments.
- **Accent (#1CB7CF):** brand highlights, active navigation, technical emphasis, and high-signal UI details.
- **Success (#22C55E):** running servers and successful operations.
- **Danger (#EF4444):** errors, destructive actions, and stopped or failed server states.
- **Warning (#F59E0B):** caution, pending risk, degraded state, and attention-needed alerts.
- **Info (#06B6D4):** informational feedback and cool technical status.
- **Base and surfaces (#0D0E0F through #383B3D):** page background and stacked UI depth.
- **Text (#E0E4E6, #979B9E, #858A8C):** primary content, supporting metadata, and muted tertiary text.

Dark mode is not optional for this product. Keep neutrals cool-tinted, never pure black. Reserve bright cyan and blue for stateful or interactive meaning rather than decorative wash.

## Typography

Use the four-font hierarchy already present in the frontend:

- **Zen Dots:** brand moments only. Use sparingly for identity and notification titles.
- **Goldman:** headings, navigation labels, controls, and command surfaces.
- **Exo 2:** body text, dense UI copy, helper text, and general product content.
- **JetBrains Mono:** console output, file paths, code, IDs, ports, versions, metrics, and other technical data.

Display fonts are part of the identity, but overuse weakens the interface. Keep long-form and dense operational content in Exo 2 or JetBrains Mono. Body copy should remain readable at dashboard density, with line lengths capped around 65 to 75 characters where prose appears.

Type sizes come from the `--xy-font-size-*` tokens rather than ad-hoc rem values. Below `body-sm` (0.875rem) there are exactly two steps: **caption** (`--xy-font-size-xs`, 0.75rem) for timestamps, helper text, and table metadata, and **micro** (`--xy-font-size-2xs`, 0.6875rem) for dense metric labels, overlines, and fine print. Do not freelance sizes in between. Caption and micro text is always Exo 2 or JetBrains Mono — never Zen Dots or Goldman, which lose legibility below roughly 0.95rem.

## Layout

Design for repeated operational workflows: scanning server status, comparing values, controlling lifecycle, editing files, reading console output, managing backups, and navigating RBAC or settings.

Use dense but organized layouts with clear alignment, predictable actions, and durable responsive behavior. Page headers should expose the current object, status, and primary actions. Avoid landing-page composition inside the app. Dashboards should prioritize useful state over decorative hero sections.

Prefer progressive disclosure over cramming every control into the first view. Keep essential server health, player counts, resource usage, lifecycle state, and recent feedback visible without forcing users to hunt.

## Elevation & Depth

Depth comes from the surface scale, subtle borders, and controlled shadows:

- Page foundation: `--xy-base`.
- Primary app surfaces: `--xy-surface-0` and `--xy-surface-1`.
- Raised panels, toolbars, and active containers: `--xy-surface-2`.
- Hover, selected, or stronger separation states: `--xy-surface-3` and `--xy-surface-4`.

Use borders from `--xy-border`, `--xy-border-hover`, and semantic border tokens. Shadows should be functional and low-noise, never glossy. Do not rely on heavy blur or glassmorphism as a default treatment.

## Shapes

The UI should feel technical and crisp. Use modest radii:

- 4px for small controls and compact technical elements.
- 6px for buttons and input controls.
- 8px for cards, dialogs, repeated items, and larger panels.
- 12px for hero panels, banners, and prominent framed sections.
- Pill (`--xy-radius-pill`) for badges, chips, status dots, and other fully rounded elements.

Always use the `--xy-radius-*` tokens; do not hardcode radii like `1rem` or `999px`. Avoid pill-heavy layouts unless the element is semantically a badge, chip, or segmented control. Do not nest cards inside cards.

## Components

Buttons should use Quasar primitives styled through tokens and overrides. Use icons for tool actions where a familiar symbol exists, and include tooltips for icon-only controls. Primary buttons should be reserved for the next most important action on the screen.

Cards are for repeated items, modals, and genuinely framed tools. Do not turn every section into a card. For dashboards, combine unframed layout bands, tables, compact lists, segmented controls, and status strips before reaching for identical card grids.

Forms should be compact, clear, and forgiving. Labels must be explicit. Helper text should explain consequences, not repeat the label. Destructive actions need clear visual hierarchy and confirmation proportional to risk.

Console and file-browser surfaces should feel native to server administration: monospaced where useful, stable dimensions, no layout shift, strong scroll behavior, and quick access to frequent actions.

Notifications should use existing semantic tint classes and readable iconography. Status should never rely on color alone; pair color with text, icons, shape, or position.

## Do's and Don'ts

Do:

- Use existing `--xy-*` tokens before adding new values.
- Extend `design-tokens.css` for utilities and `overrides.css` for Quasar component styling.
- Keep the app dark-only, layered, and cool-tinted.
- Make status, feedback, and available actions obvious.
- Use cyan and blue accents for interactive or stateful meaning.
- Preserve reduced-motion behavior and avoid animating layout properties.

Don't:

- Do not hardcode colors in components.
- Do not create generic admin-template screens.
- Do not use decorative side-stripe borders, gradient text, default glassmorphism, or repeated icon-heading-text card grids.
- Do not hide server lifecycle actions, health, or errors behind vague menus.
- Do not replace accepted `v-html` console and tooltip rendering unless the trust model changes.
- Do not add durable project documentation under `/docs/`, which is intentionally local scratch space.
