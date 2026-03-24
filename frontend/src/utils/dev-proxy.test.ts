import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'

import { afterEach, describe, expect, it } from 'vitest'

import { resolveBackendProxyTarget } from '../../dev-proxy.mjs'

describe('resolveBackendProxyTarget', () => {
  const tempDirs: string[] = []

  afterEach(() => {
    for (const tempDir of tempDirs.splice(0)) {
      rmSync(tempDir, { force: true, recursive: true })
    }
  })

  it('uses the project HTTP_PORT from the root env file when no explicit backend URL is set', () => {
    const tempDir = mkdtempSync(path.join(tmpdir(), 'xylona-dev-proxy-'))
    tempDirs.push(tempDir)
    const envFilePath = path.join(tempDir, '.env')
    writeFileSync(envFilePath, 'HTTP_PORT=9091\n', 'utf8')

    expect(resolveBackendProxyTarget({ processEnv: {}, envFilePath })).toBe('http://localhost:9091')
  })

  it('prefers an explicit backend URL over host and port values', () => {
    expect(
      resolveBackendProxyTarget({
        processEnv: {
          BACKEND_URL: 'http://localhost:9191',
          HOST: '127.0.0.1',
          HTTP_HOST: 'localhost',
          HTTP_PORT: '9091',
        },
      }),
    ).toBe('http://localhost:9191')
  })

  it('prefers HOST over legacy HTTP_HOST when both are provided', () => {
    expect(
      resolveBackendProxyTarget({
        processEnv: {
          HOST: '127.0.0.1',
          HTTP_HOST: 'localhost',
          HTTP_PORT: '9091',
        },
      }),
    ).toBe('http://127.0.0.1:9091')
  })

  it('normalizes wildcard bind hosts to localhost for the proxy target', () => {
    expect(
      resolveBackendProxyTarget({
        processEnv: {
          HOST: '0.0.0.0',
          HTTP_PORT: '9091',
        },
      }),
    ).toBe('http://localhost:9091')
  })
})
