<script lang="ts" setup>
import { ref } from 'vue'
import { GetXylonaClient } from '@/utils/shared'
import { create } from '@bufbuild/protobuf'
import { GetSteamAppDetailsRequestSchema } from '@/proto/xylona_pb'

const emit = defineEmits<{
  (e: 'select', value: { appId: string; name: string }): void
}>()

const appIdInput = ref('')
const loading = ref(false)
const lookupError = ref('')
const lookupResult = ref<{
  appId: string
  name: string
  type: string
  windowsSupport: boolean
  linuxSupport: boolean
  installDirectory: string
} | null>(null)

function lookupExample(id: string) {
  appIdInput.value = id
  void lookupAppId()
}

async function lookupAppId(): Promise<void> {
  const trimmed = appIdInput.value.trim()
  if (trimmed.length === 0) {
    return
  }

  loading.value = true
  lookupError.value = ''
  lookupResult.value = null

  try {
    const client = GetXylonaClient()
    const req = create(GetSteamAppDetailsRequestSchema, { appId: trimmed })
    const response = await client.getSteamAppDetails(req)

    if (!response.detailsAvailable || !response.details) {
      lookupError.value = `No information found for AppID ${trimmed}. Check the ID and try again.`
      return
    }

    lookupResult.value = {
      appId: response.details.appId,
      name: response.details.name,
      type: response.details.windowsSupport || response.details.linuxSupport ? 'Tool' : '',
      windowsSupport: response.details.windowsSupport,
      linuxSupport: response.details.linuxSupport,
      installDirectory: response.details.installDirectory,
    }
  } catch (err: unknown) {
    console.error('Steam app lookup failed:', err)
    lookupError.value = 'Failed to look up AppID. Please check your connection and try again.'
  } finally {
    loading.value = false
  }
}

function confirmSelection(): void {
  if (!lookupResult.value) {
    return
  }
  emit('select', {
    appId: lookupResult.value.appId,
    name: lookupResult.value.name,
  })
}

function clearResult(): void {
  lookupResult.value = null
  lookupError.value = ''
  appIdInput.value = ''
}
</script>

<template>
  <div class="steam-app-lookup">
    <p class="text-body2 q-mb-md" style="color: var(--xy-text-secondary)">
      Enter the Steam AppID for the game's <strong>dedicated server</strong>. You can find this on
      the game's
      <a href="https://steamdb.info/" rel="noopener" style="color: var(--xy-accent)" target="_blank"
        >SteamDB page</a
      >
      or the
      <a
        href="https://developer.valvesoftware.com/wiki/Dedicated_Servers_List"
        rel="noopener"
        style="color: var(--xy-accent)"
        target="_blank"
        >Valve Dedicated Servers List</a
      >.
    </p>

    <div class="row items-center q-gutter-sm">
      <q-input
        v-model="appIdInput"
        :error="lookupError.length > 0"
        :error-message="lookupError"
        :loading="loading"
        class="col"
        label="Dedicated Server AppID"
        outlined
        placeholder="e.g. 896660 (Valheim), 294420 (7DTD)"
        type="number"
        @keydown.enter="lookupAppId">
        <template #prepend>
          <q-icon name="mdi-steam" />
        </template>
      </q-input>

      <q-btn
        :disable="appIdInput.trim().length === 0"
        :loading="loading"
        color="primary"
        label="Look Up"
        unelevated
        @click="lookupAppId" />
    </div>

    <!-- Lookup Result -->
    <q-card v-if="lookupResult" bordered class="q-mt-md lookup-result-card" flat>
      <q-card-section>
        <div class="row items-center q-gutter-sm q-mb-sm">
          <q-icon color="positive" name="mdi-check-circle" size="sm" />
          <span class="text-subtitle1 text-weight-medium">{{ lookupResult.name }}</span>
          <q-badge :label="'AppID: ' + lookupResult.appId" color="grey-8" />
        </div>

        <div class="row q-gutter-md text-body2" style="color: var(--xy-text-secondary)">
          <div v-if="lookupResult.windowsSupport" class="row items-center q-gutter-xs">
            <q-icon name="mdi-microsoft-windows" size="xs" />
            <span>Windows</span>
          </div>
          <div v-if="lookupResult.linuxSupport" class="row items-center q-gutter-xs">
            <q-icon name="mdi-linux" size="xs" />
            <span>Linux</span>
          </div>
          <div v-if="lookupResult.installDirectory" class="row items-center q-gutter-xs">
            <q-icon name="mdi-folder" size="xs" />
            <span>{{ lookupResult.installDirectory }}</span>
          </div>
        </div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn color="negative" dense flat label="Clear" no-caps @click="clearResult" />
        <q-btn
          color="primary"
          dense
          label="Use This Server"
          no-caps
          unelevated
          @click="confirmSelection" />
      </q-card-actions>
    </q-card>

    <!-- Common AppID examples -->
    <div class="q-mt-md text-caption" style="color: var(--xy-text-muted)">
      <strong>Common AppIDs:</strong>
      <span
        v-for="example in [
          { id: '896660', name: 'Valheim' },
          { id: '294420', name: '7 Days to Die' },
          { id: '376030', name: 'ARK: SE' },
          { id: '232250', name: 'TF2' },
          { id: '740', name: 'CS2' },
          { id: '2394010', name: 'Palworld' },
        ]"
        :key="example.id"
        class="example-chip"
        @click="lookupExample(example.id)">
        {{ example.name }} ({{ example.id }})
      </span>
    </div>
  </div>
</template>

<style scoped>
.steam-app-lookup {
  max-width: 600px;
}

.lookup-result-card {
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.example-chip {
  display: inline-block;
  padding: 2px 8px;
  margin: 2px 4px;
  border-radius: 4px;
  background: var(--xy-surface-2);
  cursor: pointer;
  transition: background 0.15s;
}

.example-chip:hover {
  background: var(--xy-accent);
  color: var(--xy-text-emphasis-strong);
}
</style>
