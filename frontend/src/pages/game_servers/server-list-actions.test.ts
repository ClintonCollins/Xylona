import { describe, expect, it } from 'vitest'
import { Status } from '@/proto/shared_pb'
import type { DisplayRow } from './server-list-cache'
import {
  canStartServer,
  canStopServer,
  getStartableServers,
  getStoppableServers,
} from './server-list-actions'

function buildRow(id: string, status: Status): DisplayRow {
  return {
    compositeId: `local/${id}`,
    id,
    isLocal: true,
    displayName: `Server ${id}`,
    gameName: 'Minecraft',
    userName: 'User',
    statusEnum: status,
    nodeName: 'Local',
    isStale: false,
    sourceNodeId: '',
  }
}

describe('server-list-actions', () => {
  it('canStartServer returns true for offline and unknown statuses', () => {
    const tests = [
      { name: 'offline', status: Status.OFFLINE, want: true },
      { name: 'unknown', status: Status.UNKNOWN, want: true },
      { name: 'online', status: Status.ONLINE, want: false },
      { name: 'installing', status: Status.INSTALLING, want: false },
      { name: 'updating', status: Status.UPDATING, want: false },
    ]

    for (const tt of tests) {
      const got = canStartServer(tt.status)
      expect(got, tt.name).toBe(tt.want)
    }
  })

  it('canStopServer returns true only for online status', () => {
    const tests = [
      { name: 'online', status: Status.ONLINE, want: true },
      { name: 'offline', status: Status.OFFLINE, want: false },
      { name: 'unknown', status: Status.UNKNOWN, want: false },
      { name: 'installing', status: Status.INSTALLING, want: false },
      { name: 'updating', status: Status.UPDATING, want: false },
    ]

    for (const tt of tests) {
      const got = canStopServer(tt.status)
      expect(got, tt.name).toBe(tt.want)
    }
  })

  it('getStartableServers and getStoppableServers filter correctly', () => {
    const servers: DisplayRow[] = [
      buildRow('a', Status.UNKNOWN),
      buildRow('b', Status.OFFLINE),
      buildRow('c', Status.ONLINE),
      buildRow('d', Status.UPDATING),
    ]

    const startableServerIDs = getStartableServers(servers).map((server) => server.id)
    const stoppableServerIDs = getStoppableServers(servers).map((server) => server.id)

    expect(startableServerIDs).toEqual(['a', 'b'])
    expect(stoppableServerIDs).toEqual(['c'])
  })
})
