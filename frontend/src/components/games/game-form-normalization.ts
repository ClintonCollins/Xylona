export function normalizeSteamAppID(value: unknown): string {
  if (typeof value === 'string') {
    return value.trim()
  }
  if (typeof value === 'number' || typeof value === 'bigint') {
    return `${value}`
  }
  return ''
}
