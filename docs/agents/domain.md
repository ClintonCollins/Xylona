# Domain Docs

Xylona uses one domain context.

## Before exploring

- Read the repo-root `CONTEXT.md` before domain-sensitive work.
- Read ADRs under `docs/adr/` that touch the area being changed.
- If either source is absent, proceed silently. Create or update domain documentation only when a term or decision is actually resolved.

## Use the glossary's vocabulary

When your output names a domain concept (in an issue title, a refactor proposal, a hypothesis, a test name), use the term as defined in `CONTEXT.md`. Don't drift to synonyms the glossary explicitly avoids.

If a needed concept is absent, first reconsider whether existing project language already covers it. Otherwise note the genuine glossary gap.

## Flag ADR conflicts

If your output contradicts an existing ADR, surface it explicitly rather than silently overriding:

> _Contradicts ADR-0007 (event-sourced orders), but worth reopening because…_
