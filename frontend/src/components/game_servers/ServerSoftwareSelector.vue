<template>
  <div>
    <q-dialog
      v-model="showChangeDialog"
      persistent
      backdrop-filter="brightness(15%)"
      aria-labelledby="software-change-title">
      <q-card class="change-dialog-card">
        <q-card-section class="change-dialog-header">
          <div id="software-change-title" class="change-dialog-title">Change Server Software</div>
          <div class="change-dialog-subtitle">Select new software and version for this server</div>
        </q-card-section>

        <q-card-section class="change-dialog-body">
          <div class="dialog-current">
            <div class="dialog-current-icon">
              <q-icon name="dns" size="16px" color="accent" />
            </div>
            <div>
              <div class="dialog-current-label">Currently active</div>
              <div>
                <span class="dialog-current-value">{{ currentSoftwareDisplayName }}</span>
                <span v-if="currentVersion" class="dialog-current-version">{{
                  currentVersion
                }}</span>
              </div>
            </div>
          </div>

          <div class="dialog-fields">
            <div class="dialog-field">
              <q-select
                v-model="selectedSoftwareId"
                outlined
                dense
                label="Software"
                emit-value
                map-options
                :display-value="selectedSoftwareName || undefined"
                :options="softwareSelectOptions"
                :loading="loadingOptions"
                :disable="saving || installStatus === 'installing'"
                options-selected-class="selected-option"
                @update:model-value="onSoftwareChange" />
            </div>
            <div class="dialog-field">
              <q-select
                v-if="selectedSoftwareId !== '' && versions.length > 0"
                v-model="selectedVersionId"
                outlined
                dense
                label="Version"
                emit-value
                map-options
                :options="versionSelectOptions"
                :loading="loadingVersions"
                :disable="saving || loadingVersions || installStatus === 'installing'"
                options-selected-class="selected-option" />
            </div>
          </div>

          <div class="dialog-warning">
            <q-icon name="warning" size="16px" color="warning" class="dialog-warning-icon" />
            <span>
              Switching server software may affect installed mods. Mods will be preserved but may
              not be compatible with {{ selectedSoftwareName || 'the new software' }}.
            </span>
          </div>
        </q-card-section>

        <q-card-actions class="change-dialog-footer" align="right">
          <q-btn
            flat
            no-caps
            label="Cancel"
            class="dialog-cancel-btn"
            @click="cancelChangeDialog" />
          <q-btn
            no-caps
            label="Apply"
            color="primary"
            :loading="saving"
            :disable="!canApply || installStatus === 'installing'"
            @click="applyServerSoftware" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, onUnmounted, ref } from 'vue'

import {
  GetServerSoftwareOptionsRequestSchema,
  GetServerSoftwareStatusRequestSchema,
  GetServerSoftwareVersionsRequestSchema,
  SetServerSoftwareRequestSchema,
} from '@/proto/xylona_pb'
import type { ServerSoftwareOption, SoftwareVersion } from '@/proto/shared_pb'
import { GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import type { ServerSoftwareOperationEvent } from './ServerSoftwareSelector.types'

interface Props {
  gameServerId: string
  gameId: string
  gameName: string
  currentSoftware?: string
  currentVersion?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'software-changed': []
  'software-operation-state': [event: ServerSoftwareOperationEvent]
}>()

const $q = useQuasar()

const softwareOptions = ref<ServerSoftwareOption[]>([])
const versions = ref<SoftwareVersion[]>([])
const selectedSoftwareId = ref('')
const selectedVersionId = ref('')
const loadingOptions = ref(false)
const loadingVersions = ref(false)
const saving = ref(false)
const showChangeDialog = ref(false)
const installStatus = ref<'idle' | 'installing' | 'complete' | 'failed'>('idle')

const softwareSelectOptions = computed(() =>
  softwareOptions.value.map((opt) => ({
    label: opt.name,
    value: opt.id,
  })),
)

const versionSelectOptions = computed(() =>
  versions.value.map((v) => ({
    label: v.versionString || v.versionId,
    value: v.versionId,
  })),
)

const selectedSoftwareName = computed(() => {
  const found = softwareOptions.value.find((opt) => opt.id === selectedSoftwareId.value)
  return found?.name ?? ''
})

const currentSoftwareDisplayName = computed(() => {
  if (props.currentSoftware) {
    const found = softwareOptions.value.find((opt) => opt.id === props.currentSoftware)
    if (found) {
      return found.name
    }
  }
  return props.gameName || 'Unknown'
})

const selectedSoftwareHasJarSource = computed(() => {
  const found = softwareOptions.value.find((opt) => opt.id === selectedSoftwareId.value)
  return (found?.jarSource ?? '') !== ''
})

const canApply = computed(() => {
  if (selectedSoftwareId.value === '' || saving.value || loadingVersions.value) return false
  if (versions.value.length > 0) return selectedVersionId.value !== ''
  return true
})

const selectedVersionLabel = computed(() => {
  if (selectedVersionId.value === '') {
    return ''
  }
  const found = versions.value.find((version) => version.versionId === selectedVersionId.value)
  return found?.versionString || found?.versionId || selectedVersionId.value
})

onMounted(async () => {
  XylonaEventBus.on('serverSoftwareInstall', handleInstallEvent)

  await fetchSoftwareOptions()
  await checkInstallStatus()
})

onUnmounted(() => {
  XylonaEventBus.off('serverSoftwareInstall', handleInstallEvent)
})

function openChangeDialog(): void {
  if (props.currentSoftware) {
    selectedSoftwareId.value = props.currentSoftware
  }
  showChangeDialog.value = true
}

function cancelChangeDialog(): void {
  showChangeDialog.value = false
}

async function checkInstallStatus(): Promise<void> {
  try {
    const response = await GetXylonaClient().getServerSoftwareStatus(
      create(GetServerSoftwareStatusRequestSchema, {
        gameServerId: props.gameServerId,
      }),
    )
    if (response.status === 'installing') {
      installStatus.value = 'installing'
      emitOperationState('installing', response.softwareId)
    } else if (response.status === 'failed') {
      installStatus.value = 'failed'
      emitOperationState('failed', response.softwareId, response.error)
    }
  } catch {
    // Non-critical — ignore
  }
}

async function fetchSoftwareOptions(): Promise<void> {
  loadingOptions.value = true
  try {
    const response = await GetXylonaClient().getServerSoftwareOptions(
      create(GetServerSoftwareOptionsRequestSchema, {
        gameId: props.gameId,
      }),
    )
    softwareOptions.value = response.options

    if (props.currentSoftware) {
      const match = softwareOptions.value.find((opt) => opt.id === props.currentSoftware)
      if (match) {
        selectedSoftwareId.value = match.id
        await fetchVersions()
      }
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    console.error('Failed to fetch server software options:', err)
  } finally {
    loadingOptions.value = false
  }
}

async function fetchVersions(): Promise<void> {
  if (selectedSoftwareId.value === '') {
    versions.value = []
    return
  }
  if (!selectedSoftwareHasJarSource.value) {
    versions.value = []
    return
  }
  loadingVersions.value = true
  try {
    const response = await GetXylonaClient().getServerSoftwareVersions(
      create(GetServerSoftwareVersionsRequestSchema, {
        gameId: props.gameId,
        softwareId: selectedSoftwareId.value,
      }),
    )
    versions.value = response.versions
    if (versions.value.length > 0) {
      selectedVersionId.value = versions.value[0].versionId
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    console.error('Failed to fetch software versions:', err)
    $q.notify({
      caption: `Failed to fetch versions: ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loadingVersions.value = false
  }
}

async function onSoftwareChange(): Promise<void> {
  selectedVersionId.value = ''
  versions.value = []
  await fetchVersions()
}

async function applyServerSoftware(): Promise<void> {
  saving.value = true
  try {
    const response = await GetXylonaClient().setServerSoftware(
      create(SetServerSoftwareRequestSchema, {
        gameServerId: props.gameServerId,
        softwareId: selectedSoftwareId.value,
        versionId: selectedVersionId.value,
      }),
    )
    showChangeDialog.value = false

    if (response.status === 'installing') {
      installStatus.value = 'installing'
      emitOperationState('installing', selectedSoftwareId.value)
    } else {
      installStatus.value = 'complete'
      emitOperationState('complete', selectedSoftwareId.value)
      emit('software-changed')
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    showChangeDialog.value = false
    installStatus.value = 'failed'
    emitOperationState('failed', selectedSoftwareId.value, err.message)
  } finally {
    saving.value = false
  }
}

function emitOperationState(
  status: ServerSoftwareOperationEvent['status'],
  softwareId: string,
  error = '',
): void {
  const resolvedSoftwareId = softwareId || props.currentSoftware || selectedSoftwareId.value
  emit('software-operation-state', {
    status,
    softwareId: resolvedSoftwareId,
    softwareName: resolveSoftwareName(resolvedSoftwareId),
    versionLabel: selectedVersionLabel.value || props.currentVersion || undefined,
    error: error || undefined,
  })
}

function resolveSoftwareName(softwareId: string): string {
  if (softwareId !== '') {
    const found = softwareOptions.value.find((opt) => opt.id === softwareId)
    if (found) {
      return found.name
    }
  }
  return currentSoftwareDisplayName.value
}

function handleInstallEvent(
  gameServerId: string,
  status: string,
  error: string,
  softwareId: string,
): void {
  if (gameServerId !== props.gameServerId) {
    return
  }

  if (status === 'installing') {
    installStatus.value = 'installing'
    emitOperationState('installing', softwareId)
    return
  }

  if (status === 'complete') {
    installStatus.value = 'idle'
    emitOperationState('complete', softwareId)
    void fetchSoftwareOptions()
    emit('software-changed')
    return
  }

  if (status === 'failed') {
    installStatus.value = 'failed'
    emitOperationState('failed', softwareId, error)
  }
}

defineExpose({
  softwareOptions,
  versions,
  currentSoftwareDisplayName,
  openChangeDialog,
})
</script>

<style scoped>
/* Dialog styles */
.change-dialog-card {
  width: 420px;
  max-width: 90vw;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
}

.change-dialog-header {
  padding-bottom: 0.75rem;
  border-bottom: 1px solid var(--xy-border);
}

.change-dialog-title {
  font-family: var(--xy-font-display);
  font-size: 1.05rem;
  font-weight: 700;
  color: var(--xy-text-primary);
}

.change-dialog-subtitle {
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  margin-top: 0.2rem;
}

.change-dialog-body {
  display: flex;
  flex-direction: column;
  gap: 0.875rem;
}

.dialog-current {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  padding: 0.5rem 0.75rem;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: 6px;
}

.dialog-current-icon {
  width: 28px;
  height: 28px;
  border-radius: 5px;
  background: var(--xy-surface-2);
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.dialog-current-label {
  font-size: 0.65rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--xy-text-muted);
  line-height: 1;
}

.dialog-current-value {
  font-family: var(--xy-font-display);
  font-size: 0.85rem;
  color: var(--xy-text-primary);
}

.dialog-current-version {
  font-family: var(--xy-font-mono);
  font-size: 0.72rem;
  color: var(--xy-text-muted);
  margin-left: 0.3rem;
}

.dialog-fields {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0.625rem;
}

.dialog-warning {
  display: flex;
  align-items: flex-start;
  gap: 0.5rem;
  padding: 0.5rem 0.625rem;
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: 6px;
  font-size: 0.78rem;
  color: var(--xy-text-secondary);
  line-height: 1.4;
}

.dialog-warning-icon {
  flex-shrink: 0;
  margin-top: 1px;
}

.change-dialog-footer {
  border-top: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
}

.dialog-cancel-btn {
  border: 1px solid var(--xy-border);
  color: var(--xy-text-secondary);
}

.dialog-cancel-btn:hover {
  border-color: var(--xy-text-muted);
  color: var(--xy-text-primary);
}

/* Install progress */
.install-progress {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  border-radius: 6px;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  margin-top: 0.5rem;
}

.install-progress-spinner {
  flex-shrink: 0;
}

.install-progress-text {
  font-size: 0.8rem;
  color: var(--xy-text-secondary);
}

/* Install error */
.install-error {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.375rem 0.625rem;
  border-radius: 6px;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-danger-border, var(--xy-border));
  margin-top: 0.5rem;
}

.install-error-icon {
  flex-shrink: 0;
}

.install-error-text {
  font-size: 0.8rem;
  color: var(--xy-text-secondary);
  flex: 1;
}

.install-error-retry {
  flex-shrink: 0;
}
</style>
