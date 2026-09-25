<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { onBeforeUnmount, onMounted, ref, shallowRef, watch } from 'vue'

import { GetPublicMinecraftMapRequestSchema, type MinecraftMapView } from '@/proto/xylona_pb'
import { setPageTitle } from '@/utils/page-title'
import { GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ identifier: string }>()
const pollIntervalMilliseconds = 10_000
const viewerRefreshMilliseconds = 45 * 60 * 1_000
const mapView = shallowRef<MinecraftMapView | null>(null)
watch(
  () => mapView.value?.gameServerName,
  (name) => {
    if (name) setPageTitle(`${name} live map`)
  },
)
const viewerURL = ref('')
const viewerURLSetAt = ref(0)
const loading = ref(false)
const invalidLink = ref(false)
const loadError = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

function assignViewerURL(nextViewerURL: string): void {
  if (nextViewerURL === '') {
    viewerURL.value = ''
    viewerURLSetAt.value = 0
    return
  }
  if (viewerURL.value === '' || Date.now() - viewerURLSetAt.value >= viewerRefreshMilliseconds) {
    viewerURL.value = nextViewerURL
    viewerURLSetAt.value = Date.now()
  }
}

async function loadMap(): Promise<void> {
  if (loading.value || props.identifier === '') {
    invalidLink.value = props.identifier === ''
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getPublicMinecraftMap(
      create(GetPublicMinecraftMapRequestSchema, { publicIdentifier: props.identifier }),
    )
    mapView.value = response.map ?? null
    assignViewerURL(response.map?.viewerUrl ?? '')
    invalidLink.value = false
    loadError.value = false
  } catch (unknownError: unknown) {
    console.error(unknownError)
    const connectError = ConnectError.from(unknownError)
    if (connectError.code === Code.NotFound) {
      mapView.value = null
      assignViewerURL('')
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
  <main class="public-minecraft-map">
    <header class="public-minecraft-map__header">
      <div class="public-minecraft-map__brand">Xylona</div>
      <div class="public-minecraft-map__title">
        <span>Minecraft live map</span>
        <h1>{{ mapView?.gameServerName || 'Shared server' }}</h1>
      </div>
      <q-badge v-if="mapView?.available" color="positive" label="Live" rounded />
    </header>

    <section v-if="invalidLink" class="public-minecraft-map__state">
      <q-icon name="link_off" size="48px" />
      <h2>This map link is not available</h2>
      <p>It may be incomplete, replaced, or revoked by the server administrator.</p>
    </section>
    <section v-else-if="viewerURL" class="public-minecraft-map__viewer-shell">
      <iframe
        class="public-minecraft-map__viewer"
        :src="viewerURL"
        title="Shared Minecraft live world map"
        referrerpolicy="no-referrer"
        sandbox="allow-scripts allow-forms allow-popups allow-downloads" />
    </section>
    <section v-else class="public-minecraft-map__state">
      <q-spinner v-if="loading && mapView === null" color="primary" size="42px" />
      <q-icon v-else :name="loadError ? 'error_outline' : 'radar'" size="48px" />
      <h2>{{ loadError ? 'Map temporarily unavailable' : 'Map is preparing' }}</h2>
      <p>{{ mapView?.statusMessage || 'Waiting for the shared world map…' }}</p>
    </section>
  </main>
</template>

<style scoped>
.public-minecraft-map {
  display: flex;
  min-height: 100dvh;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-base);
}

.public-minecraft-map__header {
  display: flex;
  min-height: 64px;
  align-items: center;
  gap: var(--xy-space-lg);
  padding: 0 var(--xy-space-sm);
}

.public-minecraft-map__brand {
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
}

.public-minecraft-map__title {
  display: grid;
  flex: 1;
  gap: var(--xy-space-2xs);
  padding-left: var(--xy-space-lg);
  border-left: 1px solid var(--xy-border);
}

.public-minecraft-map__title span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.public-minecraft-map__title h1,
.public-minecraft-map__state h2 {
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.public-minecraft-map__title h1 {
  margin: 0;
  font-size: var(--xy-font-size-base);
  letter-spacing: normal;
}

.public-minecraft-map__viewer-shell {
  flex: 1;
  min-height: calc(100dvh - 96px);
  overflow: hidden;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  background: var(--xy-surface-1);
}

.public-minecraft-map__viewer {
  width: 100%;
  height: 100%;
  min-height: calc(100dvh - 96px);
  border: 0;
}

.public-minecraft-map__state {
  display: grid;
  flex: 1;
  place-items: center;
  align-content: center;
  text-align: center;
}

.public-minecraft-map__state h2 {
  margin: var(--xy-space-md) 0 var(--xy-space-sm);
  font-size: var(--xy-font-size-xl);
}

.public-minecraft-map__state p {
  max-width: 52ch;
  margin: 0;
  color: var(--xy-text-secondary);
}

@media (max-width: 599px) {
  .public-minecraft-map {
    padding: var(--xy-space-xs);
  }

  .public-minecraft-map__header {
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm);
  }

  .public-minecraft-map__title {
    padding-left: var(--xy-space-sm);
  }
}
</style>
