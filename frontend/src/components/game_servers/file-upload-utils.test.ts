import { describe, expect, it } from 'vitest'

import { buildUploadPath, stripLeadingPathSeparator } from './file-upload-utils'

describe('stripLeadingPathSeparator', () => {
  it('strips leading forward slash', () => {
    expect(stripLeadingPathSeparator('/dir/file.txt', 'file.txt')).toBe('dir/file.txt')
  })

  it('strips leading backslash', () => {
    expect(stripLeadingPathSeparator('\\dir\\file.txt', 'file.txt')).toBe('dir\\file.txt')
  })

  it('returns fileName when fullPath is empty', () => {
    expect(stripLeadingPathSeparator('', 'fallback.txt')).toBe('fallback.txt')
  })

  it('returns fullPath unchanged when no leading separator', () => {
    expect(stripLeadingPathSeparator('dir/file.txt', 'file.txt')).toBe('dir/file.txt')
  })
})

describe('buildUploadPath', () => {
  it('returns empty string when base path is empty and no relative path info', () => {
    const result = buildUploadPath('', '/', '', 'file.txt')
    expect(result).toBe('')
  })

  it('uses base path with separator when base path is non-empty', () => {
    const result = buildUploadPath('/server/data', '/', 'subdir/file.txt', 'file.txt')
    expect(result).toBe('/server/data/subdir')
  })

  it('handles Windows path separator', () => {
    const result = buildUploadPath('C:\\server', '\\', 'saves\\world\\file.txt', 'file.txt')
    expect(result).toBe('C:\\server\\saves\\world')
  })

  it('falls back to fileName parsing when webkitRelativePath is empty', () => {
    const result = buildUploadPath('/server', '/', '', 'subdir/file.txt')
    expect(result).toBe('/server/subdir')
  })

  it('handles empty base path with webkitRelativePath', () => {
    const result = buildUploadPath('', '/', 'dir/subdir/file.txt', 'file.txt')
    expect(result).toBe('dir/subdir')
  })
})
