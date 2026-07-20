<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { onBeforeUnmount, onMounted, ref } from 'vue'

import { GetPublicMinecraftMapRequestSchema, type MinecraftMapView } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const pollIntervalMilliseconds = 10_000
const mapView = ref<MinecraftMapView | null>(null)
const loading = ref(false)
const invalidLink = ref(false)
const loadError = ref(false)
const token = ref('')
let pollTimer: ReturnType<typeof setInterval> | undefined

async function loadMap(): Promise<void> {
  if (loading.value || token.value === '') {
    invalidLink.value = token.value === ''
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getPublicMinecraftMap(
      create(GetPublicMinecraftMapRequestSchema, { shareToken: token.value }),
    )
    mapView.value = response.map ?? null
    invalidLink.value = false
    loadError.value = false
  } catch (unknownError: unknown) {
    console.error(unknownError)
    const connectError = ConnectError.from(unknownError)
    if (connectError.code === Code.NotFound) {
      mapView.value = null
      invalidLink.value = true
      loadError.value = false
    } else {
      loadError.value = true
    }
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  token.value = window.location.hash.slice(1).trim()
  void loadMap()
  pollTimer = setInterval(() => void loadMap(), pollIntervalMilliseconds)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) clearInterval(pollTimer)
})
</script>

<template>
  <main class="public-minecraft-map">
    <header class="public-minecraft-map__header">
      <div class="public-minecraft-map__brand">XYLONA</div>
      <div class="public-minecraft-map__title">
        <span>Minecraft live map</span>
        <strong>{{ mapView?.gameServerName || 'Shared server' }}</strong>
      </div>
      <q-badge v-if="mapView?.available" color="positive" label="Live" rounded />
    </header>

    <section v-if="invalidLink" class="public-minecraft-map__state">
      <q-icon name="link_off" size="48px" />
      <h1>This map link is not available</h1>
      <p>It may be incomplete, replaced, or revoked by the server administrator.</p>
    </section>
    <section v-else-if="mapView?.viewerUrl" class="public-minecraft-map__viewer-shell">
      <iframe
        class="public-minecraft-map__viewer"
        :src="mapView.viewerUrl"
        title="Shared Minecraft live world map"
        referrerpolicy="no-referrer"
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-downloads" />
    </section>
    <section v-else class="public-minecraft-map__state">
      <q-spinner v-if="loading && mapView === null" color="primary" size="42px" />
      <q-icon v-else :name="loadError ? 'error_outline' : 'radar'" size="48px" />
      <h1>{{ loadError ? 'Map temporarily unavailable' : 'Map is preparing' }}</h1>
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
  font-size: var(--xy-font-size-lg);
  letter-spacing: 0.08em;
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

.public-minecraft-map__title strong,
.public-minecraft-map__state h1 {
  font-family: var(--xy-font-heading);
  font-weight: 500;
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

.public-minecraft-map__state h1 {
  margin: var(--xy-space-md) 0 var(--xy-space-sm);
  font-size: var(--xy-font-size-xl);
}

.public-minecraft-map__state p {
  max-width: 52ch;
  margin: 0;
  color: var(--xy-text-secondary);
}

@media (max-width: 600px) {
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
