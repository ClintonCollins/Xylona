<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { copyToClipboard, useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'

import {
  type GameServerMapShareSettings,
  GetOrCreateGameServerMapShareSettingsRequestSchema,
  UpdateGameServerMapShareSettingsRequestSchema,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const props = defineProps<{ gameServerId: string }>()
const emit = defineEmits<{ close: [] }>()
const quasar = useQuasar()
const identifierPattern = /^[A-Za-z0-9_-]{3,64}$/

const settings = ref<GameServerMapShareSettings | null>(null)
const publicIdentifier = ref('')
const enabled = ref(false)
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const identifierError = ref('')

const dirty = computed(() => {
  const saved = settings.value
  return (
    saved !== null &&
    (publicIdentifier.value !== saved.publicIdentifier || enabled.value !== saved.enabled)
  )
})
const formValid = computed(() => identifierPattern.test(publicIdentifier.value))
const validationMessage = computed(() => {
  if (identifierError.value !== '') return identifierError.value
  if (publicIdentifier.value.length < 3) return 'Use at least 3 characters.'
  if (publicIdentifier.value.length > 64) return 'Use no more than 64 characters.'
  if (!formValid.value) return 'Use only letters, numbers, underscores, or hyphens.'
  return ''
})
const publicURL = computed(() =>
  settings.value ? `${window.location.origin}${settings.value.publicPath}` : '',
)
const savedLinkEnabled = computed(() => settings.value?.enabled === true)

function applySettings(next: GameServerMapShareSettings): void {
  settings.value = next
  publicIdentifier.value = next.publicIdentifier
  enabled.value = next.enabled
  identifierError.value = ''
}

async function loadSettings(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    const response = await GetXylonaClient().getOrCreateGameServerMapShareSettings(
      create(GetOrCreateGameServerMapShareSettingsRequestSchema, {
        gameServerId: props.gameServerId,
      }),
    )
    if (!response.settings) throw new Error('The map link settings response was empty.')
    applySettings(response.settings)
  } catch (unknownError: unknown) {
    loadError.value = ConnectErrorToString(ConnectError.from(unknownError))
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  if (!settings.value || !formValid.value || !dirty.value) return
  saving.value = true
  identifierError.value = ''
  try {
    const response = await GetXylonaClient().updateGameServerMapShareSettings(
      create(UpdateGameServerMapShareSettingsRequestSchema, {
        gameServerId: props.gameServerId,
        publicIdentifier: publicIdentifier.value,
        enabled: enabled.value,
      }),
    )
    if (!response.settings) throw new Error('The saved map link settings response was empty.')
    applySettings(response.settings)
    quasar.notify({ type: 'positive', message: 'Public map link settings saved.' })
  } catch (unknownError: unknown) {
    const connectError = ConnectError.from(unknownError)
    if (connectError.code === Code.AlreadyExists) {
      identifierError.value = 'This public identifier is unavailable. Choose another.'
    } else {
      quasar.notify({
        type: 'negative',
        message: ConnectErrorToString(connectError),
      })
    }
  } finally {
    saving.value = false
  }
}

async function copyPublicLink(): Promise<void> {
  try {
    await copyToClipboard(publicURL.value)
    quasar.notify({ type: 'positive', message: 'Public map link copied.' })
  } catch (unknownError: unknown) {
    console.error(unknownError)
    quasar.notify({ type: 'negative', message: 'Could not copy the public map link.' })
  }
}

function openPublicLink(): void {
  window.open(publicURL.value, '_blank', 'noopener,noreferrer')
}

onMounted(loadSettings)
</script>

<template>
  <q-card class="map-share-settings" aria-labelledby="map-share-settings-title">
    <q-card-section class="map-share-settings__header">
      <div>
        <h2 id="map-share-settings-title">Public live map</h2>
        <p>Manage the one public link for this game server.</p>
      </div>
      <q-btn
        aria-label="Close public map link settings"
        flat
        icon="close"
        round
        @click="emit('close')" />
    </q-card-section>
    <q-separator />

    <q-card-section v-if="loading" class="map-share-settings__state" role="status">
      <q-spinner color="primary" size="36px" />
      <span>Loading public map settings</span>
    </q-card-section>
    <q-card-section v-else-if="loadError" class="map-share-settings__state" role="alert">
      <q-icon name="sync_problem" size="36px" />
      <strong>Public map settings could not be loaded.</strong>
      <span>{{ loadError }}</span>
      <q-btn color="primary" label="Try again" no-caps @click="loadSettings" />
    </q-card-section>
    <template v-else-if="settings">
      <q-card-section class="map-share-settings__body">
        <div class="map-share-settings__toggle">
          <div>
            <strong>Public availability</strong>
            <p>Disabled links return the same unavailable response as unknown links.</p>
          </div>
          <q-toggle v-model="enabled" aria-label="Enable public live map" color="primary" />
        </div>

        <q-input
          v-model="publicIdentifier"
          autocapitalize="off"
          autocomplete="off"
          autocorrect="off"
          :error="validationMessage !== ''"
          :error-message="validationMessage"
          hint="3–64 case-sensitive letters, numbers, underscores, or hyphens"
          label="Public identifier"
          maxlength="64"
          outlined
          spellcheck="false"
          @update:model-value="identifierError = ''" />

        <div class="map-share-settings__link">
          <span>Saved public link</span>
          <code>{{ publicURL }}</code>
          <div>
            <q-btn
              :disable="!savedLinkEnabled"
              flat
              icon="content_copy"
              label="Copy"
              no-caps
              @click="copyPublicLink" />
            <q-btn
              :disable="!savedLinkEnabled"
              flat
              icon="open_in_new"
              label="Open"
              no-caps
              @click="openPublicLink" />
          </div>
        </div>

        <q-banner class="map-share-settings__warning" rounded>
          <template #avatar><q-icon color="warning" name="warning_amber" /></template>
          Anyone who knows or guesses this identifier can view the live map. Search indexing is
          blocked, but the link is not private.
        </q-banner>
      </q-card-section>

      <q-separator />
      <q-card-actions class="map-share-settings__footer" align="between">
        <span aria-live="polite">{{ dirty ? 'Unsaved changes' : 'All changes saved' }}</span>
        <q-btn
          color="primary"
          :disable="!dirty || !formValid"
          label="Save settings"
          :loading="saving"
          no-caps
          @click="save" />
      </q-card-actions>
    </template>
  </q-card>
</template>

<style scoped>
.map-share-settings {
  width: min(620px, calc(100vw - 32px));
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
}

.map-share-settings__header,
.map-share-settings__footer,
.map-share-settings__toggle {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.map-share-settings__header {
  align-items: flex-start;
}

.map-share-settings__header h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 500;
}

.map-share-settings__header p,
.map-share-settings__toggle p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.map-share-settings__body {
  display: grid;
  gap: var(--xy-space-lg);
}

.map-share-settings__toggle {
  align-items: flex-start;
  padding: var(--xy-space-base);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.map-share-settings__link {
  display: grid;
  gap: var(--xy-space-sm);
}

.map-share-settings__link > span {
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
}

.map-share-settings__link code {
  padding: var(--xy-space-base);
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  background: var(--xy-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.map-share-settings__link > div {
  display: flex;
  gap: var(--xy-space-sm);
}

.map-share-settings__warning {
  color: var(--xy-text-secondary);
  background: var(--xy-warning-bg-faint);
  border: 1px solid var(--xy-warning-border);
}

.map-share-settings__state {
  display: grid;
  min-height: 260px;
  place-content: center;
  justify-items: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  text-align: center;
}

.map-share-settings__footer {
  padding: var(--xy-space-base) var(--xy-space-md);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

@media (max-width: 599px) {
  .map-share-settings__footer {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
