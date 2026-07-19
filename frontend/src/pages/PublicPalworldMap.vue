<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { onBeforeUnmount, onMounted, ref } from 'vue'

import PalworldLiveMap from '@/components/palworld/PalworldLiveMap.vue'
import { GetPublicPalworldMapRequestSchema, type PalworldMapView } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const pollIntervalMs = 15_000
const mapView = ref<PalworldMapView | null>(null)
const loading = ref(false)
const invalidLink = ref(false)
const loadError = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

function shareToken(): string {
  return window.location.hash.slice(1).trim()
}

async function loadMap(): Promise<void> {
  const token = shareToken()
  if (loading.value || token === '') {
    invalidLink.value = token === ''
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getPublicPalworldMap(
      create(GetPublicPalworldMapRequestSchema, { shareToken: token }),
    )
    mapView.value = response.map ?? null
    invalidLink.value = false
    loadError.value = false
  } catch (unknownError: unknown) {
    console.error(unknownError)
    if (mapView.value === null) {
      invalidLink.value = true
    } else {
      loadError.value = true
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadMap()
  pollTimer = setInterval(() => void loadMap(), pollIntervalMs)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) {
    clearInterval(pollTimer)
  }
})
</script>

<template>
  <main class="public-palworld-map">
    <header class="public-palworld-map__header">
      <div class="public-palworld-map__brand">XYLONA</div>
      <div class="public-palworld-map__title">
        <span>Palworld live map</span>
        <strong>{{ mapView?.serverName || 'Shared server' }}</strong>
      </div>
    </header>

    <div v-if="invalidLink" class="public-palworld-map__invalid">
      <q-icon name="link_off" size="48px" />
      <h1>This map link is not available</h1>
      <p>It may be incomplete, expired, or revoked by the server administrator.</p>
    </div>
    <palworld-live-map
      v-else
      :load-error="loadError"
      :loading="loading"
      public-mode
      :view="mapView"
      @refresh="loadMap" />
  </main>
</template>

<style scoped>
.public-palworld-map {
  min-height: 100dvh;
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-base);
}

.public-palworld-map__header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-lg);
  min-height: 64px;
  padding: 0 var(--xy-space-sm) var(--xy-space-md);
}

.public-palworld-map__brand {
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-lg);
  letter-spacing: 0.08em;
}

.public-palworld-map__title {
  display: grid;
  gap: var(--xy-space-2xs);
  padding-left: var(--xy-space-lg);
  border-left: 1px solid var(--xy-border);
}

.public-palworld-map__title span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.public-palworld-map__title strong {
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.public-palworld-map__invalid {
  display: grid;
  place-items: center;
  align-content: center;
  min-height: calc(100dvh - 96px);
  text-align: center;
}

.public-palworld-map__invalid h1 {
  margin: var(--xy-space-md) 0 var(--xy-space-sm);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-xl);
}

.public-palworld-map__invalid p {
  max-width: 48ch;
  margin: 0;
  color: var(--xy-text-secondary);
}

@media (max-width: 600px) {
  .public-palworld-map {
    padding: var(--xy-space-xs);
  }

  .public-palworld-map__header {
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm);
  }

  .public-palworld-map__title {
    padding-left: var(--xy-space-sm);
  }
}
</style>
