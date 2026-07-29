// Verifies the Xylona ds-bundle actually renders: styles.css closure resolves,
// all four brand families load as real webfonts, and --xy-* tokens resolve.
// Serves ds-bundle over http (font loading via file:// is unreliable).
import { createServer } from 'node:http'
import { readFileSync, existsSync } from 'node:fs'
import { join, extname } from 'node:path'
import { chromium } from 'playwright'

const BUNDLE = process.argv[2]
const OUT = process.argv[3]
const MIME = {
  '.css': 'text/css',
  '.html': 'text/html',
  '.js': 'text/javascript',
  '.woff2': 'font/woff2',
  '.woff': 'font/woff',
}

// The probe page: exercises each family, the type scale, colors, radii,
// spacing, and the shipped utility classes.
const FAMILIES = [
  ['brand', 'Zen Dots', '--xy-font-brand'],
  ['heading', 'Goldman', '--xy-font-heading'],
  ['body', 'Exo 2', '--xy-font-body'],
  ['mono', 'JetBrains Mono', '--xy-font-mono'],
]
const SURFACES = ['base', 'surface-0', 'surface-1', 'surface-2', 'surface-3', 'surface-4']
const SEMANTIC = ['primary', 'accent', 'success', 'danger', 'warning', 'info', 'purple']

const page = `<!doctype html><html><head><meta charset="utf-8">
<link rel="stylesheet" href="/styles.css">
<style>
  body { background: var(--xy-base); color: var(--xy-text-primary);
         font-family: var(--xy-font-body); padding: var(--xy-space-xl); margin: 0; }
  .row { display: flex; gap: var(--xy-space-sm); margin-bottom: var(--xy-space-lg); flex-wrap: wrap; }
  .sw { width: 96px; height: 56px; border-radius: var(--xy-radius-md);
        border: 1px solid var(--xy-border); font-size: var(--xy-font-size-2xs);
        display: flex; align-items: flex-end; padding: var(--xy-space-xs); }
</style></head><body>
${FAMILIES.map(
  ([k, fam, tok]) =>
    `<div id="fam-${k}" style="font-family: var(${tok}); font-size: 2rem; margin-bottom: 12px">${fam} — Xylona 0123</div>`,
).join('\n')}
<div class="row">${SURFACES.map(
  (s) => `<div class="sw" style="background: var(--xy-${s})">${s}</div>`,
).join('')}</div>
<div class="row">${SEMANTIC.map(
  (s) =>
    `<div class="sw" style="background: var(--xy-${s}); color: var(--xy-text-on-dark)">${s}</div>`,
).join('')}</div>
<div class="row">
  <div class="bg-xy-success-tint sw">success-tint</div>
  <div class="bg-xy-danger-tint sw">danger-tint</div>
  <div class="bg-xy-warning-tint sw">warning-tint</div>
  <div class="bg-xy-info-tint sw">info-tint</div>
</div>
<div class="xy-page-header"><h1 class="xy-page-title">Page title utility</h1></div>
<div class="xy-section-overline">section overline utility</div>
</body></html>`

const server = createServer((req, res) => {
  const url = decodeURIComponent(req.url.split('?')[0])
  if (url === '/' || url === '/index.html') {
    res.writeHead(200, { 'content-type': 'text/html' })
    return res.end(page)
  }
  const f = join(BUNDLE, url)
  if (!existsSync(f)) {
    res.writeHead(404)
    return res.end('nope')
  }
  res.writeHead(200, { 'content-type': MIME[extname(f)] ?? 'application/octet-stream' })
  res.end(readFileSync(f))
})

await new Promise((r) => server.listen(0, r))
const port = server.address().port

const browser = await chromium.launch()
const pg = await browser.newPage({ viewport: { width: 1100, height: 900 } })
const netFail = []
pg.on('requestfailed', (r) => netFail.push(r.url()))
pg.on('response', (r) => {
  if (r.status() >= 400) netFail.push(`${r.status()} ${r.url()}`)
})

await pg.goto(`http://127.0.0.1:${port}/`, { waitUntil: 'networkidle' })
await pg.evaluate(() => document.fonts.ready)

const result = await pg.evaluate((fams) => {
  const cs = getComputedStyle(document.documentElement)
  const tok = (n) => cs.getPropertyValue(n).trim()
  // A family counts as loaded only if the browser reports a real face for it.
  const loaded = fams.map(([, fam]) => ({
    family: fam,
    loaded: document.fonts.check(`16px "${fam}"`),
  }))
  // Measure each family's rendered width — identical widths across all four
  // would mean everything silently fell back to one system font.
  const widths = fams.map(
    ([k]) => document.getElementById(`fam-${k}`).getBoundingClientRect().width,
  )
  return {
    loaded,
    widths,
    sampleTokens: {
      base: tok('--xy-base'),
      primary: tok('--xy-primary'),
      accent: tok('--xy-accent'),
      radiusMd: tok('--xy-radius-md'),
      spaceMd: tok('--xy-space-md'),
      fontMono: tok('--xy-font-mono'),
    },
    // color-mix() tokens only resolve if the browser supports them — check one.
    computedMix: getComputedStyle(document.body).backgroundColor,
    tokenCount: Array.from(document.styleSheets)
      .flatMap((s) => {
        try {
          return Array.from(s.cssRules)
        } catch {
          return []
        }
      })
      .filter((r) => r.style)
      .flatMap((r) => Array.from(r.style))
      .filter((p) => p.startsWith('--xy-')).length,
  }
}, FAMILIES)

await pg.screenshot({ path: OUT, fullPage: true })
await browser.close()
server.close()

console.log(JSON.stringify({ ...result, netFail }, null, 2))
