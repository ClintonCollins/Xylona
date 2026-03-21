import type { ImportedField, ParserAdapter, ParserAdapterResult } from '../types'
import { coerceValue, inferType, keyToTitle } from '../infer'

export const propertiesParser: ParserAdapter = {
  name: 'properties',
  parse(content: string): ParserAdapterResult {
    const fields: ImportedField[] = []
    const lines = content.split(/\r?\n/)

    for (const line of lines) {
      const trimmed = line.trim()

      if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('!')) {
        continue
      }

      const sepIndex = findSeparator(trimmed)
      if (sepIndex === -1) continue

      const key = trimmed.slice(0, sepIndex).trim()
      const rawValue = trimmed.slice(sepIndex + 1).trim()

      if (!key) continue

      const type = inferType(rawValue)
      fields.push({
        key,
        value: coerceValue(rawValue, type),
        type,
        title: keyToTitle(key),
        allowMultiple: false,
        group: '',
      })
    }

    return { fields, errors: [] }
  },
}

function findSeparator(line: string): number {
  for (let i = 0; i < line.length; i++) {
    if ((line[i] === '=' || line[i] === ':') && (i === 0 || line[i - 1] !== '\\')) {
      return i
    }
  }
  return -1
}
