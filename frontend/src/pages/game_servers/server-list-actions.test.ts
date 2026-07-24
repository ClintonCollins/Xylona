import { describe, expect, it } from 'vitest'
import { Status } from '@/proto/shared_pb'
import type { DisplayRow } from './server-list-cache'
import {
  buildLifecycleConfirmation,
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

  it('buildLifecycleConfirmation requires a confirm only when players are online', () => {
    const tests = [
      {
        name: 'stop single server with no players is instant',
        action: 'stop' as const,
        servers: [{ displayName: 'Alpha', playerCount: 0 }],
        want: null,
      },
      {
        name: 'restart across servers with no players is instant',
        action: 'restart' as const,
        servers: [
          { displayName: 'Alpha', playerCount: 0 },
          { displayName: 'Beta', playerCount: 0 },
        ],
        want: null,
      },
      {
        name: 'no servers is instant',
        action: 'stop' as const,
        servers: [],
        want: null,
      },
      {
        name: 'stop single server with players names the count',
        action: 'stop' as const,
        servers: [{ displayName: 'Alpha', playerCount: 3 }],
        want: {
          title: 'Stop Alpha?',
          message: '3 players are online and will be disconnected.',
          confirmLabel: 'Stop server',
          confirmColor: 'negative',
        },
      },
      {
        name: 'stop single server with one player uses singular copy',
        action: 'stop' as const,
        servers: [{ displayName: 'Alpha', playerCount: 1 }],
        want: {
          title: 'Stop Alpha?',
          message: '1 player is online and will be disconnected.',
          confirmLabel: 'Stop server',
          confirmColor: 'negative',
        },
      },
      {
        name: 'restart single server with players warns about the restart window',
        action: 'restart' as const,
        servers: [{ displayName: 'Beta', playerCount: 2 }],
        want: {
          title: 'Restart Beta?',
          message: '2 players are online and will be disconnected while the server restarts.',
          confirmLabel: 'Restart server',
          confirmColor: 'warning',
        },
      },
      {
        name: 'bulk stop names total players and server count',
        action: 'stop' as const,
        servers: [
          { displayName: 'Alpha', playerCount: 2 },
          { displayName: 'Beta', playerCount: 0 },
          { displayName: 'Gamma', playerCount: 3 },
        ],
        want: {
          title: 'Stop 3 servers?',
          message: '5 players are online across 3 servers and will be disconnected.',
          confirmLabel: 'Stop servers',
          confirmColor: 'negative',
        },
      },
      {
        name: 'bulk restart names total players and server count',
        action: 'restart' as const,
        servers: [
          { displayName: 'Alpha', playerCount: 1 },
          { displayName: 'Beta', playerCount: 0 },
        ],
        want: {
          title: 'Restart 2 servers?',
          message:
            '1 player is online across 2 servers and will be disconnected while the servers restart.',
          confirmLabel: 'Restart servers',
          confirmColor: 'warning',
        },
      },
    ]

    for (const tt of tests) {
      const got = buildLifecycleConfirmation(tt.action, tt.servers)
      expect(got, tt.name).toEqual(tt.want)
    }
  })
})
