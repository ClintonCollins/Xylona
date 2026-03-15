import { describe, expect, it } from 'vitest'

import {
  createNodePairingPayload,
  normalizeNodePairingBaseURL,
  parseNodePairingPayload,
} from './node-pairing'

describe('normalizeNodePairingBaseURL', () => {
  it('trims whitespace and trailing slash', () => {
    expect(normalizeNodePairingBaseURL(' https://example.com/ ')).toBe('https://example.com')
  })

  it('throws when base URL is empty after trimming', () => {
    expect(() => normalizeNodePairingBaseURL('   ')).toThrow('Base URL is required')
  })

  it('throws on unsupported protocol', () => {
    expect(() => normalizeNodePairingBaseURL('ftp://example.com')).toThrow(
      'Base URL must use http or https',
    )
  })

  it('throws on malformed URLs', () => {
    expect(() => normalizeNodePairingBaseURL('http://')).toThrow('Base URL must be a valid URL')
  })
})

describe('createNodePairingPayload', () => {
  it('creates compact pairing json fields', () => {
    const payloadText = createNodePairingPayload('https://example.com/', ' secret ')
    const payload = JSON.parse(payloadText) as { base_url: string; secret_key: string }
    expect(payload).toEqual({
      base_url: 'https://example.com',
      secret_key: 'secret',
    })
  })

  it('throws for empty secret key', () => {
    expect(() => createNodePairingPayload('https://example.com', '   ')).toThrow(
      'Secret key is required',
    )
  })
})

describe('parseNodePairingPayload', () => {
  it('parses and normalizes payload', () => {
    const parsed = parseNodePairingPayload(
      '{"base_url":"https://example.com/","secret_key":" abc "}',
    )
    expect(parsed).toEqual({
      base_url: 'https://example.com',
      secret_key: 'abc',
    })
  })

  it('throws for invalid json payload', () => {
    expect(() => parseNodePairingPayload('not-json')).toThrow('Pairing JSON is invalid')
  })

  it('throws for non-object payloads', () => {
    expect(() => parseNodePairingPayload('["not","object"]')).toThrow(
      'Pairing JSON must be an object',
    )
  })

  it('throws when base_url is missing', () => {
    expect(() => parseNodePairingPayload('{"secret_key":"abc"}')).toThrow(
      'Base URL is required',
    )
  })

  it('throws for missing secret key', () => {
    expect(() =>
      parseNodePairingPayload('{"base_url":"https://example.com","secret_key":""}'),
    ).toThrow('secret_key is required')
  })
})
