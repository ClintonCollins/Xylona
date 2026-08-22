<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import GameServerMapShareSettings from '@/components/game_servers/GameServerMapShareSettings.vue'
import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import {
  GetGameServerRequestSchema,
  GetSevenDaysToDieMapRequestSchema,
  type SevenDaysToDieMapView,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const pollIntervalMilliseconds = 5_000

const route = useRoute()
const mapView = ref<SevenDaysToDieMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const canManage = ref(false)
const shareOpen = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})

async function loadPermissions(): Promise<void> {
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerID.value }),
    )
    canManage.value =
      response.gameServer?.effectivePermissions.includes('game_server.settings') ?? false
  } catch (unknownError: unknown) {
    console.error(unknownError)
  }
}

async function loadMap(): Promise<void> {
  if (loading.value || gameServerID.value === '') {
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getSevenDaysToDieMap(
      create(GetSevenDaysToDieMapRequestSchema, { gameServerId: gameServerID.value }),
    )
    mapView.value = response.map ?? null
    loadError.value = false
  } catch (unknownError: unknown) {
    loadError.value = true
    console.error(unknownError)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadPermissions()
  void loadMap()
  pollTimer = setInterval(() => void loadMap(), pollIntervalMilliseconds)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) {
    clearInterval(pollTimer)
  }
})
</script>

<template>
  <div class="seven-days-map-page">
    <header class="seven-days-map-page__header">
      <div class="seven-days-map-page__heading">
        <span class="seven-days-map-page__heading-icon"><q-icon name="map" /></span>
        <div>
          <span>7 Days to Die</span>
          <h1>Live world map</h1>
          <p>Live and last-known player positions across the world.</p>
        </div>
      </div>
      <div v-if="canManage" class="seven-days-map-page__actions">
        <q-btn color="primary" icon="share" label="Public link" no-caps @click="shareOpen = true" />
      </div>
    </header>

    <seven-days-to-die-live-map
      :load-error="loadError"
      :loading="loading"
      :view="mapView"
      @refresh="loadMap" />

    <q-dialog v-model="shareOpen">
      <game-server-map-share-settings :game-server-id="gameServerID" @close="shareOpen = false" />
    </q-dialog>
  </div>
</template>

<style scoped>
.seven-days-map-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--xy-space-md);
  min-height: 0;
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
}

.seven-days-map-page__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-lg);
}

.seven-days-map-page__heading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
}

.seven-days-map-page__heading-icon {
  display: grid;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  place-items: center;
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
  border: 1px solid var(--xy-accent-border);
  border-radius: var(--xy-radius-md);
}

.seven-days-map-page__heading > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.seven-days-map-page__heading span {
  color: var(--xy-accent);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.seven-days-map-page__heading h1 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.seven-days-map-page__heading h1 {
  font-size: var(--xy-font-size-xl);
}

.seven-days-map-page__heading p {
  margin: 0;
  color: var(--xy-text-secondary);
}

.seven-days-map-page__actions {
  display: flex;
  gap: var(--xy-space-sm);
}

@media (max-width: 760px) {
  .seven-days-map-page {
    padding: var(--xy-space-sm) 0 0;
  }

  .seven-days-map-page__header {
    align-items: flex-start;
    padding: 0 var(--xy-space-sm);
  }

  .seven-days-map-page__heading p {
    display: none;
  }

  .seven-days-map-page__actions {
    flex-direction: column;
  }
}
</style>
