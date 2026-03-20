import type { ImportedField, ParserAdapter, ParserAdapterResult } from '../types'
import { coerceValue, inferType, keyToTitle } from '../infer'

export const xmlParser: ParserAdapter = {
  name: 'xml',
  parse(content: string): ParserAdapterResult {
    try {
      const parser = new DOMParser()
      const doc = parser.parseFromString(content, 'application/xml')

      const parseError = doc.querySelector('parsererror')
      if (parseError) {
        return { fields: [], errors: [parseError.textContent || 'Invalid XML'] }
      }

      const root = doc.documentElement

      const attrResult = tryAttributeMode(root)
      if (attrResult) {
        return attrResult
      }

      const fields = parseElementMode(root, '')
      return {
        fields,
        errors: [],
        xml_key_mode: { mode: 'elements' },
      }
    } catch (err) {
      return { fields: [], errors: [err instanceof Error ? err.message : 'Invalid XML'] }
    }
  },
}

function tryAttributeMode(root: Element): ParserAdapterResult | null {
  const children = Array.from(root.children)
  if (children.length < 2) return null

  const tagName = children[0].tagName
  const allSameTag = children.every((el) => el.tagName === tagName)
  if (!allSameTag) return null

  const firstAttrs = Array.from(children[0].attributes).map((a) => a.name)
  if (firstAttrs.length < 2) return null

  const consistent = children.every((el) => {
    const attrs = Array.from(el.attributes).map((a) => a.name)
    return attrs.length >= 2 && firstAttrs.every((a) => attrs.includes(a))
  })
  if (!consistent) return null

  const keyAttr = firstAttrs[0]
  const valueAttr = firstAttrs[1]

  const fields: ImportedField[] = []
  for (const child of children) {
    const key = child.getAttribute(keyAttr)
    const rawValue = child.getAttribute(valueAttr)
    if (!key) continue

    const type = inferType(rawValue)
    fields.push({
      key,
      value: coerceValue(rawValue, type),
      type,
      title: keyToTitle(key),
      allowMultiple: false,
    })
  }

  if (fields.length === 0) return null

  return {
    fields,
    errors: [],
    xml_key_mode: {
      mode: 'attributes',
      element: tagName,
      key_attr: keyAttr,
      value_attr: valueAttr,
    },
  }
}

function parseElementMode(element: Element, prefix: string): ImportedField[] {
  const fields: ImportedField[] = []

  for (const child of Array.from(element.children)) {
    const key = prefix ? `${prefix}.${child.tagName}` : child.tagName

    if (child.children.length > 0) {
      fields.push(...parseElementMode(child, key))
    } else {
      const rawValue = child.textContent?.trim() ?? ''
      const type = inferType(rawValue)
      fields.push({
        key,
        value: coerceValue(rawValue, type),
        type,
        title: keyToTitle(key),
        allowMultiple: false,
      })
    }
  }

  return fields
}
