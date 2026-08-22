<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'

import GameServerMapShareSettings from '@/components/game_servers/GameServerMapShareSettings.vue'
import {
  GetMinecraftMapRequestSchema,
  UpdateMinecraftMapConfigRequestSchema,
  type MinecraftMapView,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const pollIntervalMilliseconds = 5_000
const viewerRefreshMilliseconds = 45 * 60 * 1_000
const route = useRoute()
const quasar = useQuasar()
const mapView = ref<MinecraftMapView | null>(null)
const viewerURL = ref('')
const viewerURLSetAt = ref(0)
const loading = ref(false)
const loadError = ref(false)
const settingsOpen = ref(false)
const shareOpen = ref(false)
const savingSettings = ref(false)
const settingsForm = reactive({ enabled: false, worldName: 'world', accepted: false })
let pollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const canManage = computed(() => mapView.value?.canManage ?? false)
const statusTone = computed(() => {
  const status = mapView.value?.status ?? ''
  if (status === 'ready') return 'positive'
  if (status === 'rendering' || status === 'waiting_for_world') return 'warning'
  if (status === 'disabled') return 'grey-7'
  return 'negative'
})
const statusLabel = computed(() => {
  switch (mapView.value?.status) {
    case 'ready':
      return 'Live'
    case 'rendering':
      return 'Rendering'
    case 'waiting_for_world':
      return 'Waiting for world'
    case 'disabled':
      return 'Disabled'
    case 'node_upgrade_required':
      return 'Node update needed'
    default:
      return mapView.value?.status ? 'Unavailable' : 'Loading'
  }
})

async function loadMap(): Promise<void> {
  if (loading.value || gameServerID.value === '') return
  loading.value = true
  try {
    const response = await GetXylonaClient().getMinecraftMap(
      create(GetMinecraftMapRequestSchema, { gameServerId: gameServerID.value }),
    )
    mapView.value = response.map ?? null
    const nextViewerURL = response.map?.viewerUrl ?? ''
    if (nextViewerURL === '') {
      viewerURL.value = ''
      viewerURLSetAt.value = 0
    } else if (
      nextViewerURL !== '' &&
      (viewerURL.value === '' || Date.now() - viewerURLSetAt.value >= viewerRefreshMilliseconds)
    ) {
      viewerURL.value = nextViewerURL
      viewerURLSetAt.value = Date.now()
    }
    loadError.value = false
  } catch (unknownError: unknown) {
    loadError.value = true
    console.error(unknownError)
  } finally {
    loading.value = false
  }
}

function openSettings(): void {
  settingsForm.enabled = mapView.value?.enabled ?? false
  settingsForm.worldName = mapView.value?.worldName || 'world'
  settingsForm.accepted = false
  settingsOpen.value = true
}

async function saveSettings(): Promise<void> {
  if (settingsForm.enabled && !mapView.value?.bluemapDownloadAccepted && !settingsForm.accepted) {
    quasar.notify({ type: 'warning', message: 'Accept the BlueMap resource download to continue.' })
    return
  }
  savingSettings.value = true
  try {
    const response = await GetXylonaClient().updateMinecraftMapConfig(
      create(UpdateMinecraftMapConfigRequestSchema, {
        gameServerId: gameServerID.value,
        enabled: settingsForm.enabled,
        worldName: settingsForm.worldName.trim() || 'world',
        acceptBluemapDownload: settingsForm.accepted,
      }),
    )
    mapView.value = response.map ?? null
    if (!settingsForm.enabled) {
      viewerURL.value = ''
    }
    settingsOpen.value = false
    quasar.notify({
      type: 'positive',
      message: settingsForm.enabled
        ? 'Minecraft live map enabled. Restart the server once to activate live player positions.'
        : 'Minecraft live map disabled.',
    })
    await loadMap()
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    savingSettings.value = false
  }
}

onMounted(() => {
  void loadMap()
  pollTimer = setInterval(() => void loadMap(), pollIntervalMilliseconds)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) clearInterval(pollTimer)
})
</script>

<template>
  <div class="minecraft-map-page">
    <header class="minecraft-map-page__header">
      <div class="minecraft-map-page__heading">
        <span class="minecraft-map-page__heading-icon"><q-icon name="public" /></span>
        <div>
          <span>Minecraft · BlueMap</span>
          <h1>Live world map</h1>
          <p>Rendered terrain with live player positions across vanilla and modded servers.</p>
        </div>
      </div>
      <div class="minecraft-map-page__actions">
        <q-badge :color="statusTone" rounded :label="statusLabel" />
        <q-btn
          v-if="canManage"
          dense
          flat
          icon="tune"
          label="Map setup"
          no-caps
          @click="openSettings" />
        <q-btn
          v-if="canManage"
          color="primary"
          dense
          :disable="!mapView?.enabled"
          icon="ios_share"
          label="Public link"
          no-caps
          @click="shareOpen = true" />
      </div>
    </header>

    <section v-if="viewerURL" class="minecraft-map-page__viewer-shell">
      <iframe
        class="minecraft-map-page__viewer"
        :src="viewerURL"
        title="Minecraft live world map"
        referrerpolicy="no-referrer"
        sandbox="allow-same-origin allow-scripts allow-forms allow-popups allow-downloads" />
    </section>

    <section v-else class="minecraft-map-page__state">
      <q-spinner v-if="loading && mapView === null" color="primary" size="42px" />
      <q-icon
        v-else
        :color="loadError ? 'negative' : mapView?.enabled ? 'warning' : 'accent'"
        :name="loadError ? 'error_outline' : mapView?.enabled ? 'radar' : 'map'"
        size="48px" />
      <h2 v-if="loadError">Could not load the Minecraft map</h2>
      <h2 v-else-if="mapView?.enabled">{{ statusLabel }}</h2>
      <h2 v-else>Bring this world online</h2>
      <p v-if="loadError">The map service did not respond. Check the node connection and retry.</p>
      <p v-else>{{ mapView?.statusMessage || 'Loading Minecraft map status…' }}</p>
      <div
        v-if="mapView?.enabled && mapView.status === 'rendering'"
        class="minecraft-map-page__render-note">
        <q-icon name="info_outline" />
        Initial rendering can take several minutes on a large world. Explored chunks appear as they
        finish.
      </div>
      <q-btn
        v-if="!mapView?.enabled && canManage"
        color="primary"
        icon="auto_awesome"
        label="Set up live map"
        no-caps
        @click="openSettings" />
      <q-btn v-else-if="loadError" icon="refresh" label="Retry" no-caps outline @click="loadMap" />
    </section>

    <q-dialog v-model="settingsOpen">
      <q-card class="minecraft-map-dialog">
        <q-card-section class="minecraft-map-dialog__heading">
          <div>
            <span>World renderer</span>
            <h2>Minecraft map setup</h2>
          </div>
          <q-btn v-close-popup aria-label="Close Minecraft map setup" flat icon="close" round />
        </q-card-section>
        <q-separator />
        <q-card-section class="minecraft-map-dialog__content">
          <div class="minecraft-map-dialog__provider">
            <q-icon color="accent" name="deployed_code" size="34px" />
            <div>
              <strong>BlueMap companion</strong>
              <span>
                Xylona runs a verified standalone BlueMap renderer beside this server. It works with
                vanilla and modded worlds without exposing another public port.
              </span>
            </div>
          </div>
          <q-toggle v-model="settingsForm.enabled" color="primary" label="Enable live map" />
          <q-input
            v-model="settingsForm.worldName"
            :disable="!settingsForm.enabled"
            hint="The level-name folder, usually world"
            label="World folder"
            maxlength="80"
            outlined />
          <q-checkbox
            v-if="settingsForm.enabled && !mapView?.bluemapDownloadAccepted"
            v-model="settingsForm.accepted"
            color="primary"
            label="Allow BlueMap to download Mojang client resources required to render blocks" />
          <p class="minecraft-map-dialog__fine-print">
            Xylona serves only the checksum-verified BlueMap web application through the panel.
            Existing BlueMap plugins can continue running but are not exposed through Xylona.
            Restart Minecraft once after first setup so Xylona can provide live player positions
            over its managed RCON channel. Managed rendering requires Java 21 for Minecraft 1.x
            worlds and Java 25 for Minecraft 26.x worlds.
          </p>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat label="Cancel" no-caps />
          <q-btn
            :loading="savingSettings"
            color="primary"
            label="Save map setup"
            no-caps
            @click="saveSettings" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="shareOpen">
      <game-server-map-share-settings :game-server-id="gameServerID" @close="shareOpen = false" />
    </q-dialog>
  </div>
</template>

<style scoped>
.minecraft-map-page {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: var(--xy-space-lg);
  background:
    radial-gradient(
      circle at 12% 0%,
      color-mix(in srgb, var(--q-primary) 11%, transparent),
      transparent 34%
    ),
    var(--xy-surface-1);
}

.minecraft-map-page__header,
.minecraft-map-page__heading,
.minecraft-map-page__actions,
.minecraft-map-dialog__heading,
.minecraft-map-dialog__provider {
  display: flex;
  align-items: center;
}

.minecraft-map-page__header {
  justify-content: space-between;
  gap: var(--xy-space-lg);
}

.minecraft-map-page__heading {
  gap: var(--xy-space-md);
}

.minecraft-map-page__heading-icon {
  display: grid;
  width: 48px;
  height: 48px;
  flex: 0 0 auto;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--q-primary) 42%, var(--xy-border));
  border-radius: var(--xy-radius-md);
  color: var(--xy-accent);
  background: color-mix(in srgb, var(--q-primary) 12%, var(--xy-surface-3));
  font-size: 25px;
}

.minecraft-map-page__heading span,
.minecraft-map-dialog__heading span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.minecraft-map-page h1,
.minecraft-map-page h2,
.minecraft-map-dialog h2 {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.minecraft-map-page h1 {
  font-size: var(--xy-font-size-xl);
}

.minecraft-map-page__heading p,
.minecraft-map-page__state p,
.minecraft-map-dialog__provider span,
.minecraft-map-dialog__fine-print {
  margin: 0;
  color: var(--xy-text-secondary);
}

.minecraft-map-page__actions {
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--xy-space-sm);
}

.minecraft-map-page__viewer-shell {
  flex: 1;
  min-height: 520px;
  overflow: hidden;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  background: var(--xy-base);
  box-shadow: var(--xy-shadow-lg);
}

.minecraft-map-page__viewer {
  width: 100%;
  height: 100%;
  min-height: 520px;
  border: 0;
  background: var(--xy-base);
}

.minecraft-map-page__state {
  display: grid;
  flex: 1;
  min-height: 480px;
  place-items: center;
  align-content: center;
  gap: var(--xy-space-md);
  padding: var(--xy-space-xl);
  border: 1px dashed var(--xy-border);
  border-radius: var(--xy-radius-lg);
  text-align: center;
  background: color-mix(in srgb, var(--xy-surface-2) 85%, transparent);
}

.minecraft-map-page__state p {
  max-width: 62ch;
}

.minecraft-map-page__render-note {
  display: flex;
  max-width: 680px;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  border: 1px solid color-mix(in srgb, var(--q-warning) 30%, var(--xy-border));
  border-radius: var(--xy-radius-md);
  color: var(--xy-text-secondary);
  background: color-mix(in srgb, var(--q-warning) 8%, var(--xy-surface-2));
  text-align: left;
}

.minecraft-map-dialog {
  width: min(620px, calc(100vw - 32px));
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
}

.minecraft-map-dialog__heading {
  justify-content: space-between;
}

.minecraft-map-dialog__content {
  display: grid;
  gap: var(--xy-space-md);
}

.minecraft-map-dialog__provider {
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  background: var(--xy-surface-3);
}

.minecraft-map-dialog__provider div {
  display: grid;
  gap: var(--xy-space-xs);
}

@media (max-width: 700px) {
  .minecraft-map-page {
    padding: var(--xy-space-md);
  }

  .minecraft-map-page__header {
    align-items: flex-start;
    flex-direction: column;
  }

  .minecraft-map-page__actions {
    width: 100%;
    justify-content: flex-start;
  }

  .minecraft-map-page__viewer-shell,
  .minecraft-map-page__viewer {
    min-height: 460px;
  }
}
</style>
