<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { defineAsyncComponent, onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

import SevenDaysToDieWorldOverview from '@/components/seven_days_to_die/SevenDaysToDieWorldOverview.vue'
import {
  GetPublicSevenDaysToDieMapRequestSchema,
  type SevenDaysToDieMapView,
} from '@/proto/xylona_pb'
import { setPageTitle } from '@/utils/page-title'
import { GetXylonaClient } from '@/utils/shared'

const SevenDaysToDieLiveMap = defineAsyncComponent(
  () => import('@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'),
)

const props = defineProps<{ identifier: string }>()
const pollIntervalMilliseconds = 5_000
// Each poll replaces the view wholesale, so deep reactivity would only re-proxy the snapshot.
const mapView = shallowRef<SevenDaysToDieMapView | null>(null)
watch(
  () => mapView.value?.gameServerName,
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
    const response = await GetXylonaClient().getPublicSevenDaysToDieMap(
      create(GetPublicSevenDaysToDieMapRequestSchema, { publicIdentifier: props.identifier }),
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
  pollTimer = setInterval(() => void loadMap(), pollIntervalMilliseconds)
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

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  stopPolling()
})
</script>

<template>
  <main class="public-seven-days-map">
    <header class="public-seven-days-map__header">
      <div class="public-seven-days-map__brand">Xylona</div>
      <div class="public-seven-days-map__title">
        <span>7 Days to Die live map</span>
        <h1>{{ mapView?.gameServerName || 'Shared server' }}</h1>
      </div>
    </header>

    <div v-if="invalidLink" class="public-seven-days-map__invalid">
      <q-icon name="link_off" size="48px" />
      <h2>This map link is not available</h2>
      <p>It may be incomplete, replaced, or revoked by the server administrator.</p>
    </div>
    <div v-else-if="loadError && mapView === null" class="public-seven-days-map__invalid">
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
    <seven-days-to-die-live-map
      v-else
      class="public-seven-days-map__map"
      :load-error="loadError"
      :loading="loading"
      :public-identifier="identifier"
      :view="mapView"
      @refresh="loadMap" />
    <seven-days-to-die-world-overview
      v-if="!invalidLink && !(loadError && mapView === null)"
      class="public-seven-days-map__overview"
      :show-tactical="!loadError"
      :status-loading="loading && mapView === null"
      :view="mapView" />
  </main>
</template>

<style scoped>
.public-seven-days-map {
  display: flex;
  height: 100dvh;
  min-height: 100dvh;
  flex-direction: column;
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-base);
}

.public-seven-days-map__header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-lg);
  min-height: 64px;
  padding: 0 var(--xy-space-sm) var(--xy-space-md);
}

.public-seven-days-map__brand {
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
}

.public-seven-days-map__title {
  display: grid;
  gap: var(--xy-space-2xs);
  padding-left: var(--xy-space-lg);
  border-left: 1px solid var(--xy-border);
}

.public-seven-days-map__title span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.public-seven-days-map__title h1 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  font-weight: 500;
  letter-spacing: normal;
}

.public-seven-days-map__invalid {
  display: grid;
  flex: 1;
  place-items: center;
  align-content: center;
  text-align: center;
}

.public-seven-days-map__invalid h2 {
  margin: var(--xy-space-md) 0 var(--xy-space-sm);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-xl);
}

.public-seven-days-map__invalid p {
  max-width: 48ch;
  margin: 0;
  color: var(--xy-text-secondary);
}

.public-seven-days-map__map {
  min-width: 0;
}

.public-seven-days-map__overview {
  margin-top: var(--xy-space-md);
}

@media (min-width: 1024px) {
  .public-seven-days-map {
    display: grid;
    grid-template-areas:
      'header header'
      'map overview';
    grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
    grid-template-rows: auto minmax(0, 1fr);
    column-gap: var(--xy-space-md);
  }

  .public-seven-days-map__header {
    grid-area: header;
  }

  .public-seven-days-map__map {
    grid-area: map;
  }

  .public-seven-days-map__overview {
    grid-area: overview;
    min-width: 0;
    margin-top: 0;
  }
}

@media (max-width: 599px) {
  /* The map card grows to its content on phones, so the page scrolls. */
  .public-seven-days-map {
    height: auto;
    padding: var(--xy-space-xs);
  }

  .public-seven-days-map__header {
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm);
  }

  .public-seven-days-map__title {
    padding-left: var(--xy-space-sm);
  }
}
</style>
