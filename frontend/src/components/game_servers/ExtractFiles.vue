<template>
    <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(25%)">
        <q-card class="full-width">
            <q-card-section>
                <q-card-title>
                    <div class="text-h6">{{ extractTitle }}</div>
                </q-card-title>
            </q-card-section>
            <q-card-section v-if="!extractSubmitting">
                <q-form class="q-pa-lg">
                    <div class="row wrap q-col-gutter-md justify-between">
                        <q-input class="col-12" outlined v-model="fullDestinationPath" hint="Leave this blank to extract to the current directory." label="Folder to extract files to" :autofocus="true"/>
                    </div>
                </q-form>
            </q-card-section>
            <q-card-section v-else>
                <q-card-section>
                    <div>
                        <div class="text-caption">{{ bytesToSize(extractCurrentBytes) }} out of
                            {{ bytesToSize(extractTotalBytes) }} extracted ( {{ submitPercent }}% )
                        </div>
                        <div class="text-caption"></div>
                        <div class="text-caption">{{ filesExtracted }} / {{ totalFiles }} files extracted</div>
                        <div class="text-caption">{{ currentExtractFile }}</div>
                        <q-linear-progress :value="submitProgress" color="primary" stripe size="lg"/>
                    </div>
                </q-card-section>
            </q-card-section>
            <q-card-actions v-if="!extractSubmitting" align="right">
                <q-btn label="Cancel" color="primary" @click="showDialog = false" flat/>
                <q-btn label="Extract" color="primary" @click="extractFiles()"/>
            </q-card-actions>
            <q-card-actions v-else align="right">
                <q-btn label="Cancel" color="negative" @click="abortExtract"/>
            </q-card-actions>
        </q-card>
    </q-dialog>
</template>

<script setup lang="ts">
import { ConnectError } from '@connectrpc/connect'
import { QBtn, QCard, QCardSection, QDialog, QInput, useQuasar } from 'quasar'
import { GameServerFilesDecompressionRequest } from 'src/proto/gameserver_files_operations_pb'
import { GameServerFilesExtractProgress } from 'src/proto/xylona_pb'
import {
    bytesToSize, GetRelativeFilePath,
    GetXylonaClientCallback
} from 'src/utils/shared'
import { ref } from 'vue'

const props = defineProps({
    gameServerId: {
        type: String,
        required: true
    },
    gameServerPath: {
        type: String,
        required: true
    },
    path: {
        type: String,
        required: true
    },
    fullArchivePath: {
        type: String,
        required: true
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
    default: false
})

const emit = defineEmits(['submit'])

async function extractFiles() {
    extractSubmitting.value = true
    abortedExtract.value = false

    const request = new GameServerFilesDecompressionRequest()
    request.gameServerId = props.gameServerId
    request.fullFilePath = props.fullArchivePath
    request.destinationBasePath = GetRelativeFilePath(props.gameServerPath, props.path, fullDestinationPath.value)


    abortController.value = new AbortController()
    GetXylonaClientCallback().gameServerFilesExtract(request, (response) => {
        const resp: GameServerFilesExtractProgress = response as GameServerFilesExtractProgress
        currentExtractFile.value = resp.filesCompressed === resp.totalFiles ? 'All files Extracted!' : `Current file: ${resp.currentFile}`
        extractCurrentBytes.value = resp.filesCompressed === resp.totalFiles ? Number(resp.totalBytes) : Number(resp.bytesExtracted)
        extractTotalBytes.value = Number(resp.totalBytes)
        filesExtracted.value = Number(resp.filesExtracted)
        totalFiles.value = Number(resp.totalFiles)
        submitProgress.value = resp.filesCompressed === resp.totalFiles ? 1 : (extractCurrentBytes.value / extractTotalBytes.value)
        submitPercent.value = Math.round(submitProgress.value * 100)
    }, (err?: ConnectError) => {
        setTimeout(() => {
            resetExtractStats()
            extractSubmitting.value = false
            showDialog.value = false
            emit('submit')
        }, 100)
        if (err) {
            console.error(err)
            $q.notify({
                caption: `Error extracting files. ${err.message}`,
                type: 'xylona-error',
                position: 'top',
                timeout: 5000
            })
            return
        }
        if (abortedExtract.value) {
            $q.notify({
                caption: `Files extraction aborted.`,
                type: 'xylona-alert',
                position: 'top',
                timeout: 3000
            })
            return
        }
        $q.notify({
            caption: `Extracted ${totalFiles.value} files successfully.`,
            type: 'xylona-success',
            position: 'top',
            timeout: 3000
        })
    }, {signal: abortController.value.signal})
}

function resetExtractStats() {
    fullDestinationPath.value = ''
    submitProgress.value = 0
    submitPercent.value = 0
    extractCurrentBytes.value = 0
    extractTotalBytes.value = 0
    currentExtractFile.value = ''
    filesExtracted.value = 0
    totalFiles.value = 0
}

function abortExtract() {
    if (abortController.value) {
        abortedExtract.value = true
        abortController.value.abort()
    }
}


</script>

<style scoped>

</style>
