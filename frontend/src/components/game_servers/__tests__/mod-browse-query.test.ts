/**
 * Unit tests for the pure logic used in ModBrowse pagination:
 * - Page size localStorage persistence
 * - Query param validation
 * - Page clamping
 *
 * These functions mirror the logic in ModBrowse.vue's <script setup>.
 * Since they are not exported from the component, we re-implement them here
 * as standalone functions matching the component's exact behavior.
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'

// --- Constants matching ModBrowse.vue ---
const VALID_SORT_VALUES = ['downloads', 'updated', 'newest', 'relevance'] as const
const PAGE_SIZE_OPTIONS = [12, 20, 40, 60] as const
const PAGE_SIZE_STORAGE_KEY = 'xylona-mod-browse-page-size'
const DEFAULT_PAGE_SIZE = 20

// --- Pure functions extracted from ModBrowse.vue ---

function loadPageSize(storage: Storage): number {
  const stored = storage.getItem(PAGE_SIZE_STORAGE_KEY)
  if (stored !== null) {
    const parsed = parseInt(stored, 10)
    if ((PAGE_SIZE_OPTIONS as readonly number[]).includes(parsed)) {
      return parsed
    }
  }
  return DEFAULT_PAGE_SIZE
}

interface ParsedQuery {
  searchQuery: string
  sortBy: string
  activeSource: string
  gameVersionFilter: string
  categoryFilter: string[]
  currentPage: number
}

function parseQueryParams(q: Record<string, string | string[] | undefined>): ParsedQuery {
  const result: ParsedQuery = {
    searchQuery: '',
    sortBy: 'downloads',
    activeSource: '',
    gameVersionFilter: '',
    categoryFilter: [],
    currentPage: 1,
  }

  if (typeof q.q === 'string' && q.q !== '') {
    result.searchQuery = q.q
  }

  if (typeof q.sort === 'string' && (VALID_SORT_VALUES as readonly string[]).includes(q.sort)) {
    if (q.sort === 'relevance' && result.searchQuery.trim() === '') {
      result.sortBy = 'downloads'
    } else {
      result.sortBy = q.sort
    }
  }

  if (typeof q.source === 'string') {
    result.activeSource = q.source
  }

  if (typeof q.version === 'string') {
    result.gameVersionFilter = q.version
  }

  const rawCats = q.categories
  if (Array.isArray(rawCats)) {
    result.categoryFilter = rawCats.filter((c): c is string => typeof c === 'string' && c !== '')
  } else if (typeof rawCats === 'string' && rawCats !== '') {
    result.categoryFilter = [rawCats]
  }

  if (typeof q.page === 'string') {
    const parsed = parseInt(q.page, 10)
    if (!isNaN(parsed) && parsed > 0) {
      result.currentPage = parsed
    }
  }

  return result
}

/**
 * Determines whether the current page should be clamped down.
 * Returns the clamped page number, or the same page if no clamping needed.
 *
 * Mirrors the logic in performSearch():
 *   if (results.length === 0 && totalCount > 0 && maxPage < currentPage)
 */
function clampPage(
  currentPage: number,
  totalCount: number,
  pageSize: number,
  resultsEmpty: boolean,
): { page: number; clamped: boolean } {
  const maxPage = Math.max(1, Math.ceil(totalCount / pageSize))
  if (resultsEmpty && totalCount > 0 && maxPage < currentPage) {
    return { page: maxPage, clamped: true }
  }
  return { page: currentPage, clamped: false }
}

// --- Tests ---

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
    expect(loadPageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('returns stored value when it is a valid option (12)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '12')
    expect(loadPageSize(mockStorage)).toBe(12)
  })

  it('returns stored value when it is a valid option (40)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '40')
    expect(loadPageSize(mockStorage)).toBe(40)
  })

  it('returns stored value when it is a valid option (60)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '60')
    expect(loadPageSize(mockStorage)).toBe(60)
  })

  it('falls back to default for an invalid numeric value (999)', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '999')
    expect(loadPageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for zero', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '0')
    expect(loadPageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for a negative number', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '-20')
    expect(loadPageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for a non-numeric string', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, 'banana')
    expect(loadPageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })

  it('falls back to default for an empty string', () => {
    mockStorage.setItem(PAGE_SIZE_STORAGE_KEY, '')
    expect(loadPageSize(mockStorage)).toBe(DEFAULT_PAGE_SIZE)
  })
})

describe('ModBrowse query param validation', () => {
  it('parses a valid page number', () => {
    const parsed = parseQueryParams({ page: '3' })
    expect(parsed.currentPage).toBe(3)
  })

  it('defaults page to 1 when not provided', () => {
    const parsed = parseQueryParams({})
    expect(parsed.currentPage).toBe(1)
  })

  it('rejects a non-numeric page value', () => {
    const parsed = parseQueryParams({ page: 'abc' })
    expect(parsed.currentPage).toBe(1)
  })

  it('rejects page value of zero', () => {
    const parsed = parseQueryParams({ page: '0' })
    expect(parsed.currentPage).toBe(1)
  })

  it('rejects a negative page value', () => {
    const parsed = parseQueryParams({ page: '-5' })
    expect(parsed.currentPage).toBe(1)
  })

  it('validates known sort values', () => {
    for (const sort of VALID_SORT_VALUES) {
      const parsed = parseQueryParams({ q: 'test', sort })
      // relevance is valid when query is present
      expect(VALID_SORT_VALUES as readonly string[]).toContain(parsed.sortBy)
    }
  })

  it('ignores unknown sort values and keeps default', () => {
    const parsed = parseQueryParams({ sort: 'bogus' })
    expect(parsed.sortBy).toBe('downloads')
  })

  it('normalizes relevance sort to downloads when query is empty', () => {
    const parsed = parseQueryParams({ sort: 'relevance' })
    expect(parsed.sortBy).toBe('downloads')
  })

  it('normalizes relevance sort to downloads when query is whitespace only', () => {
    const parsed = parseQueryParams({ q: '   ', sort: 'relevance' })
    expect(parsed.sortBy).toBe('downloads')
  })

  it('keeps relevance sort when query is present', () => {
    const parsed = parseQueryParams({ q: 'fabric', sort: 'relevance' })
    expect(parsed.sortBy).toBe('relevance')
  })

  it('parses search query from q param', () => {
    const parsed = parseQueryParams({ q: 'worldedit' })
    expect(parsed.searchQuery).toBe('worldedit')
  })

  it('ignores empty q param', () => {
    const parsed = parseQueryParams({ q: '' })
    expect(parsed.searchQuery).toBe('')
  })

  it('parses source param', () => {
    const parsed = parseQueryParams({ source: 'modrinth' })
    expect(parsed.activeSource).toBe('modrinth')
  })

  it('parses version param', () => {
    const parsed = parseQueryParams({ version: '1.20.4' })
    expect(parsed.gameVersionFilter).toBe('1.20.4')
  })

  it('parses categories from an array and filters empty strings', () => {
    const parsed = parseQueryParams({ categories: ['adventure', '', 'utility'] })
    expect(parsed.categoryFilter).toEqual(['adventure', 'utility'])
  })

  it('parses a single category string', () => {
    const parsed = parseQueryParams({ categories: 'technology' })
    expect(parsed.categoryFilter).toEqual(['technology'])
  })

  it('ignores empty category string', () => {
    const parsed = parseQueryParams({ categories: '' })
    expect(parsed.categoryFilter).toEqual([])
  })

  it('defaults categories to empty array when not provided', () => {
    const parsed = parseQueryParams({})
    expect(parsed.categoryFilter).toEqual([])
  })
})

describe('ModBrowse page clamping', () => {
  it('clamps when currentPage exceeds maxPage', () => {
    // totalCount=50, pageSize=20 => maxPage=3, currentPage=5 => should clamp to 3
    const result = clampPage(5, 50, 20, true)
    expect(result.clamped).toBe(true)
    expect(result.page).toBe(3)
  })

  it('does NOT clamp when currentPage equals maxPage', () => {
    // totalCount=60, pageSize=20 => maxPage=3, currentPage=3 => no clamp
    const result = clampPage(3, 60, 20, true)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(3)
  })

  it('does NOT clamp when on the first page', () => {
    const result = clampPage(1, 100, 20, false)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(1)
  })

  it('does NOT clamp when results are not empty even if page seems high', () => {
    // The component only clamps when results.length === 0
    const result = clampPage(10, 5, 20, false)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(10)
  })

  it('does NOT clamp when totalCount is zero', () => {
    // No results at all — don't clamp (the empty state shows instead)
    const result = clampPage(3, 0, 20, true)
    expect(result.clamped).toBe(false)
    expect(result.page).toBe(3)
  })

  it('clamps to 1 when totalCount is less than pageSize and currentPage > 1', () => {
    // totalCount=5, pageSize=20 => maxPage=1, currentPage=2, results empty
    const result = clampPage(2, 5, 20, true)
    expect(result.clamped).toBe(true)
    expect(result.page).toBe(1)
  })
})
