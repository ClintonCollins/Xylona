import * as path from 'path'
import { execFileSync } from 'child_process'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..', '..')
const E2E_DIR = import.meta.dirname

// Load .env for custom credentials or port overrides (optional).
try {
  process.loadEnvFile(path.join(import.meta.dirname, '..', '.env'))
} catch {
  // .env is optional — shell env vars or CI secrets take precedence
}

const HTTP_PORT = process.env['E2E_HTTP_PORT'] ?? '9091'
const NODE_PORT = process.env['E2E_NODE_PORT'] ?? '9501'
const E2E_MODE = process.env['E2E_MODE'] ?? 'local-controller'
const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

export default async function globalSetup(): Promise<void> {
  console.log(`[E2E Setup] Delegating to Go orchestrator (${E2E_MODE})...`)
  execFileSync(
    'go',
    [
      'run',
      './cmd/e2e',
      'setup',
      '--mode',
      E2E_MODE,
      '--http-port',
      HTTP_PORT,
      '--node-port',
      NODE_PORT,
      '--admin-username',
      ADMIN_USERNAME,
      '--admin-password',
      ADMIN_PASSWORD,
      '--e2e-dir',
      E2E_DIR,
      '--project-root',
      PROJECT_ROOT,
    ],
    { cwd: PROJECT_ROOT, stdio: 'inherit', timeout: 300_000 },
  )
}
