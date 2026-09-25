import { describe, expect, it } from 'vitest'
import { formatDownloads, iconGradient } from './mod-sources'

describe('formatDownloads', () => {
  it.each([
    { downloads: 0n, expected: '0' },
    { downloads: 999n, expected: '999' },
    { downloads: 1000n, expected: '1.0K' },
    { downloads: 999999n, expected: '1000.0K' },
    { downloads: 1000000n, expected: '1.0M' },
  ])('formats $downloads as $expected', ({ downloads, expected }) => {
    expect(formatDownloads(downloads)).toBe(expected)
  })
})

describe('iconGradient', () => {
  it.each([
    { name: '', category: 1 },
    { name: 'abc', category: 3 },
    { name: '😀', category: 4 },
  ])('picks a stable category token for "$name"', ({ name, category }) => {
    const token = `var(--xy-category-${category})`
    expect(iconGradient(name)).toBe(
      `linear-gradient(135deg, color-mix(in srgb, ${token} 60%, var(--xy-base)), color-mix(in srgb, ${token} 40%, var(--xy-base)))`,
    )
  })
})
