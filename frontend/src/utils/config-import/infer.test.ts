import { describe, expect, it } from 'vitest'
import { coerceValue, groupToTitle, inferType, keyToTitle } from './infer'

describe('inferType', () => {
  it('detects native booleans', () => {
    expect(inferType(true)).toBe('boolean')
    expect(inferType(false)).toBe('boolean')
  })

  it('detects string booleans', () => {
    expect(inferType('true')).toBe('boolean')
    expect(inferType('false')).toBe('boolean')
    expect(inferType('True')).toBe('boolean')
    expect(inferType('FALSE')).toBe('boolean')
  })

  it('detects native integers', () => {
    expect(inferType(42)).toBe('integer')
    expect(inferType(0)).toBe('integer')
    expect(inferType(-5)).toBe('integer')
  })

  it('detects string integers', () => {
    expect(inferType('42')).toBe('integer')
    expect(inferType('0')).toBe('integer')
    expect(inferType('-100')).toBe('integer')
  })

  it('detects native floats as number', () => {
    expect(inferType(3.14)).toBe('number')
    expect(inferType(-0.5)).toBe('number')
  })

  it('detects string floats as number', () => {
    expect(inferType('3.14')).toBe('number')
    expect(inferType('-0.001')).toBe('number')
  })

  it('defaults to string for non-numeric text', () => {
    expect(inferType('hello')).toBe('string')
    expect(inferType('')).toBe('string')
    expect(inferType('abc123')).toBe('string')
  })

  it('defaults to string for null/undefined', () => {
    expect(inferType(null)).toBe('string')
    expect(inferType(undefined)).toBe('string')
  })
})

describe('coerceValue', () => {
  it('coerces string to boolean', () => {
    expect(coerceValue('true', 'boolean')).toBe(true)
    expect(coerceValue('false', 'boolean')).toBe(false)
  })

  it('coerces string to integer', () => {
    expect(coerceValue('42', 'integer')).toBe(42)
    expect(coerceValue('-5', 'integer')).toBe(-5)
  })

  it('coerces string to number', () => {
    expect(coerceValue('3.14', 'number')).toBe(3.14)
  })

  it('returns string as-is for string type', () => {
    expect(coerceValue('hello', 'string')).toBe('hello')
  })

  it('returns native values unchanged', () => {
    expect(coerceValue(42, 'integer')).toBe(42)
    expect(coerceValue(true, 'boolean')).toBe(true)
  })

  it('handles null/undefined', () => {
    expect(coerceValue(null, 'string')).toBeNull()
    expect(coerceValue(undefined, 'integer')).toBeUndefined()
  })
})

describe('keyToTitle', () => {
  it('converts kebab-case to Title Case', () => {
    expect(keyToTitle('max-players')).toBe('Max Players')
  })

  it('converts snake_case to Title Case', () => {
    expect(keyToTitle('server_port')).toBe('Server Port')
  })

  it('converts camelCase to Title Case', () => {
    expect(keyToTitle('maxPlayers')).toBe('Max Players')
    expect(keyToTitle('serverPort')).toBe('Server Port')
  })

  it('uses last segment of dot-path keys', () => {
    expect(keyToTitle('server.network.max-players')).toBe('Max Players')
    expect(keyToTitle('gameplay.difficulty')).toBe('Difficulty')
  })

  it('handles single word', () => {
    expect(keyToTitle('port')).toBe('Port')
  })

  it('handles empty string', () => {
    expect(keyToTitle('')).toBe('')
  })
})

describe('groupToTitle', () => {
  it('capitalizes all segments of a dot-path', () => {
    expect(groupToTitle('server.network')).toBe('Server Network')
  })

  it('converts kebab-case segments', () => {
    expect(groupToTitle('world-gen')).toBe('World Gen')
  })

  it('converts snake_case segments', () => {
    expect(groupToTitle('game_settings')).toBe('Game Settings')
  })

  it('handles single word', () => {
    expect(groupToTitle('network')).toBe('Network')
  })

  it('handles empty string', () => {
    expect(groupToTitle('')).toBe('')
  })

  it('handles camelCase segments', () => {
    expect(groupToTitle('serverNetwork')).toBe('Server Network')
  })
})
