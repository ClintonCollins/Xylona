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
                                  :options="props.archiveTypeOptions"
                                  label="Archive Type">
                            <template v-slot:prepend>
                                <q-icon name="event"/>
                            </template>
                        </q-select>
                        <q-input class="col-12" outlined v-model="archiveName" label="Name" :autofocus="true"/>
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
        </q-card>
    </q-dialog>
</template>

<script setup lang="ts">
import { ConnectError } from '@connectrpc/connect'
import { QBtn, QCard, QCardSection, QDialog, QInput } from 'quasar'
import {
    GameServerFilesCompressionRequest,
    GameServerFilesCompressionType
} from 'src/proto/gameserver_files_operations_pb'
import { File as xylonaFile } from 'src/proto/xylona_pb'
import { ArchiveTypeToString, bytesToSize, GetXylonaClient, GetXylonaClientCallback } from 'src/utils/shared'
import { ref, Ref } from 'vue'

const props = defineProps({
    archiveTypeOptions: {
        type: Array<string>,
        default: [
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
        ]
    },
    gameServerId: {
        type: String,
        required: true
    },
    loading: {
        type: Boolean,
        default: false
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

const archiveSubmitting = ref(false)
const submitProgress = ref(0)
const submitPercent = ref(0)
const archiveCurrentBytes = ref(0)
const archiveTotalBytes = ref(0)
const currentArchiveFile = ref('')
const archiveTitle = ref('Archive Files')
const filesArchived = ref(0)
const totalFiles = ref(0)

const showDialog = defineModel('showDialog', {
    type: Boolean,
    default: false
})
const archiveName = defineModel('archiveName', {
    type: String,
    default: '',
    required: true
})
const archiveType = defineModel('archiveType', {
    type: Number,
    default: GameServerFilesCompressionType.ZIP,
    required: true
})

const emit = defineEmits(['submit'])

async function archiveFiles() {
    archiveSubmitting.value = true
    console.log(props.selectedFiles)
    const request = new GameServerFilesCompressionRequest()
    request.gameServerId = props.gameServerId
    request.fileName = archiveName.value
    request.compressionType = archiveType.value
    request.sourcePath = props.path
    request.filePaths = props.selectedFiles.map((file: xylonaFile) => {
        return getRelativeFilePath(file.name)
    })

    GetXylonaClientCallback().gameServerFilesArchive(request, (response) => {
        currentArchiveFile.value = response.filesCompressed === response.totalFiles ? 'All files archived!' : `Current file: ${response.currentFile}`
        archiveCurrentBytes.value = response.filesCompressed === response.totalFiles ? Number(response.totalBytes) : Number(response.bytesCompressed)
        archiveTotalBytes.value = Number(response.totalBytes)
        filesArchived.value = Number(response.filesCompressed)
        totalFiles.value = Number(response.totalFiles)
        submitProgress.value = response.filesCompressed === response.totalFiles ? 1 : (archiveCurrentBytes.value / archiveTotalBytes.value)
        submitPercent.value = Math.round(submitProgress.value * 100)
    }, (err?: ConnectError) => {
        if (err) {
            console.log(err.code, err.message)
            console.error(err)
            alert(err)
        }
        setTimeout(() => {
            archiveSubmitting.value = false
            showDialog.value = false
            emit('submit')
        }, 1500)
    })
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
