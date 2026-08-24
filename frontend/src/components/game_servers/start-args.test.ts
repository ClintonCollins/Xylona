import { describe, expect, it } from 'vitest'

import { buildPlaceholderVars, resolveStartCommandBase } from './start-args'

describe('buildPlaceholderVars', () => {
  it('includes SERVER_ID for unique placeholder resolution', () => {
    expect(
      buildPlaceholderVars({
        id: 'server-123',
      }),
    ).toMatchObject({
      SERVER_ID: 'server-123',
    })
  })

  it('includes MAX_MEMORY_MB for memory-based placeholder resolution', () => {
    expect(
      buildPlaceholderVars({
        maxMemoryMb: 4096n,
      }),
    ).toMatchObject({
      MAX_MEMORY_MB: '4096',
    })
  })

  it('includes derived and runtime placeholder values', () => {
    expect(
      buildPlaceholderVars({
        port: 27015n,
        queryPort: 27020n,
        backupDirectory: '/srv/backups',
        setMaxPlayers: 12n,
      }),
    ).toMatchObject({
      PORT_PLUS_1: '27016',
      PORT_PLUS_2: '27017',
      QUERY_PORT_PLUS_1: '27021',
      BACKUP_DIR: '/srv/backups',
      SET_PLAYERS: '12',
    })
  })
})

describe('resolveStartCommandBase', () => {
  const variables = {
    INSTALL_DIR: '/srv/game',
    SERVER_ID: 'server-123',
    BACKUP_DIR: '/srv/backups',
    SERVER_NAME: 'Test server',
    IP: '127.0.0.1',
    PORT: '27015',
    QUERY_PORT: '27020',
    MAX_MEMORY_MB: '4096',
    MAX_PLAYERS: '16',
    SET_PLAYERS: '12',
  }

  it.each([
    ['%GAMESERVER_DIRECTORY%', '/srv/game'],
    ['%GAMESERVER_ID%', 'server-123'],
    ['%GAMESERVER_BACKUP_DIRECTORY%', '/srv/backups'],
    ['%GAMESERVER_NAME%', 'Test server'],
    ['%GAMESERVER_IP%', '127.0.0.1'],
    ['%GAMESERVER_PORT%', '27015'],
    ['%GAMESERVER_QUERY_PORT%', '27020'],
    ['%GAMESERVER_MAX_MEMORY_MB%', '4096'],
    ['%GAMESERVER_MAX_PLAYERS%', '16'],
    ['%GAMESERVER_SET_PLAYERS%', '12'],
  ])('resolves legacy placeholder %s', (placeholder, expected) => {
    expect(resolveStartCommandBase(placeholder, variables)).toBe(expected)
  })
})
