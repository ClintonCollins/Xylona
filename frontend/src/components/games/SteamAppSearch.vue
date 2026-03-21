<script setup lang="ts">
import { ref, watch } from 'vue'
import { GetXylonaClient } from '@/utils/shared'
import { create } from '@bufbuild/protobuf'
import { SearchSteamAppsRequestSchema } from '@/proto/xylona_pb'
import type { SteamApp } from '@/proto/shared_pb'

const emit = defineEmits<{
  (e: 'select', value: { appId: string; name: string }): void
}>()

const searchText = ref('')
const results = ref<SteamApp[]>([])
const loading = ref(false)
const showResults = ref(false)
const manualMode = ref(false)
const manualAppId = ref('')

let debounceTimer: ReturnType<typeof setTimeout> | undefined

watch(searchText, (query) => {
  if (debounceTimer !== undefined) {
    clearTimeout(debounceTimer)
  }

  if (query.length < 2) {
    results.value = []
    showResults.value = false
    return
  }

  debounceTimer = setTimeout(() => {
    void performSearch(query)
  }, 300)
})

async function performSearch(query: string): Promise<void> {
  loading.value = true
  showResults.value = true
  try {
    const client = GetXylonaClient()
    const req = create(SearchSteamAppsRequestSchema, { query })
    const response = await client.searchSteamApps(req)
    results.value = response.apps
  } catch (err: unknown) {
    console.error('Steam app search failed:', err)
    results.value = []
  } finally {
    loading.value = false
  }
}

function selectApp(app: SteamApp): void {
  searchText.value = `${app.name} (${app.appId})`
  showResults.value = false
  emit('select', { appId: app.appId, name: app.name })
}

function submitManualAppId(): void {
  const trimmed = manualAppId.value.trim()
  if (trimmed.length === 0) {
    return
  }
  emit('select', { appId: trimmed, name: '' })
}

function toggleManualMode(): void {
  manualMode.value = !manualMode.value
  results.value = []
  showResults.value = false
  searchText.value = ''
  manualAppId.value = ''
}

function onInputBlur(): void {
  // Delay hiding results so click events on items can fire first
  setTimeout(() => {
    showResults.value = false
  }, 200)
}
</script>

<template>
  <div class="steam-app-search">
    <template v-if="!manualMode">
      <q-input
        v-model="searchText"
        outlined
        label="Search Steam Apps"
        placeholder="Type to search (e.g. Minecraft, Valheim)..."
        :loading="loading"
        @blur="onInputBlur"
        @focus="showResults = results.length > 0">
        <template #prepend>
          <q-icon name="mdi-magnify" />
        </template>
        <template v-if="loading" #append>
          <q-spinner color="primary" size="1em" />
        </template>
      </q-input>

      <q-list v-if="showResults && results.length > 0" class="steam-app-results" bordered separator>
        <q-item
          v-for="app in results"
          :key="app.appId"
          clickable
          @mousedown.prevent="selectApp(app)">
          <q-item-section side>
            <q-badge color="grey-8" :label="app.appId" />
          </q-item-section>
          <q-item-section>
            <q-item-label>{{ app.name }}</q-item-label>
          </q-item-section>
        </q-item>
      </q-list>

      <div
        v-if="showResults && !loading && results.length === 0 && searchText.length >= 2"
        class="steam-app-no-results text-body2 text-secondary q-pa-sm">
        No results found.
      </div>
    </template>

    <template v-else>
      <q-input
        v-model="manualAppId"
        outlined
        label="Steam App ID"
        placeholder="Enter the numeric App ID (e.g. 294420)"
        type="number"
        @keydown.enter="submitManualAppId">
        <template #prepend>
          <q-icon name="mdi-steam" />
        </template>
        <template #append>
          <q-btn
            flat
            dense
            color="primary"
            label="Apply"
            :disable="manualAppId.trim().length === 0"
            @click="submitManualAppId" />
        </template>
      </q-input>
    </template>

    <div class="q-mt-xs">
      <q-btn
        flat
        dense
        no-caps
        size="sm"
        color="accent"
        :label="manualMode ? 'Search by name instead' : 'Enter AppID manually'"
        @click="toggleManualMode" />
    </div>
  </div>
</template>

<style scoped>
.steam-app-search {
  position: relative;
}

.steam-app-results {
  position: absolute;
  z-index: 100;
  left: 0;
  right: 0;
  max-height: 280px;
  overflow-y: auto;
  background: var(--xy-surface-2, #1e2021);
  border-color: var(--xy-border, #2a2d2f);
  border-radius: 0 0 4px 4px;
}

.steam-app-no-results {
  color: var(--xy-text-secondary, #979b9e);
}
</style>
