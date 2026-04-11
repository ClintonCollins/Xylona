# Xylona Frontend

The frontend lives in `frontend/`, but the contributor guide and canonical
workflow are documented in the root [README](../README.md).

Use pnpm from the repository root for common frontend tasks:

```bash
pnpm --dir frontend install
pnpm --dir frontend run dev
pnpm --dir frontend run lint
pnpm --dir frontend run test
pnpm --dir frontend run build
```

For config details, see [quasar.config.mjs](quasar.config.mjs) and the
[Quasar CLI Vite config docs](https://v2.quasar.dev/quasar-cli-vite/quasar-config-js).
