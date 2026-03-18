import * as path from 'path'
import { execSync } from 'child_process'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..', '..')
const E2E_DIR = import.meta.dirname

export default async function globalSetup(): Promise<void> {
  console.log('[Federation Setup] Delegating to Go orchestrator...')
  execSync(
    `go run ./cmd/e2e federation-setup` +
      ` --e2e-dir "${E2E_DIR}"` +
      ` --project-root "${PROJECT_ROOT}"`,
    { cwd: PROJECT_ROOT, stdio: 'inherit', timeout: 300_000 },
  )
}
