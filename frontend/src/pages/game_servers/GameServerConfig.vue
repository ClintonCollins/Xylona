<template>
  <div class="config-page">
    <!-- Loading state -->
    <div v-if="loading" class="config-loading">
      <q-spinner-dots size="40px" color="primary" />
      <div class="text-xy-secondary q-mt-sm">Loading configuration...</div>
    </div>

    <!-- No schemas defined -->
    <div v-else-if="configFiles.length === 0" class="config-empty">
      <q-icon name="tune" size="48px" class="text-xy-muted q-mb-md" />
      <div class="text-subtitle1 text-xy-secondary">No configuration schemas defined</div>
      <div class="text-caption text-xy-muted q-mt-sm">
        Configuration schemas are defined on the game definition by a superuser.
      </div>
    </div>

    <!-- Main layout: sidebar + editor -->
    <div v-else class="config-layout">
      <config-file-sidebar
        :config-files="configFiles"
        :selected-path="selectedFilePath"
        @select="handleFileSelect" />

      <div class="config-editor-panel">
        <!-- No file selected -->
        <div v-if="!selectedFilePath" class="config-placeholder">
          <q-icon name="arrow_back" size="32px" class="text-xy-muted q-mb-sm" />
          <div class="text-xy-secondary">Select a config file from the sidebar</div>
        </div>

        <!-- File editor -->
        <config-file-editor
          v-else
          :file-path="selectedFilePath"
          :format="selectedFileFormat"
          :category="selectedFileCategory"
          :category-color="selectedFileCategoryColor"
          :fields="fileFields"
          :advanced-fields="fileAdvancedFields"
          :validation-errors="validationErrors"
          :is-missing="selectedFileIsMissing"
          :saving="saving"
          :generating="generating"
          @save="handleSave"
          @generate="handleGenerate"
          @update-advanced="handleUpdateAdvanced" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { GetXylonaClient, ConnectErrorToString } from '@/utils/shared'
import {
  GetGameServerConfigFilesRequestSchema,
  GetGameServerConfigFileRequestSchema,
  UpdateGameServerConfigFileRequestSchema,
  GenerateGameServerConfigFileRequestSchema,
  ConfigFieldDataSchema,
} from '@/proto/xylona_pb'
import type {
  ConfigFileInfo,
  ConfigFieldData,
  AdvancedField,
  ConfigValidationError,
  AdvancedField as AdvancedFieldType,
} from '@/proto/xylona_pb'
import ConfigFileSidebar from '@/components/game_servers/ConfigFileSidebar.vue'
import ConfigFileEditor from '@/components/game_servers/ConfigFileEditor.vue'

const $q = useQuasar()
const route = useRoute()
const gameServerId = route.params.id as string

const loading = ref(true)
const saving = ref(false)
const generating = ref(false)
const configFiles = ref<ConfigFileInfo[]>([])
const selectedFilePath = ref('')
const selectedFileIsMissing = ref(false)
const fileFields = ref<ConfigFieldData[]>([])
const fileAdvancedFields = ref<AdvancedField[]>([])
const validationErrors = ref<ConfigValidationError[]>([])
const pendingAdvancedUpdates = ref<AdvancedField[]>([])

const CATEGORY_COLORS = [
  '#3B82F6',
  '#22C55E',
  '#F59E0B',
  '#8B5CF6',
  '#EF4444',
  '#06B6D4',
  '#EC4899',
  '#F97316',
]

const categoryColorMap = computed(() => {
  const map = new Map<string, string>()
  const categories = [...new Set(configFiles.value.map((f) => f.category || 'Uncategorized'))]
  categories.forEach((cat, i) => {
    map.set(cat, CATEGORY_COLORS[i % CATEGORY_COLORS.length])
  })
  return map
})

const selectedFile = computed(() =>
  configFiles.value.find((f) => f.path === selectedFilePath.value),
)
const selectedFileFormat = computed(() => selectedFile.value?.format || '')
const selectedFileCategory = computed(() => selectedFile.value?.category || '')
const selectedFileCategoryColor = computed(
  () => categoryColorMap.value.get(selectedFileCategory.value) || CATEGORY_COLORS[0],
)

onMounted(async () => {
  await loadConfigFiles()
})

async function loadConfigFiles() {
  loading.value = true
  try {
    const request = create(GetGameServerConfigFilesRequestSchema, {
      gameServerId,
    })
    const response = await GetXylonaClient().getGameServerConfigFiles(request)
    configFiles.value = response.configFiles
    // Auto-select first file if none selected
    if (!selectedFilePath.value && configFiles.value.length > 0) {
      await handleFileSelect(configFiles.value[0].path, !configFiles.value[0].existsOnDisk)
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loading.value = false
  }
}

async function handleFileSelect(path: string, isMissing: boolean) {
  selectedFilePath.value = path
  selectedFileIsMissing.value = isMissing
  validationErrors.value = []
  pendingAdvancedUpdates.value = []

  if (isMissing) {
    // For missing files, still load field data (shows defaults)
  }

  try {
    const request = create(GetGameServerConfigFileRequestSchema, {
      gameServerId,
      filePath: path,
    })
    const response = await GetXylonaClient().getGameServerConfigFile(request)
    fileFields.value = [...response.fields].sort((a, b) => a.key.localeCompare(b.key))
    fileAdvancedFields.value = [...response.advancedFields].sort((a, b) =>
      a.key.localeCompare(b.key),
    )
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
    fileFields.value = []
    fileAdvancedFields.value = []
  }
}

async function handleSave(fieldValues: Map<string, string>) {
  saving.value = true
  validationErrors.value = []

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
      $q.notify({
        type: 'xylona-success',
        caption: `${selectedFilePath.value} saved successfully`,
        position: 'top',
        timeout: 3000,
      })
      // Reload the file to get fresh state
      await handleFileSelect(selectedFilePath.value, false)
      // Refresh file list to update exists status
      await loadConfigFiles()
    } else {
      validationErrors.value = response.errors
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    saving.value = false
  }
}

async function handleGenerate() {
  generating.value = true
  try {
    const request = create(GenerateGameServerConfigFileRequestSchema, {
      gameServerId,
      filePath: selectedFilePath.value,
    })
    const response = await GetXylonaClient().generateGameServerConfigFile(request)

    if (response.success) {
      $q.notify({
        type: 'xylona-success',
        caption: `${selectedFilePath.value} generated successfully`,
        position: 'top',
        timeout: 3000,
      })
      selectedFileIsMissing.value = false
      // Reload
      await handleFileSelect(selectedFilePath.value, false)
      await loadConfigFiles()
    }
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    generating.value = false
  }
}

function handleUpdateAdvanced(fields: AdvancedField[]) {
  pendingAdvancedUpdates.value = fields
}
</script>

<style scoped>
.config-page {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.config-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
}

.config-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  text-align: center;
}

.config-layout {
  display: flex;
  flex: 1;
  min-height: 0;
}

.config-editor-panel {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.config-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  height: 300px;
  font-size: 0.9rem;
}
</style>
