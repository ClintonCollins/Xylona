import { describe, expect, it } from 'vitest'
import { jsonParser } from './json'
import { yamlParser } from './yaml'
import { tomlParser } from './toml'
import { xmlParser } from './xml'
import { iniParser } from './ini'
import { propertiesParser } from './properties'

describe('jsonParser', () => {
  it('parses a simple JSON object', () => {
    const result = jsonParser.parse('{"port": 25565, "motd": "Hello"}')
    expect(result.errors).toEqual([])
    expect(result.fields).toHaveLength(2)
    expect(result.fields[0]).toMatchObject({ key: 'port', type: 'integer', value: 25565 })
    expect(result.fields[1]).toMatchObject({ key: 'motd', type: 'string', value: 'Hello' })
  })

  it('flattens nested JSON', () => {
    const result = jsonParser.parse('{"server": {"port": 25565}}')
    expect(result.fields).toHaveLength(1)
    expect(result.fields[0]).toMatchObject({ key: 'server.port', type: 'integer' })
  })

  it('returns errors for invalid JSON', () => {
    const result = jsonParser.parse('not json')
    expect(result.errors.length).toBeGreaterThan(0)
    expect(result.fields).toEqual([])
  })

  it('rejects non-object JSON (array, string)', () => {
    const result = jsonParser.parse('[1, 2, 3]')
    expect(result.errors.length).toBeGreaterThan(0)
  })

  it('detects boolean values', () => {
    const result = jsonParser.parse('{"enabled": true}')
    expect(result.fields[0]).toMatchObject({ type: 'boolean', value: true })
  })

  it('handles uniform arrays with allow-multiple', () => {
    const result = jsonParser.parse('{"mods": ["a", "b", "c"]}')
    expect(result.fields).toHaveLength(1)
    expect(result.fields[0]).toMatchObject({ key: 'mods', allowMultiple: true })
  })
})

describe('yamlParser', () => {
  it('parses a simple YAML document', () => {
    const content = 'port: 25565\nmotd: Hello\n'
    const result = yamlParser.parse(content)
    expect(result.errors).toEqual([])
    expect(result.fields).toHaveLength(2)
    expect(result.fields[0]).toMatchObject({ key: 'port', type: 'integer' })
  })

  it('parses nested YAML', () => {
    const content = 'server:\n  network:\n    port: 25565\n'
    const result = yamlParser.parse(content)
    expect(result.fields[0]).toMatchObject({ key: 'server.network.port' })
  })

  it('returns errors for non-object YAML (plain scalar)', () => {
    const result = yamlParser.parse('just a string')
    expect(result.errors.length).toBeGreaterThan(0)
  })
})

describe('tomlParser', () => {
  it('parses a simple TOML document', () => {
    const content = 'port = 25565\nmotd = "Hello"\n'
    const result = tomlParser.parse(content)
    expect(result.errors).toEqual([])
    expect(result.fields).toHaveLength(2)
    expect(result.fields[0]).toMatchObject({ key: 'port', type: 'integer' })
  })

  it('parses TOML tables as nested keys', () => {
    const content = '[server]\nport = 25565\n'
    const result = tomlParser.parse(content)
    expect(result.fields[0]).toMatchObject({ key: 'server.port' })
  })

  it('returns errors for invalid TOML', () => {
    const result = tomlParser.parse('not: valid: toml:')
    expect(result.errors.length).toBeGreaterThan(0)
  })
})

describe('xmlParser', () => {
  it('parses element-style XML', () => {
    const content = '<server><port>25565</port><motd>Hello</motd></server>'
    const result = xmlParser.parse(content)
    expect(result.errors).toEqual([])
    expect(result.fields.length).toBeGreaterThanOrEqual(2)
    expect(result.xml_key_mode?.mode).toBe('elements')
  })

  it('detects attribute-style XML (7DTD pattern)', () => {
    const content = `<ServerSettings>
      <property name="ServerPort" value="26900"/>
      <property name="ServerName" value="My Server"/>
    </ServerSettings>`
    const result = xmlParser.parse(content)
    expect(result.errors).toEqual([])
    expect(result.xml_key_mode?.mode).toBe('attributes')
    expect(result.xml_key_mode?.element).toBe('property')
    expect(result.xml_key_mode?.key_attr).toBe('name')
    expect(result.xml_key_mode?.value_attr).toBe('value')
    expect(result.fields).toContainEqual(
      expect.objectContaining({ key: 'ServerPort', type: 'integer' }),
    )
  })

  it('returns errors for invalid XML', () => {
    const result = xmlParser.parse('not xml at all')
    expect(result.errors.length).toBeGreaterThan(0)
  })
})

describe('iniParser', () => {
  it('parses a simple INI file', () => {
    const content = '[server]\nport=25565\nmotd=Hello\n'
    const result = iniParser.parse(content)
    expect(result.errors).toEqual([])
    expect(result.fields).toHaveLength(2)
    expect(result.fields[0]).toMatchObject({ key: 'server.port', type: 'integer' })
  })

  it('handles keys without sections', () => {
    const content = 'port=25565\nmotd=Hello\n'
    const result = iniParser.parse(content)
    expect(result.fields).toHaveLength(2)
    expect(result.fields[0]).toMatchObject({ key: 'port', type: 'integer' })
  })

  it('returns errors for empty content', () => {
    const result = iniParser.parse('')
    expect(result.fields).toEqual([])
  })
})

describe('propertiesParser', () => {
  it('parses key=value pairs', () => {
    const content = 'server-port=25565\nmotd=Hello World\n'
    const result = propertiesParser.parse(content)
    expect(result.errors).toEqual([])
    expect(result.fields).toHaveLength(2)
    expect(result.fields[0]).toMatchObject({ key: 'server-port', type: 'integer' })
  })

  it('supports key:value syntax', () => {
    const content = 'port:25565\n'
    const result = propertiesParser.parse(content)
    expect(result.fields[0]).toMatchObject({ key: 'port', type: 'integer' })
  })

  it('skips # and ! comments', () => {
    const content = '# comment\n! also comment\nport=25565\n'
    const result = propertiesParser.parse(content)
    expect(result.fields).toHaveLength(1)
  })

  it('skips blank lines', () => {
    const content = 'port=25565\n\nmotd=Hello\n'
    const result = propertiesParser.parse(content)
    expect(result.fields).toHaveLength(2)
  })

  it('handles values with = in them', () => {
    const content = 'query=select * from x where a=b\n'
    const result = propertiesParser.parse(content)
    expect(result.fields[0]).toMatchObject({
      key: 'query',
      value: 'select * from x where a=b',
    })
  })
})
