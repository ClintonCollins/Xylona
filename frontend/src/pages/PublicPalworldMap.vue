<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { defineAsyncComponent, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'

import { GetPublicPalworldMapRequestSchema, type PalworldMapView } from '@/proto/xylona_pb'
import { setPageTitle } from '@/utils/page-title'
import { GetXylonaClient } from '@/utils/shared'

const PalworldLiveMap = defineAsyncComponent(
  () => import('@/components/palworld/PalworldLiveMap.vue'),
)

const props = defineProps<{ identifier: string }>()
const pollIntervalMs = 5_000
// Each poll replaces the view wholesale, so deep reactivity would only re-proxy
// every actor in the snapshot for nothing.
const mapView = shallowRef<PalworldMapView | null>(null)
watch(
  () => mapView.value?.serverName,
  (name) => {
    if (name) setPageTitle(`${name} live map`)
  },
)
const loading = ref(false)
const invalidLink = ref(false)
const loadError = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

async function loadMap(): Promise<void> {
  if (loading.value || props.identifier === '') {
    invalidLink.value = props.identifier === ''
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getPublicPalworldMap(
      create(GetPublicPalworldMapRequestSchema, { publicIdentifier: props.identifier }),
    )
    mapView.value = response.map ?? null
    invalidLink.value = false
    loadError.value = false
  } catch (unknownError: unknown) {
    console.error(unknownError)
    // Only NotFound means the link is gone; anything else is a temporary outage.
    if (ConnectError.from(unknownError).code === Code.NotFound) {
      mapView.value = null
      invalidLink.value = true
      loadError.value = false
      stopPolling()
    } else {
      loadError.value = true
    }
  } finally {
    loading.value = false
  }
}

function stopPolling() {
  if (pollTimer !== undefined) {
    clearInterval(pollTimer)
    pollTimer = undefined
  }
}

function startPolling() {
  stopPolling()
  void loadMap()
  pollTimer = setInterval(() => {
    void loadMap()
  }, pollIntervalMs)
}

function handleVisibilityChange() {
  // A revoked link (NotFound) stays gone, so it never resumes polling.
  if (document.visibilityState === 'hidden' || invalidLink.value) {
    stopPolling()
    return
  }
  startPolling()
}

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  handleVisibilityChange()
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  stopPolling()
})
</script>

<template>
  <main class="public-palworld-map">
    <header class="public-palworld-map__header">
      <div class="public-palworld-map__brand">Xylona</div>
      <div class="public-palworld-map__title">
        <span>Palworld live map</span>
        <h1>{{ mapView?.serverName || 'Shared server' }}</h1>
      </div>
    </header>

    <div v-if="invalidLink" class="public-palworld-map__invalid">
      <q-icon name="link_off" size="48px" />
      <h2>This map link is not available</h2>
      <p>It may be incomplete, expired, or revoked by the server administrator.</p>
    </div>
    <div v-else-if="loadError && mapView === null" class="public-palworld-map__invalid">
      <q-icon name="cloud_off" size="48px" />
      <h2>Map temporarily unavailable</h2>
      <p>The map could not be reached. Try again in a moment.</p>
      <q-btn
        class="q-mt-md"
        color="primary"
        label="Retry"
        :loading="loading"
        no-caps
        @click="loadMap" />
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
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
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

.public-palworld-map__title h1 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  font-weight: 500;
  letter-spacing: normal;
}

.public-palworld-map__invalid {
  display: grid;
  place-items: center;
  align-content: center;
  min-height: calc(100dvh - 96px);
  text-align: center;
}

.public-palworld-map__invalid h2 {
  margin: var(--xy-space-md) 0 var(--xy-space-sm);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-xl);
}

.public-palworld-map__invalid p {
  max-width: 48ch;
  margin: 0;
  color: var(--xy-text-secondary);
}

@media (max-width: 599px) {
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
