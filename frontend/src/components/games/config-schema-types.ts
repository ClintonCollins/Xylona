export interface SchemaProperty {
  type?: string
  'x-managed'?: {
    source: string
  }
  [key: string]: unknown
}

export interface ConfigSchemaEntry {
  path: string
  format: string
  category: string
  generate_before_start: boolean
  xml_key_mode?: {
    mode: string
    element: string
    key_attr: string
    value_attr: string
  }
  schema?: {
    type: string
    properties: Record<string, SchemaProperty>
  }
}
