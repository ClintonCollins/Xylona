import { AlertEventType } from '@/proto/shared_pb'

export function formatDuration(seconds: number): string {
  if (seconds % 3600 === 0) return `${seconds / 3600}h`
  if (seconds % 60 === 0) return `${seconds / 60}m`
  return `${seconds}s`
}

export function isFiniteNonNegativeNumber(value: unknown): boolean {
  if (value === null || value === undefined || value === '') return false
  const numericValue = Number(value)
  return Number.isFinite(numericValue) && numericValue >= 0
}

export function isNonNegativeInteger(value: unknown): boolean {
  if (value === null || value === undefined || value === '') return false
  const numericValue = Number(value)
  return Number.isInteger(numericValue) && numericValue >= 0
}

export function readPositiveInteger(value: unknown): number {
  return isNonNegativeInteger(value) && Number(value) > 0 ? Number(value) : 0
}

const percentEventTypes = [
  AlertEventType.CPU_THRESHOLD,
  AlertEventType.MEMORY_THRESHOLD,
  AlertEventType.DISK_THRESHOLD,
  AlertEventType.NODE_CPU_THRESHOLD,
  AlertEventType.NODE_MEMORY_THRESHOLD,
  AlertEventType.NODE_DISK_THRESHOLD,
]

function thresholdUnit(eventType: AlertEventType): string {
  return percentEventTypes.includes(eventType) ? '%' : ''
}

/** Turns a server status name such as PRE_START into "Pre Start". */
export function alertStatusLabel(status: string): string {
  return status
    .toLowerCase()
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

function parseJSONObject(text: string): Record<string, unknown> | null {
  try {
    const parsed: unknown = JSON.parse(text)
    return typeof parsed === 'object' && parsed !== null
      ? (parsed as Record<string, unknown>)
      : null
  } catch {
    return null
  }
}

/** Describes an alert rule's stored condition JSON for tables and cards. */
export function formatCondition(eventType: AlertEventType, condition: string): string {
  if (!condition) {
    if (eventType === AlertEventType.CRASH) return 'Any crash'
    if (eventType === AlertEventType.STATUS_CHANGE) return 'Any status'
    return '-'
  }

  const parsed = parseJSONObject(condition)
  if (!parsed) return condition

  if ('operator' in parsed && 'value' in parsed) {
    const unit = thresholdUnit(eventType)
    const parts = [`${String(parsed['operator'])} ${String(parsed['value'])}${unit}`]
    const forSeconds = readPositiveInteger(parsed['for_seconds'])
    const cooldownSeconds = readPositiveInteger(parsed['cooldown_seconds'])
    const repeatSeconds = readPositiveInteger(parsed['repeat_seconds'])
    const recoveryValue = parsed['recovery_value']

    if (forSeconds > 0) parts.push(`for ${formatDuration(forSeconds)}`)
    if (typeof recoveryValue === 'number' && Number.isFinite(recoveryValue)) {
      parts.push(`recover at ${recoveryValue}${unit}`)
    }
    if (cooldownSeconds > 0) parts.push(`${formatDuration(cooldownSeconds)} cooldown`)
    if (repeatSeconds > 0) parts.push(`repeat ${formatDuration(repeatSeconds)}`)

    return parts.join(' · ')
  }

  const statuses = parsed['statuses']
  if (Array.isArray(statuses)) {
    // The evaluator matches every transition when the list is empty.
    return statuses.length === 0
      ? 'Any status'
      : statuses.map(String).map(alertStatusLabel).join(', ')
  }

  return condition
}

function formatReading(value: number): string {
  return String(Math.round(value * 10) / 10)
}

/** Describes an alert history entry's event data, such as "92.3% (threshold 90%)". */
export function formatAlertEventData(eventType: AlertEventType, eventData: string): string {
  if (!eventData) return '-'
  const data = parseJSONObject(eventData)
  if (!data) return eventData

  if (eventType === AlertEventType.CRASH) {
    const exitCode = data['exit_code']
    return typeof exitCode === 'number' ? `Exit code ${exitCode}` : eventData
  }

  if (eventType === AlertEventType.STATUS_CHANGE) {
    const oldStatus = data['old_status']
    const newStatus = data['new_status']
    if (typeof newStatus !== 'string' || newStatus === '') return eventData
    return typeof oldStatus === 'string' && oldStatus !== ''
      ? `${alertStatusLabel(oldStatus)} → ${alertStatusLabel(newStatus)}`
      : alertStatusLabel(newStatus)
  }

  const current = data['current_value']
  const threshold = data['threshold']
  if (typeof current !== 'number' || typeof threshold !== 'number') return eventData
  const unit = thresholdUnit(eventType)
  const reading = `${formatReading(current)}${unit} (threshold ${formatReading(threshold)}${unit})`
  return data['direction'] === 'resolved' ? `Recovered: ${reading}` : reading
}
