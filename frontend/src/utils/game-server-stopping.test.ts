import { describe, expect, it } from 'vitest'

import { Status } from '@/proto/shared_pb'
import { isServerStopping } from '@/utils/game-server-stopping'
import { XylonaEventBus } from '@/utils/shared'

describe('game-server-stopping', () => {
  it('tracks a stop from any view until the process leaves the running states', () => {
    XylonaEventBus.emit('gameServerStopping', 'server-1', true)
    expect(isServerStopping('server-1', Status.ONLINE)).toBe(true)
    expect(isServerStopping('server-2', Status.ONLINE)).toBe(false)

    // Still running while the game saves and shuts down.
    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Alpha', Status.ONLINE)
    expect(isServerStopping('server-1', Status.PRE_START)).toBe(true)

    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Alpha', Status.OFFLINE)
    expect(isServerStopping('server-1', Status.ONLINE)).toBe(false)
  })

  it('never reports an offline server as stopping, even when its exit arrives first', () => {
    XylonaEventBus.emit('gameServerStatus', 'server-3', 'Gamma', Status.OFFLINE)
    XylonaEventBus.emit('gameServerStopping', 'server-3', true)
    expect(isServerStopping('server-3', Status.OFFLINE)).toBe(false)
  })

  it('ends the phase when the stop request fails or the connection is re-established', () => {
    XylonaEventBus.emit('gameServerStopping', 'server-1', true)
    XylonaEventBus.emit('gameServerStopping', 'server-1', false)
    expect(isServerStopping('server-1', Status.ONLINE)).toBe(false)

    XylonaEventBus.emit('gameServerStopping', 'server-2', true)
    XylonaEventBus.emit('websocketConnected')
    expect(isServerStopping('server-2', Status.ONLINE)).toBe(false)
  })
})
