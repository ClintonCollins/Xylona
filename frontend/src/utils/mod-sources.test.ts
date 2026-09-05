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
    { name: '', hue1: 0, hue2: 40 },
    { name: 'abc', hue1: 234, hue2: 274 },
    { name: '😀', hue1: 259, hue2: 299 },
  ])('preserves the fallback colors for "$name"', ({ name, hue1, hue2 }) => {
    expect(iconGradient(name)).toBe(
      `linear-gradient(135deg, hsl(${hue1}, 60%, 40%), hsl(${hue2}, 60%, 30%))`,
    )
  })
})
