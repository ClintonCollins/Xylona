import type { Node, NodeResourceSnapshot } from '@/proto/shared_pb'

export interface NodeHealthBadge {
  color: string
  icon: string
  label: string
}

export function nodeHealthBadge(node: Pick<Node, 'healthStatus'>): NodeHealthBadge {
  switch (node.healthStatus) {
    case 'offline':
      return { color: 'negative', icon: 'cloud_off', label: 'Offline' }
    case 'disabled':
      return { color: 'warning', icon: 'pause_circle', label: 'Disabled' }
    case 'healthy':
      return { color: 'positive', icon: 'check_circle', label: 'Healthy' }
    default:
      return { color: 'grey-6', icon: 'help', label: 'Unknown' }
  }
}

export type NodeResource = 'cpu' | 'memory' | 'disk'

// One threshold table for the node list, its phone cards and the node detail page.
export const nodeResourceThresholds: Record<NodeResource, { warn: number; danger: number }> = {
  cpu: { warn: 85, danger: 95 },
  memory: { warn: 85, danger: 95 },
  disk: { warn: 80, danger: 92 },
}

// What the node's disk reading measures: the backend reads usage of the path "/", which on
// Windows resolves to the root of the drive holding the process's working directory.
export const nodeDiskVolume =
  "the volume mounted at / (on Windows, the drive holding the node's working directory)"

export interface NodeResourceHealth {
  level: 'ok' | 'warn' | 'danger' | 'unknown'
  label: string
  glyph: string
}

export function nodeResourceHealth(
  resource: NodeResource,
  percent: number | null | undefined,
): NodeResourceHealth {
  if (percent === null || percent === undefined || !Number.isFinite(percent)) {
    return { level: 'unknown', label: 'No data', glyph: '' }
  }
  const { warn, danger } = nodeResourceThresholds[resource]
  if (percent >= danger) return { level: 'danger', label: 'Critical', glyph: '●' }
  if (percent >= warn) return { level: 'warn', label: 'High', glyph: '▲' }
  return { level: 'ok', label: 'Nominal', glyph: '' }
}

// The readings a live snapshot or a history point carries. A flagged reading
// is one the node could not take; its zeroed value is not a real 0%.
export type NodeReadings = Pick<
  NodeResourceSnapshot,
  | 'cpuPercent'
  | 'cpuUnavailable'
  | 'memoryPercent'
  | 'memoryUsedBytes'
  | 'memoryUnavailable'
  | 'diskPercent'
  | 'diskUsedBytes'
  | 'diskUnavailable'
>

export interface NodeReadingValues {
  cpuPercent: number | null
  memoryPercent: number | null
  memoryUsedBytes: number | null
  diskPercent: number | null
  diskUsedBytes: number | null
}

// Null marks an unavailable reading, so charts draw a gap instead of a 0.
export function nodeReadingValues(readings: NodeReadings): NodeReadingValues {
  return {
    cpuPercent: nodeResourcePercent(readings, 'cpu'),
    memoryPercent: nodeResourcePercent(readings, 'memory'),
    memoryUsedBytes: readings.memoryUnavailable ? null : Number(readings.memoryUsedBytes),
    diskPercent: nodeResourcePercent(readings, 'disk'),
    diskUsedBytes: readings.diskUnavailable ? null : Number(readings.diskUsedBytes),
  }
}

export function nodeResourcePercent(readings: NodeReadings, resource: NodeResource): number | null {
  switch (resource) {
    case 'cpu':
      return readings.cpuUnavailable ? null : readings.cpuPercent
    case 'memory':
      return readings.memoryUnavailable ? null : readings.memoryPercent
    case 'disk':
      return readings.diskUnavailable ? null : readings.diskPercent
  }
}

export function nodeResourceUnavailableReason(resource: NodeResource): string {
  return `The node couldn't read its ${resource === 'cpu' ? 'CPU' : resource} usage.`
}

const hourMs = 60 * 60 * 1000
const dayMs = 24 * hourMs

// Days until the disk fills at the least-squares growth rate of the given samples.
// Needs a day of history so an hour of noise can't predict anything, and stays quiet
// beyond ~30 days, where the estimate is too weak to act on. Unavailable readings are skipped.
export function projectDaysUntilDiskFull(
  allSamples: readonly { timestampMs: number; diskUsedBytes: number | null }[],
  totalBytes: number | null,
): number | null {
  const samples = allSamples.filter(
    (sample): sample is { timestampMs: number; diskUsedBytes: number } =>
      sample.diskUsedBytes !== null,
  )
  const first = samples[0]
  const last = samples[samples.length - 1]
  if (!totalBytes || !first || !last || last.timestampMs - first.timestampMs < dayMs) return null
  const meanX = samples.reduce((sum, sample) => sum + sample.timestampMs, 0) / samples.length
  const meanY = samples.reduce((sum, sample) => sum + sample.diskUsedBytes, 0) / samples.length
  let covariance = 0
  let variance = 0
  for (const sample of samples) {
    covariance += (sample.timestampMs - meanX) * (sample.diskUsedBytes - meanY)
    variance += (sample.timestampMs - meanX) ** 2
  }
  const bytesPerMs = variance > 0 ? covariance / variance : 0
  if (bytesPerMs <= 0) return null
  const days = (totalBytes - last.diskUsedBytes) / bytesPerMs / dayMs
  return days <= 30 ? Math.max(days, 0) : null
}

// "v0.9.3-0.20260921231240-90ac69c62e7e" → { short: "v0.9.3", build: "90ac69c" }
export function splitNodeVersion(version: string | undefined): { short: string; build: string } {
  const trimmed = (version ?? '').trim()
  if (!trimmed) return { short: '', build: '' }
  const dash = trimmed.indexOf('-')
  if (dash === -1) return { short: trimmed, build: '' }
  const build = trimmed.slice(trimmed.lastIndexOf('-') + 1)
  return { short: trimmed.slice(0, dash), build: build.slice(0, 7) }
}

export function nodeLastSeenMs(node: Pick<Node, 'lastSeenAt'>): number | null {
  const seconds = node.lastSeenAt?.seconds
  if (seconds === undefined || seconds <= 0n) return null
  return Number(seconds) * 1000
}
