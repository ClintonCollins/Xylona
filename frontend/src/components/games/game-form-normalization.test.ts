import { describe, expect, it } from 'vitest'

import { normalizeSteamAppID } from './game-form-normalization'

describe('normalizeSteamAppID', () => {
  it('keeps string app ids as trimmed strings', () => {
    expect(normalizeSteamAppID(' 294420 ')).toBe('294420')
  })

  it('converts numeric app ids into strings', () => {
    expect(normalizeSteamAppID(294420)).toBe('294420')
    expect(normalizeSteamAppID(294420n)).toBe('294420')
  })

  it('falls back to an empty string for unsupported values', () => {
    expect(normalizeSteamAppID(undefined)).toBe('')
    expect(normalizeSteamAppID(null)).toBe('')
    expect(normalizeSteamAppID({})).toBe('')
  })
})
