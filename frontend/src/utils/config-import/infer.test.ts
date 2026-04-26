import { describe, expect, it } from 'vitest'
import { coerceValue, groupToTitle, inferType, keyToTitle } from './infer'

describe('inferType', () => {
  it.each([
    [true, 'boolean'],
    [false, 'boolean'],
    ['true', 'boolean'],
    ['FALSE', 'boolean'],
    [42, 'integer'],
    [0, 'integer'],
    [-5, 'integer'],
    ['42', 'integer'],
    ['-100', 'integer'],
    [3.14, 'number'],
    [-0.5, 'number'],
    ['3.14', 'number'],
    ['-0.001', 'number'],
    ['hello', 'string'],
    ['', 'string'],
    ['abc123', 'string'],
    [null, 'string'],
    [undefined, 'string'],
  ])('infers %s as %s', (input, want) => {
    expect(inferType(input)).toBe(want)
  })
})

describe('coerceValue', () => {
  it.each([
    ['true', 'boolean', true],
    ['false', 'boolean', false],
    ['42', 'integer', 42],
    ['-5', 'integer', -5],
    ['3.14', 'number', 3.14],
    ['hello', 'string', 'hello'],
    [42, 'integer', 42],
    [true, 'boolean', true],
    [null, 'string', null],
    [undefined, 'integer', undefined],
  ])('coerces %s as %s', (input, type, want) => {
    expect(coerceValue(input, type as Parameters<typeof coerceValue>[1])).toBe(want)
  })
})

describe('keyToTitle', () => {
  it.each([
    ['max-players', 'Max Players'],
    ['server_port', 'Server Port'],
    ['maxPlayers', 'Max Players'],
    ['serverPort', 'Server Port'],
    ['server.network.max-players', 'Max Players'],
    ['gameplay.difficulty', 'Difficulty'],
    ['port', 'Port'],
    ['', ''],
  ])('formats %s', (input, want) => {
    expect(keyToTitle(input)).toBe(want)
  })
})

describe('groupToTitle', () => {
  it.each([
    ['server.network', 'Server Network'],
    ['world-gen', 'World Gen'],
    ['game_settings', 'Game Settings'],
    ['network', 'Network'],
    ['', ''],
    ['serverNetwork', 'Server Network'],
  ])('formats %s', (input, want) => {
    expect(groupToTitle(input)).toBe(want)
  })
})
