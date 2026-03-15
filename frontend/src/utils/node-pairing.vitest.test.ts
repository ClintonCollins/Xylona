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

  it('throws on unsupported protocol', () => {
    expect(() => normalizeNodePairingBaseURL('ftp://example.com')).toThrow(
      'Base URL must use http or https',
    )
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

  it('throws for missing secret key', () => {
    expect(() =>
      parseNodePairingPayload('{"base_url":"https://example.com","secret_key":""}'),
    ).toThrow('secret_key is required')
  })
})
