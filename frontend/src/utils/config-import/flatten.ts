export interface FlatEntry {
  key: string
  value: unknown
  allowMultiple?: boolean
}

/**
 * Flatten a nested object to dot-path key/value pairs.
 * Mirrors the Go backend's cfgparse.Flatten() behavior.
 *
 * Uniform primitive arrays (all same type) collapse to a single
 * key with allowMultiple=true and the first element as value.
 * Mixed or object arrays flatten to indexed dot-paths.
 */
export function flattenObject(obj: Record<string, unknown>, prefix = ''): FlatEntry[] {
  const entries: FlatEntry[] = []

  for (const [rawKey, value] of Object.entries(obj)) {
    const key = prefix ? `${prefix}.${rawKey}` : rawKey

    if (value === null || value === undefined) {
      entries.push({ key, value })
      continue
    }

    if (Array.isArray(value)) {
      entries.push(...flattenArray(key, value))
      continue
    }

    if (typeof value === 'object') {
      entries.push(...flattenObject(value as Record<string, unknown>, key))
      continue
    }

    entries.push({ key, value })
  }

  return entries
}

function flattenArray(key: string, arr: unknown[]): FlatEntry[] {
  if (arr.length === 0) return []

  // Check if all elements are the same primitive type
  const firstType = typeof arr[0]
  const allSamePrimitive =
    arr.every((el) => typeof el === firstType) &&
    arr.every((el) => el !== null && typeof el !== 'object')

  if (allSamePrimitive) {
    return [{ key, value: arr[0], allowMultiple: true }]
  }

  // Mixed or object arrays: flatten with numeric indices
  const entries: FlatEntry[] = []
  for (let i = 0; i < arr.length; i++) {
    const indexKey = `${key}.${i}`
    const el = arr[i]

    if (el !== null && typeof el === 'object' && !Array.isArray(el)) {
      entries.push(...flattenObject(el as Record<string, unknown>, indexKey))
    } else {
      entries.push({ key: indexKey, value: el })
    }
  }
  return entries
}
