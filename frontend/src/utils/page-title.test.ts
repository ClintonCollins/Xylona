import { describe, expect, it } from 'vitest'

import { formatPageTitle } from './page-title'

describe('formatPageTitle', () => {
  it.each([
    { parts: [], want: 'Xylona' },
    { parts: ['Game servers'], want: 'Game servers · Xylona' },
    { parts: ['Survival', 'Backups'], want: 'Survival · Backups · Xylona' },
    { parts: ['', undefined, ' Files '], want: 'Files · Xylona' },
  ])('formats $parts as $want', ({ parts, want }) => {
    expect(formatPageTitle(...parts)).toBe(want)
  })
})
