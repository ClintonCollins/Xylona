import { describe, expect, it } from 'vitest'
import { groupFields, filterFields } from './config-field-helpers'

function makeField(key: string, group: string, title = '', value = '') {
  return { key, group, title, value }
}

describe('groupFields', () => {
  it('groups fields by group value', () => {
    const fields = [
      makeField('port', 'network', 'Port'),
      makeField('motd', 'gameplay', 'MOTD'),
      makeField('ip', 'network', 'IP'),
    ]
    const groups = groupFields(fields)
    expect(groups).toHaveLength(2)
    expect(groups[0].name).toBe('network')
    expect(groups[0].fields).toHaveLength(2)
    expect(groups[1].name).toBe('gameplay')
    expect(groups[1].fields).toHaveLength(1)
  })

  it('puts ungrouped fields in General at the top', () => {
    const fields = [makeField('port', 'network'), makeField('motd', '')]
    const groups = groupFields(fields)
    expect(groups[0].name).toBe('')
    expect(groups[0].displayName).toBe('General')
    expect(groups[1].name).toBe('network')
  })

  it('preserves first-occurrence group order', () => {
    const fields = [
      makeField('difficulty', 'gameplay'),
      makeField('port', 'network'),
      makeField('motd', 'gameplay'),
    ]
    const groups = groupFields(fields)
    expect(groups[0].name).toBe('gameplay')
    expect(groups[1].name).toBe('network')
  })

  it('handles all fields ungrouped', () => {
    const fields = [makeField('port', ''), makeField('motd', '')]
    const groups = groupFields(fields)
    expect(groups).toHaveLength(1)
    expect(groups[0].displayName).toBe('General')
  })

  it('handles empty fields array', () => {
    expect(groupFields([])).toEqual([])
  })
})

describe('filterFields', () => {
  it('filters by key substring', () => {
    const fields = [
      makeField('server-port', '', 'Server Port', '25565'),
      makeField('motd', '', 'MOTD', 'Hello'),
    ]
    const result = filterFields(fields, 'port')
    expect(result).toHaveLength(1)
    expect(result[0].key).toBe('server-port')
  })

  it('filters by title substring', () => {
    const fields = [
      makeField('server-port', '', 'Server Port', '25565'),
      makeField('motd', '', 'Message of the Day', 'Hello'),
    ]
    const result = filterFields(fields, 'message')
    expect(result).toHaveLength(1)
    expect(result[0].key).toBe('motd')
  })

  it('filters by value substring', () => {
    const fields = [
      makeField('motd', '', 'MOTD', 'Hello World'),
      makeField('port', '', 'Port', '25565'),
    ]
    const result = filterFields(fields, 'hello')
    expect(result).toHaveLength(1)
    expect(result[0].key).toBe('motd')
  })

  it('is case-insensitive', () => {
    const fields = [makeField('ServerPort', '', 'Server Port')]
    expect(filterFields(fields, 'serverport')).toHaveLength(1)
    expect(filterFields(fields, 'SERVERPORT')).toHaveLength(1)
  })

  it('returns all fields for empty query', () => {
    const fields = [makeField('a', '', '', ''), makeField('b', '', '', '')]
    expect(filterFields(fields, '')).toHaveLength(2)
  })
})
