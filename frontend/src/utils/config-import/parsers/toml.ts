import { parse as parseTOML } from 'smol-toml'
import type { ImportedField, ParserAdapter, ParserAdapterResult } from '../types'
import { flattenObject } from '../flatten'
import { coerceValue, inferType, keyToTitle } from '../infer'

export const tomlParser: ParserAdapter = {
  name: 'toml',
  parse(content: string): ParserAdapterResult {
    try {
      const parsed = parseTOML(content) as Record<string, unknown>
      const flat = flattenObject(parsed)
      const fields: ImportedField[] = flat.map((entry) => {
        const type = inferType(entry.value)
        return {
          key: entry.key,
          value: coerceValue(entry.value, type),
          type,
          title: keyToTitle(entry.key),
          allowMultiple: entry.allowMultiple ?? false,
        }
      })
      return { fields, errors: [] }
    } catch (err) {
      return { fields: [], errors: [err instanceof Error ? err.message : 'Invalid TOML'] }
    }
  },
}
