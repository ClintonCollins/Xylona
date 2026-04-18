# Xylona Frontend

The frontend lives in `frontend/`, but the contributor guide and canonical
workflow are documented in the root [README](../README.md).

Use Bun inside `frontend/` for common frontend tasks:

```bash
bun install
bun run dev
bun run lint
bun run test
bun run build
```

On Windows, `bun run test` and Playwright commands currently use Bun's
Node-compatible path rather than Bun-native `--bun` execution because Vitest's
worker startup is not fully compatible there yet.

For config details, see [quasar.config.mjs](quasar.config.mjs) and the
[Quasar CLI Vite config docs](https://v2.quasar.dev/quasar-cli-vite/quasar-config-js).
