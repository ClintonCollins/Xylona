import { readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

function readSource(relativeUrl: string): string {
  return readFileSync(fileURLToPath(new URL(relativeUrl, import.meta.url)), 'utf8')
}

describe('game server notification patterns', () => {
  it('keeps alerts and mods on shared notification helpers', () => {
    const sources = [
      ['GameServerAlerts.vue', readSource('./GameServerAlerts.vue')],
      ['GameServerMods.vue', readSource('./GameServerMods.vue')],
    ] as const

    for (const [name, source] of sources) {
      expect(source, `${name} should not use raw $q.notify`).not.toContain('$q.notify({')
      expect(source, `${name} should not construct ConnectError inline`).not.toContain(
        'ConnectError.from(',
      )
    }
  })
})
