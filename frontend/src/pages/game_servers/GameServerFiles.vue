<template>
    <q-card-section>
        <FileUploaderDrop :game-server-id="gameServerId" :path-separator="pathSeparator" :path="path"
                          :upload-u-r-l="uploadURL" :target-element="fileListContainer"
                          @uploaded-files="listDirectoryFiles(path)"
                          v-model:file-uploader-dialog="fileUploaderDialog">
            <div ref="fileListContainer" class="col-xs-12 file-list-container bg-neutral-glass-4 q-pa-sm"
                 style="border-radius: .5rem">
                <div class="row q-py-sm justify-end q-gutter-x-md">
                    <q-btn v-show="deleteButtonEnabled" @click="deleteFilesDialog = true" class="bg-error">Delete
                    </q-btn>
                    <q-btn v-show="downloadButtonEnabled" class="bg-blue">Download</q-btn>
                    <q-btn v-show="zipButtonEnabled" @click="archiveFilesDialog = true" class="bg-teal">
                        Archive/Compress
                    </q-btn>
                    <q-btn v-show="extractButtonEnabled" @click="extractFilesDialog = true" class="bg-green">Extract
                    </q-btn>
                    <q-btn class="bg-alert" label="Upload from URL" @click="fileUploaderDialog = true"></q-btn>
                    <q-btn class="bg-success" label="Upload" @click="fileUploaderDialog = true"></q-btn>
                </div>
                <div class="row q-py-sm">
                    <div class="col-xs-12">
                        <q-input @keydown.prevent.enter="updatePathFromInput"
                                 :prefix="gameServer.directory + pathSeparator"
                                 v-model="path" outlined dense></q-input>
                    </div>
                </div>
                <div class="row file-list-header q-px-sm">
                    <div class="col-xs-2 col-md-2 col-lg-1">
                        <q-checkbox v-model="selectAllFiles" label="All"></q-checkbox>
                    </div>
                    <div class="col-xs-4">Name</div>
                    <div class="col-xs-3">Size</div>
                    <div class="col-xs-3">Modified</div>
                </div>
                <q-separator class="q-my-sm"></q-separator>
                <div id="file-list" ref="filesList">
                    <div class="row file-list-body-row q-px-sm" :class="fileIsSelectedClass(directory)"
                         v-for="directory in directories">
                        <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
                            <q-checkbox v-if="directory.name !== '..'" :val="directory"
                                        v-model="selectedFiles"></q-checkbox>
                        </div>
                        <div class="col-xs-4 file-div file-list-cell" @click="clickDirectory(directory)">
                            <q-icon size="xs" color="amber" :name="tabFolderFilled" left></q-icon>
                            {{ directory.name }}
                        </div>
                        <div class="col-xs-3 file-list-cell">{{ bytesToSize(Number(directory.size)) }}</div>
                        <div class="col-xs-3 file-list-cell">{{ toTimestamp(directory.lastModified) }}</div>
                    </div>
                    <div class="row file-list-body-row q-px-sm" :class="fileIsSelectedClass(file)" v-for="file in files"
                         :data-file-name="file.name"
                         draggable="false">
                        <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
                            <q-checkbox :val="file" v-model="selectedFiles"></q-checkbox>
                        </div>
                        <div class="col-xs-4 file-div file-list-cell" @click="clickFile(file)">
                            <q-icon size="xs" :style="'color:'+ getColorFromFilenameExtension(file.name)"
                                    :name="getIconFromFilenameExtension(file.name)" left></q-icon>
                            <span class="file-name">{{ file.name }}</span>
                        </div>
                        <div class="col-xs-3 file-list-cell">{{ bytesToSize(Number(file.size)) }}</div>
                        <div class="col-xs-3 file-list-cell">{{ toTimestamp(file.lastModified) }}</div>
                    </div>
                </div>
                <q-menu ref="contextMenu" touch-position context-menu>
                    <q-list>
                        <q-item clickable v-ripple @click="selectAllFiles = true">
                            <q-item-section> Select All</q-item-section>
                        </q-item>
                        <q-item clickable v-ripple @click="selectAllFiles = false">
                            <q-item-section> Deselect All</q-item-section>
                        </q-item>
                    </q-list>
                </q-menu>
            </div>
        </FileUploaderDrop>
    </q-card-section>
    <q-dialog no-shake persistent v-model="editorModal" backdrop-filter="blur(6px) brightness(15%)">
        <Editor v-model:code-input="editingFileContent" :file-name="editingFilename" :game-server-id="gameServerId"
                :full-file-path="editingFilePath"></Editor>
    </q-dialog>
    <ArchiveFiles @submit="archiveFilesDialogSubmitted" @cancel="archiveFilesDialog = false"
                  v-model:show-dialog="archiveFilesDialog" v-model:archive-name="archiveName"
                  :path-separator="pathSeparator" :path="path" :selected-files="selectedFiles"
                  :game-server-id="gameServerId">
    </ArchiveFiles>
    <ExtractFiles @submit="extractFilesDialogSubmitted" @cancel="extractFilesDialog = false"
                  :game-server-path="gameServer.directory"
                  v-model:show-dialog="extractFilesDialog" :game-server-id="gameServerId" :path="path"
                  :full-archive-path="GetRelativeFilePath(gameServer.directory, path, selectedFiles[0]?.name)">
    </ExtractFiles>
    <DeleteGameServerFilesDialog @files-deleted="deleteFilesSubmitted()" :files-to-delete="selectedFiles"
                                 :current-path="path" :path-separator="pathSeparator"
                                 :game-server-i-d="gameServerId" v-model:show-dialog="deleteFilesDialog">
    </DeleteGameServerFilesDialog>
</template>

<script setup lang="ts">
import { Timestamp } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import Editor from 'components/Editor.vue'
import ArchiveFiles from 'components/game_servers/ArchiveFiles.vue'
import ArchiveFilesDialog from 'components/game_servers/ArchiveFilesDialog.vue'
import DeleteGameServerFilesDialog from 'components/game_servers/DeleteGameServerFilesDialog.vue'
import ExtractFiles from 'components/game_servers/ExtractFiles.vue'
import ExtractGameServerFiles from 'components/game_servers/ExtractGameServerFiles.vue'
import FileUploaderDrop from 'components/game_servers/FileUploaderDrop.vue'
import dayjs from 'dayjs'
import { QMenu, useQuasar } from 'quasar'
import { tabFolderFilled } from 'quasar-extras-svg-icons/tabler-icons-v2'
import {
    GameServerFilesCompressionType
} from 'src/proto/gameserver_files_operations_pb'
import {
    DownloadFileRequest,
    File as xylonaFile,
    GameServer,
    GetGameServerRequest,
    ListDirectoryFilesRequest,
    ListDirectoryFilesResponse
} from 'src/proto/xylona_pb'
import {
    bytesToSize,
    getColorFromFilenameExtension,
    getIconFromFilenameExtension, GetRelativeFilePath,
    GetXylonaClient
} from 'src/utils/shared'
import { computed, onMounted, ref, Ref, watch } from 'vue'
import { useRoute } from 'vue-router'

const $q = useQuasar()
const uploadURL: Ref<string> = ref('/api/file/upload')

const route = useRoute()
const gameServerId: Ref<string> = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
const gameServer: Ref<GameServer> = ref(new GameServer()) as Ref<GameServer>
const files: Ref<Array<xylonaFile>> = ref([])
const directories: Ref<Array<xylonaFile>> = ref([])
const selectedFiles: Ref<Array<xylonaFile>> = ref([])
const selectAllFiles: Ref<boolean> = ref(false)
const path: Ref<string> = ref('')
const editorModal: Ref<boolean> = ref(false)
const editingFilename: Ref<string> = ref('')
const editingFilePath: Ref<string> = ref('')
const editingFileContent: Ref<string> = ref('')
const contextMenu: Ref<QMenu | null> = ref(null)
const fileListContainer: Ref<HTMLElement | null> = ref(null)
const fileUploaderDialog: Ref<boolean> = ref(false)

// Archive
const archiveFilesDialog: Ref<boolean> = ref(false)
const archiveName: Ref<string> = ref('')

// Extract
const extractFilesDialog: Ref<boolean> = ref(false)

// Delete
const deleteFilesDialog: Ref<boolean> = ref(false)

const allowedExtractExtensions: string[] = ['.zip', '.zst', '.gz', '.bz2', '.xz', '.7z']
const allowedFileEditExtensions: string[] = ['.txt', '.cfg', '.json', '.xml', '.yml', '.yaml', '.ini', '.log']

async function archiveFilesDialogSubmitted() {
    selectedFiles.value = []
    void listDirectoryFiles(path.value)
}

async function extractFilesDialogSubmitted() {
    selectedFiles.value = []
    void listDirectoryFiles(path.value)
}

async function deleteFilesSubmitted() {
    selectedFiles.value = []
    void listDirectoryFiles(path.value)
}

function fileIsSelectedClass(file: xylonaFile) {
    if (selectedFiles.value.includes(file)) {
        return 'bg-neutral-glass-4'
    }
    return ''
}

const deleteButtonEnabled = computed(() => {
    const selected = sanitizeSelectedFiles()
    return selected.length > 0
})

const downloadButtonEnabled = computed(() => {
    const selected = sanitizeSelectedFiles()
    if (selected.length <= 0) {
        return false
    }
    for (let i = 0; i < selected.length; i++) {
        if (selected[i].isDirectory) {
            return false
        }
    }
    return true
})

const zipButtonEnabled = computed(() => {
    const selected = sanitizeSelectedFiles()
    return selected.length > 0
})

const extractButtonEnabled = computed(() => {
    const selected = sanitizeSelectedFiles()
    if (selected.length != 1) {
        return false
    }

    let match = false
    for (let i = 0; i < selected.length; i++) {
        const selectedName = selected[i].name
        allowedExtractExtensions.forEach((extension) => {
            if (selectedName.endsWith(extension)) {
                match = true
                return match
            }
        })
    }
    return match
})

function sanitizeSelectedFiles(): xylonaFile[] {
    const sanitizedFiles: xylonaFile[] = []
    selectedFiles.value.forEach((file) => {
        if (file.name === '..') {
            return
        }
        sanitizedFiles.push(file)
    })
    return sanitizedFiles
}

onMounted(async () => {
    const hashedPath = window.location.hash
    if (hashedPath.length > 0) {
        let deHashed = hashedPath.substring(1)
        path.value = deHashed.replaceAll('/', pathSeparator.value)
    }
    void listDirectoryFiles(path.value)
    void getGameServerDetails()
    const draggableElements = document.querySelectorAll('div[draggable="true"]')
    for (let i = 0; i < draggableElements.length; i++) {
        attachDragStartEvent(draggableElements[i] as HTMLElement)
    }

})

function attachDragStartEvent(draggableElement: HTMLElement) {
    draggableElement.addEventListener('dragstart', (event: DragEvent) => {
        const fileName: string = (event.target as HTMLElement).dataset.fileName as string
        event.dataTransfer?.setData('filename', fileName)
    })
}

watch(selectAllFiles, (newValue) => {
    const sanitizedDirectories = directories.value.filter((directory) => {
        return directory.name !== '..'
    })
    if (newValue) {
        selectedFiles.value = sanitizedDirectories.concat(files.value)
        return
    }
    if (selectedFiles.value.length === sanitizedDirectories.length + files.value.length) {
        selectedFiles.value = []
    }
})

watch(selectedFiles, (newValue) => {
    if (selectAllFiles) {
        const sanitizedDirectories = directories.value.filter((directory) => {
            return directory.name !== '..'
        })
        selectAllFiles.value = newValue.length === sanitizedDirectories.length + files.value.length
    }
})

const pathSeparator = computed(() => {
    if (gameServer.value.directory.indexOf('\\') !== -1) {
        return '\\'
    }
    return '/'
})

async function clickDirectory(directory: xylonaFile) {
    if (directory.name === '..') {
        let pathSplit = path.value.lastIndexOf('/') !== -1 ? path.value.split('/') : path.value.split('\\')
        pathSplit.pop()
        path.value = pathSplit.join(pathSeparator.value)
    } else {
        path.value = path.value + pathSeparator.value + directory.name
    }
    if (path.value.length >= 1) {
        if (path.value[0] === pathSeparator.value) {
            path.value = path.value.substring(1)
        }
    }
    window.location.hash = path.value
    await listDirectoryFiles(path.value)
}

async function clickFile(file: xylonaFile) {
    // If file is an allowed file type for editing, open the editor.
    const fileExtension = file.name.substring(file.name.lastIndexOf('.'))
    if (allowedFileEditExtensions.includes(fileExtension)) {
        await readFileOctetStream(file.name)
        return
    }
    // If file is not an allowed file type for editing, download the file.
    await downloadGameServerFile(file.name)
}

function updatePathFromInput() {
    if (path.value.length >= 1) {
        if (path.value[0] === pathSeparator.value) {
            path.value = path.value.substring(1)
        }
    }
    listDirectoryFiles(path.value)
}

async function listDirectoryFiles(directoryPath: string) {
    const request = new ListDirectoryFilesRequest()
    try {
        request.gameServerId = gameServerId.value
        request.path = directoryPath
        const response: ListDirectoryFilesResponse = await GetXylonaClient().listDirectoryFiles(request)
        directories.value = []
        files.value = []
        const upDirectory = new xylonaFile()
        upDirectory.name = '..'
        upDirectory.size = BigInt(0)
        upDirectory.isDirectory = true
        upDirectory.lastModified = new Timestamp()
        directories.value.push(upDirectory)
        response.files.forEach((file) => {
            if (file.isDirectory) {
                directories.value.push(file)
            } else {
                files.value.push(file)
            }
        })
    } catch (err) {
        if (err instanceof ConnectError) {
            if (err.code === Code.NotFound) {
                path.value = ''
                setTimeout(() => {
                    window.location.hash = ''
                    listDirectoryFiles(path.value)
                }, 100)
                alert('Directory does not exist.')
                return
            }
            console.error(`Error listing directory files: ${err.code} ${err.message}`)
            return
        }
        console.error(err)
    } finally {
    }
}

async function readFileOctetStream(fileName: string) {
    $q.loading.show({
        message: 'Reading file...',
        delay: 100
    })
    const fullFilePath = GetRelativeFilePath(gameServer.value.directory, path.value, fileName)
    const fileRequest = new DownloadFileRequest()
    fileRequest.gameServerId = gameServerId.value
    fileRequest.path = fullFilePath
    try {
        const response = await fetch('/api/file/get', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: fileRequest.toJsonString()
        })
        const data = await response.text()
        editingFilename.value = fileName
        editingFileContent.value = data
        editingFilePath.value = fullFilePath
        editorModal.value = true
    } catch (e) {
        console.error(e)
        $q.notify({
            caption: `Error reading file ${fileName}.`,
            type: 'xylona-error',
            position: 'top',
            timeout: 5000
        })
    } finally {
        $q.loading.hide()
    }
}

async function downloadGameServerFile(fileName: string) {
    const fullFilePath = GetRelativeFilePath(gameServer.value.directory, path.value, fileName)
    const rawURL = `${window.location.protocol}//${window.location.host}/api/file/download/${gameServerId.value}/${fullFilePath}`
    const url = encodeURI(rawURL)
    // If the URL will fit in a GET request, without hitting browser URL length limits, use GET.
    if (url.length < 2000) {
        const a = document.createElement('a')
        a.href = url
        a.download = fileName
        a.target = '_blank'
        a.style.display = 'none'
        document.body.appendChild(a)
        a.click()
        document.body.removeChild(a)
        return
    }
    // If the URL is too long for a GET request, use POST.
    try {
        const downloadForm = document.createElement('form')
        downloadForm.method = 'POST'
        downloadForm.action = '/api/file/download'
        downloadForm.style.display = 'none'

        const gameServerIDInput = document.createElement('input')
        gameServerIDInput.name = 'gameServerId'
        gameServerIDInput.value = gameServerId.value
        downloadForm.appendChild(gameServerIDInput)

        const filePathInput = document.createElement('input')
        filePathInput.name = 'path'
        filePathInput.value = fullFilePath
        downloadForm.appendChild(filePathInput)

        downloadForm.target = '_blank'
        document.body.appendChild(downloadForm)
        downloadForm.submit()

        document.body.removeChild(downloadForm)
    } catch (e) {
        console.error(e)
        $q.notify({
            caption: `Error reading file ${fileName}.`,
            type: 'xylona-error',
            position: 'top',
            timeout: 5000
        })
    }
}

function toTimestamp(date: Timestamp | undefined) {
    if (date === undefined) {
        return ''
    }
    return dayjs(date.toDate()).format('MM/DD/YYYY HH:mm:ss A')
}

async function getGameServerDetails() {
    const request = new GetGameServerRequest()
    try {
        request.id = gameServerId.value
        const response = await GetXylonaClient().getGameServer(request)
        if (response.gameServer === undefined) {
            return
        }
        gameServer.value = response.gameServer
    } catch (e) {
        console.error(e)
    }
}

// function getRelativeFilePath(...filePaths: string[]): string {
//     if (path.value === '') {
//         return filePaths.join(pathSeparator.value)
//     }
//     return path.value + pathSeparator.value + filePaths.join(pathSeparator.value)
// }

</script>

<style scoped>
.file-list-header {
    font-weight: bold;
    align-items: center;
    min-width: 720px;
    font-size: 1rem;
}

.file-list-body-row {
    align-items: center;
    height: 2rem;
    font-size: 1rem;
}

.file-list-cell {
    height: 100%;
    display: flex;
    align-items: center;
}

.file-list-body-row:hover {
    background-color: var(--bg-dark-grey);
}

.file-list-container {
    font-family: "Oxygen Mono", monospace;
    border: 2px solid var(--bg-neutral-opaque);
}

#file-list {
    /* 1080p and below */
    @media (height <= 1080px) {
        height: 60dvh;
    }
    /* 1440p */
    @media (1080px < height <= 1440px) {
        height: 65dvh;
    }
    /* 4K and above */
    @media (height > 1440px) {
        height: 75dvh;
    }
    overflow: scroll;
}

.file-div:hover {
    cursor: pointer;
    font-weight: 700;
}
</style>
