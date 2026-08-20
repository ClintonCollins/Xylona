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
