<template>
    <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(25%)">
        <q-card class="full-width">
            <q-card-section>
                <q-card-title>
                    <div class="text-h6">{{ archiveTitle }}</div>
                </q-card-title>
            </q-card-section>
            <q-card-section v-if="!archiveSubmitting">
                <q-form class="q-pa-lg">
                    <div class="row wrap q-col-gutter-md justify-between">
                        <q-select class="col-12" outlined v-model="archiveType" emit-value map-options
                                  @update:model-value="archiveSuffix = ArchiveTypeToExtension(archiveType)"
                                  :options="archiveTypeOptions" label="Archive Type">
                            <template v-slot:prepend>
                                <q-icon name="event"/>
                            </template>
                        </q-select>
                        <q-input placeholder="example-archive" name="archive-name" aria-autocomplete="none" class="col-12"
                                 :suffix="archiveSuffix" outlined v-model="archiveName" label="Archive name" autofocus/>
                    </div>
                </q-form>
            </q-card-section>
            <q-card-section v-else>
                <q-card-section>
                    <div>
                        <div class="text-caption">{{ bytesToSize(archiveCurrentBytes) }} out of
                            {{ bytesToSize(archiveTotalBytes) }} archived ( {{ submitPercent }}% )
                        </div>
                        <div class="text-caption"></div>
                        <div class="text-caption">{{ filesArchived }} / {{ totalFiles }} files archived</div>
                        <div class="text-caption">{{ currentArchiveFile }}</div>
                        <q-linear-progress :value="submitProgress" color="primary" stripe size="lg"/>
                    </div>
                </q-card-section>
            </q-card-section>
            <q-card-actions v-if="!archiveSubmitting" align="right">
                <q-btn label="Cancel" color="primary" @click="showDialog = false" flat/>
                <q-btn label="Archive" color="primary" @click="archiveFiles()"/>
            </q-card-actions>
            <q-card-actions v-else align="right">
                <q-btn label="Cancel" color="negative" @click="abortArchive"/>
            </q-card-actions>
        </q-card>
    </q-dialog>
</template>

<script setup lang="ts">
import { ConnectError } from '@connectrpc/connect'
import { QBtn, QCard, QCardSection, QDialog, QInput, useQuasar } from 'quasar'
import {
    GameServerFilesCompressionRequest,
    GameServerFilesCompressionType
} from 'src/proto/gameserver_files_operations_pb'
import { File as xylonaFile } from 'src/proto/xylona_pb'
import {
    ArchiveTypeToExtension,
    ArchiveTypeToString,
    bytesToSize,
    GetXylonaClient,
    GetXylonaClientCallback
} from 'src/utils/shared'
import { ref, Ref } from 'vue'

const props = defineProps({
    gameServerId: {
        type: String,
        required: true
    },
    path: {
        type: String,
        required: true
    },
    selectedFiles: {
        type: Array<xylonaFile>,
        default: []
    },
    pathSeparator: {
        type: String,
        default: '/'
    }
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
    {label: ArchiveTypeToString(GameServerFilesCompressionType.ZIP), value: GameServerFilesCompressionType.ZIP},
    {
        label: ArchiveTypeToString(GameServerFilesCompressionType.GZIP),
        value: GameServerFilesCompressionType.GZIP
    },
    {
        label: ArchiveTypeToString(GameServerFilesCompressionType.BZIP2),
        value: GameServerFilesCompressionType.BZIP2
    },
    {label: ArchiveTypeToString(GameServerFilesCompressionType.ZST), value: GameServerFilesCompressionType.ZST},
    {label: ArchiveTypeToString(GameServerFilesCompressionType.XZ), value: GameServerFilesCompressionType.XZ}
])

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
    type: Boolean,
    default: false
})
const archiveName = defineModel('archiveName', {
    type: String,
    default: '',
    required: true
})

const emit = defineEmits(['submit'])

async function archiveFiles() {
    archiveSubmitting.value = true
    abortedArchive.value = false
    console.log(props.selectedFiles)
    const request = new GameServerFilesCompressionRequest()
    request.gameServerId = props.gameServerId
    request.fileName = archiveName.value
    request.compressionType = archiveType.value
    request.sourcePath = props.path
    request.filePaths = props.selectedFiles.map((file: xylonaFile) => {
        return getRelativeFilePath(file.name)
    })

    abortController.value = new AbortController()
    GetXylonaClientCallback().gameServerFilesArchive(request, (response) => {
        currentArchiveFile.value = response.filesCompressed === response.totalFiles ? 'All files archived!' : `Current file: ${response.currentFile}`
        archiveCurrentBytes.value = response.filesCompressed === response.totalFiles ? Number(response.totalBytes) : Number(response.bytesCompressed)
        archiveTotalBytes.value = Number(response.totalBytes)
        filesArchived.value = Number(response.filesCompressed)
        totalFiles.value = Number(response.totalFiles)
        submitProgress.value = response.filesCompressed === response.totalFiles ? 1 : (archiveCurrentBytes.value / archiveTotalBytes.value)
        submitPercent.value = Math.round(submitProgress.value * 100)
    }, (err?: ConnectError) => {
        setTimeout(() => {
            archiveSubmitting.value = false
            showDialog.value = false
            archiveType.value = DEFAULT_ARCHIVE_TYPE
            archiveSuffix.value = DEFAULT_ARCHIVE_SUFFIX
            archiveName.value = ''
            emit('submit')
        }, 100)
        if (err) {
            console.error(err)
            $q.notify({
                caption: `Error archiving files. ${err.message}`,
                type: 'xylona-error',
                position: 'top-right',
                timeout: 3000
            })
            return
        }
        if (abortedArchive.value) {
            $q.notify({
                caption: `Files archiving aborted.`,
                type: 'xylona-alert',
                position: 'top-right',
                timeout: 3000
            })
            return
        }
        $q.notify({
            caption: `Files archived successfully.`,
            type: 'xylona-success',
            position: 'top-right',
            timeout: 3000
        })
    }, {signal: abortController.value.signal})
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
    const a = props.path + props.pathSeparator + filePaths.join(props.pathSeparator)
    console.log(a)
    return a
}


</script>

<style scoped>

</style>
