import * as path from 'path'
import { execSync } from 'child_process'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..', '..')
const E2E_DIR = import.meta.dirname

// Load .env for BACKEND_URL / admin credentials (optional).
try {
  process.loadEnvFile(path.join(import.meta.dirname, '..', '.env'))
} catch {
  // .env is optional — shell env vars or CI secrets take precedence
}

const BACKEND_URL = process.env['BACKEND_URL'] ?? 'http://localhost:8080'
const ADMIN_USERNAME = process.env['E2E_ADMIN_USERNAME'] ?? 'admin'
const ADMIN_PASSWORD = process.env['E2E_ADMIN_PASSWORD'] ?? 'admin'

export default async function globalSetup(): Promise<void> {
  console.log('[E2E Setup] Delegating to Go orchestrator...')
  execSync(
    `go run ./cmd/e2e single-setup` +
      ` --backend-url "${BACKEND_URL}"` +
      ` --admin-username "${ADMIN_USERNAME}"` +
      ` --admin-password "${ADMIN_PASSWORD}"` +
      ` --e2e-dir "${E2E_DIR}"` +
      ` --project-root "${PROJECT_ROOT}"`,
    { cwd: PROJECT_ROOT, stdio: 'inherit', timeout: 120_000 },
  )
}
