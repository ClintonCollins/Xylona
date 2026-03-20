/** A single field extracted from a parsed config file. */
export interface ImportedField {
  key: string
  value: unknown
  type: 'string' | 'integer' | 'number' | 'boolean'
  title: string
  allowMultiple: boolean
}

/** Result returned by a single parser adapter. */
export interface ParserAdapterResult {
  fields: ImportedField[]
  errors: string[]
  xml_key_mode?: XmlKeyMode
}

/** XML key mode configuration. */
export interface XmlKeyMode {
  mode: 'elements' | 'attributes'
  element?: string
  key_attr?: string
  value_attr?: string
}

/** A parser adapter that can parse a specific config format. */
export interface ParserAdapter {
  name: string
  parse(content: string): ParserAdapterResult
}

/** Result of parsing with a single format, including scoring. */
export interface ParseResult {
  format: string
  fields: ImportedField[]
  score: number
  errors: string[]
  xml_key_mode?: XmlKeyMode
}

/** Overall detection result emitted by ConfigImportInput. */
export interface ImportDetectionResult {
  format: string | null
  fields: ImportedField[]
  alternativeFormats: { format: string; fieldCount: number }[]
  filename: string | null
  xml_key_mode?: XmlKeyMode
}

/** Supported field types for inference. */
export type FieldType = 'string' | 'integer' | 'number' | 'boolean'
