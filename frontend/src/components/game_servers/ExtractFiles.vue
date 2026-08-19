<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="full-width">
      <q-form @submit.prevent="extractFiles">
        <q-card-section>
          <div id="dialog-title" class="text-h6">{{ extractTitle }}</div>
        </q-card-section>
        <q-card-section v-if="!extractSubmitting">
          <div class="q-pa-lg">
            <div class="row wrap q-col-gutter-md justify-between">
              <q-input
                v-model="fullDestinationPath"
                :autofocus="true"
                class="col-12"
                hint="Leave this blank to extract to the current directory."
                label="Folder to extract files to"
                outlined />
            </div>
          </div>
        </q-card-section>
        <q-card-section v-else>
          <div aria-live="polite" role="status">
            <div class="text-caption">
              {{ bytesToSize(extractCurrentBytes) }} out of
              {{ bytesToSize(extractTotalBytes) }} extracted ({{ submitPercent }}%)
            </div>
            <div class="text-caption">{{ filesExtracted }} / {{ totalFiles }} files extracted</div>
            <div class="file-progress-name text-caption font-mono">{{ currentExtractFile }}</div>
            <q-linear-progress
              aria-label="Extraction progress"
              :value="submitProgress"
              color="primary"
              size="lg"
              stripe />
          </div>
        </q-card-section>
        <q-card-actions v-if="!extractSubmitting" align="right">
          <q-btn color="primary" flat label="Cancel" @click="showDialog = false" />
          <q-btn color="primary" label="Extract" type="submit" />
        </q-card-actions>
        <q-card-actions v-else align="right">
          <q-btn color="negative" label="Stop extracting" @click="abortExtract" />
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
  GameServerFilesDecompressionRequest,
  GameServerFilesDecompressionRequestSchema,
} from '@/proto/gameserver_files_operations_pb'
import { bytesToSize, GetRelativeFilePath, GetXylonaClientCallback } from '@/utils/shared'
import { ref } from 'vue'
import { GameServerFilesExtractProgress } from '@/proto/shared_pb'

const props = defineProps({
  gameServerId: {
    type: String,
    required: true,
  },
  gameServerPath: {
    type: String,
    required: true,
  },
  path: {
    type: String,
    required: true,
  },
  fullArchivePath: {
    type: String,
    required: true,
  },
})

const extractSubmitting = ref(false)
const submitProgress = ref(0)
const submitPercent = ref(0)
const extractCurrentBytes = ref(0)
const extractTotalBytes = ref(0)
const currentExtractFile = ref('')
const extractTitle = ref('Extract Files')
const filesExtracted = ref(0)
const totalFiles = ref(0)
const abortController = ref<AbortController | null>(null)
const abortedExtract = ref(false)
const fullDestinationPath = ref('')

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

async function extractFiles() {
  if (extractSubmitting.value) {
    return
  }
  extractSubmitting.value = true
  abortedExtract.value = false

  const request: GameServerFilesDecompressionRequest = create(
    GameServerFilesDecompressionRequestSchema,
    {},
  )
  request.gameServerId = props.gameServerId
  request.fullFilePath = props.fullArchivePath
  request.destinationBasePath = GetRelativeFilePath(
    props.gameServerPath,
    props.path,
    fullDestinationPath.value,
  )

  abortController.value = new AbortController()
  GetXylonaClientCallback().gameServerFilesExtract(
    request,
    (response) => {
      const resp: GameServerFilesExtractProgress = response as GameServerFilesExtractProgress
      currentExtractFile.value =
        resp.filesExtracted === resp.totalFiles
          ? 'All files extracted!'
          : `Current file: ${resp.currentFile}`
      extractCurrentBytes.value =
        resp.filesExtracted === resp.totalFiles
          ? Number(resp.totalBytes)
          : Number(resp.bytesExtracted)
      extractTotalBytes.value = Number(resp.totalBytes)
      filesExtracted.value = Number(resp.filesExtracted)
      totalFiles.value = Number(resp.totalFiles)
      submitProgress.value =
        resp.filesExtracted === resp.totalFiles
          ? 1
          : extractCurrentBytes.value / extractTotalBytes.value
      submitPercent.value = Math.round(submitProgress.value * 100)
    },
    (err?: ConnectError) => {
      extractSubmitting.value = false
      if (abortedExtract.value) {
        resetExtractStats()
        showDialog.value = false
        emit('submit')
        $q.notify({
          caption: `Files extraction aborted.`,
          type: 'xylona-alert',
          position: 'top',
          timeout: 3000,
        })
        return
      }
      if (err) {
        resetExtractProgress()
        console.error(err)
        $q.notify({
          caption: `Error extracting files. ${err.message}`,
          type: 'xylona-error',
          position: 'top',
          timeout: 5000,
        })
        return
      }
      $q.notify({
        caption: `Extracted ${totalFiles.value} files successfully.`,
        type: 'xylona-success',
        position: 'top',
        timeout: 3000,
      })
      resetExtractStats()
      showDialog.value = false
      emit('submit')
    },
    { signal: abortController.value.signal },
  )
}

function resetExtractProgress() {
  submitProgress.value = 0
  submitPercent.value = 0
  extractCurrentBytes.value = 0
  extractTotalBytes.value = 0
  currentExtractFile.value = ''
  filesExtracted.value = 0
  totalFiles.value = 0
}

function resetExtractStats() {
  fullDestinationPath.value = ''
  resetExtractProgress()
}

function abortExtract() {
  if (abortController.value) {
    abortedExtract.value = true
    abortController.value.abort()
  }
}
</script>

<style scoped>
.file-progress-name {
  overflow-wrap: anywhere;
}
</style>
