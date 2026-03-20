import type { FieldType } from './types'

/**
 * Infer a schema field type from a parsed config value.
 * Handles native types (from JSON/YAML/TOML) and string values (from Properties/INI).
 */
export function inferType(value: unknown): FieldType {
  if (value === null || value === undefined) return 'string'

  if (typeof value === 'boolean') return 'boolean'

  if (typeof value === 'number') {
    return Number.isInteger(value) ? 'integer' : 'number'
  }

  if (typeof value === 'string') {
    const trimmed = value.trim()
    if (trimmed === '') return 'string'

    // Boolean strings
    if (trimmed.toLowerCase() === 'true' || trimmed.toLowerCase() === 'false') {
      return 'boolean'
    }

    // Numeric strings — must be the entire string
    const num = Number(trimmed)
    if (!isNaN(num) && trimmed !== '') {
      return trimmed.includes('.') ? 'number' : 'integer'
    }
  }

  return 'string'
}

/**
 * Coerce a raw value to the inferred native type.
 * Used to store defaults as their proper types in the schema.
 */
export function coerceValue(value: unknown, type: FieldType): unknown {
  if (value === null || value === undefined) return value
  if (typeof value === 'string') {
    switch (type) {
      case 'boolean':
        return value.trim().toLowerCase() === 'true'
      case 'integer':
        return parseInt(value, 10)
      case 'number':
        return parseFloat(value)
      default:
        return value
    }
  }
  return value
}

/**
 * Convert a dot-path config key to a human-readable Title Case label.
 * Uses the last segment and splits on kebab-case, snake_case, or camelCase.
 */
export function keyToTitle(key: string): string {
  if (!key) return ''

  // Use the last segment of dot-path keys
  const segments = key.split('.')
  const lastSegment = segments[segments.length - 1]

  // Split on hyphens, underscores, or camelCase boundaries
  const words = lastSegment
    .replace(/([a-z])([A-Z])/g, '$1 $2') // camelCase
    .replace(/[-_]+/g, ' ') // kebab/snake
    .trim()
    .split(/\s+/)

  return words.map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ')
}
