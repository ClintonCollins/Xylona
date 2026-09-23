import { describe, expect, it } from 'vitest'

import { isEditableFileName } from './file-editing'

describe('isEditableFileName', () => {
  it.each([
    ['server.properties', true],
    ['config.TOML', true],
    ['nginx.conf', true],
    ['README.md', true],
    ['.env', true],
    ['MicrosoftGame.Config', true],
    ['README', true],
    ['LICENSE', true],
    ['world.zip', false],
    ['server.jar', false],
    ['icon.png', false],
  ])('%s -> %s', (name, want) => {
    expect(isEditableFileName(name)).toBe(want)
  })
})
