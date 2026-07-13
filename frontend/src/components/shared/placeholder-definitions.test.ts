import { describe, expect, it } from 'vitest'

import { getManagedSourceLabel } from './placeholder-definitions'

describe('local console managed source labels', () => {
  it('describes the controller-owned 7 Days to Die fields', () => {
    expect(getManagedSourceLabel('xylona.local_console_enabled')).toBe('Local Management Console')
    expect(getManagedSourceLabel('xylona.local_console_password')).toBe(
      'Local-only Console Password',
    )
  })
})
