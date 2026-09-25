<template>
  <div class="config-page xy-page-content">
    <page-header class="config-page-header" title="Configuration" />

    <!-- Loading state -->
    <div v-if="loading" class="config-loading">
      <q-spinner-dots color="primary" size="40px" />
      <div class="text-xy-secondary q-mt-sm">Loading configuration...</div>
    </div>

    <!-- Load failure -->
    <q-banner
      v-else-if="loadError"
      class="xy-banner-negative"
      data-test="config-load-error"
      dense
      inline-actions
      role="alert">
      <template #avatar>
        <q-icon name="sync_problem" />
      </template>
      <strong>Config files could not be loaded.</strong> {{ loadError }}
      <template #action>
        <q-btn
          aria-label="Retry loading config files"
          flat
          icon="refresh"
          label="Retry"
          no-caps
          @click="loadConfigFiles()" />
      </template>
    </q-banner>

    <!-- No schemas defined -->
    <empty-state
      v-else-if="configFiles.length === 0"
      :description="
        canEditGame
          ? 'Define config files on this game to edit their settings here.'
          : 'A superuser can define config schemas on the game to enable structured editing here.'
      "
      icon="tune"
      title="No config files for this game yet">
      <template v-if="canEditGame || canViewFiles" #actions>
        <q-btn
          v-if="canEditGame"
          :to="{ path: `/games/${gameId}/edit`, state: gameFormTabHistoryState('config') }"
          color="primary"
          data-test="config-edit-game"
          icon="edit"
          label="Define config files"
          no-caps />
        <q-btn
          v-if="canViewFiles"
          :to="`/game-servers/${getGameServerId()}/files`"
          flat
          icon="folder"
          label="Browse files"
          no-caps />
      </template>
    </empty-state>

    <!-- Main layout: sidebar + editor -->
    <div v-else class="config-layout">
      <config-file-sidebar
        v-if="configFiles.length > 1"
        :config-files="configFiles"
        :selected-path="selectedFilePath"
        @select="handleFileSelect" />

      <div class="config-editor-panel">
        <q-select
          v-if="configFiles.length > 1"
          :model-value="selectedFilePath"
          :options="fileOptions"
          class="config-file-select"
          dense
          emit-value
          label="Config file"
          map-options
          options-dense
          outlined
          @update:model-value="selectFileByPath" />

        <!-- No file selected -->
        <div v-if="!selectedFilePath" class="config-placeholder">
          <q-icon class="text-xy-muted q-mb-sm" name="arrow_back" size="32px" />
          <div class="text-xy-secondary">Choose a config file to start editing</div>
        </div>

        <q-banner
          v-else-if="fileLoadError"
          class="xy-banner-negative"
          data-test="config-file-load-error"
          dense
          inline-actions
          role="alert">
          <template #avatar>
            <q-icon name="sync_problem" />
          </template>
          <strong>{{ selectedFilePath }} could not be loaded.</strong> {{ fileLoadError }}
          <template #action>
            <q-btn
              :aria-label="`Retry loading ${selectedFilePath}`"
              flat
              icon="refresh"
              label="Retry"
              no-caps
              @click="handleFileSelect(selectedFilePath, selectedFileIsMissing)" />
          </template>
        </q-banner>

        <!-- File editor -->
        <template v-else>
          <seven-days-to-die-sandbox-inspector
            v-if="hasSandboxCode"
            :game-server-id="getGameServerId()"
            :refresh-key="sandboxInspectorRefreshKey" />
          <config-file-editor
            ref="editorRef"
            :advanced-fields="fileAdvancedFields"
            :category="selectedFileCategory"
            :category-color="selectedFileCategoryColor"
            :fields="fileFields"
            :file-path="selectedFilePath"
            :format="selectedFileFormat"
            :generating="generating"
            :is-missing="selectedFileIsMissing"
            :saving="saving"
            :validation-errors="validationErrors"
            @discard="handleDiscard"
            @generate="handleGenerate"
            @save="handleSave"
            @update-advanced="handleUpdateAdvanced" />
        </template>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import type {
  AdvancedField,
  AdvancedField as AdvancedFieldType,
  ConfigFieldData,
  ConfigFileInfo,
  ConfigValidationError,
} from '@/proto/xylona_pb'
import {
  ConfigFieldDataSchema,
  GenerateGameServerConfigFileRequestSchema,
  GetGameServerRequestSchema,
  GetGameServerConfigFileRequestSchema,
  GetGameServerConfigFilesRequestSchema,
  UpdateGameServerConfigFileRequestSchema,
} from '@/proto/xylona_pb'
import ConfigFileSidebar from '@/components/game_servers/ConfigFileSidebar.vue'
import ConfigFileEditor from '@/components/game_servers/ConfigFileEditor.vue'
import SevenDaysToDieSandboxInspector from '@/components/game_servers/SevenDaysToDieSandboxInspector.vue'
import { gameFormTabHistoryState } from '@/components/games/useGameFormTabs'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import { useUserAuthStore } from '@/stores/xylona'
import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'
import {
  buildCategoryColorMap,
  CATEGORY_COLORS,
} from '@/components/game_servers/config-field-helpers'

const route = useRoute()
const authStore = useUserAuthStore()

function getGameServerId(): string {
  const routeID = route?.params?.id
  if (routeID instanceof Array) {
    return routeID[0] ?? ''
  }
  return routeID ?? ''
}

const editorRef = ref<InstanceType<typeof ConfigFileEditor> | null>(null)

const editorHasChanges = computed(() => editorRef.value?.hasChanges ?? false)

const { confirmDiscard } = useUnsavedChangesGuard(editorHasChanges)

const loading = ref(true)
const loadError = ref('')
const fileLoadError = ref('')
const saving = ref(false)
const generating = ref(false)
const configFiles = ref<ConfigFileInfo[]>([])
const selectedFilePath = ref('')
const selectedFileIsMissing = ref(false)
const fileFields = ref<ConfigFieldData[]>([])
const fileAdvancedFields = ref<AdvancedField[]>([])
const validationErrors = ref<ConfigValidationError[]>([])
const pendingAdvancedUpdates = ref<AdvancedField[]>([])
const sandboxInspectorRefreshKey = ref(0)
const gameId = ref('')
const canViewFiles = ref(false)
const isSevenDaysToDie = computed(() => gameId.value === '7_days_to_die')
const canEditGame = computed(() => gameId.value !== '' && Boolean(authStore.user?.superUser))

const categoryColorMap = computed(() => buildCategoryColorMap(configFiles.value))

const selectedFile = computed(() =>
  configFiles.value.find((f) => f.path === selectedFilePath.value),
)
const selectedFileFormat = computed(() => selectedFile.value?.format || '')
const selectedFileCategory = computed(() => selectedFile.value?.category || '')
const selectedFileCategoryColor = computed(
  () => categoryColorMap.value.get(selectedFileCategory.value) || CATEGORY_COLORS[0],
)
const fileOptions = computed(() =>
  configFiles.value.map((file) => ({
    label: file.existsOnDisk ? file.path : `${file.path} (missing)`,
    value: file.path,
  })),
)
const hasSandboxCode = computed(
  () => isSevenDaysToDie.value && fileFields.value.some((field) => field.key === 'SandboxCode'),
)

onMounted(async () => {
  await Promise.all([loadConfigFiles(), loadGameServerIdentity()])
})

async function loadGameServerIdentity() {
  const gameServerId = getGameServerId()
  if (gameServerId === '') {
    return
  }
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerId }),
    )
    gameId.value = response.gameServer?.gameId ?? ''
    canViewFiles.value =
      response.gameServer?.effectivePermissions?.includes('game_server.files.view') ?? false
  } catch (unknownErr: unknown) {
    // Only the empty-state links and the 7 Days to Die inspector depend on it.
    console.error(ConnectError.from(unknownErr))
  }
}

async function loadConfigFiles(showLoading = true) {
  if (showLoading) {
    loading.value = true
    loadError.value = ''
  }

  const gameServerId = getGameServerId()
  if (gameServerId === '') {
    configFiles.value = []
    if (showLoading) {
      loading.value = false
    }
    return
  }

  try {
    const request = create(GetGameServerConfigFilesRequestSchema, {
      gameServerId,
    })
    const response = await GetXylonaClient().getGameServerConfigFiles(request)
    configFiles.value = response.configFiles
    // Auto-select first file if none selected
    const firstFile = configFiles.value[0]
    if (!selectedFilePath.value && firstFile) {
      await handleFileSelect(firstFile.path, !firstFile.existsOnDisk)
    }
  } catch (unknownErr: unknown) {
    if (showLoading) {
      loadError.value = ConnectErrorToString(ConnectError.from(unknownErr))
    } else {
      // A background refresh keeps the list that is already on screen.
      notifyConnectError(unknownErr)
    }
  } finally {
    if (showLoading) {
      loading.value = false
    }
  }
}

async function handleFileSelect(path: string, isMissing: boolean) {
  const gameServerId = getGameServerId()
  if (gameServerId === '') {
    return
  }

  // Guard against switching files with unsaved changes
  if (
    path !== selectedFilePath.value &&
    !(await confirmDiscard('You have unsaved changes. Discard them and switch files?'))
  ) {
    return
  }

  selectedFilePath.value = path
  selectedFileIsMissing.value = isMissing
  validationErrors.value = []
  pendingAdvancedUpdates.value = []
  fileLoadError.value = ''

  // Missing files still load field data, which shows the defaults.
  try {
    const request = create(GetGameServerConfigFileRequestSchema, {
      gameServerId,
      filePath: path,
    })
    const response = await GetXylonaClient().getGameServerConfigFile(request)
    fileFields.value = [...response.fields]
    fileAdvancedFields.value = [...response.advancedFields]
  } catch (unknownErr: unknown) {
    fileLoadError.value = ConnectErrorToString(ConnectError.from(unknownErr))
    fileFields.value = []
    fileAdvancedFields.value = []
  }
}

function selectFileByPath(path: string) {
  const file = configFiles.value.find((f) => f.path === path)
  if (file) {
    void handleFileSelect(file.path, !file.existsOnDisk)
  }
}

async function handleSave(fieldValues: Map<string, string>) {
  const gameServerId = getGameServerId()
  if (gameServerId === '' || selectedFilePath.value === '') {
    return
  }

  saving.value = true
  validationErrors.value = []
  let saved = false

  try {
    const fields: ConfigFieldData[] = []
    for (const [key, value] of fieldValues) {
      fields.push(
        create(ConfigFieldDataSchema, {
          key,
          value,
        }),
      )
    }

    const request = create(UpdateGameServerConfigFileRequestSchema, {
      gameServerId,
      filePath: selectedFilePath.value,
      fields,
      advancedFields:
        pendingAdvancedUpdates.value.length > 0
          ? (pendingAdvancedUpdates.value as AdvancedFieldType[])
          : (fileAdvancedFields.value as AdvancedFieldType[]),
    })
    const response = await GetXylonaClient().updateGameServerConfigFile(request)

    if (response.success) {
      saved = true
      if (fieldValues.has('SandboxCode')) sandboxInspectorRefreshKey.value += 1
      notifySuccess(`${selectedFilePath.value} saved successfully`)
      // Reload the file to get fresh state
      await handleFileSelect(selectedFilePath.value, false)
      // Refresh file list to update exists status (without showing loading spinner)
      await loadConfigFiles(false)
    } else {
      validationErrors.value = response.errors
    }
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    saving.value = false
    // Edits clear only once the server accepted them; a failed save keeps them.
    if (saved) editorRef.value?.confirmSaved()
  }
}

function handleDiscard() {
  validationErrors.value = []
  pendingAdvancedUpdates.value = []
  // A new array makes the advanced fields panel drop its local edits.
  fileAdvancedFields.value = [...fileAdvancedFields.value]
}

async function handleGenerate() {
  const gameServerId = getGameServerId()
  if (gameServerId === '' || selectedFilePath.value === '') {
    return
  }

  generating.value = true
  try {
    const request = create(GenerateGameServerConfigFileRequestSchema, {
      gameServerId,
      filePath: selectedFilePath.value,
    })
    const response = await GetXylonaClient().generateGameServerConfigFile(request)

    if (response.success) {
      notifySuccess(`${selectedFilePath.value} generated successfully`)
      selectedFileIsMissing.value = false
      // Reload
      await handleFileSelect(selectedFilePath.value, false)
      await loadConfigFiles()
    }
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    generating.value = false
  }
}

function handleUpdateAdvanced(fields: AdvancedField[]) {
  pendingAdvancedUpdates.value = fields
}
</script>

<style scoped>
.config-page-header {
  margin-bottom: var(--xy-space-sm);
}

.config-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
}

/* The page scrolls as a whole inside the server layout; the editor header and
   the file list stay pinned while the settings scroll past. */
.config-layout {
  display: flex;
  align-items: flex-start;
}

.config-editor-panel {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.config-file-select {
  display: none;
}

.config-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  font-size: var(--xy-font-size-sm);
}

@media (max-width: 599px) {
  .config-file-select {
    display: flex;
    margin-bottom: var(--xy-space-sm);
  }

  .config-placeholder {
    height: 200px;
  }
}
</style>
