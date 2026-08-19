<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="full-width">
      <q-form @submit.prevent="archiveFiles">
        <q-card-section>
          <div id="dialog-title" class="text-h6">{{ archiveTitle }}</div>
        </q-card-section>
        <q-card-section v-if="!archiveSubmitting">
          <div class="q-pa-lg">
            <div class="row wrap q-col-gutter-md justify-between">
              <q-select
                v-model="archiveType"
                :options="archiveTypeOptions"
                class="col-12"
                emit-value
                label="Archive type"
                map-options
                outlined
                @update:model-value="archiveSuffix = ArchiveTypeToExtension(archiveType)">
                <template #prepend>
                  <q-icon name="event" />
                </template>
              </q-select>
              <q-input
                v-model="archiveName"
                :suffix="archiveSuffix"
                aria-autocomplete="none"
                autofocus
                class="col-12"
                label="Archive name"
                name="archive-name"
                outlined
                placeholder="example-archive"
                :rules="[validateArchiveName]" />
            </div>
          </div>
        </q-card-section>
        <q-card-section v-else>
          <div aria-live="polite" role="status">
            <div class="text-caption">
              {{ bytesToSize(archiveCurrentBytes) }} out of
              {{ bytesToSize(archiveTotalBytes) }} archived ({{ submitPercent }}%)
            </div>
            <div class="text-caption">{{ filesArchived }} / {{ totalFiles }} files archived</div>
            <div class="file-progress-name text-caption font-mono">{{ currentArchiveFile }}</div>
            <q-linear-progress
              aria-label="Archive progress"
              :value="submitProgress"
              color="primary"
              size="lg"
              stripe />
          </div>
        </q-card-section>
        <q-card-actions v-if="!archiveSubmitting" align="right">
          <q-btn color="primary" flat label="Cancel" @click="showDialog = false" />
          <q-btn color="primary" label="Archive" type="submit" />
        </q-card-actions>
        <q-card-actions v-else align="right">
          <q-btn color="negative" label="Stop archiving" @click="abortArchive" />
        </q-card-actions>
      </q-form>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { QBtn, QCard, QCardSection, QDialog, QInput, useQuasar } from 'quasar'
import {
  File as xylonaFile,
  GameServerFilesCompressionRequest,
  GameServerFilesCompressionRequestSchema,
  GameServerFilesCompressionType,
} from '@/proto/gameserver_files_operations_pb'
import {
  ArchiveTypeToExtension,
  ArchiveTypeToString,
  bytesToSize,
  GetXylonaClientCallback,
} from '@/utils/shared'
import { ref } from 'vue'

const props = defineProps({
  gameServerId: {
    type: String,
    required: true,
  },
  path: {
    type: String,
    required: true,
  },
  selectedFiles: {
    type: Array<xylonaFile>,
    default: [],
  },
  pathSeparator: {
    type: String,
    default: '/',
  },
})

const DEFAULT_ARCHIVE_TYPE = GameServerFilesCompressionType.ZST
const DEFAULT_ARCHIVE_SUFFIX = ArchiveTypeToExtension(DEFAULT_ARCHIVE_TYPE)

const archiveSubmitting = ref(false)
const submitProgress = ref(0)
const submitPercent = ref(0)
const archiveCurrentBytes = ref(0)
const archiveTotalBytes = ref(0)
const currentArchiveFile = ref('')
const archiveTitle = ref('Archive Files')
const filesArchived = ref(0)
const totalFiles = ref(0)
const abortController = ref<AbortController | null>(null)
const abortedArchive = ref(false)
const archiveType = ref(DEFAULT_ARCHIVE_TYPE)
const archiveSuffix = ref(DEFAULT_ARCHIVE_SUFFIX)
const archiveTypeOptions = ref([
  {
    label: ArchiveTypeToString(GameServerFilesCompressionType.ZIP),
    value: GameServerFilesCompressionType.ZIP,
  },
  {
    label: ArchiveTypeToString(GameServerFilesCompressionType.GZIP),
    value: GameServerFilesCompressionType.GZIP,
  },
  {
    label: ArchiveTypeToString(GameServerFilesCompressionType.BZIP2),
    value: GameServerFilesCompressionType.BZIP2,
  },
  {
    label: ArchiveTypeToString(GameServerFilesCompressionType.ZST),
    value: GameServerFilesCompressionType.ZST,
  },
  {
    label: ArchiveTypeToString(GameServerFilesCompressionType.XZ),
    value: GameServerFilesCompressionType.XZ,
  },
])

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})
const archiveName = defineModel('archiveName', {
  type: String,
  default: '',
  required: true,
})

const emit = defineEmits(['submit'])

function validateArchiveName(value: string): true | string {
  return value.trim() === '' ? 'Enter an archive name.' : true
}

async function archiveFiles() {
  if (archiveSubmitting.value || validateArchiveName(archiveName.value) !== true) {
    return
  }
  archiveSubmitting.value = true
  abortedArchive.value = false

  const request: GameServerFilesCompressionRequest = create(
    GameServerFilesCompressionRequestSchema,
    {},
  )
  request.gameServerId = props.gameServerId
  request.fullDestinationFilePath = getRelativeFilePath(archiveName.value)
  request.compressionType = archiveType.value
  request.fullFilePaths = props.selectedFiles.map((file: xylonaFile) => {
    return getRelativeFilePath(file.name)
  })

  abortController.value = new AbortController()
  GetXylonaClientCallback().gameServerFilesArchive(
    request,
    (response) => {
      currentArchiveFile.value =
        response.filesCompressed === response.totalFiles
          ? 'All files archived!'
          : `Current file: ${response.currentFile}`
      archiveCurrentBytes.value =
        response.filesCompressed === response.totalFiles
          ? Number(response.totalBytes)
          : Number(response.bytesCompressed)
      archiveTotalBytes.value = Number(response.totalBytes)
      filesArchived.value = Number(response.filesCompressed)
      totalFiles.value = Number(response.totalFiles)
      submitProgress.value =
        response.filesCompressed === response.totalFiles
          ? 1
          : archiveCurrentBytes.value / archiveTotalBytes.value
      submitPercent.value = Math.round(submitProgress.value * 100)
    },
    (err?: ConnectError) => {
      archiveSubmitting.value = false
      if (abortedArchive.value) {
        resetArchiveStats()
        showDialog.value = false
        emit('submit')
        $q.notify({
          caption: `Files archiving aborted.`,
          type: 'xylona-alert',
          position: 'top',
          timeout: 3000,
        })
        return
      }
      if (err) {
        resetArchiveProgress()
        console.error(err)
        $q.notify({
          caption: `Error archiving files. ${err.message}`,
          type: 'xylona-error',
          position: 'top',
          timeout: 3000,
        })
        return
      }
      resetArchiveStats()
      showDialog.value = false
      emit('submit')
      $q.notify({
        caption: `Files archived successfully.`,
        type: 'xylona-success',
        position: 'top',
        timeout: 3000,
      })
    },
    { signal: abortController.value.signal },
  )
}

function resetArchiveProgress() {
  submitProgress.value = 0
  submitPercent.value = 0
  archiveCurrentBytes.value = 0
  archiveTotalBytes.value = 0
  currentArchiveFile.value = ''
  filesArchived.value = 0
  totalFiles.value = 0
}

function resetArchiveStats() {
  archiveType.value = DEFAULT_ARCHIVE_TYPE
  archiveSuffix.value = DEFAULT_ARCHIVE_SUFFIX
  archiveName.value = ''
  resetArchiveProgress()
}

function abortArchive() {
  if (abortController.value) {
    abortedArchive.value = true
    abortController.value.abort()
  }
}

function getRelativeFilePath(...filePaths: string[]): string {
  if (props.path === '') {
    return filePaths.join(props.pathSeparator)
  }
  return props.path + props.pathSeparator + filePaths.join(props.pathSeparator)
}
</script>

<style scoped>
.file-progress-name {
  overflow-wrap: anywhere;
}
</style>
