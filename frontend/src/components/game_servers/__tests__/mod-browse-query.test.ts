import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  clampModBrowsePage,
  DEFAULT_PAGE_SIZE,
  loadModBrowsePageSize,
  PAGE_SIZE_STORAGE_KEY,
  VALID_SORT_VALUES,
  parseModBrowseQuery,
} from '../mod-browse-query'

describe('ModBrowse page size localStorage persistence', () => {
  let mockStorage: Storage

  beforeEach(() => {
    const store: Record<string, string> = {}
    mockStorage = {
      getItem: vi.fn((key: string) => store[key] ?? null),
      setItem: vi.fn((key: string, value: string) => {
        store[key] = value
      }),
      removeItem: vi.fn((key: string) => {
        delete store[key]
      }),
      clear: vi.fn(() => {
        for (const key of Object.keys(store)) {
          delete store[key]
        }
      }),
      get length() {
        return Object.keys(store).length
      },
      key: vi.fn((_index: number) => null),
    }
  })

  it('returns default (20) when nothing is stored', () => {
    expect(loadModBrowsePageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('returns stored value when it is a valid option (12)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '12')
    expect(loadModBrowsePageSize(mockStorage)).toBe(12)
  })

  it('returns stored value when it is a valid option (40)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '40')
    expect(loadModBrowsePageSize(mockStorage)).toBe(40)
  })

  it('returns stored value when it is a valid option (60)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '60')
    expect(loadModBrowsePageSize(mockStorage)).toBe(60)
  })

  it('falls back to default for an invalid numeric value (999)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '999')
    expect(loadModBrowsePageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for zero', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '0')
    expect(loadModBrowsePageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for a negative number', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '-20')
    expect(loadModBrowsePageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for a non-numeric string', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, 'banana')
    expect(loadModBrowsePageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for an empty string', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '')
    expect(loadModBrowsePageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })
})

describe('ModBrowse query param validation', () => {
  it('parses a valid page number', () => {
    const parsed = parseModBrowseQuery({ page: '3' })
    expect(parsed.currentPage).toBe(3)
  })

  it('defaults page to 1 when not provided', () => {
    const parsed = parseModBrowseQuery({})
    expect(parsed.currentPage).toBe(1)
  })

  it('rejects a non-numeric page value', () => {
    const parsed = parseModBrowseQuery({ page: 'abc' })
    expect(parsed.currentPage).toBe(1)
  })

  it('rejects page value of zero', () => {
    const parsed = parseModBrowseQuery({ page: '0' })
    expect(parsed.currentPage).toBe(1)
  })

  it('rejects a negative page value', () => {
    const parsed = parseModBrowseQuery({ page: '-5' })
    expect(parsed.currentPage).toBe(1)
  })

  it('validates known sort values', () => {
    for (const sort of VALID_SORT_VALUES) {
      const parsed = parseModBrowseQuery({ q: 'test', sort })
      // relevance is valid when query is present
      expect(VALID_SORT_VALUES as readonly string[]).toContain(parsed.sortBy)
    }
  })

  it('ignores unknown sort values and keeps default', () => {
    const parsed = parseModBrowseQuery({ sort: 'bogus' })
    expect(parsed.sortBy).toBe('downloads')
  })

  it('normalizes relevance sort to downloads when query is empty', () => {
    const parsed = parseModBrowseQuery({ sort: 'relevance' })
    expect(parsed.sortBy).toBe('downloads')
  })

  it('normalizes relevance sort to downloads when query is whitespace only', () => {
    const parsed = parseModBrowseQuery({ q: '   ', sort: 'relevance' })
    expect(parsed.sortBy).toBe('downloads')
  })

  it('keeps relevance sort when query is present', () => {
    const parsed = parseModBrowseQuery({ q: 'fabric', sort: 'relevance' })
    expect(parsed.sortBy).toBe('relevance')
  })

  it('parses search query from q param', () => {
    const parsed = parseModBrowseQuery({ q: 'worldedit' })
    expect(parsed.searchQuery).toBe('worldedit')
  })

  it('ignores empty q param', () => {
    const parsed = parseModBrowseQuery({ q: '' })
    expect(parsed.searchQuery).toBe('')
  })

  it('parses source param', () => {
    const parsed = parseModBrowseQuery({ source: 'modrinth' })
    expect(parsed.activeSource).toBe('modrinth')
  })

  it('parses version param', () => {
    const parsed = parseModBrowseQuery({ version: '1.20.4' })
    expect(parsed.gameVersionFilter).toBe('1.20.4')
  })

  it('parses categories from an array and filters empty strings', () => {
    const parsed = parseModBrowseQuery({ categories: ['adventure', '', 'utility'] })
    expect(parsed.categoryFilter).toEqual(['adventure', 'utility'])
  })

  it('parses a single category string', () => {
    const parsed = parseModBrowseQuery({ categories: 'technology' })
    expect(parsed.categoryFilter).toEqual(['technology'])
  })

  it('ignores empty category string', () => {
    const parsed = parseModBrowseQuery({ categories: '' })
    expect(parsed.categoryFilter).toEqual([])
  })

  it('defaults categories to empty array when not provided', () => {
    const parsed = parseModBrowseQuery({})
    expect(parsed.categoryFilter).toEqual([])
  })
})

describe('ModBrowse page clamping', () => {
  it('clamps when currentPage exceeds maxPage', () => {
    // totalCount=50, pageSize=20 => maxPage=3, currentPage=5 => should clamp to 3
    const result = clampModBrowsePage(5, 50, 20, 0)
    expect(result.clamped).toBe(true)
    expect(result.page).toBe(3)
  })

  it('does NOT clamp when currentPage equals maxPage', () => {
    // totalCount=60, pageSize=20 => maxPage=3, currentPage=3 => no clamp
    const result = clampModBrowsePage(3, 60, 20, 0)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(3)
  })

  it('does NOT clamp when on the first page', () => {
    const result = clampModBrowsePage(1, 100, 20, 10)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(1)
  })

  it('does NOT clamp when results are not empty even if page seems high', () => {
    // The component only clamps when results.length === 0
    const result = clampModBrowsePage(10, 5, 20, 1)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(10)
  })

  it('does NOT clamp when totalCount is zero', () => {
    // No results at all — don't clamp (the empty state shows instead)
    const result = clampModBrowsePage(3, 0, 20, 0)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(3)
  })

  it('clamps to 1 when totalCount is less than pageSize and currentPage > 1', () => {
    // totalCount=5, pageSize=20 => maxPage=1, currentPage=2, results empty
    const result = clampModBrowsePage(2, 5, 20, 0)
    expect(result.clamped).toBe(true)
    expect(result.page).toBe(1)
  })
})
