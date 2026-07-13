import { describe, expect, it } from 'vitest'

import { resolveConsoleStreamChunk } from './console-stream-sequence'

describe('resolveConsoleStreamChunk', () => {
  it('replaces on reset, ignores replayed sequences, and still appends legacy chunks', () => {
    const cases = [
      {
        name: 'legacy chunk',
        lastSequence: 5n,
        chunk: { sequence: 0n, reset: false },
        expected: { action: 'append', nextSequence: 5n },
      },
      {
        name: 'older replay',
        lastSequence: 5n,
        chunk: { sequence: 4n, reset: false },
        expected: { action: 'ignore', nextSequence: 5n },
      },
      {
        name: 'duplicate replay',
        lastSequence: 5n,
        chunk: { sequence: 5n, reset: false },
        expected: { action: 'ignore', nextSequence: 5n },
      },
      {
        name: 'new live output',
        lastSequence: 5n,
        chunk: { sequence: 6n, reset: false },
        expected: { action: 'append', nextSequence: 6n },
      },
      {
        name: 'atomic replay reset',
        lastSequence: 6n,
        chunk: { sequence: 12n, reset: true },
        expected: { action: 'replace', nextSequence: 12n },
      },
    ] as const

    for (const testCase of cases) {
      expect(
        resolveConsoleStreamChunk(testCase.lastSequence, testCase.chunk),
        testCase.name,
      ).toEqual(testCase.expected)
    }
  })
})
