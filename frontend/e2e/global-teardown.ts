import * as path from 'path'
import { execFileSync } from 'child_process'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..', '..')
const E2E_DIR = import.meta.dirname

// Load .env for custom credentials or port overrides (optional).
try {
  process.loadEnvFile(path.join(import.meta.dirname, '..', '.env'))
} catch {
  // .env is optional
}

const E2E_MODE = process.env['E2E_MODE'] ?? 'local-controller'

export default async function globalTeardown(): Promise<void> {
  console.log(`[E2E Teardown] Delegating to Go orchestrator (${E2E_MODE})...`)
  try {
    execFileSync('go', ['run', './cmd/e2e', 'teardown', '--mode', E2E_MODE, '--e2e-dir', E2E_DIR], {
      cwd: PROJECT_ROOT,
      stdio: 'inherit',
      timeout: 60_000,
    })
  } catch (err) {
    console.warn(`[E2E Teardown] Go orchestrator returned an error (non-fatal): ${err}`)
  }
}
