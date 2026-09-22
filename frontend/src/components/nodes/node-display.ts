import type { Node } from '@/proto/shared_pb'

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
