import { describe, expect, it } from 'vitest'
import { filterFields, groupFields } from './config-field-helpers'

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
    expect(groups[0]?.name).toBe('network')
    expect(groups[0]?.fields).toHaveLength(2)
    expect(groups[1]?.name).toBe('gameplay')
    expect(groups[1]?.fields).toHaveLength(1)
  })

  it('puts ungrouped fields in General at the top', () => {
    const fields = [makeField('port', 'network'), makeField('motd', '')]
    const groups = groupFields(fields)
    expect(groups[0]?.name).toBe('')
    expect(groups[0]?.displayName).toBe('General')
    expect(groups[1]?.name).toBe('network')
  })

  it('preserves first-occurrence group order', () => {
    const fields = [
      makeField('difficulty', 'gameplay'),
      makeField('port', 'network'),
      makeField('motd', 'gameplay'),
    ]
    const groups = groupFields(fields)
    expect(groups[0]?.name).toBe('gameplay')
    expect(groups[1]?.name).toBe('network')
  })

  it('handles all fields ungrouped', () => {
    const fields = [makeField('port', ''), makeField('motd', '')]
    const groups = groupFields(fields)
    expect(groups).toHaveLength(1)
    expect(groups[0]?.displayName).toBe('General')
  })

  it('handles empty fields array', () => {
    expect(groupFields([])).toEqual([])
  })

  it('respects explicit groupOrder parameter', () => {
    const fields = [
      makeField('difficulty', 'gameplay'),
      makeField('port', 'network'),
      makeField('threads', 'performance'),
    ]
    const groups = groupFields(fields, ['network', 'performance', 'gameplay'])
    expect(groups.map((g) => g.name)).toEqual(['network', 'performance', 'gameplay'])
  })

  it('puts General first even with explicit groupOrder', () => {
    const fields = [makeField('motd', ''), makeField('port', 'network')]
    const groups = groupFields(fields, ['network'])
    expect(groups[0]?.name).toBe('')
    expect(groups[0]?.displayName).toBe('General')
    expect(groups[1]?.name).toBe('network')
  })

  it('appends unlisted groups after explicitly ordered ones', () => {
    const fields = [makeField('a', 'alpha'), makeField('b', 'beta'), makeField('g', 'gamma')]
    const groups = groupFields(fields, ['beta'])
    expect(groups[0]?.name).toBe('beta')
    expect(groups.slice(1).map((g) => g.name)).toEqual(['alpha', 'gamma'])
  })

  it('ignores empty groupOrder array (fallback to first-occurrence)', () => {
    const fields = [
      makeField('difficulty', 'gameplay'),
      makeField('port', 'network'),
      makeField('motd', 'gameplay'),
    ]
    const withEmpty = groupFields(fields, [])
    const withoutParam = groupFields(fields)
    expect(withEmpty.map((g) => g.name)).toEqual(withoutParam.map((g) => g.name))
  })

  it('filters out groups in groupOrder that have no fields', () => {
    const fields = [makeField('port', 'network')]
    const groups = groupFields(fields, ['missing', 'network'])
    expect(groups).toHaveLength(1)
    expect(groups[0]?.name).toBe('network')
  })

  it('handles all fields in one group with groupOrder', () => {
    const fields = [makeField('port', 'net'), makeField('ip', 'net')]
    const groups = groupFields(fields, ['net'])
    expect(groups).toHaveLength(1)
    expect(groups[0]?.name).toBe('net')
    expect(groups[0]?.fields).toHaveLength(2)
  })
})

describe('filterFields', () => {
  it.each([
    {
      name: 'filters by key substring',
      fields: [
        makeField('server-port', '', 'Server Port', '25565'),
        makeField('motd', '', 'MOTD', 'Hello'),
      ],
      query: 'port',
      wantKeys: ['server-port'],
    },
    {
      name: 'filters by title substring',
      fields: [
        makeField('server-port', '', 'Server Port', '25565'),
        makeField('motd', '', 'Message of the Day', 'Hello'),
      ],
      query: 'message',
      wantKeys: ['motd'],
    },
    {
      name: 'filters by value substring',
      fields: [
        makeField('motd', '', 'MOTD', 'Hello World'),
        makeField('port', '', 'Port', '25565'),
      ],
      query: 'hello',
      wantKeys: ['motd'],
    },
    {
      name: 'is case-insensitive',
      fields: [makeField('ServerPort', '', 'Server Port')],
      query: 'SERVERPORT',
      wantKeys: ['ServerPort'],
    },
    {
      name: 'returns all fields for empty query',
      fields: [makeField('a', '', '', ''), makeField('b', '', '', '')],
      query: '',
      wantKeys: ['a', 'b'],
    },
  ])('$name', ({ fields, query, wantKeys }) => {
    expect(filterFields(fields, query).map((field) => field.key)).toEqual(wantKeys)
  })
})
