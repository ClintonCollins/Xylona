import { describe, expect, it } from 'vitest'

import { Status } from '@/proto/shared_pb'
import { isServerStopping } from '@/utils/game-server-stopping'
import { XylonaEventBus } from '@/utils/shared'

describe('game-server-stopping', () => {
  it('tracks a stop from any view until the process leaves the running states', () => {
    XylonaEventBus.emit('gameServerStopping', 'server-1', true)
    expect(isServerStopping('server-1')).toBe(true)
    expect(isServerStopping('server-2')).toBe(false)

    // Still running while the game saves and shuts down.
    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Alpha', Status.ONLINE)
    expect(isServerStopping('server-1')).toBe(true)

    XylonaEventBus.emit('gameServerStatus', 'server-1', 'Alpha', Status.OFFLINE)
    expect(isServerStopping('server-1')).toBe(false)
  })

  it('ends the phase when the stop request fails or the connection is re-established', () => {
    XylonaEventBus.emit('gameServerStopping', 'server-1', true)
    XylonaEventBus.emit('gameServerStopping', 'server-1', false)
    expect(isServerStopping('server-1')).toBe(false)

    XylonaEventBus.emit('gameServerStopping', 'server-2', true)
    XylonaEventBus.emit('websocketConnected')
    expect(isServerStopping('server-2')).toBe(false)
  })
})
