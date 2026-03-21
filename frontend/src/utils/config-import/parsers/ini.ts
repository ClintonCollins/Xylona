import { parse as parseINI } from 'ini'
import type { ImportedField, ParserAdapter, ParserAdapterResult } from '../types'
import { coerceValue, inferType, keyToTitle } from '../infer'

export const iniParser: ParserAdapter = {
  name: 'ini',
  parse(content: string): ParserAdapterResult {
    try {
      const parsed = parseINI(content)
      const fields: ImportedField[] = []

      for (const [key, value] of Object.entries(parsed)) {
        if (typeof value === 'object' && value !== null) {
          for (const [subKey, subValue] of Object.entries(value as Record<string, string>)) {
            const fullKey = `${key}.${subKey}`
            const type = inferType(subValue)
            fields.push({
              key: fullKey,
              value: coerceValue(subValue, type),
              type,
              title: keyToTitle(fullKey),
              allowMultiple: false,
              group: key,
            })
          }
        } else {
          const type = inferType(value)
          fields.push({
            key,
            value: coerceValue(value, type),
            type,
            title: keyToTitle(key),
            allowMultiple: false,
            group: '',
          })
        }
      }

      return { fields, errors: [] }
    } catch (err) {
      return { fields: [], errors: [err instanceof Error ? err.message : 'Invalid INI'] }
    }
  },
}
