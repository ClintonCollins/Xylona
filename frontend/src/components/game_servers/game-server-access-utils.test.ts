import { describe, expect, it } from 'vitest'

import { formatProtoTimestamp, hostFromBaseURL } from './game-server-access-utils'

describe('hostFromBaseURL', () => {
  it('returns host for full https URL', () => {
    expect(hostFromBaseURL('https://example.com:8443')).toBe('example.com:8443')
  })

  it('returns original host when scheme is missing', () => {
    expect(hostFromBaseURL('example.com:8080')).toBe('example.com:8080')
  })

  it('returns empty string for empty input', () => {
    expect(hostFromBaseURL('')).toBe('')
  })
})

describe('formatProtoTimestamp', () => {
  it('formats a valid timestamp', () => {
    const value = formatProtoTimestamp({ seconds: BigInt(1700000000) })
    expect(value).not.toBe('Unknown time')
  })

  it('returns unknown time for missing value', () => {
    expect(formatProtoTimestamp(undefined)).toBe('Unknown time')
  })
})
