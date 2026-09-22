import { describe, expect, it } from 'vitest'

import { Status } from '@/proto/shared_pb'

import { detectStartFailure, processExitedMessage, startFailureWindowMs } from './start-failure'

describe('detectStartFailure', () => {
  const t0 = 1_000_000

  it('flags an offline transition shortly after a start request', () => {
    const result = detectStartFailure(
      Status.OFFLINE,
      { startRequestedAt: t0, stopRequestedAt: 0 },
      t0 + 5_000,
    )
    expect(result).toEqual({ at: t0 + 5_000, message: processExitedMessage })
  })

  it('clears the failure once the server comes online', () => {
    expect(
      detectStartFailure(Status.ONLINE, { startRequestedAt: t0, stopRequestedAt: 0 }, t0 + 1),
    ).toBeNull()
  })

  it('ignores an offline transition the operator asked for', () => {
    expect(
      detectStartFailure(
        Status.OFFLINE,
        { startRequestedAt: t0, stopRequestedAt: t0 + 10 },
        t0 + 20,
      ),
    ).toBeUndefined()
  })

  it('ignores an offline transition outside the start window or with no start', () => {
    expect(
      detectStartFailure(
        Status.OFFLINE,
        { startRequestedAt: t0, stopRequestedAt: 0 },
        t0 + startFailureWindowMs + 1,
      ),
    ).toBeUndefined()
    expect(
      detectStartFailure(Status.OFFLINE, { startRequestedAt: 0, stopRequestedAt: 0 }, t0),
    ).toBeUndefined()
    expect(
      detectStartFailure(Status.PRE_START, { startRequestedAt: t0, stopRequestedAt: 0 }, t0 + 1),
    ).toBeUndefined()
  })
})
