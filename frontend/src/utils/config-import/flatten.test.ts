import { describe, expect, it } from 'vitest'
import { flattenObject } from './flatten'

describe('flattenObject', () => {
  it('flattens nested objects to dot-paths', () => {
    const input = { server: { port: 25565, motd: 'Hello' } }
    const result = flattenObject(input)
    expect(result).toEqual([
      { key: 'server.port', value: 25565 },
      { key: 'server.motd', value: 'Hello' },
    ])
  })

  it('handles deeply nested objects', () => {
    const input = { a: { b: { c: 'deep' } } }
    const result = flattenObject(input)
    expect(result).toEqual([{ key: 'a.b.c', value: 'deep' }])
  })

  it('returns top-level primitives as-is', () => {
    const input = { port: 25565, motd: 'Hello', enabled: true }
    const result = flattenObject(input)
    expect(result).toEqual([
      { key: 'port', value: 25565 },
      { key: 'motd', value: 'Hello' },
      { key: 'enabled', value: true },
    ])
  })

  it('handles empty object', () => {
    const result = flattenObject({})
    expect(result).toEqual([])
  })

  it('collapses uniform primitive arrays to allow-multiple', () => {
    const input = { mods: ['mod_a', 'mod_b', 'mod_c'] }
    const result = flattenObject(input)
    expect(result).toEqual([{ key: 'mods', value: 'mod_a', allowMultiple: true }])
  })

  it('flattens mixed-type arrays to indexed dot-paths', () => {
    const input = { items: ['text', 42] }
    const result = flattenObject(input)
    expect(result).toEqual([
      { key: 'items.0', value: 'text' },
      { key: 'items.1', value: 42 },
    ])
  })

  it('flattens arrays of objects to indexed dot-paths', () => {
    const input = { servers: [{ name: 'a' }, { name: 'b' }] }
    const result = flattenObject(input)
    expect(result).toEqual([
      { key: 'servers.0.name', value: 'a' },
      { key: 'servers.1.name', value: 'b' },
    ])
  })

  it('handles empty arrays', () => {
    const input = { items: [] as unknown[] }
    const result = flattenObject(input)
    expect(result).toEqual([])
  })

  it('handles null and undefined values', () => {
    const input = { a: null, b: undefined }
    const result = flattenObject(input)
    expect(result).toEqual([
      { key: 'a', value: null },
      { key: 'b', value: undefined },
    ])
  })
})
