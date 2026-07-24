import { describe, expect, it } from 'vitest'

import {
  completeConsoleCommandInput,
  matchConsoleCommands,
  type ConsoleCommandSearchEntry,
} from './console-command-matcher'

const commands: ConsoleCommandSearchEntry[] = [
  {
    command: 'whitelist add',
    syntax: 'whitelist add <targets>',
    summary: 'Adds players to the allowlist.',
    category: 'Players',
    aliases: ['allowlist add'],
    keywords: ['access'],
    availability: 'Available when the allowlist is enabled.',
  },
  {
    command: 'save-all',
    syntax: 'save-all [flush]',
    summary: 'Saves server state to disk.',
    category: 'World',
    aliases: ['/save-all'],
  },
  {
    command: 'stop',
    summary: 'Stops the server safely.',
    category: 'Lifecycle',
  },
]
const whitelistCommand = commands.find((command) => command.command === 'whitelist add')
const saveAllCommand = commands.find((command) => command.command === 'save-all')
if (!whitelistCommand || !saveAllCommand) {
  throw new Error('Expected console command matcher fixtures were not created')
}

describe('console command matching and completion', () => {
  it('orders an empty search by category and command', () => {
    expect(matchConsoleCommands(commands, '').map((match) => match.entry.command)).toEqual([
      'stop',
      'whitelist add',
      'save-all',
    ])
  })

  it('ranks canonical prefix matches ahead of metadata matches', () => {
    const matches = matchConsoleCommands(commands, 'sa')

    expect(matches.map((match) => match.entry.command)).toEqual(['save-all', 'stop'])
    expect(matches[0]?.field).toBe('command')
    expect(matches[1]?.field).toBe('summary')
  })

  it('matches aliases and ignores a leading slash', () => {
    const aliasMatch = matchConsoleCommands(commands, 'allowlist a')
    const slashMatch = matchConsoleCommands(commands, '/save')

    expect(aliasMatch[0]?.entry.command).toBe('whitelist add')
    expect(aliasMatch[0]?.field).toBe('alias')
    expect(slashMatch[0]?.entry.command).toBe('save-all')
  })

  it('searches rich command metadata', () => {
    expect(matchConsoleCommands(commands, 'access')[0]?.entry.command).toBe('whitelist add')
    expect(matchConsoleCommands(commands, 'server state')[0]?.entry.command).toBe('save-all')
  })

  it('returns no match for unknown input', () => {
    expect(matchConsoleCommands(commands, 'custom-plugin-command')).toEqual([])
  })

  it('completes canonical command text and preserves arguments', () => {
    expect(completeConsoleCommandInput('white a Steve Alex', whitelistCommand)).toBe(
      'whitelist add Steve Alex',
    )
  })

  it('uses canonical console syntax when completing an alias', () => {
    expect(completeConsoleCommandInput('/save', saveAllCommand)).toBe('save-all ')
    expect(completeConsoleCommandInput('allowlist a Steve', whitelistCommand)).toBe(
      'whitelist add Steve',
    )
  })
})
