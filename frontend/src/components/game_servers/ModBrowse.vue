<template>
  <div class="mod-browse">
    <!-- Toolbar: search + source filters -->
    <div class="browse-toolbar">
      <q-input
        v-model="searchQuery"
        aria-label="Search mods"
        class="browse-search-input"
        clearable
        dense
        outlined
        placeholder="Search mods..."
        @clear="searchQuery = ''"
        @update:model-value="debouncedSearch">
        <template #prepend>
          <q-icon aria-hidden="true" class="text-xy-muted" name="search" size="xs" />
        </template>
      </q-input>

      <q-select
        v-model="sortBy"
        :options="sortOptions"
        aria-label="Sort by"
        class="browse-sort-select"
        dense
        emit-value
        map-options
        outlined />

      <div aria-label="Filter by source" class="source-chips" role="group">
        <q-btn
          :class="{ 'chip-inactive': activeSource !== '' }"
          :color="activeSource === '' ? 'primary' : undefined"
          :outline="activeSource !== ''"
          :unelevated="activeSource === ''"
          dense
          label="All"
          no-caps
          size="sm"
          @click="setActiveSource('')" />
        <q-btn
          v-for="src in sources"
          :key="src.id"
          :class="{ 'chip-inactive': activeSource !== src.id }"
          :outline="activeSource !== src.id"
          :unelevated="activeSource === src.id"
          dense
          no-caps
          size="sm"
          @click="setActiveSource(src.id)">
          <span :style="sourceBadgeStyle(src.id)" aria-hidden="true" class="source-chip-badge">
            {{ sourceLabel(src.id) }}
          </span>
          <span class="source-chip-name">{{ sourceDisplayName(src.id) }}</span>
        </q-btn>
      </div>

      <q-btn
        v-if="hasActiveFilters"
        aria-label="Reset all filters"
        class="reset-filters-btn"
        dense
        flat
        icon="filter_list_off"
        label="Reset"
        no-caps
        size="sm"
        @click="resetFilters" />
    </div>

    <!-- Filters row -->
    <div
      v-if="versionSelectOptions.length > 0 || availableCategories.length > 0"
      class="browse-filters">
      <q-select
        v-if="versionSelectOptions.length > 0"
        v-model="gameVersionFilter"
        :options="versionSelectOptions"
        aria-label="Filter by game version"
        class="browse-version-input"
        clearable
        dense
        emit-value
        map-options
        outlined
        @clear="gameVersionFilter = ''" />

      <q-select
        v-if="availableCategories.length > 0"
        v-model="categoryFilter"
        :options="availableCategories"
        aria-label="Filter by category"
        class="browse-category-select"
        dense
        emit-value
        multiple
        outlined
        placeholder="Categories"
        use-chips />
    </div>

    <!-- Loading state -->
    <div v-if="loading && results.length === 0" class="browse-loading">
      <q-spinner color="primary" size="2rem" />
      <span class="text-xy-muted">Loading mods...</span>
    </div>

    <!-- Empty: no results -->
    <div v-else-if="hasSearched && results.length === 0" class="browse-empty">
      <q-icon aria-hidden="true" class="text-xy-muted" name="search_off" size="3rem" />
      <div class="browse-empty-title text-xy-secondary">No mods found</div>
      <div class="browse-empty-subtitle text-xy-muted">
        Try a different search term or source filter.
      </div>
    </div>

    <!-- Results grid -->
    <div v-else-if="results.length > 0" class="browse-grid-scroll">
      <div class="browse-grid">
        <article
          v-for="mod in results"
          :key="`${mod.source}-${mod.sourceId}`"
          class="mod-card"
          @click="emit('view-details', mod.source, mod.sourceId)">
          <button
            :aria-label="`View details for ${mod.name}`"
            class="mod-card-details"
            type="button">
            <!-- Icon -->
            <div class="mod-card-icon-wrapper">
              <img
                v-if="mod.iconUrl"
                :alt="`${mod.name} icon`"
                :src="mod.iconUrl"
                class="mod-card-icon-img"
                loading="lazy"
                @error="($event.target as HTMLImageElement).style.display = 'none'" />
              <div
                v-if="!mod.iconUrl"
                :style="{ background: iconGradient(mod.name) }"
                aria-hidden="true"
                class="mod-card-icon-fallback">
                {{ mod.name.charAt(0).toUpperCase() }}
              </div>
            </div>

            <!-- Content -->
            <div class="mod-card-content">
              <div class="mod-card-header">
                <span class="mod-card-name">{{ mod.name }}</span>
                <span
                  :style="sourceBadgeStyle(mod.source)"
                  :title="sourceDisplayName(mod.source)"
                  class="source-badge">
                  {{ sourceLabel(mod.source) }}
                </span>
              </div>
              <div class="mod-card-author text-xy-muted">by {{ mod.author }}</div>
              <div class="mod-card-desc text-xy-secondary">{{ mod.description }}</div>

              <!-- Categories -->
              <div v-if="mod.categories.length > 0" class="mod-card-categories">
                <span
                  v-for="cat in mod.categories.slice(0, 3)"
                  :key="cat"
                  class="mod-card-category">
                  {{ cat }}
                </span>
              </div>
            </div>
          </button>

          <!-- Footer -->
          <div class="mod-card-footer">
            <div class="mod-card-footer-left">
              <span class="mod-card-downloads text-xy-muted">
                <q-icon aria-hidden="true" name="download" size="xs" />
                {{ formatDownloads(mod.downloads) }}
              </span>
              <span v-if="mod.dateModified" class="mod-card-updated text-xy-muted">
                <q-icon aria-hidden="true" name="schedule" size="xs" />
                {{ formatRelativeDate(mod.dateModified) }}
              </span>
            </div>

            <span v-if="isModInstalled(mod.source, mod.sourceId)" class="installed-badge">
              <q-icon aria-hidden="true" name="check_circle" size="xs" />
              Installed
            </span>
            <q-btn
              v-else
              color="primary"
              dense
              icon="add"
              label="Install"
              no-caps
              size="sm"
              @click="onInstallClick($event, mod.source, mod.sourceId)" />
          </div>
        </article>
      </div>

      <!-- Pagination footer -->
      <div v-if="results.length > 0" class="browse-pagination-footer">
        <q-select
          v-model="pageSize"
          :options="pageSizeOptions"
          aria-label="Results per page"
          class="page-size-select"
          dense
          emit-value
          map-options
          outlined />

        <q-pagination
          v-if="hasKnownTotalCount"
          :max="totalPages"
          :max-pages="7"
          :model-value="currentPage"
          active-color="primary"
          active-design="unelevated"
          active-text-color="white"
          aria-label="Page navigation"
          boundary-links
          class="browse-pagination"
          direction-links
          icon-first="first_page"
          icon-last="last_page"
          icon-next="chevron_right"
          icon-prev="chevron_left"
          @update:model-value="onPageChange" />

        <span class="browse-result-count text-xy-muted">
          {{ resultCountLabel }}
        </span>
      </div>
    </div>

    <!-- Error banner -->
    <div v-if="errorMessage" class="browse-error">
      <q-icon aria-hidden="true" color="negative" name="error" size="sm" />
      <span>{{ errorMessage }}</span>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import type { InstalledMod, ModSearchResult } from '@/proto/shared_pb'
import { GetModCategoriesRequestSchema, SearchModsRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  formatDownloads,
  iconGradient,
  sourceBadgeStyle,
  sourceDisplayName,
  sourceLabel,
} from '@/utils/mod-sources'
import {
  buildModBrowseQuery,
  clampModBrowsePage,
  loadModBrowsePageSize,
  PAGE_SIZE_OPTIONS,
  PAGE_SIZE_STORAGE_KEY,
  parseModBrowseQuery,
} from './mod-browse-query'

interface ModSource {
  id: string
  searchParams: Record<string, unknown>
}

interface Props {
  gameServerId: string
  installedMods: InstalledMod[]
  sources: ModSource[]
  availableVersions?: string[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'view-details': [source: string, sourceId: string]
  install: [source: string, sourceId: string]
}>()

const route = useRoute()
const router = useRouter()

const pageSize = ref(loadModBrowsePageSize(localStorage))

const searchQuery = ref('')
const activeSource = ref('')
const loading = ref(false)
const hasSearched = ref(false)
const results = ref<ModSearchResult[]>([])
const errorMessage = ref('')
const currentPage = ref(1)
const totalCount = ref(0)
const suppressWatchSearch = ref(false)
const searchRequestId = ref(0)

const sortBy = ref<string>('downloads')
const gameVersionFilter = ref('')
const categoryFilter = ref<string[]>([])
const availableCategories = ref<string[]>([])

let debounceTimer: ReturnType<typeof setTimeout> | undefined

const versionSelectOptions = computed(() => {
  const versions = props.availableVersions ?? []
  if (versions.length === 0) return []
  return [{ label: 'All Versions', value: '' }, ...versions.map((v) => ({ label: v, value: v }))]
})

const sortOptions = computed(() => {
  const options = [
    { label: 'Most Downloaded', value: 'downloads' },
    { label: 'Recently Updated', value: 'updated' },
    { label: 'Newest', value: 'newest' },
  ]
  if (searchQuery.value.trim()) {
    options.push({ label: 'Relevance', value: 'relevance' })
  }
  return options
})

function initFromQuery(): void {
  const parsed = parseModBrowseQuery(route.query)
  suppressWatchSearch.value = true
  searchQuery.value = parsed.searchQuery
  sortBy.value = parsed.sortBy
  activeSource.value = parsed.activeSource
  gameVersionFilter.value = parsed.gameVersionFilter
  categoryFilter.value = parsed.categoryFilter
  currentPage.value = parsed.currentPage
  void nextTick().then(() => {
    suppressWatchSearch.value = false
  })
}

function syncQueryParams(): void {
  const query = buildModBrowseQuery(route.query, {
    searchQuery: searchQuery.value,
    sortBy: sortBy.value,
    activeSource: activeSource.value,
    gameVersionFilter: gameVersionFilter.value,
    categoryFilter: categoryFilter.value,
    currentPage: currentPage.value,
  })
  void router.replace({ query })
}

const hasActiveFilters = computed(() => {
  return (
    searchQuery.value.trim() !== '' ||
    sortBy.value !== 'downloads' ||
    activeSource.value !== '' ||
    gameVersionFilter.value !== '' ||
    categoryFilter.value.length > 0 ||
    currentPage.value !== 1
  )
})

function resetFilters(): void {
  suppressWatchSearch.value = true
  searchQuery.value = ''
  sortBy.value = 'downloads'
  activeSource.value = ''
  gameVersionFilter.value = ''
  categoryFilter.value = []
  currentPage.value = 1
  suppressWatchSearch.value = false
  void router.replace({ query: {} })
  void performSearch()
}

onMounted(() => {
  initFromQuery()
  void performSearch()
  void loadCategories()
})

watch(sortBy, () => {
  if (!suppressWatchSearch.value) {
    void resetAndSearch()
  }
})

watch(gameVersionFilter, () => {
  if (!suppressWatchSearch.value) {
    void resetAndSearch()
  }
})

watch(categoryFilter, () => {
  if (!suppressWatchSearch.value) {
    void resetAndSearch()
  }
})

function debouncedSearch(): void {
  if (debounceTimer !== undefined) {
    clearTimeout(debounceTimer)
  }
  debounceTimer = setTimeout(() => {
    void resetAndSearch()
  }, 300)
}

function setActiveSource(source: string): void {
  // Toggle: clicking the already-active source deselects it (back to "All")
  if (activeSource.value === source) {
    activeSource.value = ''
  } else {
    activeSource.value = source
  }
  void resetAndSearch()
}

async function resetAndSearch(): Promise<void> {
  currentPage.value = 1
  syncQueryParams()
  await performSearch()
}

async function performSearch(retried: boolean = false): Promise<void> {
  loading.value = true
  errorMessage.value = ''

  searchRequestId.value++
  const thisRequestId = searchRequestId.value

  try {
    const client = GetXylonaClient()
    const request = create(SearchModsRequestSchema, {
      gameServerId: props.gameServerId,
      query: searchQuery.value.trim(),
      source: activeSource.value,
      page: currentPage.value,
      pageSize: pageSize.value,
      sortBy: sortBy.value,
      gameVersion: gameVersionFilter.value,
      categories: categoryFilter.value,
    })
    const response = await client.searchMods(request)

    if (thisRequestId !== searchRequestId.value) {
      return
    }

    results.value = response.results
    totalCount.value = response.totalCount

    const pageClamp = clampModBrowsePage(
      currentPage.value,
      totalCount.value,
      pageSize.value,
      results.value.length,
    )
    if (!retried && pageClamp.clamped) {
      currentPage.value = pageClamp.page
      syncQueryParams()
      await performSearch(true)
      return
    }

    hasSearched.value = true

    const gridEl = document.querySelector('.browse-grid-scroll')
    if (gridEl) {
      gridEl.scrollTop = 0
    }
  } catch (err: unknown) {
    if (thisRequestId !== searchRequestId.value) {
      return
    }
    if (err instanceof ConnectError) {
      errorMessage.value = ConnectErrorToString(err)
    } else {
      errorMessage.value = 'An unexpected error occurred while searching.'
    }
    results.value = []
  } finally {
    if (thisRequestId === searchRequestId.value) {
      loading.value = false
    }
  }
}

watch(pageSize, () => {
  localStorage.setItem(PAGE_SIZE_STORAGE_KEY, String(pageSize.value))
  currentPage.value = 1
  syncQueryParams()
  void performSearch()
})

function onPageChange(newPage: number): void {
  currentPage.value = newPage
  syncQueryParams()
  void performSearch()
}

const totalPages = computed(() => {
  if (totalCount.value <= 0) return 1
  return Math.ceil(totalCount.value / pageSize.value)
})

const hasKnownTotalCount = computed(() => totalCount.value >= 0)

const resultCountLabel = computed(() => {
  if (!hasKnownTotalCount.value) {
    return `Showing ${results.value.length} result${results.value.length === 1 ? '' : 's'}`
  }
  return `${totalCount.value} mod${totalCount.value === 1 ? '' : 's'} found`
})

const pageSizeOptions = PAGE_SIZE_OPTIONS.map((size) => ({
  label: `${size} per page`,
  value: size,
}))

async function loadCategories(): Promise<void> {
  try {
    const response = await GetXylonaClient().getModCategories(
      create(GetModCategoriesRequestSchema, {
        gameServerId: props.gameServerId,
      }),
    )
    availableCategories.value = response.categories
  } catch {
    // Non-critical — silently ignore
  }
}

function onInstallClick(event: Event | undefined, source: string, sourceId: string): void {
  if (event?.stopPropagation) {
    event.stopPropagation()
  }
  emit('install', source, sourceId)
}

function isModInstalled(source: string, sourceId: string): boolean {
  return props.installedMods.some((mod) => mod.source === source && mod.sourceId === sourceId)
}

// --- Display helpers ---

function formatRelativeDate(dateStr: string): string {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  if (diffMs < 0) return 'just now'

  const seconds = Math.floor(diffMs / 1000)
  const minutes = Math.floor(seconds / 60)
  const hours = Math.floor(minutes / 60)
  const days = Math.floor(hours / 24)
  const months = Math.floor(days / 30)
  const years = Math.floor(days / 365)

  if (years > 0) return `${years} year${years === 1 ? '' : 's'} ago`
  if (months > 0) return `${months} month${months === 1 ? '' : 's'} ago`
  if (days > 0) return `${days} day${days === 1 ? '' : 's'} ago`
  if (hours > 0) return `${hours} hour${hours === 1 ? '' : 's'} ago`
  if (minutes > 0) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`
  return 'just now'
}
</script>

<style scoped>
.mod-browse {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  overflow: hidden;
}

/* ---- Toolbar ---- */
.browse-toolbar {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background-color: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  flex-shrink: 0;
  flex-wrap: wrap;
}

.browse-search-input {
  flex: 1;
  max-width: 320px;
  min-width: 180px;
}

.browse-sort-select {
  min-width: 160px;
  max-width: 200px;
}

.source-chips {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  flex-wrap: wrap;
}

.chip-inactive {
  opacity: 0.7;
}

.source-chip-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  height: 16px;
  border-radius: 3px;
  font-size: 0.6rem;
  font-weight: 700;
  margin-right: 4px;
  flex-shrink: 0;
}

.source-chip-name {
  font-size: 0.8rem;
}

/* ---- Reset filters ---- */
.reset-filters-btn {
  color: var(--xy-text-muted);
  font-size: 0.8rem;
}

.reset-filters-btn:hover {
  color: var(--xy-text-primary);
}

/* ---- Filters row ---- */
.browse-filters {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-md);
  background-color: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  flex-shrink: 0;
  flex-wrap: wrap;
}

.browse-version-input {
  min-width: 160px;
  max-width: 200px;
}

.browse-category-select {
  min-width: 200px;
  max-width: 320px;
}

/* ---- Loading / Empty states ---- */
.browse-loading,
.browse-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-2xl) var(--xy-space-md);
}

.browse-empty-title {
  font-size: 0.9rem;
  font-weight: 500;
}

.browse-empty-subtitle {
  font-size: 0.8rem;
}

/* ---- Grid scroll ---- */
.browse-grid-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--xy-space-md);
  background-color: var(--xy-base);
}

.browse-grid-scroll::-webkit-scrollbar {
  width: 6px;
}

.browse-grid-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.browse-grid-scroll::-webkit-scrollbar-thumb {
  background: var(--xy-surface-4);
  border-radius: 3px;
}

/* ---- Grid ---- */
.browse-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: var(--xy-space-md);
}

/* ---- Pagination footer ---- */
.browse-pagination-footer {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md) 0 var(--xy-space-sm);
  flex-wrap: wrap;
}

.page-size-select {
  min-width: 130px;
  max-width: 160px;
}

.browse-pagination :deep(.q-btn) {
  min-width: 32px;
  min-height: 32px;
}

.browse-result-count {
  font-size: 0.8rem;
  white-space: nowrap;
}

/* ---- Card ---- */
.mod-card {
  display: flex;
  flex-direction: column;
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  padding: var(--xy-space-md);
  transition:
    background-color var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
  width: 100%;
}

.mod-card:has(.mod-card-details:hover) {
  background-color: var(--xy-surface-2);
  border-color: var(--xy-primary);
}

.mod-card:has(.mod-card-details:focus-visible) {
  outline: 2px solid var(--xy-primary);
  outline-offset: 2px;
}

.mod-card-details {
  display: flex;
  flex: 1;
  flex-direction: column;
  width: 100%;
  padding: 0;
  color: inherit;
  font: inherit;
  text-align: left;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.mod-card-details:focus-visible {
  outline: none;
}

/* ---- Card icon ---- */
.mod-card-icon-wrapper {
  width: 48px;
  height: 48px;
  border-radius: 8px;
  overflow: hidden;
  flex-shrink: 0;
  margin-bottom: var(--xy-space-sm);
}

.mod-card-icon-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mod-card-icon-fallback {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.2rem;
  font-weight: 700;
  color: var(--xy-text-on-color);
}

/* ---- Card content ---- */
.mod-card-content {
  flex: 1;
  min-height: 0;
}

.mod-card-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  margin-bottom: 2px;
}

.mod-card-name {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
  min-width: 0;
}

.mod-card-author {
  font-size: 0.7rem;
  margin-bottom: var(--xy-space-xs);
}

.mod-card-desc {
  font-size: 0.75rem;
  line-height: 1.4;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  margin-bottom: var(--xy-space-xs);
}

/* ---- Card categories ---- */
.mod-card-categories {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  margin-bottom: var(--xy-space-sm);
}

.mod-card-category {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 0.6rem;
  font-weight: 500;
  background-color: var(--xy-surface-3);
  color: var(--xy-text-muted);
  line-height: 1.5;
  text-transform: capitalize;
}

/* ---- Card footer ---- */
.mod-card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding-top: var(--xy-space-sm);
  border-top: 1px solid var(--xy-border);
}

.mod-card-footer-left {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mod-card-downloads,
.mod-card-updated {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.7rem;
}

/* ---- Source badge ---- */
.source-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 4px;
  font-size: 0.65rem;
  font-weight: 700;
  flex-shrink: 0;
}

/* ---- Installed badge ---- */
.installed-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.75rem;
  color: var(--xy-success);
  font-weight: 500;
}

/* ---- Error banner ---- */
.browse-error {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background-color: color-mix(in srgb, var(--xy-danger) 10%, var(--xy-surface-1));
  border-top: 1px solid var(--xy-danger);
  color: var(--xy-danger);
  font-size: 0.8rem;
  flex-shrink: 0;
}

/* ---- Mobile ---- */
@media (max-width: 767px) {
  .browse-toolbar {
    padding: var(--xy-space-xs) var(--xy-space-sm);
  }

  .browse-search-input {
    max-width: none;
    flex: 1 1 100%;
  }

  .browse-sort-select {
    max-width: none;
    flex: 1 1 100%;
  }

  .browse-filters {
    padding: var(--xy-space-xs) var(--xy-space-sm);
  }

  .browse-version-input {
    max-width: none;
    flex: 1 1 100%;
  }

  .browse-category-select {
    max-width: none;
    flex: 1 1 100%;
  }

  .browse-grid {
    grid-template-columns: 1fr;
  }

  .browse-pagination-footer {
    gap: var(--xy-space-sm);
  }

  .page-size-select {
    min-width: 110px;
    max-width: 130px;
  }
}
</style>
