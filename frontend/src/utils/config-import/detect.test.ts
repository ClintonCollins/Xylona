import { describe, expect, it } from 'vitest'
import { detectAndParse } from './index'

describe('detectAndParse', () => {
  it('detects JSON format', () => {
    const result = detectAndParse('{"port": 25565, "motd": "Hello"}')
    expect(result.format).toBe('json')
    expect(result.fields.length).toBe(2)
    expect(result.alternativeFormats).toEqual([])
  })

  it('detects YAML format', () => {
    const content = 'server:\n  port: 25565\n  motd: Hello\n'
    const result = detectAndParse(content)
    expect(result.format).toBe('yaml')
    expect(result.fields.length).toBeGreaterThan(0)
  })

  it('detects TOML format', () => {
    const content = '[server]\nport = 25565\nmotd = "Hello"\n'
    const result = detectAndParse(content)
    expect(result.format).toBe('toml')
    expect(result.fields.length).toBeGreaterThan(0)
  })

  it('detects XML format', () => {
    const content = '<server><port>25565</port></server>'
    const result = detectAndParse(content)
    expect(result.format).toBe('xml')
  })

  it('detects Properties format', () => {
    const content = 'server-port=25565\nmotd=Hello World\nenable-rcon=true\n'
    const result = detectAndParse(content)
    expect(result.format).toBe('properties')
  })

  it('detects INI format (sections)', () => {
    const content = '[server]\nport=25565\nmotd=Hello\n\n[gameplay]\ndifficulty=hard\n'
    const result = detectAndParse(content)
    expect(result.format).toBe('ini')
  })

  it('returns null format for empty content', () => {
    const result = detectAndParse('')
    expect(result.format).toBeNull()
  })

  it('returns null format for unparseable content', () => {
    const result = detectAndParse('§±@#$%^&*')
    expect(result.format).toBeNull()
  })

  it('detects attribute-style XML with xml_key_mode', () => {
    const content = `<ServerSettings>
      <property name="ServerPort" value="26900"/>
      <property name="ServerName" value="My Server"/>
    </ServerSettings>`
    const result = detectAndParse(content)
    expect(result.format).toBe('xml')
    expect(result.xml_key_mode?.mode).toBe('attributes')
  })

  it('penalizes YAML for content that looks like Properties', () => {
    const content = 'server-port=25565\nmax-players=20\nenable-rcon=true\nlevel-seed=\n'
    const result = detectAndParse(content)
    expect(result.format).not.toBe('yaml')
  })

  it('prefers INI over Properties when sections are present', () => {
    const content = '[server]\nport=25565\nmotd=Hello\n\n[gameplay]\ndifficulty=hard\n'
    const result = detectAndParse(content)
    expect(result.format).toBe('ini')
  })

  it('handles key=value without sections (Properties vs INI ambiguity)', () => {
    const content = 'port=25565\nmotd=Hello World\nenabled=true\n'
    const result = detectAndParse(content)
    expect(result.format).not.toBeNull()
    expect(result.fields.length).toBeGreaterThan(0)
  })
})
