import { describe, expect, it } from 'vitest'
import { Status } from '@/proto/shared_pb'
import type { DisplayRow } from './server-list-cache'
import {
  canRestartServer,
  canStartServer,
  canStopServer,
  canUpdateServer,
  getRestartableServers,
  getStartableServers,
  getStoppableServers,
  getUpdateableServers,
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
    version: '1.0.0',
    canUpdate: true,
  }
}

describe('server-list-actions', () => {
  it('canStartServer returns true only for confirmed offline status', () => {
    const tests = [
      { name: 'offline', status: Status.OFFLINE, want: true },
      { name: 'unknown', status: Status.UNKNOWN, want: false },
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

    expect(startableServerIDs).toEqual(['b'])
    expect(stoppableServerIDs).toEqual(['c'])
  })

  it('classifies restart and update eligibility from lifecycle state and capability', () => {
    const tests = [
      {
        name: 'online update-capable server',
        server: buildRow('online', Status.ONLINE),
        wantRestart: true,
        wantUpdate: true,
      },
      {
        name: 'offline update-capable server',
        server: buildRow('offline', Status.OFFLINE),
        wantRestart: false,
        wantUpdate: true,
      },
      {
        name: 'busy update-capable server',
        server: buildRow('busy', Status.UPDATING),
        wantRestart: false,
        wantUpdate: false,
      },
      {
        name: 'online server without update provider',
        server: { ...buildRow('unsupported', Status.ONLINE), canUpdate: false },
        wantRestart: true,
        wantUpdate: false,
      },
    ]

    for (const tt of tests) {
      expect(canRestartServer(tt.server.statusEnum), tt.name).toBe(tt.wantRestart)
      expect(canUpdateServer(tt.server), tt.name).toBe(tt.wantUpdate)
    }

    expect(getRestartableServers(tests.map((tt) => tt.server)).map((server) => server.id)).toEqual([
      'online',
      'unsupported',
    ])
    expect(getUpdateableServers(tests.map((tt) => tt.server)).map((server) => server.id)).toEqual([
      'online',
      'offline',
    ])
  })
})
