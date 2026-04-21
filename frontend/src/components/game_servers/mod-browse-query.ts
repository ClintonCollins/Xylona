export const VALID_SORT_VALUES = ['downloads', 'updated', 'newest', 'relevance'] as const
export const PAGE_SIZE_OPTIONS = [12, 20, 40, 60] as const
export const PAGE_SIZE_STORAGE_KEY = 'xylona-mod-browse-page-size'
export const DEFAULT_PAGE_SIZE = 20

const MANAGED_QUERY_KEYS = new Set(['q', 'sort', 'source', 'version', 'categories', 'page'])

export type ModBrowseSortValue = (typeof VALID_SORT_VALUES)[number]
export type ModBrowseQueryValue = string | string[] | null | undefined
export type ModBrowseQuery = Record<string, ModBrowseQueryValue>

export interface ParsedModBrowseQuery {
  searchQuery: string
  sortBy: ModBrowseSortValue
  activeSource: string
  gameVersionFilter: string
  categoryFilter: string[]
  currentPage: number
}

export interface ModBrowseQueryState {
  searchQuery: string
  sortBy: string
  activeSource: string
  gameVersionFilter: string
  categoryFilter: string[]
  currentPage: number
}

export function isModBrowseSortValue(value: string): value is ModBrowseSortValue {
  return (VALID_SORT_VALUES as readonly string[]).includes(value)
}

export function loadModBrowsePageSize(storage: Pick<Storage, 'getItem'>): number {
  const stored = storage.getItem(PAGE_SIZE_STORAGE_KEY)
  if (stored !== null) {
    const parsed = parseInt(stored, 10)
    if ((PAGE_SIZE_OPTIONS as readonly number[]).includes(parsed)) {
      return parsed
    }
  }
  return DEFAULT_PAGE_SIZE
}

export function parseModBrowseQuery(query: ModBrowseQuery): ParsedModBrowseQuery {
  const result: ParsedModBrowseQuery = {
    searchQuery: '',
    sortBy: 'downloads',
    activeSource: '',
    gameVersionFilter: '',
    categoryFilter: [],
    currentPage: 1,
  }

  const rawQuery = query['q']
  if (typeof rawQuery === 'string' && rawQuery !== '') {
    result.searchQuery = rawQuery
  }

  const rawSort = query['sort']
  if (typeof rawSort === 'string' && isModBrowseSortValue(rawSort)) {
    if (rawSort === 'relevance' && result.searchQuery.trim() === '') {
      result.sortBy = 'downloads'
    } else {
      result.sortBy = rawSort
    }
  }

  const rawSource = query['source']
  if (typeof rawSource === 'string') {
    result.activeSource = rawSource
  }

  const rawVersion = query['version']
  if (typeof rawVersion === 'string') {
    result.gameVersionFilter = rawVersion
  }

  const rawCategories = query['categories']
  if (Array.isArray(rawCategories)) {
    result.categoryFilter = rawCategories.filter((category): category is string => {
      return typeof category === 'string' && category !== ''
    })
  } else if (typeof rawCategories === 'string' && rawCategories !== '') {
    result.categoryFilter = [rawCategories]
  }

  const rawPage = query['page']
  if (typeof rawPage === 'string') {
    const parsed = parseInt(rawPage, 10)
    if (!isNaN(parsed) && parsed > 0) {
      result.currentPage = parsed
    }
  }

  return result
}

export function buildModBrowseQuery(
  currentQuery: ModBrowseQuery,
  state: ModBrowseQueryState,
): Record<string, string | string[]> {
  const nextQuery: Record<string, string | string[]> = {}

  for (const [key, value] of Object.entries(currentQuery)) {
    if (MANAGED_QUERY_KEYS.has(key) || value == null) {
      continue
    }
    if (Array.isArray(value)) {
      nextQuery[key] = value.filter((item): item is string => typeof item === 'string')
    } else if (typeof value === 'string') {
      nextQuery[key] = value
    }
  }

  const trimmedQuery = state.searchQuery.trim()
  if (trimmedQuery !== '') {
    nextQuery['q'] = trimmedQuery
  }
  if (state.sortBy !== 'downloads') {
    nextQuery['sort'] = state.sortBy
  }
  if (state.activeSource !== '') {
    nextQuery['source'] = state.activeSource
  }
  if (state.gameVersionFilter !== '') {
    nextQuery['version'] = state.gameVersionFilter
  }
  if (state.categoryFilter.length > 0) {
    nextQuery['categories'] = state.categoryFilter
  }
  if (state.currentPage !== 1) {
    nextQuery['page'] = String(state.currentPage)
  }

  return nextQuery
}

export function clampModBrowsePage(
  currentPage: number,
  totalCount: number,
  pageSize: number,
  resultsLength: number,
): { page: number; clamped: boolean } {
  const maxPage = Math.max(1, Math.ceil(totalCount / pageSize))
  if (resultsLength === 0 && totalCount > 0 && maxPage < currentPage) {
    return { page: maxPage, clamped: true }
  }
  return { page: currentPage, clamped: false }
}
