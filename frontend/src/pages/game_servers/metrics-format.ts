const byteUnits = ['B', 'KB', 'MB', 'GB', 'TB'] as const

export function formatMetricBytes(value: number | null, fractionDigits = 1): string {
  if (value === null || !Number.isFinite(value)) return 'Unknown'
  if (value <= 0) return '0 B'

  const unitIndex = Math.min(Math.floor(Math.log(value) / Math.log(1024)), byteUnits.length - 1)
  const scaled = value / 1024 ** unitIndex
  return `${scaled.toFixed(unitIndex === 0 ? 0 : fractionDigits)} ${byteUnits[unitIndex]}`
}

export function formatMetricRate(value: number | null): string {
  const formatted = formatMetricBytes(value)
  return formatted === 'Unknown' ? formatted : `${formatted}/s`
}

export function formatMetricPercent(value: number | null, fractionDigits = 1): string {
  return value === null || !Number.isFinite(value) ? 'Unknown' : `${value.toFixed(fractionDigits)}%`
}

export function formatMetricNumber(value: number | null, fractionDigits = 1): string {
  return value === null || !Number.isFinite(value) ? 'Unknown' : value.toFixed(fractionDigits)
}

export function formatMetricDuration(seconds: number | null): string {
  if (seconds === null || !Number.isFinite(seconds)) return 'Unknown'
  if (seconds < 60) return `${Math.max(Math.floor(seconds), 0)}s`
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return [days > 0 ? `${days}d` : '', hours > 0 ? `${hours}h` : '', `${minutes}m`]
    .filter(Boolean)
    .join(' ')
}

export function formatMetricTimestamp(timestampMs: number | null): string {
  if (timestampMs === null || !Number.isFinite(timestampMs)) return 'Unknown'
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(timestampMs)
}

export function formatMetricAge(timestampMs: number | null, nowMs = Date.now()): string {
  if (timestampMs === null || !Number.isFinite(timestampMs)) return 'Unknown age'
  const ageSeconds = Math.max(Math.round((nowMs - timestampMs) / 1000), 0)
  if (ageSeconds < 5) return 'Just now'
  if (ageSeconds < 60) return `${ageSeconds}s ago`
  if (ageSeconds < 3600) return `${Math.floor(ageSeconds / 60)}m ago`
  if (ageSeconds < 86400) return `${Math.floor(ageSeconds / 3600)}h ago`
  return `${Math.floor(ageSeconds / 86400)}d ago`
}
