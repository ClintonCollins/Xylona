import YAML from 'yaml'
import type { ImportedField, ParserAdapter, ParserAdapterResult } from '../types'
import { flattenObject } from '../flatten'
import { coerceValue, inferType, keyToTitle } from '../infer'

export const yamlParser: ParserAdapter = {
  name: 'yaml',
  parse(content: string): ParserAdapterResult {
    try {
      const parsed = YAML.parse(content)
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        return { fields: [], errors: ['YAML root must be a mapping'] }
      }
      const flat = flattenObject(parsed)
      const fields: ImportedField[] = flat.map((entry) => {
        const type = inferType(entry.value)
        return {
          key: entry.key,
          value: coerceValue(entry.value, type),
          type,
          title: keyToTitle(entry.key),
          allowMultiple: entry.allowMultiple ?? false,
          group: entry.group ?? '',
        }
      })
      return { fields, errors: [] }
    } catch (err) {
      return { fields: [], errors: [err instanceof Error ? err.message : 'Invalid YAML'] }
    }
  },
}
