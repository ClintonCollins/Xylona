<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { computed, onMounted, ref } from 'vue'
import { copyToClipboard, useQuasar } from 'quasar'

import {
  GameServerStatusPageConnectionAddressSchema,
  type GameServerStatusPageSettings,
  GetOrCreateGameServerStatusPageSettingsRequestSchema,
  ListUsersRequestSchema,
  UpdateGameServerStatusPageSettingsRequestSchema,
  type User,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const emit = defineEmits<{ close: [] }>()
const $q = useQuasar()
const authStore = useUserAuthStore()
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const identifierError = ref('')
const addressError = ref('')
const settings = ref<GameServerStatusPageSettings | null>(null)
const users = ref<User[]>([])
const ownerID = ref(authStore.user?.id ?? '')
const title = ref('')
const publicIdentifier = ref('')
const enabled = ref(false)
const addresses = ref<Record<string, string>>({})
const savedFingerprint = ref('')

const isSuperUser = computed(() => authStore.user?.superUser === true)
const ownerOptions = computed(() =>
  users.value.map((user) => ({ label: user.userName, value: user.id })),
)
const publicURL = computed(() =>
  settings.value ? `${window.location.origin}${settings.value.publicPath}` : '',
)
const formFingerprint = computed(() =>
  JSON.stringify({
    ownerID: ownerID.value,
    title: title.value,
    publicIdentifier: publicIdentifier.value,
    enabled: enabled.value,
    addresses: addresses.value,
  }),
)
const dirty = computed(
  () => settings.value !== null && formFingerprint.value !== savedFingerprint.value,
)
const formValid = computed(
  () =>
    title.value.trim().length >= 1 &&
    [...title.value.trim()].length <= 80 &&
    /^[A-Za-z0-9_-]{3,64}$/.test(publicIdentifier.value),
)

function applySettings(next: GameServerStatusPageSettings) {
  settings.value = next
  ownerID.value = next.ownerId
  title.value = next.title
  publicIdentifier.value = next.publicIdentifier
  enabled.value = next.enabled
  addresses.value = Object.fromEntries(
    next.servers.map((server) => [server.id, server.publicConnectionAddress ?? '']),
  )
  savedFingerprint.value = formFingerprint.value
  identifierError.value = ''
  addressError.value = ''
}

async function loadSettings(nextOwnerID = ownerID.value) {
  loading.value = true
  loadError.value = ''
  try {
    const response = await GetXylonaClient().getOrCreateGameServerStatusPageSettings(
      create(GetOrCreateGameServerStatusPageSettingsRequestSchema, {
        ownerId: isSuperUser.value ? nextOwnerID : '',
      }),
    )
    if (!response.settings) throw new Error('The status page settings response was empty.')
    applySettings(response.settings)
  } catch (error: unknown) {
    loadError.value = ConnectErrorToString(ConnectError.from(error))
  } finally {
    loading.value = false
  }
}

function confirmDiscard(): Promise<boolean> {
  if (!dirty.value) return Promise.resolve(true)
  return new Promise((resolve) => {
    $q.dialog({
      title: 'Discard unsaved changes?',
      message: 'Your status page changes have not been saved.',
      cancel: true,
      persistent: true,
      ok: { label: 'Discard', color: 'negative' },
    })
      .onOk(() => resolve(true))
      .onCancel(() => resolve(false))
      .onDismiss(() => resolve(false))
  })
}

async function changeOwner(nextOwnerID: string) {
  if (!(await confirmDiscard())) return
  await loadSettings(nextOwnerID)
}

async function requestClose() {
  if (await confirmDiscard()) emit('close')
}

async function save() {
  if (!settings.value || !formValid.value) return
  saving.value = true
  identifierError.value = ''
  addressError.value = ''
  try {
    const response = await GetXylonaClient().updateGameServerStatusPageSettings(
      create(UpdateGameServerStatusPageSettingsRequestSchema, {
        ownerId: isSuperUser.value ? ownerID.value : '',
        title: title.value,
        publicIdentifier: publicIdentifier.value,
        enabled: enabled.value,
        connectionAddresses: settings.value.servers.map((server) =>
          create(GameServerStatusPageConnectionAddressSchema, {
            gameServerId: server.id,
            publicConnectionAddress: addresses.value[server.id]?.trim() ?? '',
          }),
        ),
      }),
    )
    if (!response.settings) throw new Error('The saved status page settings response was empty.')
    applySettings(response.settings)
    $q.notify({ type: 'positive', message: 'Status page settings saved' })
  } catch (error: unknown) {
    const connectError = ConnectError.from(error)
    if (connectError.code === Code.AlreadyExists) {
      identifierError.value = 'This public identifier is unavailable. Choose another.'
    } else if (connectError.code === Code.InvalidArgument) {
      addressError.value = connectError.message
    } else {
      $q.notify({ type: 'xylona-error', caption: ConnectErrorToString(connectError) })
    }
  } finally {
    saving.value = false
  }
}

async function copyPublicLink() {
  try {
    await copyToClipboard(publicURL.value)
    $q.notify({ type: 'positive', message: 'Public link copied' })
  } catch {
    $q.notify({ type: 'negative', message: 'Could not copy the public link.' })
  }
}

function openPublicLink() {
  window.open(publicURL.value, '_blank', 'noopener,noreferrer')
}

onMounted(async () => {
  if (isSuperUser.value) {
    try {
      const response = await GetXylonaClient().listUsers(create(ListUsersRequestSchema, {}))
      users.value = response.users
    } catch (error: unknown) {
      loadError.value = ConnectErrorToString(ConnectError.from(error))
      loading.value = false
      return
    }
  }
  await loadSettings()
})
</script>

<template>
  <aside class="status-settings-panel" aria-labelledby="status-settings-title">
    <header class="status-settings-panel__header">
      <div>
        <h2 id="status-settings-title">Public status page</h2>
        <p>Publish a safe, read-only view of owned game servers.</p>
      </div>
      <q-btn
        aria-label="Close public status page settings"
        flat
        icon="close"
        round
        @click="requestClose" />
    </header>

    <div v-if="loading" class="status-settings-panel__state" role="status">
      <q-spinner color="primary" size="36px" />
      <span>Loading settings</span>
    </div>
    <div v-else-if="loadError" class="status-settings-panel__state" role="alert">
      <q-icon name="sync_problem" size="36px" />
      <strong>Settings could not be loaded.</strong>
      <span>{{ loadError }}</span>
      <q-btn color="primary" label="Try again" @click="loadSettings()" />
    </div>
    <template v-else-if="settings">
      <div class="status-settings-panel__body">
        <q-select
          v-if="isSuperUser"
          :model-value="ownerID"
          emit-value
          label="Page owner"
          map-options
          option-label="label"
          option-value="value"
          outlined
          :options="ownerOptions"
          @update:model-value="changeOwner" />

        <div class="status-settings-toggle">
          <div>
            <strong>Public availability</strong>
            <p>Disabled pages use the same unavailable response as unknown links.</p>
          </div>
          <q-toggle v-model="enabled" aria-label="Enable public status page" />
        </div>

        <q-input
          v-model="title"
          counter
          label="Page title"
          maxlength="80"
          outlined
          :rules="[(value: string) => value.trim().length > 0 || 'Enter a page title']" />

        <q-input
          v-model="publicIdentifier"
          :error="identifierError !== ''"
          :error-message="identifierError"
          hint="3–64 case-sensitive letters, numbers, underscores, or hyphens"
          label="Public identifier"
          outlined />

        <div class="status-settings-link">
          <span>Saved public link</span>
          <code>{{ publicURL }}</code>
          <div>
            <q-btn
              :disable="!settings.enabled"
              dense
              flat
              icon="content_copy"
              label="Copy"
              @click="copyPublicLink" />
            <q-btn
              :disable="!settings.enabled"
              dense
              flat
              icon="open_in_new"
              label="Open"
              @click="openPublicLink" />
          </div>
        </div>

        <section class="status-settings-addresses" aria-labelledby="public-addresses-title">
          <div>
            <h3 id="public-addresses-title">Public connection addresses</h3>
            <p>
              Override only what visitors should use. Query, RCON, and node addresses stay private.
            </p>
          </div>
          <q-input
            v-for="server in settings.servers"
            :key="server.id"
            v-model="addresses[server.id]"
            :hint="`Default: ${server.configuredConnectionAddress}`"
            :label="server.name"
            outlined
            placeholder="Use default address" />
          <p v-if="addressError" class="text-negative" role="alert">{{ addressError }}</p>
          <p v-if="settings.servers.length === 0" class="status-settings-addresses__empty">
            This owner has no game servers.
          </p>
        </section>
      </div>

      <footer class="status-settings-panel__footer">
        <span v-if="dirty">Unsaved changes</span>
        <span v-else>All changes saved</span>
        <q-btn
          color="primary"
          :disable="!dirty || !formValid"
          label="Save settings"
          :loading="saving"
          @click="save" />
      </footer>
    </template>
  </aside>
</template>

<style scoped>
.status-settings-panel {
  position: sticky;
  top: calc(var(--xy-header-stack-height) + var(--xy-space-lg));
  overflow: hidden;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-lg);
  box-shadow: var(--xy-shadow-lg);
}

.status-settings-panel__header,
.status-settings-panel__footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-base);
  padding: var(--xy-space-md);
}

.status-settings-panel__header {
  align-items: flex-start;
  border-bottom: 1px solid var(--xy-border);
}

.status-settings-panel__header h2,
.status-settings-addresses h3 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-weight: 600;
}

.status-settings-panel__header h2 {
  font-size: var(--xy-font-size-lg);
}

.status-settings-panel__header p,
.status-settings-toggle p,
.status-settings-addresses p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.status-settings-panel__body {
  display: grid;
  max-height: calc(100dvh - 230px);
  gap: var(--xy-space-lg);
  overflow-y: auto;
  padding: var(--xy-space-md);
}

.status-settings-toggle {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding: var(--xy-space-base);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.status-settings-link,
.status-settings-addresses {
  display: grid;
  gap: var(--xy-space-sm);
}

.status-settings-link > span {
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
}

.status-settings-link code {
  overflow: hidden;
  padding: var(--xy-space-base);
  color: var(--xy-text-primary);
  background: var(--xy-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  overflow-wrap: anywhere;
}

.status-settings-link > div {
  display: flex;
  gap: var(--xy-space-sm);
}

.status-settings-addresses h3 {
  font-size: var(--xy-font-size-base);
}

.status-settings-addresses__empty {
  padding: var(--xy-space-md);
  text-align: center;
  border: 1px dashed var(--xy-border-hover);
  border-radius: var(--xy-radius-md);
}

.status-settings-panel__state {
  display: grid;
  min-height: 280px;
  place-content: center;
  justify-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
}

.status-settings-panel__footer {
  color: var(--xy-text-muted);
  border-top: 1px solid var(--xy-border);
  font-size: var(--xy-font-size-xs);
}

@media (max-width: 1023px) {
  .status-settings-panel {
    position: static;
  }

  .status-settings-panel__body {
    max-height: none;
  }
}

@media (max-width: 599px) {
  .status-settings-panel__footer {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
