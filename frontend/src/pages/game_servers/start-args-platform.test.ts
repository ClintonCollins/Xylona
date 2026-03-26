import { describe, expect, it } from 'vitest'

import { resolveStartArgsPlatform } from './start-args-platform'

describe('resolveStartArgsPlatform', () => {
  it('returns windows when node OS is windows', () => {
    expect(resolveStartArgsPlatform('windows', true, true)).toBe('windows')
  })

  it('returns linux when node OS is linux', () => {
    expect(resolveStartArgsPlatform('linux', true, true)).toBe('linux')
  })

  it('returns the single configured platform when node OS is unknown', () => {
    expect(resolveStartArgsPlatform('', true, false)).toBe('linux')
    expect(resolveStartArgsPlatform('', false, true)).toBe('windows')
  })

  it('returns null when node OS is unknown and both platforms are configured', () => {
    expect(resolveStartArgsPlatform('', true, true)).toBeNull()
  })
})
