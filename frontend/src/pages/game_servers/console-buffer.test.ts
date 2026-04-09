import { describe, expect, it } from 'vitest'

import { splitConsoleChunk, trimConsoleLines } from './console-buffer'

describe('console-buffer', () => {
  it('splits parsed console chunks on line boundaries while keeping newlines', () => {
    expect(splitConsoleChunk('first line\nsecond line\nthird line')).toEqual([
      'first line\n',
      'second line\n',
      'third line',
    ])
  })

  it('drops whole oldest entries when the console exceeds the max size', () => {
    const result = trimConsoleLines(
      [
        { id: 1, html: '12345' },
        { id: 2, html: '67890' },
        { id: 3, html: 'abc' },
      ],
      8,
    )

    expect(result).toEqual({
      lines: [
        { id: 2, html: '67890' },
        { id: 3, html: 'abc' },
      ],
      totalChars: 8,
      truncated: true,
    })
  })

  it('drops a single oversized HTML entry instead of slicing through tags', () => {
    const result = trimConsoleLines(
      [
        {
          id: 1,
          html: "<span class='text-red-5'>ERROR</span>".repeat(16),
        },
      ],
      100,
    )

    expect(result).toEqual({
      lines: [],
      totalChars: 0,
      truncated: true,
    })
  })
})
