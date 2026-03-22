<template>
  <div class="mod-browse">
    <!-- Toolbar: search + source filters -->
    <div class="browse-toolbar">
      <q-input
        v-model="searchQuery"
        dense
        outlined
        placeholder="Search mods..."
        aria-label="Search mods"
        class="browse-search-input"
        clearable
        @clear="searchQuery = ''"
        @update:model-value="debouncedSearch">
        <template #prepend>
          <q-icon name="search" size="xs" class="text-xy-muted" aria-hidden="true" />
        </template>
      </q-input>

      <q-select
        v-model="sortBy"
        :options="sortOptions"
        dense
        outlined
        emit-value
        map-options
        aria-label="Sort by"
        class="browse-sort-select" />

      <div class="source-chips" role="group" aria-label="Filter by source">
        <q-btn
          :outline="activeSource !== ''"
          :unelevated="activeSource === ''"
          :color="activeSource === '' ? 'primary' : undefined"
          :class="{ 'chip-inactive': activeSource !== '' }"
          size="sm"
          dense
          no-caps
          label="All"
          @click="setActiveSource('')" />
        <q-btn
          v-for="src in sources"
          :key="src.id"
          :outline="activeSource !== src.id"
          :unelevated="activeSource === src.id"
          size="sm"
          dense
          no-caps
          :class="{ 'chip-inactive': activeSource !== src.id }"
          @click="setActiveSource(src.id)">
          <span class="source-chip-badge" :style="sourceBadgeStyle(src.id)" aria-hidden="true">
            {{ sourceLabel(src.id) }}
          </span>
          <span class="source-chip-name">{{ sourceDisplayName(src.id) }}</span>
        </q-btn>
      </div>
    </div>

    <!-- Filters row -->
    <div v-if="gameVersionFilter || availableCategories.length > 0" class="browse-filters">
      <q-input
        v-model="gameVersionFilter"
        dense
        outlined
        placeholder="Game version filter..."
        aria-label="Filter by game version"
        class="browse-version-input"
        clearable
        @clear="gameVersionFilter = ''" />

      <q-select
        v-if="availableCategories.length > 0"
        v-model="categoryFilter"
        :options="availableCategories"
        dense
        outlined
        multiple
        use-chips
        emit-value
        aria-label="Filter by category"
        placeholder="Categories"
        class="browse-category-select" />
    </div>

    <!-- Loading state -->
    <div v-if="loading && results.length === 0" class="browse-loading">
      <q-spinner color="primary" size="2rem" />
      <span class="text-xy-muted">Loading mods...</span>
    </div>

    <!-- Empty: no results -->
    <div v-else-if="hasSearched && results.length === 0" class="browse-empty">
      <q-icon name="search_off" size="3rem" class="text-xy-muted" aria-hidden="true" />
      <div class="browse-empty-title text-xy-secondary">No mods found</div>
      <div class="browse-empty-subtitle text-xy-muted">
        Try a different search term or source filter.
      </div>
    </div>

    <!-- Results grid -->
    <div v-else-if="results.length > 0" class="browse-grid-scroll">
      <div class="browse-grid">
        <button
          v-for="mod in results"
          :key="`${mod.source}-${mod.sourceId}`"
          type="button"
          class="mod-card"
          :aria-label="`View details for ${mod.name}`"
          @click="emit('view-details', mod.source, mod.sourceId)">
          <!-- Icon -->
          <div class="mod-card-icon-wrapper">
            <img
              v-if="mod.iconUrl"
              :src="mod.iconUrl"
              :alt="`${mod.name} icon`"
              class="mod-card-icon-img"
              loading="lazy"
              @error="($event.target as HTMLImageElement).style.display = 'none'" />
            <div
              v-if="!mod.iconUrl"
              class="mod-card-icon-fallback"
              :style="{ background: iconGradient(mod.name) }"
              aria-hidden="true">
              {{ mod.name.charAt(0).toUpperCase() }}
            </div>
          </div>

          <!-- Content -->
          <div class="mod-card-content">
            <div class="mod-card-header">
              <span class="mod-card-name">{{ mod.name }}</span>
              <span
                class="source-badge"
                :style="sourceBadgeStyle(mod.source)"
                :title="sourceDisplayName(mod.source)">
                {{ sourceLabel(mod.source) }}
              </span>
            </div>
            <div class="mod-card-author text-xy-muted">by {{ mod.author }}</div>
            <div class="mod-card-desc text-xy-secondary">{{ mod.description }}</div>
          </div>

          <!-- Footer -->
          <div class="mod-card-footer">
            <span class="mod-card-downloads text-xy-muted">
              <q-icon name="download" size="xs" aria-hidden="true" />
              {{ formatDownloads(mod.downloads) }}
            </span>

            <span v-if="isModInstalled(mod.source, mod.sourceId)" class="installed-badge">
              <q-icon name="check_circle" size="xs" aria-hidden="true" />
              Installed
            </span>
            <q-btn
              v-else
              color="primary"
              size="sm"
              dense
              no-caps
              label="Install"
              icon="add"
              @click="onInstallClick($event, mod.source, mod.sourceId)" />
          </div>
        </button>
      </div>

      <!-- Load More -->
      <div v-if="hasMoreResults" class="browse-load-more">
        <q-btn
          outline
          color="primary"
          no-caps
          :loading="loadingMore"
          label="Load More"
          icon="expand_more"
          @click="loadMore" />
      </div>
    </div>

    <!-- Error banner -->
    <div v-if="errorMessage" class="browse-error">
      <q-icon name="error" size="sm" color="negative" aria-hidden="true" />
      <span>{{ errorMessage }}</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import type { InstalledMod, ModSearchResult } from '@/proto/shared_pb'
import { GetModCategoriesRequestSchema, SearchModsRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

interface ModSource {
  id: string
  searchParams: Record<string, unknown>
}

interface Props {
  gameServerId: string
  installedMods: InstalledMod[]
  sources: ModSource[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'view-details': [source: string, sourceId: string]
  install: [source: string, sourceId: string]
}>()

const PAGE_SIZE = 20

const searchQuery = ref('')
const activeSource = ref('')
const loading = ref(false)
const loadingMore = ref(false)
const hasSearched = ref(false)
const results = ref<ModSearchResult[]>([])
const errorMessage = ref('')
const currentPage = ref(1)
const totalCount = ref(0)

const sortBy = ref<string>('downloads')
const gameVersionFilter = ref('')
const categoryFilter = ref<string[]>([])
const availableCategories = ref<string[]>([])

let debounceTimer: ReturnType<typeof setTimeout> | undefined

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

const hasMoreResults = computed(() => {
  return results.value.length < totalCount.value
})

onMounted(() => {
  void performSearch()
  void loadCategories()
})

watch(sortBy, () => {
  void resetAndSearch()
})

watch(gameVersionFilter, () => {
  void resetAndSearch()
})

watch(categoryFilter, () => {
  void resetAndSearch()
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
  activeSource.value = source
  void resetAndSearch()
}

async function resetAndSearch(): Promise<void> {
  currentPage.value = 1
  results.value = []
  await performSearch()
}

async function performSearch(): Promise<void> {
  loading.value = true
  errorMessage.value = ''

  try {
    const client = GetXylonaClient()
    const request = create(SearchModsRequestSchema, {
      gameServerId: props.gameServerId,
      query: searchQuery.value.trim(),
      source: activeSource.value,
      page: currentPage.value,
      pageSize: PAGE_SIZE,
      sortBy: sortBy.value,
      gameVersion: gameVersionFilter.value,
      categories: categoryFilter.value,
    })
    const response = await client.searchMods(request)
    if (currentPage.value === 1) {
      results.value = response.results
    } else {
      results.value = [...results.value, ...response.results]
    }
    totalCount.value = response.totalCount
    hasSearched.value = true
  } catch (err: unknown) {
    if (err instanceof ConnectError) {
      errorMessage.value = ConnectErrorToString(err)
    } else {
      errorMessage.value = 'An unexpected error occurred while searching.'
    }
    if (currentPage.value === 1) {
      results.value = []
    }
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

async function loadMore(): Promise<void> {
  loadingMore.value = true
  currentPage.value += 1
  await performSearch()
}

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

const SOURCE_BADGES: Record<string, { bg: string; letter: string; name: string }> = {
  modrinth: { bg: '#1BD96A', letter: 'M', name: 'Modrinth' },
  hangar: { bg: '#2196F3', letter: 'H', name: 'Hangar' },
  thunderstore: { bg: '#0066FF', letter: 'T', name: 'Thunderstore' },
  steam_workshop: { bg: '#1B2838', letter: 'S', name: 'Steam Workshop' },
  papermc: { bg: '#2196F3', letter: 'P', name: 'PaperMC' },
}

function sourceBadgeStyle(source: string): Record<string, string> {
  const config = SOURCE_BADGES[source]
  if (!config) return { backgroundColor: 'var(--xy-surface-3)', color: 'var(--xy-text-primary)' }
  return { backgroundColor: config.bg, color: '#FFFFFF' }
}

function sourceLabel(source: string): string {
  return SOURCE_BADGES[source]?.letter ?? source.charAt(0).toUpperCase()
}

function sourceDisplayName(source: string): string {
  return SOURCE_BADGES[source]?.name ?? source
}

function iconGradient(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  const hue1 = Math.abs(hash) % 360
  const hue2 = (hue1 + 40) % 360
  return `linear-gradient(135deg, hsl(${hue1}, 60%, 40%), hsl(${hue2}, 60%, 30%))`
}

function formatDownloads(downloads: bigint): string {
  const num = Number(downloads)
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`
  return num.toString()
}
</script>

<style scoped>
.mod-browse {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
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

/* ---- Load More ---- */
.browse-load-more {
  display: flex;
  justify-content: center;
  padding: var(--xy-space-lg) 0 var(--xy-space-sm);
}

/* ---- Card ---- */
.mod-card {
  display: flex;
  flex-direction: column;
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  padding: var(--xy-space-md);
  cursor: pointer;
  transition:
    background-color var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
  text-align: left;
  font-family: inherit;
  font-size: inherit;
  color: inherit;
  width: 100%;
}

.mod-card:hover {
  background-color: var(--xy-surface-2);
  border-color: var(--xy-primary);
}

.mod-card:focus-visible {
  outline: 2px solid var(--xy-primary);
  outline-offset: 2px;
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
  color: #fff;
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
  margin-bottom: var(--xy-space-sm);
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

.mod-card-downloads {
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
}
</style>
