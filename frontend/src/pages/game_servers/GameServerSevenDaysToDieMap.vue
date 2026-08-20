<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { copyToClipboard, useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import {
  GetGameServerRequestSchema,
  GetSevenDaysToDieMapRequestSchema,
  ListSevenDaysToDieMapSharesRequestSchema,
  RegenerateSevenDaysToDieMapShareRequestSchema,
  RevokeSevenDaysToDieMapShareRequestSchema,
  type SevenDaysToDieMapShare,
  type SevenDaysToDieMapView,
} from '@/proto/xylona_pb'
import { formatTimestamp } from '@/utils/format-timestamp'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const pollIntervalMilliseconds = 5_000

const route = useRoute()
const quasar = useQuasar()
const mapView = ref<SevenDaysToDieMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const canManage = ref(false)
const shareOpen = ref(false)
const shareLinks = ref<SevenDaysToDieMapShare[]>([])
const loadingShares = ref(false)
const creatingShare = ref(false)
const removingShareID = ref('')
const shareLoadError = ref('')
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

function shareURL(token: string): string {
  return `${window.location.origin}/shared/7-days-to-die-map#${token}`
}

async function loadShares(): Promise<void> {
  if (loadingShares.value || gameServerID.value === '') {
    return
  }
  loadingShares.value = true
  shareLoadError.value = ''
  try {
    const response = await GetXylonaClient().listSevenDaysToDieMapShares(
      create(ListSevenDaysToDieMapSharesRequestSchema, { gameServerId: gameServerID.value }),
    )
    shareLinks.value = response.shares
    if (mapView.value !== null) {
      mapView.value.shareEnabled = response.shares.length > 0
    }
  } catch (unknownError: unknown) {
    shareLoadError.value = ConnectErrorToString(ConnectError.from(unknownError))
  } finally {
    loadingShares.value = false
  }
}

async function regenerateShare(): Promise<void> {
  creatingShare.value = true
  try {
    const response = await GetXylonaClient().regenerateSevenDaysToDieMapShare(
      create(RegenerateSevenDaysToDieMapShareRequestSchema, {
        gameServerId: gameServerID.value,
      }),
    )
    await loadShares()
    await copyShareURL(response.shareToken)
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    creatingShare.value = false
  }
}

async function copyShareURL(token: string): Promise<void> {
  if (token === '') {
    return
  }
  try {
    await copyToClipboard(shareURL(token))
    quasar.notify({ type: 'positive', message: 'Public map link copied.' })
  } catch (unknownError: unknown) {
    console.error(unknownError)
    quasar.notify({ type: 'negative', message: 'Could not copy the public map link.' })
  }
}

async function revokeShare(shareID: string): Promise<void> {
  removingShareID.value = shareID
  try {
    await GetXylonaClient().revokeSevenDaysToDieMapShare(
      create(RevokeSevenDaysToDieMapShareRequestSchema, {
        gameServerId: gameServerID.value,
        shareId: shareID,
      }),
    )
    shareLinks.value = shareLinks.value.filter((share) => share.id !== shareID)
    if (mapView.value !== null) {
      mapView.value.shareEnabled = shareLinks.value.length > 0
    }
    quasar.notify({ type: 'positive', message: 'Public map link revoked.' })
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    removingShareID.value = ''
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
        <q-btn color="primary" icon="share" label="Share" no-caps @click="shareOpen = true" />
      </div>
    </header>

    <seven-days-to-die-live-map
      :load-error="loadError"
      :loading="loading"
      :view="mapView"
      @refresh="loadMap" />

    <q-dialog v-model="shareOpen" @show="loadShares">
      <q-card class="seven-days-map-dialog seven-days-share-dialog">
        <q-card-section class="seven-days-map-dialog__heading">
          <div>
            <span>Public access</span>
            <h2>Share live map</h2>
          </div>
          <q-btn v-close-popup aria-label="Close sharing" flat icon="close" round />
        </q-card-section>
        <q-separator />
        <q-card-section class="seven-days-share-dialog__body">
          <p>
            Anyone with the link can view the map and player positions. The link does not grant
            control-panel access.
          </p>
          <div class="seven-days-share-dialog__list-heading">
            <strong>Active links</strong>
            <span>{{ shareLinks.length }}</span>
          </div>
          <q-skeleton v-if="loadingShares" height="76px" type="rect" />
          <div v-else-if="shareLoadError" class="seven-days-share-dialog__error" role="alert">
            <q-icon name="error_outline" />
            <span>{{ shareLoadError }}</span>
            <q-btn dense flat label="Try again" no-caps @click="loadShares" />
          </div>
          <div v-else-if="shareLinks.length === 0" class="seven-days-share-dialog__empty">
            <q-icon name="link_off" />
            <span>No public links exist for this map.</span>
          </div>
          <div v-else class="seven-days-share-dialog__links" aria-live="polite">
            <div v-for="share in shareLinks" :key="share.id" class="seven-days-share-dialog__link">
              <div class="seven-days-share-dialog__link-heading">
                <span>
                  <q-icon name="link" />
                  Created {{ formatTimestamp(share.createdAt, 'at an unknown time') }}
                </span>
                <q-btn
                  :aria-label="`Remove map link created ${formatTimestamp(share.createdAt, 'at an unknown time')}`"
                  color="negative"
                  dense
                  flat
                  icon="delete_outline"
                  :loading="removingShareID === share.id"
                  round
                  @click="revokeShare(share.id)" />
              </div>
              <q-input
                v-if="share.shareToken"
                dense
                label="Public link"
                :model-value="shareURL(share.shareToken)"
                outlined
                readonly>
                <template #append>
                  <q-btn
                    aria-label="Copy public map link"
                    flat
                    icon="content_copy"
                    round
                    @click="copyShareURL(share.shareToken)" />
                </template>
              </q-input>
              <p v-else>
                This link predates link management, so its address cannot be recovered. It remains
                active until removed.
              </p>
            </div>
          </div>
          <div class="seven-days-share-dialog__actions">
            <q-btn
              color="primary"
              icon="add_link"
              label="Create public link"
              :loading="creatingShare"
              no-caps
              @click="regenerateShare" />
          </div>
        </q-card-section>
      </q-card>
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

.seven-days-map-page__header,
.seven-days-map-dialog__heading {
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

.seven-days-map-page__heading > div,
.seven-days-map-dialog__heading > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.seven-days-map-page__heading span,
.seven-days-map-dialog__heading span {
  color: var(--xy-accent);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.seven-days-map-page__heading h1,
.seven-days-map-dialog__heading h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.seven-days-map-page__heading h1 {
  font-size: var(--xy-font-size-xl);
}

.seven-days-map-page__heading p,
.seven-days-share-dialog__body p {
  margin: 0;
  color: var(--xy-text-secondary);
}

.seven-days-map-page__actions,
.seven-days-share-dialog__actions {
  display: flex;
  gap: var(--xy-space-sm);
}

.seven-days-map-dialog {
  width: min(880px, 94vw);
  max-width: none;
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-hover);
}

.seven-days-map-dialog__heading {
  padding: var(--xy-space-md) var(--xy-space-lg);
}

.seven-days-map-dialog__heading h2 {
  font-size: var(--xy-font-size-lg);
}

.seven-days-share-dialog__body {
  display: grid;
  align-content: start;
  gap: var(--xy-space-base);
}

.seven-days-share-dialog {
  width: min(560px, 94vw);
}

.seven-days-share-dialog__body {
  padding: var(--xy-space-lg);
}

.seven-days-share-dialog__list-heading,
.seven-days-share-dialog__link-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.seven-days-share-dialog__list-heading {
  color: var(--xy-text-secondary);
}

.seven-days-share-dialog__list-heading strong {
  color: var(--xy-text-primary);
}

.seven-days-share-dialog__list-heading span {
  font-family: var(--xy-font-mono);
}

.seven-days-share-dialog__links {
  display: grid;
  gap: var(--xy-space-sm);
  max-height: min(44vh, 420px);
  padding-right: var(--xy-space-xs);
  overflow-y: auto;
}

.seven-days-share-dialog__link {
  display: grid;
  gap: var(--xy-space-sm);
  min-width: 0;
  padding: var(--xy-space-base);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.seven-days-share-dialog__link-heading > span {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  overflow-wrap: anywhere;
}

.seven-days-share-dialog__link-heading .q-icon {
  flex: 0 0 auto;
  color: var(--xy-accent);
}

.seven-days-share-dialog__link p {
  font-size: var(--xy-font-size-sm);
}

.seven-days-share-dialog__empty,
.seven-days-share-dialog__error {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-base);
  border-radius: var(--xy-radius-md);
}

.seven-days-share-dialog__empty {
  color: var(--xy-text-muted);
  background: var(--xy-surface-overlay-soft);
  border: 1px dashed var(--xy-border-hover);
}

.seven-days-share-dialog__error {
  color: var(--xy-danger);
  background: var(--xy-danger-bg-faint);
  border: 1px solid var(--xy-danger-border);
}

.seven-days-share-dialog__error span {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
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
