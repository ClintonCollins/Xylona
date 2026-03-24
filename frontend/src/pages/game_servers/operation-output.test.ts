import { describe, expect, it } from 'vitest'
import {
  appendOperationOutputLines,
  normalizeOperationOutputChunk,
  resolveOperationOutputRoute,
} from './operation-output'

describe('operation-output', () => {
  it('normalizes chunks into plain text lines', () => {
    expect(
      normalizeOperationOutputChunk(
        '\u001b[36m[Xylona]\u001b[0m Downloading update\r\n[Xylona] Applying update\r\n\r\n',
      ),
    ).toEqual(['[Xylona] Downloading update', '[Xylona] Applying update'])
  })

  it('appends lines and caps the total number retained', () => {
    const next = appendOperationOutputLines(['line 1', 'line 2'], 'line 3\nline 4\n', 3)
    expect(next).toEqual(['line 2', 'line 3', 'line 4'])
  })

  it('routes offline output away from the console and into the active dialog', () => {
    expect(
      resolveOperationOutputRoute({
        isServerOffline: false,
        updateRequested: true,
        updateInProgress: false,
        softwareOperationInProgress: false,
      }),
    ).toBe('update')

    expect(
      resolveOperationOutputRoute({
        isServerOffline: true,
        updateInProgress: true,
        updateRequested: false,
        softwareOperationInProgress: false,
      }),
    ).toBe('update')

    expect(
      resolveOperationOutputRoute({
        isServerOffline: true,
        updateInProgress: false,
        updateRequested: false,
        softwareOperationInProgress: true,
      }),
    ).toBe('software')

    expect(
      resolveOperationOutputRoute({
        isServerOffline: true,
        updateInProgress: false,
        updateRequested: false,
        softwareOperationInProgress: false,
      }),
    ).toBe('discard')
  })
})
