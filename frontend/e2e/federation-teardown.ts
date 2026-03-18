import * as path from 'path'
import { execSync } from 'child_process'

const PROJECT_ROOT = path.resolve(import.meta.dirname, '..', '..')
const E2E_DIR = import.meta.dirname
const KEEP_DATA = process.env['E2E_KEEP_DATA'] === '1'

export default async function globalTeardown(): Promise<void> {
  console.log('[Federation Teardown] Delegating to Go orchestrator...')
  try {
    const keepDataFlag = KEEP_DATA ? ' --keep-data' : ''
    execSync(
      `go run ./cmd/e2e federation-teardown` +
        ` --e2e-dir "${E2E_DIR}"` +
        keepDataFlag,
      { cwd: PROJECT_ROOT, stdio: 'inherit', timeout: 60_000 },
    )
  } catch (err) {
    console.warn(`[Federation Teardown] Go orchestrator returned an error (non-fatal): ${err}`)
  }
}
