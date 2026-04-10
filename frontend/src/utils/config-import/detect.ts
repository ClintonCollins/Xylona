import type { ImportDetectionResult, ParserAdapter, ParseResult } from './types'
import { jsonParser } from './parsers/json'
import { xmlParser } from './parsers/xml'
import { tomlParser } from './parsers/toml'
import { iniParser } from './parsers/ini'
import { propertiesParser } from './parsers/properties'
import { yamlParser } from './parsers/yaml'

/** Map of format name to parser adapter. */
const PARSER_MAP: Record<string, ParserAdapter> = {
  json: jsonParser,
  xml: xmlParser,
  toml: tomlParser,
  ini: iniParser,
  properties: propertiesParser,
  yaml: yamlParser,
}

/**
 * Parse content with a specific format. Used when the user manually
 * selects an alternative format from the ambiguous results.
 */
export function parseWithFormat(format: string, content: string): ImportDetectionResult {
  const parser = PARSER_MAP[format]
  if (!parser) {
    return { format: null, fields: [], alternativeFormats: [], filename: null }
  }
  const result = parser.parse(content)
  if (result.errors.length > 0 || result.fields.length === 0) {
    return { format: null, fields: [], alternativeFormats: [], filename: null }
  }
  return {
    format,
    fields: result.fields,
    alternativeFormats: [],
    filename: null,
    xml_key_mode: result.xml_key_mode,
  }
}

/** Score multipliers per format. YAML and Properties are penalized for being overly permissive. */
const SCORE_MULTIPLIERS: Record<string, number> = {
  json: 1.0,
  xml: 1.0,
  toml: 1.0,
  ini: 1.0,
  properties: 0.5,
  yaml: 0.5,
}

/**
 * Parser priority order. Tried in this order because some formats are
 * subsets of others (e.g., valid TOML often parses as YAML).
 */
const PARSERS: ParserAdapter[] = [
  jsonParser,
  xmlParser,
  tomlParser,
  iniParser,
  propertiesParser,
  yamlParser,
]

/** Returns true if the content contains at least one INI section header line. */
function hasIniSections(content: string): boolean {
  return /^\s*\[[^\]]+\]\s*$/m.test(content)
}

/**
 * Returns true if the content has indented lines that suggest YAML nesting
 * (lines starting with 2+ spaces or a tab followed by non-whitespace).
 */
function hasIndentedStructure(content: string): boolean {
  return /^[ \t]{2,}\S/m.test(content)
}

/**
 * Returns true if all field keys look like reasonable config keys
 * (alphanumeric, hyphens, underscores, dots, colons).
 * Used to reject INI/Properties matches on garbage content.
 */
function hasValidKeys(fields: { key: string }[]): boolean {
  if (fields.length === 0) return false
  return fields.every((f) => /^[\w.:-]+$/.test(f.key))
}

/**
 * Try all parsers, score results, and return the best match.
 * Detection is "ambiguous" when the 2nd-best scores within 80% of the best.
 */
export function detect(content: string): ImportDetectionResult {
  if (!content.trim()) {
    return { format: null, fields: [], alternativeFormats: [], filename: null }
  }

  const results: ParseResult[] = []
  const contentHasSections = hasIniSections(content)
  const contentIsIndented = hasIndentedStructure(content)

  for (const parser of PARSERS) {
    const result = parser.parse(content)
    if (result.errors.length === 0 && result.fields.length > 0) {
      let multiplier = SCORE_MULTIPLIERS[parser.name] ?? 1.0

      // INI without section headers is indistinguishable from Properties.
      // Penalize INI when no [sections] are present so Properties wins for
      // flat key=value content.
      if (parser.name === 'ini' && !contentHasSections) {
        multiplier = 0.4
      }

      // If the content has indented structure (YAML-like nesting), penalize
      // flat-format parsers (Properties, INI) further so YAML can win.
      if ((parser.name === 'properties' || parser.name === 'ini') && contentIsIndented) {
        multiplier *= 0.3
      }

      // Reject INI and Properties results whose field keys contain non-word
      // characters — this filters out garbage content parsed as bare words.
      if (parser.name === 'ini' || parser.name === 'properties') {
        if (!hasValidKeys(result.fields)) {
          continue
        }
      }

      results.push({
        format: parser.name,
        fields: result.fields,
        score: result.fields.length * multiplier,
        errors: result.errors,
        xml_key_mode: result.xml_key_mode,
      })
    }
  }

  if (results.length === 0) {
    return { format: null, fields: [], alternativeFormats: [], filename: null }
  }

  // Sort by score descending
  results.sort((a, b) => b.score - a.score)

  const best = results[0]
  if (!best) {
    return { format: null, fields: [], alternativeFormats: [], filename: null }
  }

  const secondBest = results[1]

  // Ambiguous if second-best is within 80% of best
  const isAmbiguous = secondBest !== undefined && secondBest.score >= best.score * 0.8

  const alternativeFormats = isAmbiguous
    ? results.slice(1).map((r) => ({ format: r.format, fieldCount: r.fields.length }))
    : []

  return {
    format: best.format,
    fields: best.fields,
    alternativeFormats,
    filename: null,
    xml_key_mode: best.xml_key_mode,
  }
}
