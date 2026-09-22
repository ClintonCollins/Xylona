import { describe, expect, it } from 'vitest'
import { create } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { nodeHealthBadge, nodeLastSeenMs, splitNodeVersion } from './node-display'

describe('splitNodeVersion', () => {
  it('separates the semver from the build hash', () => {
    expect(splitNodeVersion('v0.9.3-0.20260921231240-90ac69c62e7e')).toEqual({
      short: 'v0.9.3',
      build: '90ac69c',
    })
    expect(splitNodeVersion('v1.2.0')).toEqual({ short: 'v1.2.0', build: '' })
    expect(splitNodeVersion(undefined)).toEqual({ short: '', build: '' })
  })
})

describe('nodeLastSeenMs', () => {
  it('treats a zero or negative (year 0001) timestamp as never seen', () => {
    expect(nodeLastSeenMs({ lastSeenAt: undefined })).toBeNull()
    expect(
      nodeLastSeenMs({ lastSeenAt: create(TimestampSchema, { seconds: -62135596800n }) }),
    ).toBeNull()
    expect(nodeLastSeenMs({ lastSeenAt: create(TimestampSchema, { seconds: 1700000000n }) })).toBe(
      1700000000000,
    )
  })
})

describe('nodeHealthBadge', () => {
  it('maps node health status to badge labels and colors', () => {
    expect(nodeHealthBadge({ healthStatus: 'healthy' })).toMatchObject({
      label: 'Healthy',
      color: 'positive',
    })
    expect(nodeHealthBadge({ healthStatus: 'offline' })).toMatchObject({
      label: 'Offline',
      color: 'negative',
    })
    expect(nodeHealthBadge({ healthStatus: 'disabled' })).toMatchObject({
      label: 'Disabled',
      color: 'warning',
    })
    expect(nodeHealthBadge({ healthStatus: '' }).label).toBe('Unknown')
  })
})
