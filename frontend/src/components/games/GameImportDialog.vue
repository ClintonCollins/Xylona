<template>
  <q-dialog v-model="dialogModel">
    <q-card class="game-import-dialog">
      <q-card-section class="game-import-dialog__header">
        <div>
          <h2 class="game-import-dialog__title font-display">Import Game JSON</h2>
          <div class="text-caption text-xy-muted">Preview changes before applying them.</div>
        </div>
        <q-btn
          aria-label="Close import dialog"
          dense
          flat
          icon="close"
          round
          @click="closeDialog" />
      </q-card-section>

      <q-separator />

      <q-card-section class="game-import-dialog__body">
        <q-file
          v-model="selectedFile"
          accept=".json,application/json"
          :disable="previewing || submitting"
          :error="readError !== ''"
          :error-message="readError"
          label="Game definition JSON"
          outlined
          @update:model-value="onSelectedFileChanged">
          <template #prepend>
            <q-icon name="upload_file" />
          </template>
        </q-file>

        <div v-if="previewResponse" class="game-import-dialog__preview">
          <div class="game-import-dialog__preview-header">
            <div>
              <div class="text-caption text-xy-muted">Definition</div>
              <div class="game-import-dialog__game-name">
                {{ previewResponse.game?.name || previewResponse.importedGameId || 'Unnamed game' }}
              </div>
              <div class="text-caption text-xy-secondary">
                {{ previewResponse.game?.id || previewResponse.importedGameId }}
              </div>
            </div>
            <q-badge :color="previewBadgeColor" :label="previewStatus" />
          </div>

          <q-banner v-if="previewResponse.validationErrors.length > 0" class="xy-banner-negative">
            <div class="text-weight-medium">Validation errors</div>
            <ul class="game-import-dialog__message-list">
              <li v-for="message in previewResponse.validationErrors" :key="message">
                {{ message }}
              </li>
            </ul>
          </q-banner>

          <q-banner v-if="previewResponse.warnings.length > 0" class="xy-banner-warning">
            <div class="text-weight-medium">Warnings</div>
            <ul class="game-import-dialog__message-list">
              <li v-for="message in previewResponse.warnings" :key="message">
                {{ message }}
              </li>
            </ul>
          </q-banner>

          <div class="game-import-dialog__impact">
            <div class="game-import-dialog__impact-line">
              <span>Affected servers</span>
              <strong>{{ affectedGameServerCountLabel }}</strong>
            </div>
            <div
              v-if="previewResponse.affectedGameServerNames.length > 0"
              class="game-import-dialog__server-list">
              <q-chip
                v-for="serverName in affectedServerPreview"
                :key="serverName"
                dense
                outline
                square>
                {{ serverName }}
              </q-chip>
              <q-chip v-if="remainingAffectedServers > 0" dense outline square>
                +{{ remainingAffectedServers }} more
              </q-chip>
            </div>
          </div>

          <div
            v-if="previewResponse.idConflict && previewResponse.validationErrors.length === 0"
            class="game-import-dialog__mode">
            <div class="text-caption text-xy-muted">Conflict action</div>
            <q-btn-toggle
              v-model="selectedMode"
              :disable="submitting"
              :options="importModeOptions"
              color="secondary"
              no-caps
              toggle-color="primary"
              unelevated />
          </div>
        </div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn :disable="submitting" flat label="Cancel" @click="closeDialog" />
        <q-btn
          :disable="!fileContent || previewing || submitting"
          :loading="previewing"
          flat
          label="Preview"
          @click="previewImport" />
        <q-btn
          :disable="!canApply"
          :loading="submitting"
          color="primary"
          label="Import"
          @click="applyImport" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { computed, ref, watch } from 'vue'
import { useQuasar } from 'quasar'
import { GameImportMode, ImportGameRequestSchema, type ImportGameResponse } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

type QFileValue = File | File[] | null

const props = defineProps({
  showDialog: {
    type: Boolean,
    required: true,
  },
})

const emit = defineEmits<{
  'update:showDialog': [showDialog: boolean]
  imported: [gameID: string]
}>()

const $q = useQuasar()
const selectedFile = ref<File | null>(null)
const fileContent = ref('')
const readError = ref('')
const previewing = ref(false)
const submitting = ref(false)
const previewResponse = ref<ImportGameResponse | null>(null)
const selectedMode = ref(GameImportMode.APPLY)

const dialogModel = computed({
  get: () => props.showDialog,
  set: (value: boolean) => emit('update:showDialog', value),
})

const importModeOptions = [
  { label: 'Update existing', value: GameImportMode.APPLY },
  { label: 'Import copy', value: GameImportMode.IMPORT_COPY },
]

const previewStatus = computed(() => {
  if (!previewResponse.value) {
    return ''
  }
  if (previewResponse.value.validationErrors.length > 0) {
    return 'Invalid'
  }
  if (previewResponse.value.idConflict) {
    return 'Conflict'
  }
  return 'Ready'
})

const previewBadgeColor = computed(() => {
  if (!previewResponse.value) {
    return 'grey-8'
  }
  if (previewResponse.value.validationErrors.length > 0) {
    return 'negative'
  }
  if (previewResponse.value.idConflict) {
    return 'warning'
  }
  return 'positive'
})

const affectedGameServerCountLabel = computed(() => {
  const count = Number(previewResponse.value?.affectedGameServerCount ?? 0n)
  return `${count} ${count === 1 ? 'server' : 'servers'}`
})

const affectedServerPreview = computed(() => {
  return previewResponse.value?.affectedGameServerNames.slice(0, 6) ?? []
})

const remainingAffectedServers = computed(() => {
  const count = previewResponse.value?.affectedGameServerNames.length ?? 0
  return Math.max(0, count - affectedServerPreview.value.length)
})

const canApply = computed(() => {
  return (
    fileContent.value !== '' &&
    previewResponse.value !== null &&
    previewResponse.value.validationErrors.length === 0 &&
    !previewing.value &&
    !submitting.value
  )
})

watch(
  () => props.showDialog,
  (showDialog) => {
    if (!showDialog) {
      resetImportState()
    }
  },
)

async function onSelectedFileChanged(value: QFileValue): Promise<void> {
  resetPreviewState()
  const file = normalizeFile(value)
  selectedFile.value = file

  if (!file) {
    return
  }

  try {
    fileContent.value = await file.text()
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    readError.value = err.message || 'Could not read the selected file.'
    return
  }

  await previewImport()
}

async function previewImport(): Promise<void> {
  if (fileContent.value === '') {
    return
  }

  previewing.value = true
  previewResponse.value = null
  readError.value = ''
  selectedMode.value = GameImportMode.APPLY

  try {
    previewResponse.value = await GetXylonaClient().importGame(
      create(ImportGameRequestSchema, {
        gameDefinitionJson: fileContent.value,
        mode: GameImportMode.PREVIEW,
      }),
    )
  } catch (unknownError: unknown) {
    notifyImportFailure('Failed to preview game JSON', unknownError)
  } finally {
    previewing.value = false
  }
}

async function applyImport(): Promise<void> {
  if (!canApply.value) {
    return
  }

  submitting.value = true

  try {
    const response = await GetXylonaClient().importGame(
      create(ImportGameRequestSchema, {
        gameDefinitionJson: fileContent.value,
        mode: selectedMode.value,
      }),
    )
    previewResponse.value = response

    if (response.validationErrors.length > 0) {
      $q.notify({
        type: 'xylona-error',
        position: 'top',
        caption: 'Game JSON did not pass validation.',
        icon: 'report_problem',
      })
      return
    }

    const importedGameID = response.importedGameId || response.game?.id || ''
    $q.notify({
      type: 'xylona-success',
      position: 'top',
      caption: `Imported ${response.game?.name || importedGameID || 'game definition'}.`,
      icon: 'check_circle',
    })

    emit('imported', importedGameID)
    dialogModel.value = false
  } catch (unknownError: unknown) {
    notifyImportFailure('Failed to import game JSON', unknownError)
  } finally {
    submitting.value = false
  }
}

function closeDialog(): void {
  dialogModel.value = false
}

function normalizeFile(value: QFileValue): File | null {
  if (Array.isArray(value)) {
    return value[0] ?? null
  }
  return value
}

function resetImportState(): void {
  selectedFile.value = null
  fileContent.value = ''
  resetPreviewState()
}

function resetPreviewState(): void {
  readError.value = ''
  previewResponse.value = null
  selectedMode.value = GameImportMode.APPLY
}

function notifyImportFailure(captionPrefix: string, unknownError: unknown): void {
  $q.notify({
    type: 'xylona-error',
    position: 'top',
    caption: `${captionPrefix}: ${ConnectErrorToString(ConnectError.from(unknownError))}`,
    icon: 'report_problem',
  })
}
</script>

<style scoped>
.game-import-dialog {
  width: min(720px, calc(100vw - 32px));
  max-width: 720px;
  background: var(--xy-surface-1);
}

.game-import-dialog__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.game-import-dialog__title {
  margin: 0;
  font-size: 1rem;
  line-height: 1.2;
  color: var(--xy-text-primary);
}

.game-import-dialog__body {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.game-import-dialog__preview {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.game-import-dialog__preview-header,
.game-import-dialog__impact {
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  background: var(--xy-surface-0);
  padding: var(--xy-space-md);
}

.game-import-dialog__preview-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.game-import-dialog__game-name {
  color: var(--xy-text-primary);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.game-import-dialog__message-list {
  margin: var(--xy-space-xs) 0 0;
  padding-left: var(--xy-space-lg);
}

.game-import-dialog__impact {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.game-import-dialog__impact-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
  color: var(--xy-text-secondary);
}

.game-import-dialog__impact-line strong {
  color: var(--xy-text-primary);
}

.game-import-dialog__server-list {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
}

.game-import-dialog__mode {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.xy-banner-negative {
  border: 1px solid color-mix(in srgb, var(--xy-danger) 35%, var(--xy-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--xy-danger) 10%, var(--xy-surface-0));
  color: var(--xy-text-primary);
}

.xy-banner-warning {
  border: 1px solid color-mix(in srgb, var(--xy-warning) 35%, var(--xy-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--xy-warning) 10%, var(--xy-surface-0));
  color: var(--xy-text-primary);
}
</style>
