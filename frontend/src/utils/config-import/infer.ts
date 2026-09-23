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

const TITLE_ACRONYMS = new Set(['API', 'HP', 'ID', 'IP', 'PVE', 'PVP', 'RCON', 'XP'])
const TITLE_JOINERS = new Set(['a', 'an', 'and', 'for', 'in', 'of', 'on', 'or', 'the', 'to'])

/**
 * Convert a group name to a display-friendly Title Case label.
 * Unlike keyToTitle, this processes ALL segments of a dot-path.
 * e.g., "server.network" -> "Server Network". A name that already has spaces
 * is a label the schema author wrote ("Guilds and Bases") and is not split
 * further. Words are capitalized except lowercase joiners such as "and";
 * mixed-case acronyms such as PvP or RCON stay intact. An all-caps name such
 * as GAME_SETTINGS is title-cased, keeping only known acronyms upper case.
 */
export function groupToTitle(group: string): string {
  const name = group.trim()
  const shouting = !/[a-z]/.test(name)
  const words = /\s/.test(name)
    ? name.split(/\s+/)
    : name
        .replace(/([a-z]{2,})([A-Z])/g, '$1 $2')
        .replace(/([A-Z]+)([A-Z][a-z])/g, '$1 $2')
        .split(/[-_.\s]+/)

  return words
    .filter(Boolean)
    .map((w, i) => {
      if (shouting) {
        if (TITLE_ACRONYMS.has(w)) return w
        w = w.toLowerCase()
      }
      if (i > 0 && TITLE_JOINERS.has(w)) return w
      return w.charAt(0).toUpperCase() + w.slice(1)
    })
    .join(' ')
}

/**
 * Convert a dot-path config key to a human-readable Title Case label.
 * Uses the last segment and splits on kebab-case, snake_case, or camelCase.
 */
export function keyToTitle(key: string): string {
  if (!key) return ''

  // Use the last segment of dot-path keys
  const segments = key.split('.')
  const lastSegment = segments[segments.length - 1] ?? ''

  // Split on hyphens, underscores, or camelCase boundaries
  const words = lastSegment
    .replace(/([a-z])([A-Z])/g, '$1 $2') // camelCase
    .replace(/[-_]+/g, ' ') // kebab/snake
    .trim()
    .split(/\s+/)

  return words.map((w) => w.charAt(0).toUpperCase() + w.slice(1).toLowerCase()).join(' ')
}
