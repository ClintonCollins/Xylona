<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { copyToClipboard, useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'

import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import {
  GetGameServerRequestSchema,
  GetSevenDaysToDieMapRequestSchema,
  RegenerateSevenDaysToDieMapShareRequestSchema,
  RevokeSevenDaysToDieMapShareRequestSchema,
  SevenDaysToDieMapMarkerSchema,
  UpdateSevenDaysToDieMapNotesRequestSchema,
  type SevenDaysToDieMapMarker,
  type SevenDaysToDieMapView,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const pollIntervalMilliseconds = 5_000
const markerIcons = [
  { label: 'Pin', value: 'location_on' },
  { label: 'Home', value: 'home' },
  { label: 'Base', value: 'fort' },
  { label: 'Loot', value: 'inventory_2' },
  { label: 'Danger', value: 'warning' },
  { label: 'Vehicle', value: 'directions_car' },
]

const route = useRoute()
const quasar = useQuasar()
const mapView = ref<SevenDaysToDieMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const canManage = ref(false)
const notesOpen = ref(false)
const shareOpen = ref(false)
const savingNotes = ref(false)
const changingShare = ref(false)
const generatedShareURL = ref('')
const editingMarkerID = ref('')
const markerForm = reactive({ name: '', note: '', icon: 'location_on', x: 0, z: 0 })
let pollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const localMarkers = computed(() => mapView.value?.markers.filter((marker) => !marker.native) ?? [])

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

function resetMarkerForm(): void {
  editingMarkerID.value = ''
  markerForm.name = ''
  markerForm.note = ''
  markerForm.icon = 'location_on'
  markerForm.x = 0
  markerForm.z = 0
}

function editMarker(marker: SevenDaysToDieMapMarker): void {
  editingMarkerID.value = marker.id
  markerForm.name = marker.name
  markerForm.note = marker.note
  markerForm.icon = marker.icon || 'location_on'
  markerForm.x = marker.x
  markerForm.z = marker.z
}

function buildMarker(): SevenDaysToDieMapMarker {
  return create(SevenDaysToDieMapMarkerSchema, {
    id: editingMarkerID.value,
    name: markerForm.name.trim(),
    note: markerForm.note.trim(),
    icon: markerForm.icon,
    x: markerForm.x,
    z: markerForm.z,
    native: false,
  })
}

async function updateNotes(markers: SevenDaysToDieMapMarker[]): Promise<boolean> {
  savingNotes.value = true
  try {
    const response = await GetXylonaClient().updateSevenDaysToDieMapNotes(
      create(UpdateSevenDaysToDieMapNotesRequestSchema, {
        gameServerId: gameServerID.value,
        markers,
      }),
    )
    if (mapView.value !== null) {
      mapView.value.markers = [
        ...mapView.value.markers.filter((marker) => marker.native),
        ...response.markers,
      ]
    }
    return true
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
    return false
  } finally {
    savingNotes.value = false
  }
}

async function saveMarker(): Promise<void> {
  if (markerForm.name.trim() === '') {
    quasar.notify({ type: 'warning', message: 'Give the map note a title.' })
    return
  }
  if (!Number.isFinite(markerForm.x) || !Number.isFinite(markerForm.z)) {
    quasar.notify({ type: 'warning', message: 'Enter valid X and Z coordinates.' })
    return
  }
  const marker = buildMarker()
  const markers = localMarkers.value.filter((existing) => existing.id !== marker.id)
  markers.push(marker)
  if (await updateNotes(markers)) {
    resetMarkerForm()
    quasar.notify({ type: 'positive', message: 'Map note saved.' })
  }
}

async function removeMarker(markerID: string): Promise<void> {
  if (await updateNotes(localMarkers.value.filter((marker) => marker.id !== markerID))) {
    if (editingMarkerID.value === markerID) {
      resetMarkerForm()
    }
    quasar.notify({ type: 'positive', message: 'Map note removed.' })
  }
}

async function regenerateShare(): Promise<void> {
  changingShare.value = true
  try {
    const response = await GetXylonaClient().regenerateSevenDaysToDieMapShare(
      create(RegenerateSevenDaysToDieMapShareRequestSchema, {
        gameServerId: gameServerID.value,
      }),
    )
    generatedShareURL.value = `${window.location.origin}/shared/7-days-to-die-map#${response.shareToken}`
    if (mapView.value !== null) {
      mapView.value.shareEnabled = true
    }
    await copyShareURL()
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    changingShare.value = false
  }
}

async function copyShareURL(): Promise<void> {
  if (generatedShareURL.value === '') {
    return
  }
  try {
    await copyToClipboard(generatedShareURL.value)
    quasar.notify({ type: 'positive', message: 'Public map link copied.' })
  } catch (unknownError: unknown) {
    console.error(unknownError)
    quasar.notify({ type: 'negative', message: 'Could not copy the public map link.' })
  }
}

async function revokeShare(): Promise<void> {
  changingShare.value = true
  try {
    await GetXylonaClient().revokeSevenDaysToDieMapShare(
      create(RevokeSevenDaysToDieMapShareRequestSchema, { gameServerId: gameServerID.value }),
    )
    generatedShareURL.value = ''
    if (mapView.value !== null) {
      mapView.value.shareEnabled = false
    }
    quasar.notify({ type: 'positive', message: 'Public map link revoked.' })
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    changingShare.value = false
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
          <p>Players, server markers, land claims, and shared team notes in one view.</p>
        </div>
      </div>
      <div v-if="canManage" class="seven-days-map-page__actions">
        <q-btn icon="edit_location_alt" label="Notes" no-caps outline @click="notesOpen = true" />
        <q-btn color="primary" icon="share" label="Share" no-caps @click="shareOpen = true" />
      </div>
    </header>

    <seven-days-to-die-live-map
      :load-error="loadError"
      :loading="loading"
      :view="mapView"
      @refresh="loadMap" />

    <q-dialog v-model="notesOpen" @hide="resetMarkerForm">
      <q-card class="seven-days-map-dialog seven-days-notes-dialog">
        <q-card-section class="seven-days-map-dialog__heading">
          <div>
            <span>Shared intelligence</span>
            <h2>Map notes</h2>
          </div>
          <q-btn v-close-popup aria-label="Close map notes" flat icon="close" round />
        </q-card-section>
        <q-separator />
        <q-card-section class="seven-days-notes-dialog__content">
          <section class="seven-days-notes-dialog__list">
            <div class="seven-days-notes-dialog__section-heading">
              <strong>Saved notes</strong>
              <span>{{ localMarkers.length }} / 100</span>
            </div>
            <div v-if="localMarkers.length === 0" class="seven-days-notes-dialog__empty">
              No Xylona notes yet. Add a base, loot cache, route hazard, or rally point.
            </div>
            <button
              v-for="marker in localMarkers"
              :key="marker.id"
              class="seven-days-notes-dialog__row"
              type="button"
              @click="editMarker(marker)">
              <q-icon :name="marker.icon || 'location_on'" />
              <span
                ><strong>{{ marker.name }}</strong
                ><small>X {{ marker.x }} · Z {{ marker.z }}</small></span
              >
              <q-btn
                :aria-label="`Remove ${marker.name}`"
                color="negative"
                dense
                flat
                icon="delete_outline"
                round
                @click.stop="removeMarker(marker.id)" />
            </button>
          </section>
          <q-form class="seven-days-notes-dialog__form" @submit="saveMarker">
            <div class="seven-days-notes-dialog__section-heading">
              <strong>{{ editingMarkerID ? 'Edit note' : 'Add note' }}</strong>
              <q-btn
                v-if="editingMarkerID"
                dense
                flat
                label="Cancel"
                no-caps
                @click="resetMarkerForm" />
            </div>
            <q-input v-model="markerForm.name" dense label="Title" maxlength="100" outlined />
            <q-input
              v-model="markerForm.note"
              autogrow
              dense
              label="Details (optional)"
              maxlength="500"
              outlined />
            <q-select
              v-model="markerForm.icon"
              dense
              emit-value
              label="Icon"
              map-options
              :options="markerIcons"
              outlined />
            <div class="seven-days-notes-dialog__coordinates">
              <q-input
                v-model.number="markerForm.x"
                dense
                label="X coordinate"
                outlined
                type="number" />
              <q-input
                v-model.number="markerForm.z"
                dense
                label="Z coordinate"
                outlined
                type="number" />
            </div>
            <q-btn
              color="primary"
              :label="editingMarkerID ? 'Save changes' : 'Add to map'"
              :loading="savingNotes"
              no-caps
              type="submit" />
          </q-form>
        </q-card-section>
      </q-card>
    </q-dialog>

    <q-dialog v-model="shareOpen">
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
            Anyone with the link can view the map, players, markers, claims, and Xylona notes. The
            link does not grant control-panel access.
          </p>
          <q-input
            v-if="generatedShareURL"
            dense
            label="New public link"
            :model-value="generatedShareURL"
            outlined
            readonly>
            <template #append>
              <q-btn
                aria-label="Copy public map link"
                flat
                icon="content_copy"
                round
                @click="copyShareURL" />
            </template>
          </q-input>
          <div v-else-if="mapView?.shareEnabled" class="seven-days-share-dialog__active">
            <q-icon name="link" />
            A public link is active. Generate a replacement if you no longer have the original.
          </div>
          <div class="seven-days-share-dialog__actions">
            <q-btn
              color="primary"
              :label="mapView?.shareEnabled ? 'Replace link' : 'Create public link'"
              :loading="changingShare"
              no-caps
              @click="regenerateShare" />
            <q-btn
              v-if="mapView?.shareEnabled"
              color="negative"
              flat
              label="Revoke access"
              :loading="changingShare"
              no-caps
              @click="revokeShare" />
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

.seven-days-notes-dialog__content {
  display: grid;
  grid-template-columns: minmax(260px, 0.9fr) minmax(320px, 1.1fr);
  gap: var(--xy-space-lg);
  padding: var(--xy-space-lg);
}

.seven-days-notes-dialog__list,
.seven-days-notes-dialog__form,
.seven-days-share-dialog__body {
  display: grid;
  align-content: start;
  gap: var(--xy-space-base);
}

.seven-days-notes-dialog__section-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  min-height: 32px;
  color: var(--xy-text-secondary);
}

.seven-days-notes-dialog__section-heading strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.seven-days-notes-dialog__empty {
  padding: var(--xy-space-lg);
  color: var(--xy-text-muted);
  background: var(--xy-surface-overlay-soft);
  border: 1px dashed var(--xy-border-hover);
  border-radius: var(--xy-radius-md);
}

.seven-days-notes-dialog__row {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: var(--xy-space-base);
  width: 100%;
  padding: var(--xy-space-sm) var(--xy-space-base);
  color: var(--xy-text-secondary);
  text-align: left;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  cursor: pointer;
}

.seven-days-notes-dialog__row:hover,
.seven-days-notes-dialog__row:focus-visible {
  border-color: var(--xy-border-active);
  outline: none;
}

.seven-days-notes-dialog__row > span {
  display: grid;
}

.seven-days-notes-dialog__row strong {
  color: var(--xy-text-primary);
}

.seven-days-notes-dialog__row small {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
}

.seven-days-notes-dialog__coordinates {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--xy-space-sm);
}

.seven-days-share-dialog {
  width: min(560px, 94vw);
}

.seven-days-share-dialog__body {
  padding: var(--xy-space-lg);
}

.seven-days-share-dialog__active {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-base);
  color: var(--xy-success);
  background: var(--xy-success-bg-faint);
  border: 1px solid var(--xy-success-border);
  border-radius: var(--xy-radius-md);
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

  .seven-days-notes-dialog__content {
    grid-template-columns: 1fr;
  }
}
</style>
