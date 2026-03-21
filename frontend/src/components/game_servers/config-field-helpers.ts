import { groupToTitle } from 'src/utils/config-import'

/** Shared palette for category color assignment. */
export const CATEGORY_COLORS = [
  '#3B82F6', // blue
  '#22C55E', // green
  '#F59E0B', // amber
  '#8B5CF6', // purple
  '#EF4444', // red
  '#06B6D4', // cyan
  '#EC4899', // pink
  '#F97316', // orange
]

/**
 * Build a category → color mapping from a list of config files.
 * Assigns colors from CATEGORY_COLORS in first-occurrence order.
 */
export function buildCategoryColorMap(
  configFiles: { category: string }[],
): Map<string, string> {
  const map = new Map<string, string>()
  const categories = [...new Set(configFiles.map((f) => f.category || 'Uncategorized'))]
  categories.forEach((cat, i) => {
    map.set(cat, CATEGORY_COLORS[i % CATEGORY_COLORS.length])
  })
  return map
}

export interface FieldGroup {
  name: string
  displayName: string
  fields: {
    key: string
    group: string
    title: string
    value: string
    [k: string]: unknown
  }[]
}

/**
 * Group fields by their group value, preserving first-occurrence order.
 * Ungrouped fields (empty group) go in a "General" group at the top.
 */
export function groupFields<T extends { key: string; group: string }>(
  fields: T[],
): { name: string; displayName: string; fields: T[] }[] {
  if (fields.length === 0) return []

  const groupOrder: string[] = []
  const groupMap = new Map<string, T[]>()

  for (const field of fields) {
    const g = field.group || ''
    if (!groupMap.has(g)) {
      groupOrder.push(g)
      groupMap.set(g, [])
    }
    groupMap.get(g)!.push(field)
  }

  // Move "General" (empty group) to the top
  const generalIdx = groupOrder.indexOf('')
  if (generalIdx > 0) {
    groupOrder.splice(generalIdx, 1)
    groupOrder.unshift('')
  }

  return groupOrder.map((name) => ({
    name,
    displayName: name ? groupToTitle(name) : 'General',
    fields: groupMap.get(name) || [],
  }))
}

/**
 * Filter fields by search query, matching on key, title, or value.
 * Case-insensitive substring match. Empty query returns all fields.
 */
export function filterFields<T extends { key: string; title: string; value: string }>(
  fields: T[],
  query: string,
): T[] {
  if (!query.trim()) return fields
  const lower = query.toLowerCase()
  return fields.filter(
    (f) =>
      f.key.toLowerCase().includes(lower) ||
      f.title.toLowerCase().includes(lower) ||
      f.value.toLowerCase().includes(lower),
  )
}
