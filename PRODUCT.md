# Product

## Register

product

## Users

Xylona serves individual gamers self-hosting servers for friends, gaming community admins managing servers for many players, and small hosting providers operating multiple nodes. They are usually checking server state, reading console output, browsing files, managing backups, adjusting configuration, or controlling server lifecycle.

The interface should be immediately understandable for a first-time self-hoster while staying fast and information-rich for power users who repeat the same operational tasks every day.

## Product Purpose

Xylona is a game server control panel built to be easy to self-host and ship as a single binary. The Go backend embeds a Quasar and Vue frontend, so the product should feel cohesive, dependable, and local-first rather than like a loose admin template.

Success means an admin can understand what is happening, take the next action with confidence, and recover from routine issues without hunting through hidden menus or noisy screens.

## Brand Personality

Powerful, sleek, futuristic.

Xylona should feel like a high-tech command center: confident, technical, and in control. The tone can be gaming-native and energetic, but it should never become chaotic, toy-like, or cute. The product should feel at home beside Discord and Steam for audience familiarity, while matching the polish and clarity users expect from tools like Linear and Vercel.

## Anti-references

Avoid generic admin panel templates, cPanel-style clutter, flat lifeless dashboards, and sterile spreadsheet-like layouts. Avoid softening the identity into a plain SaaS theme. Avoid hiding core server status, health, and actions behind vague menus or decorative panels.

Do not imitate other game server control panels. Study their domain patterns for console, file browser, server controls, backups, and configuration, then differentiate Xylona through stronger hierarchy, tighter interaction design, and a more distinctive command-center visual language.

## Design Principles

1. Command and control: every screen should make state, action, and feedback obvious.
2. Progressive disclosure: show essentials first, then reveal advanced controls and detail views when needed.
3. Gaming-native but professional: distinctive, capable, and immersive without becoming noisy.
4. Dense but legible: support operational scanning and comparison without cPanel-style overload.
5. Trust through structure: use consistent surface hierarchy, predictable navigation, and clear status semantics.

## Accessibility & Inclusion

Prioritize readability, discoverability, keyboard navigation, and strong contrast. Optimize for real-world usability over rigid score-chasing, but do not ignore common needs such as reduced motion, color-blind-safe status cues, and clear focus states.

For local browser verification, credentials may be read from `.env` when needed, but must never be printed, logged, or committed.

## Accepted Audit Boundaries

The following `v-html` usages are intentional and should not be flagged as XSS issues unless the trust model changes:

- `frontend/src/pages/game_servers/GameServerView.vue`: console output from the user's own authenticated game server, formatted by `parseConsole()`.
- `frontend/src/components/shared/ClipBoardCopy.vue`: styled tooltip HTML from application-controlled props.

Do not add DOMPurify or replace these with plain text unless the trust boundary changes.
