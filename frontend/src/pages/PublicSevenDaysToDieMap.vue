<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { onBeforeUnmount, onMounted, ref } from 'vue'

import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import SevenDaysToDieWorldOverview from '@/components/seven_days_to_die/SevenDaysToDieWorldOverview.vue'
import {
  GetPublicSevenDaysToDieMapRequestSchema,
  type SevenDaysToDieMapView,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ identifier: string }>()
const pollIntervalMilliseconds = 5_000
const mapView = ref<SevenDaysToDieMapView | null>(null)
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
  pollTimer = setInterval(() => void loadMap(), pollIntervalMilliseconds)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) {
    clearInterval(pollTimer)
  }
})
</script>

<template>
  <main class="public-seven-days-map">
    <header class="public-seven-days-map__header">
      <div class="public-seven-days-map__brand">Xylona</div>
      <div class="public-seven-days-map__title">
        <span>7 Days to Die live map</span>
        <strong>{{ mapView?.gameServerName || 'Shared server' }}</strong>
      </div>
    </header>

    <div v-if="invalidLink" class="public-seven-days-map__invalid">
      <q-icon name="link_off" size="48px" />
      <h1>This map link is not available</h1>
      <p>It may be incomplete, replaced, or revoked by the server administrator.</p>
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
      v-if="!invalidLink"
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
  font-size: 1.25rem;
  letter-spacing: 0.05em;
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

.public-seven-days-map__title strong {
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.public-seven-days-map__invalid {
  display: grid;
  flex: 1;
  place-items: center;
  align-content: center;
  text-align: center;
}

.public-seven-days-map__invalid h1 {
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

@media (min-width: 1200px) {
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

@media (max-width: 600px) {
  .public-seven-days-map {
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
