import { describe, expect, it } from 'vitest'

import { formatProtoTimestamp } from './game-server-access-utils'

describe('formatProtoTimestamp', () => {
  it('formats a valid timestamp', () => {
    const value = formatProtoTimestamp({ seconds: BigInt(1700000000) })
    expect(value).not.toBe('Unknown time')
  })

  it('returns unknown time for missing value', () => {
    expect(formatProtoTimestamp(undefined)).toBe('Unknown time')
  })

  it('formats epoch zero without returning unknown time', () => {
    const value = formatProtoTimestamp({ seconds: BigInt(0) })
    expect(value).not.toBe('Unknown time')
  })
})
