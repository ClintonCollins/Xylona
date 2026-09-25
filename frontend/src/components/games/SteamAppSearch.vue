<script lang="ts" setup>
import { ref } from 'vue'
import { GetXylonaClient } from '@/utils/shared'
import { create } from '@bufbuild/protobuf'
import { GetSteamAppDetailsRequestSchema, ListGamesRequestSchema } from '@/proto/xylona_pb'
import type { Game } from '@/proto/shared_pb'

const emit = defineEmits<{
  (e: 'select', value: { appId: string; name: string }): void
}>()

const appIdInput = ref('')
const loading = ref(false)
const lookupError = ref('')
const lookupResult = ref<{
  appId: string
  name: string
  type: string
  windowsSupport: boolean
  linuxSupport: boolean
  installDirectory: string
} | null>(null)
// Games already in the catalog with the looked-up AppID, so the wizard does not create a duplicate.
const catalogMatches = ref<Game[]>([])

async function findCatalogMatches(appId: string): Promise<Game[]> {
  try {
    const response = await GetXylonaClient().listGames(create(ListGamesRequestSchema, {}))
    return response.games.filter((game) => game.steamAppid.trim() === appId)
  } catch (err: unknown) {
    // The catalog check only prevents duplicates; the lookup itself still works without it.
    console.error('Game catalog check failed:', err)
    return []
  }
}

async function lookupAppId(): Promise<void> {
  const trimmed = appIdInput.value.trim()
  if (trimmed.length === 0) {
    return
  }

  loading.value = true
  lookupError.value = ''
  lookupResult.value = null
  catalogMatches.value = []

  try {
    const client = GetXylonaClient()
    const req = create(GetSteamAppDetailsRequestSchema, { appId: trimmed })
    const [response, matches] = await Promise.all([
      client.getSteamAppDetails(req),
      findCatalogMatches(trimmed),
    ])
    catalogMatches.value = matches

    if (!response.detailsAvailable || !response.details) {
      lookupError.value = `No information found for AppID ${trimmed}. Check the ID and try again.`
      return
    }

    lookupResult.value = {
      appId: response.details.appId,
      name: response.details.name,
      type: response.details.windowsSupport || response.details.linuxSupport ? 'Tool' : '',
      windowsSupport: response.details.windowsSupport,
      linuxSupport: response.details.linuxSupport,
      installDirectory: response.details.installDirectory,
    }
  } catch (err: unknown) {
    console.error('Steam app lookup failed:', err)
    lookupError.value = 'Failed to look up AppID. Please check your connection and try again.'
  } finally {
    loading.value = false
  }
}

function confirmSelection(): void {
  if (!lookupResult.value) {
    return
  }
  emit('select', {
    appId: lookupResult.value.appId,
    name: lookupResult.value.name,
  })
}

function clearResult(): void {
  lookupResult.value = null
  lookupError.value = ''
  appIdInput.value = ''
  catalogMatches.value = []
}
</script>

<template>
  <div class="steam-app-lookup">
    <p class="text-body2 q-mb-md" style="color: var(--xy-text-secondary)">
      Enter the Steam AppID for the game's <strong>dedicated server</strong>. You can find this on
      the game's
      <a href="https://steamdb.info/" rel="noopener" style="color: var(--xy-accent)" target="_blank"
        >SteamDB page</a
      >
      or the
      <a
        href="https://developer.valvesoftware.com/wiki/Dedicated_Servers_List"
        rel="noopener"
        style="color: var(--xy-accent)"
        target="_blank"
        >Valve Dedicated Servers List</a
      >.
    </p>

    <div class="row items-center q-gutter-sm">
      <q-input
        v-model="appIdInput"
        :error="lookupError.length > 0"
        :error-message="lookupError"
        :loading="loading"
        class="col"
        label="Dedicated Server AppID"
        outlined
        placeholder="e.g. 896660"
        type="number"
        @keydown.enter="lookupAppId">
        <template #prepend>
          <q-icon name="cloud_download" />
        </template>
      </q-input>

      <q-btn
        :disable="appIdInput.trim().length === 0"
        :loading="loading"
        :outline="lookupResult !== null"
        color="primary"
        label="Look Up"
        unelevated
        @click="lookupAppId" />
    </div>

    <!-- Lookup errors are announced by the input's own alert; this announces a found app. -->
    <div class="xy-visually-hidden" role="status">
      <template v-if="lookupResult">
        Found {{ lookupResult.name }}, AppID {{ lookupResult.appId }}.
        <template v-if="catalogMatches.length > 0">Already in your catalog.</template>
      </template>
    </div>

    <!-- Lookup Result -->
    <q-card v-if="lookupResult" bordered class="q-mt-md lookup-result-card" flat>
      <q-card-section>
        <div class="row items-center q-gutter-sm q-mb-sm">
          <q-icon color="positive" name="check_circle" size="sm" />
          <span class="text-subtitle1 text-weight-medium">{{ lookupResult.name }}</span>
          <q-badge :label="'AppID: ' + lookupResult.appId" class="bg-xy-surface-3 xy-num" />
        </div>

        <div class="row q-gutter-md text-body2" style="color: var(--xy-text-secondary)">
          <div v-if="lookupResult.windowsSupport" class="row items-center q-gutter-xs">
            <q-icon name="desktop_windows" size="xs" />
            <span>Windows</span>
          </div>
          <div v-if="lookupResult.linuxSupport" class="row items-center q-gutter-xs">
            <q-icon name="terminal" size="xs" />
            <span>Linux</span>
          </div>
          <div v-if="lookupResult.installDirectory" class="row items-center q-gutter-xs">
            <q-icon name="folder" size="xs" />
            <span>{{ lookupResult.installDirectory }}</span>
          </div>
        </div>
      </q-card-section>

      <q-card-section v-if="catalogMatches.length > 0" class="q-pt-none">
        <q-banner
          v-for="match in catalogMatches"
          :key="match.id"
          class="xy-banner-info catalog-match"
          data-test="catalog-match"
          dense>
          <template #avatar>
            <q-icon name="inventory_2" size="sm" />
          </template>
          Already in your catalog: <strong>{{ match.name }}</strong> ({{
            match.xylonaOfficial ? 'Official' : 'Custom'
          }})
          <template #action>
            <q-btn
              :to="`/games/${match.id}/edit`"
              color="primary"
              dense
              flat
              label="Open"
              no-caps />
            <q-btn :to="`/games/${match.id}/copy`" dense flat label="Copy" no-caps />
          </template>
        </q-banner>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn color="negative" dense flat label="Clear" no-caps @click="clearResult" />
        <q-btn
          v-if="catalogMatches.length === 0"
          color="primary"
          dense
          label="Use This Server"
          no-caps
          unelevated
          @click="confirmSelection" />
      </q-card-actions>
    </q-card>
  </div>
</template>

<style scoped>
.steam-app-lookup {
  max-width: 600px;
}

.lookup-result-card {
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.catalog-match + .catalog-match {
  margin-top: var(--xy-space-sm);
}
</style>
