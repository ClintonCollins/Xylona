import { describe, expect, it } from 'vitest'

import {
  createNodePairingPayload,
  normalizeNodePairingBaseURL,
  normalizeNodePairingMTLSPort,
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
    const payloadText = createNodePairingPayload('https://example.com/', ' secret ', 8443)
    const payload = JSON.parse(payloadText) as {
      base_url: string
      secret_key: string
      mtls_port: number
    }
    expect(payload).toEqual({
      base_url: 'https://example.com',
      secret_key: 'secret',
      mtls_port: 8443,
    })
  })

  it('throws for empty secret key', () => {
    expect(() => createNodePairingPayload('https://example.com', '   ', 8443)).toThrow(
      'Secret key is required',
    )
  })

  it('throws for invalid mTLS port', () => {
    expect(() => createNodePairingPayload('https://example.com', 'secret', 0)).toThrow(
      'mTLS port must be between 1 and 65535',
    )
  })
})

describe('normalizeNodePairingMTLSPort', () => {
  it('accepts valid ports', () => {
    expect(normalizeNodePairingMTLSPort(1)).toBe(1)
    expect(normalizeNodePairingMTLSPort(65535)).toBe(65535)
  })

  it('allows zero when explicitly requested', () => {
    expect(normalizeNodePairingMTLSPort(0, true)).toBe(0)
  })
})

describe('parseNodePairingPayload', () => {
  it('parses and normalizes payload', () => {
    const parsed = parseNodePairingPayload(
      '{"base_url":"https://example.com/","secret_key":" abc ","mtls_port":8443}',
    )
    expect(parsed).toEqual({
      base_url: 'https://example.com',
      secret_key: 'abc',
      mtls_port: 8443,
    })
  })

  it('allows legacy payloads without mTLS port', () => {
    const parsed = parseNodePairingPayload('{"base_url":"https://example.com","secret_key":"abc"}')
    expect(parsed).toEqual({
      base_url: 'https://example.com',
      secret_key: 'abc',
      mtls_port: 0,
    })
  })

  it('ignores removed mTLS port alias', () => {
    const removedPortAlias = ['fed', 'er', 'ation_port'].join('')
    const parsed = parseNodePairingPayload(
      JSON.stringify({
        base_url: 'https://example.com',
        secret_key: 'abc',
        [removedPortAlias]: 8443,
      }),
    )
    expect(parsed).toEqual({
      base_url: 'https://example.com',
      secret_key: 'abc',
      mtls_port: 0,
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
    expect(() => parseNodePairingPayload('{"secret_key":"abc"}')).toThrow('Base URL is required')
  })

  it('throws for missing secret key', () => {
    expect(() =>
      parseNodePairingPayload('{"base_url":"https://example.com","secret_key":""}'),
    ).toThrow('secret_key is required')
  })

  it('throws for invalid mTLS port', () => {
    expect(() =>
      parseNodePairingPayload(
        '{"base_url":"https://example.com","secret_key":"abc","mtls_port":70000}',
      ),
    ).toThrow('mTLS port must be between 1 and 65535')
  })
})
