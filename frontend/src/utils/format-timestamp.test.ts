import { timestampFromDate } from '@bufbuild/protobuf/wkt'
import { describe, expect, it } from 'vitest'

import { formatDate, formatTimestamp } from './format-timestamp'

describe('formatTimestamp', () => {
  it.each([
    {
      name: 'formats a normal date in 24-hour time',
      input: new Date(2026, 6, 21, 9, 8, 7),
      want: 'Jul 21, 2026 09:08:07',
    },
    {
      name: 'formats midnight as 00:00:00',
      input: new Date(2026, 0, 1, 0, 0, 0),
      want: 'Jan 1, 2026 00:00:00',
    },
    {
      name: 'formats afternoon hours without an AM/PM artifact',
      input: new Date(2026, 2, 5, 17, 32, 10),
      want: 'Mar 5, 2026 17:32:10',
    },
    {
      name: 'formats a protobuf Timestamp',
      input: timestampFromDate(new Date(2026, 11, 31, 23, 59, 59)),
      want: 'Dec 31, 2026 23:59:59',
    },
    {
      name: 'returns empty string for undefined by default',
      input: undefined,
      want: '',
    },
    {
      name: 'returns the fallback for undefined when one is given',
      input: undefined,
      fallback: '-',
      want: '-',
    },
    {
      name: 'returns the fallback for an invalid date',
      input: new Date('not a date'),
      fallback: '-',
      want: '-',
    },
  ])('$name', ({ input, fallback, want }) => {
    if (fallback === undefined) {
      expect(formatTimestamp(input)).toBe(want)
    } else {
      expect(formatTimestamp(input, fallback)).toBe(want)
    }
  })

  it('never emits AM/PM markers in the canonical format', () => {
    expect(formatTimestamp(new Date(2026, 2, 5, 17, 32, 10))).not.toMatch(/AM|PM/)
  })

  it.each([
    { name: 'formats a normal date', input: new Date(2026, 6, 21, 9, 8, 7), want: 'Jul 21, 2026' },
    { name: 'returns empty string for undefined', input: undefined, want: '' },
    {
      name: 'returns the fallback for an invalid date',
      input: new Date('not a date'),
      fallback: '-',
      want: '-',
    },
  ])('formatDate $name', ({ input, fallback, want }) => {
    if (fallback === undefined) {
      expect(formatDate(input)).toBe(want)
    } else {
      expect(formatDate(input, fallback)).toBe(want)
    }
  })
})
