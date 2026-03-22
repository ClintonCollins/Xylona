import * as path from 'path'
import { execSync } from 'child_process'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..', '..')
const E2E_DIR = import.meta.dirname

// Load .env for custom credentials or port overrides (optional).
try {
  process.loadEnvFile(path.join(import.meta.dirname, '..', '.env'))
} catch {
  // .env is optional
}

const HTTP_PORT = process.env['E2E_HTTP_PORT'] ?? '9091'
const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

export default async function globalTeardown(): Promise<void> {
  console.log('[E2E Teardown] Delegating to Go orchestrator...')
  try {
    execSync(
      `go run ./cmd/e2e single-teardown` +
        ` --http-port ${HTTP_PORT}` +
        ` --admin-username "${ADMIN_USERNAME}"` +
        ` --admin-password "${ADMIN_PASSWORD}"` +
        ` --e2e-dir "${E2E_DIR}"`,
      { cwd: PROJECT_ROOT, stdio: 'inherit', timeout: 60_000 },
    )
  } catch (err) {
    console.warn(`[E2E Teardown] Go orchestrator returned an error (non-fatal): ${err}`)
  }
}
