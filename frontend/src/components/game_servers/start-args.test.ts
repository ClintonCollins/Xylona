import { describe, expect, it } from 'vitest'

import { buildPlaceholderVars } from './start-args'

describe('buildPlaceholderVars', () => {
  it('includes SERVER_ID for unique placeholder resolution', () => {
    expect(
      buildPlaceholderVars({
        id: 'server-123',
      }),
    ).toMatchObject({
      SERVER_ID: 'server-123',
    })
  })

  it('includes MAX_MEMORY_MB for memory-based placeholder resolution', () => {
    expect(
      buildPlaceholderVars({
        maxMemoryMb: 4096n,
      }),
    ).toMatchObject({
      MAX_MEMORY_MB: '4096',
    })
  })
})
