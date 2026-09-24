import { describe, expect, it } from 'vitest'
import { create } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { NodeResourceSnapshotSchema } from '@/proto/shared_pb'
import {
  nodeHealthBadge,
  nodeLastSeenMs,
  nodeReadingValues,
  nodeResourceHealth,
  nodeResourcePercent,
  nodeResourceUnavailableReason,
  projectDaysUntilDiskFull,
  splitNodeVersion,
} from './node-display'

describe('node readings', () => {
  const snapshot = create(NodeResourceSnapshotSchema, {
    cpuPercent: 0,
    cpuUnavailable: true,
    memoryPercent: 40,
    memoryUsedBytes: 400n,
    diskPercent: 0,
    diskUsedBytes: 0n,
  })

  it('turns a reading the node could not take into null, and keeps a real 0', () => {
    expect(nodeReadingValues(snapshot)).toEqual({
      cpuPercent: null,
      memoryPercent: 40,
      memoryUsedBytes: 400,
      diskPercent: 0,
      diskUsedBytes: 0,
    })
    expect(
      nodeReadingValues({ ...snapshot, memoryUnavailable: true, diskUnavailable: true }),
    ).toMatchObject({ memoryPercent: null, memoryUsedBytes: null, diskUsedBytes: null })
  })

  it('rates an unavailable reading as neither healthy nor critical', () => {
    const percent = nodeResourcePercent(snapshot, 'cpu')
    expect(percent).toBeNull()
    expect(nodeResourceHealth('cpu', percent).level).toBe('unknown')
    expect(nodeResourceUnavailableReason('cpu')).toBe("The node couldn't read its CPU usage.")
    expect(nodeResourceUnavailableReason('disk')).toBe("The node couldn't read its disk usage.")
  })
})

describe('projectDaysUntilDiskFull', () => {
  const hour = 60 * 60 * 1000
  const growing = (hours: number, bytesPerHour: number) =>
    Array.from({ length: hours + 1 }, (_, index) => ({
      timestampMs: index * hour,
      diskUsedBytes: 1000 + index * bytesPerHour,
    }))

  it('projects from the trend across at least a day of samples', () => {
    // Ends at 1480 bytes growing 240 bytes/day, so 2400 bytes of headroom last 10 days.
    const samples = growing(48, 10)
    const total = 1480 + 10 * 24 * 10
    expect(projectDaysUntilDiskFull(samples, total)).toBeCloseTo(10, 5)
  })

  it('stays quiet for short spans, shrinking or flat usage, and far-off dates', () => {
    expect(projectDaysUntilDiskFull(growing(6, 10), 2000)).toBeNull()
    expect(projectDaysUntilDiskFull(growing(48, 0), 2000)).toBeNull()
    expect(projectDaysUntilDiskFull(growing(48, -5), 2000)).toBeNull()
    expect(projectDaysUntilDiskFull(growing(48, 10), 1480 + 10 * 24 * 60)).toBeNull()
    expect(projectDaysUntilDiskFull(growing(48, 10), null)).toBeNull()
  })

  it('projects past unavailable readings instead of reading them as an empty disk', () => {
    const samples = growing(48, 10).map((sample, index) =>
      index % 3 === 1 ? { ...sample, diskUsedBytes: null } : sample,
    )
    expect(projectDaysUntilDiskFull(samples, 1480 + 10 * 24 * 10)).toBeCloseTo(10, 5)
  })
})

describe('nodeResourceHealth', () => {
  it.each([
    { resource: 'memory' as const, percent: 53, level: 'ok' },
    { resource: 'disk' as const, percent: 69, level: 'ok' },
    { resource: 'cpu' as const, percent: 85, level: 'warn' },
    { resource: 'disk' as const, percent: 80, level: 'warn' },
    { resource: 'memory' as const, percent: 95, level: 'danger' },
    { resource: 'disk' as const, percent: 92, level: 'danger' },
    { resource: 'cpu' as const, percent: undefined, level: 'unknown' },
  ])('rates $resource at $percent% as $level', ({ resource, percent, level }) => {
    expect(nodeResourceHealth(resource, percent).level).toBe(level)
  })
})

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
