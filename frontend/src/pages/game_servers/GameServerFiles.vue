<template>
  <q-card-section>
    <file-uploader-drop
      v-model:file-uploader-dialog="fileUploaderDialog"
      :game-server-id="gameServerId"
      :path-separator="pathSeparator"
      :path="path"
      :upload-u-r-l="uploadURL"
      :target-element="fileListContainer"
      @uploaded-files="listDirectoryFiles(path)">
      <div
        ref="fileListContainer"
        class="col-xs-12 file-list-container bg-xy-surface-2 q-pa-sm"
        style="border-radius: 0.5rem">
        <div class="file-toolbar">
          <div class="file-toolbar-primary">
            <q-btn
              :disable="!createButtonEnabled"
              color="primary"
              icon="add"
              label="Create"
              @click="createFilesDialog = true" />
            <q-btn
              color="positive"
              icon="upload"
              label="Upload"
              @click="fileUploaderDialog = true" />
            <q-btn
              v-if="downloadButtonEnabled"
              :disable="!downloadButtonEnabled"
              flat
              icon="download"
              label="Download" />
          </div>
          <div v-if="selectedFiles.length > 0" class="file-toolbar-selection">
            <span class="text-caption text-xy-secondary">
              {{ selectedFiles.length }} selected
            </span>
            <q-btn
              v-if="renameButtonEnabled"
              flat
              dense
              icon="edit"
              label="Rename"
              @click="renameFilesDialog = true" />
            <q-btn
              v-if="moveButtonEnabled"
              flat
              dense
              icon="drive_file_move"
              label="Move"
              @click="moveFilesDialog = true" />
            <q-btn
              v-if="zipButtonEnabled"
              flat
              dense
              icon="archive"
              label="Archive"
              @click="archiveFilesDialog = true" />
            <q-btn
              v-if="extractButtonEnabled"
              flat
              dense
              icon="unarchive"
              label="Extract"
              @click="extractFilesDialog = true" />
            <q-btn
              v-if="deleteButtonEnabled"
              flat
              dense
              color="negative"
              icon="delete"
              label="Delete"
              @click="deleteFilesDialog = true" />
          </div>
          <q-space />
          <q-btn
            flat
            dense
            icon="link"
            label="URL Upload"
            class="gt-xs"
            @click="fileUploaderDialog = true" />
        </div>
        <div class="row q-py-sm">
          <div class="col-xs-12">
            <q-input
              v-model="path"
              :prefix="gameServer.directory + pathSeparator"
              outlined
              dense
              aria-label="File path"
              @keydown.prevent.enter="updatePathFromInput"></q-input>
          </div>
        </div>
        <div class="row file-list-header q-px-sm">
          <div class="col-xs-2 col-md-2 col-lg-1">
            <q-checkbox v-model="selectAllFiles" label="All"></q-checkbox>
          </div>
          <div class="col-xs-5 col-sm-4">Name</div>
          <div class="col-xs-5 col-sm-3">Size</div>
          <div class="col-xs-3 gt-sm">Modified</div>
        </div>
        <q-separator class="q-my-sm"></q-separator>
        <div v-if="directories.length === 0 && files.length === 0" class="file-empty-state">
          <q-icon name="folder_open" size="3rem" class="text-xy-muted q-mb-sm" />
          <div class="text-subtitle1 text-xy-secondary">This directory is empty</div>
          <div class="text-caption text-xy-muted q-mt-xs">
            Upload files or create new ones using the toolbar above.
          </div>
        </div>
        <div id="file-list" ref="filesList">
          <div
            v-for="directory in directories"
            :key="directory.name"
            class="row file-list-body-row q-px-sm"
            :class="fileIsSelectedClass(directory)">
            <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
              <q-checkbox
                v-if="directory.name !== '..'"
                v-model="selectedFiles"
                :val="directory"></q-checkbox>
            </div>
            <div
              class="col-xs-5 col-sm-4 file-div file-list-cell"
              role="button"
              tabindex="0"
              @click="clickDirectory(directory)"
              @keydown.enter="clickDirectory(directory)"
              @keydown.space.prevent="clickDirectory(directory)">
              <q-icon size="xs" color="amber" :name="tabFolderFilled" left></q-icon>
              <span class="file-name">{{ directory.name }}</span>
            </div>
            <div class="col-xs-5 col-sm-3 file-list-cell">
              {{ bytesToSize(Number(directory.size)) }}
            </div>
            <div class="col-xs-3 file-list-cell gt-sm">
              {{ toTimestamp(directory.lastModified) }}
            </div>
          </div>
          <div
            v-for="file in files"
            :key="file.name"
            class="row file-list-body-row q-px-sm"
            :class="fileIsSelectedClass(file)"
            :data-file-name="file.name"
            draggable="false">
            <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
              <q-checkbox v-model="selectedFiles" :val="file"></q-checkbox>
            </div>
            <div
              class="col-xs-5 col-sm-4 file-div file-list-cell"
              role="button"
              tabindex="0"
              @click="clickFile(file)"
              @keydown.enter="clickFile(file)"
              @keydown.space.prevent="clickFile(file)">
              <q-icon
                size="xs"
                :style="'color:' + getColorFromFilenameExtension(file.name)"
                :name="getIconFromFilenameExtension(file.name)"
                left></q-icon>
              <span class="file-name">{{ file.name }}</span>
            </div>
            <div class="col-xs-5 col-sm-3 file-list-cell">{{ bytesToSize(Number(file.size)) }}</div>
            <div class="col-xs-3 file-list-cell gt-sm">{{ toTimestamp(file.lastModified) }}</div>
          </div>
        </div>
        <q-menu ref="contextMenu" touch-position context-menu>
          <q-list>
            <q-item v-ripple clickable @click="selectAllFiles = true">
              <q-item-section> Select All</q-item-section>
            </q-item>
            <q-item v-ripple clickable @click="selectAllFiles = false">
              <q-item-section> Deselect All</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </div>
    </file-uploader-drop>
  </q-card-section>
  <q-dialog v-model="editorModal" no-shake persistent backdrop-filter="blur(6px) brightness(15%)">
    <editor
      v-model:code-input="editorFileContent"
      :file-name="editorFilename"
      :game-server-id="gameServerId"
      :full-file-path="editorFilePath"
      @submit="refreshFileList"></editor>
  </q-dialog>
  <archive-files
    v-model:show-dialog="archiveFilesDialog"
    v-model:archive-name="archiveName"
    :path-separator="pathSeparator"
    :path="path"
    :selected-files="selectedFiles"
    :game-server-id="gameServerId"
    @submit="refreshFileList"
    @cancel="archiveFilesDialog = false">
  </archive-files>
  <extract-files
    v-model:show-dialog="extractFilesDialog"
    :game-server-path="gameServer.directory"
    :game-server-id="gameServerId"
    :path="path"
    :full-archive-path="GetRelativeFilePath(gameServer.directory, path, selectedFiles[0]?.name)"
    @submit="refreshFileList"
    @cancel="extractFilesDialog = false">
  </extract-files>
  <create
    v-model:show-dialog="createFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :path="path"
    @submit="createFilesDialogSubmitted">
  </create>
  <rename-file
    v-model:show-dialog="renameFilesDialog"
    :old-file-name="selectedFiles[0]?.name"
    :path="path"
    :game-server-path="gameServer.directory"
    :game-server-id="gameServerId"
    @submit="refreshFileList">
  </rename-file>
  <move-files
    v-model:show-dialog="moveFilesDialog"
    :path="path"
    :game-server-path="gameServer.directory"
    :game-server-id="gameServerId"
    :selected-files="selectedFiles"
    :neighboring-directories-in-path="directories.map((f) => f.name)"
    @submit="refreshFileList">
  </move-files>
  <delete-game-server-files-dialog
    v-model:show-dialog="deleteFilesDialog"
    :files-to-delete="selectedFiles"
    :current-path="path"
    :path-separator="pathSeparator"
    :game-server-i-d="gameServerId"
    @files-deleted="refreshFileList()">
  </delete-game-server-files-dialog>
</template>

<script setup lang="ts">
import { create, toJsonString } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { defineAsyncComponent } from 'vue'

const Editor = defineAsyncComponent(() => import('@/components/Editor.vue'))
import ArchiveFiles from '@/components/game_servers/ArchiveFiles.vue'
// eslint-disable-next-line @typescript-eslint/no-unused-vars -- used in template as <create>
import Create from '@/components/game_servers/Create.vue'
import DeleteGameServerFilesDialog from '@/components/game_servers/DeleteGameServerFilesDialog.vue'
import ExtractFiles from '@/components/game_servers/ExtractFiles.vue'
import FileUploaderDrop from '@/components/game_servers/FileUploaderDrop.vue'
import MoveFiles from '@/components/game_servers/MoveFiles.vue'
import RenameFile from '@/components/game_servers/RenameFile.vue'
import dayjs from 'dayjs'
import { QMenu, useQuasar } from 'quasar'
import { tabFolderFilled } from 'quasar-extras-svg-icons/tabler-icons-v2'
import { GameServer, GameServerSchema } from 'src/proto/shared_pb'
import {
  DownloadFileRequest,
  DownloadFileRequestSchema,
  File as xylonaFile,
  FileSchema,
  ListDirectoryFilesRequest,
  ListDirectoryFilesRequestSchema,
  ListDirectoryFilesResponse,
} from 'src/proto/gameserver_files_operations_pb'
import {
  bytesToSize,
  getColorFromFilenameExtension,
  getIconFromFilenameExtension,
  GetRelativeFilePath,
  GetXylonaClient,
} from 'src/utils/shared'
import { computed, onMounted, ref, Ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Timestamp, timestampDate, TimestampSchema } from '@bufbuild/protobuf/wkt'
import { GetGameServerRequest, GetGameServerRequestSchema } from '@/proto/xylona_pb'

const $q = useQuasar()
const uploadURL: Ref<string> = ref('/api/file/upload')

const route = useRoute()
const gameServerId: Ref<string> = ref(
  route.params.id instanceof Array ? route.params.id[0] : route.params.id,
)
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const files: Ref<Array<xylonaFile>> = ref([])
const directories: Ref<Array<xylonaFile>> = ref([])
const selectedFiles: Ref<Array<xylonaFile>> = ref([])
const selectAllFiles: Ref<boolean> = ref(false)
const path: Ref<string> = ref('')
const editorModal: Ref<boolean> = ref(false)
const editorFilename: Ref<string> = ref('')
const editorFilePath: Ref<string> = ref('')
const editorFileContent: Ref<string> = ref('')
const contextMenu: Ref<QMenu | null> = ref(null)
const fileListContainer: Ref<HTMLElement | null> = ref(null)
const fileUploaderDialog: Ref<boolean> = ref(false)
const createFileName: Ref<string> = ref('')

// Archive
const archiveFilesDialog: Ref<boolean> = ref(false)
const archiveName: Ref<string> = ref('')

// Extract
const extractFilesDialog: Ref<boolean> = ref(false)

// Create
const createFilesDialog: Ref<boolean> = ref(false)

// Rename
const renameFilesDialog: Ref<boolean> = ref(false)

// Move
const moveFilesDialog: Ref<boolean> = ref(false)

// Delete
const deleteFilesDialog: Ref<boolean> = ref(false)

const allowedExtractExtensions: string[] = ['.zip', '.zst', '.gz', '.bz2', '.xz', '.7z']
const allowedFileEditExtensions: string[] = [
  '.txt',
  '.cfg',
  '.json',
  '.xml',
  '.yml',
  '.yaml',
  '.ini',
  '.log',
  '.properties',
  '.sh',
  '.ps1',
  '.bat',
  '.py',
  '.js',
  '.ts',
]

async function refreshFileList() {
  selectedFiles.value = []
  void listDirectoryFiles(path.value)
}

async function createFilesDialogSubmitted(
  success: boolean,
  data: {
    fileName: string
    fullFilePath: string
    isDir: boolean
  } | null,
) {
  void listDirectoryFiles(path.value)
  createFilesDialog.value = false

  if (!success) {
    return
  }
  if (!data || data.isDir) {
    return
  }
  editorFilename.value = createFileName.value
  editorFileContent.value = ''
  editorFilePath.value = data.fullFilePath
  editorModal.value = true
}

function fileIsSelectedClass(file: xylonaFile) {
  if (selectedFiles.value.includes(file)) {
    return 'bg-xy-surface-2'
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

const createButtonEnabled = computed(() => {
  return selectedFiles.value.length === 0
})

const renameButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return selected.length === 1
})

const moveButtonEnabled = computed(() => {
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
    const deHashed = hashedPath.substring(1)
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
  if (selectAllFiles.value) {
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
    const pathSplit =
      path.value.lastIndexOf('/') !== -1 ? path.value.split('/') : path.value.split('\\')
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
  void listDirectoryFiles(path.value)
}

async function listDirectoryFiles(directoryPath: string) {
  const request: ListDirectoryFilesRequest = create(ListDirectoryFilesRequestSchema, {})
  try {
    request.gameServerId = gameServerId.value
    request.path = directoryPath
    const response: ListDirectoryFilesResponse = await GetXylonaClient().listDirectoryFiles(request)
    directories.value = []
    files.value = []
    const upDirectory: xylonaFile = create(FileSchema)
    upDirectory.name = '..'
    upDirectory.size = BigInt(0)
    upDirectory.isDirectory = true
    upDirectory.lastModified = create(TimestampSchema, {})
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
          void listDirectoryFiles(path.value)
        }, 100)
        $q.notify({
          caption: `Directory not found.`,
          type: 'xylona-error',
          position: 'top',
          timeout: 5000,
        })
        return
      }
      console.error(`Error listing directory files: ${err.code} ${err.message}`)
      return
    }
    console.error(err)
  } finally {
    // Ensure loading state is always cleared
  }
}

async function readFileOctetStream(fileName: string) {
  $q.loading.show({
    message: 'Reading file...',
    delay: 100,
  })
  const fullFilePath = GetRelativeFilePath(gameServer.value.directory, path.value, fileName)
  const fileRequest: DownloadFileRequest = create(DownloadFileRequestSchema, {})
  fileRequest.gameServerId = gameServerId.value
  fileRequest.path = fullFilePath
  try {
    const response = await fetch('/api/file/get', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: toJsonString(DownloadFileRequestSchema, fileRequest),
    })
    const data = await response.text()
    editorFilename.value = fileName
    editorFileContent.value = data
    editorFilePath.value = fullFilePath
    editorModal.value = true
  } catch (e) {
    console.error(e)
    $q.notify({
      caption: `Error reading file ${fileName}.`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    $q.loading.hide()
  }
}

async function downloadGameServerFile(fileName: string) {
  const fullFilePath = GetRelativeFilePath(gameServer.value.directory, path.value, fileName)
  const encodedGameServerID = encodeURIComponent(gameServerId.value)
  const encodedFilePath = encodeURIComponent(fullFilePath)
  const rawURL = `${window.location.protocol}//${window.location.host}/api/file/download/${encodedGameServerID}/${encodedFilePath}`
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
      timeout: 5000,
    })
  }
}

function toTimestamp(date: Timestamp | undefined) {
  if (date === undefined) {
    return ''
  }
  const JSDate: Date = timestampDate(date)
  return dayjs(JSDate).format('MM/DD/YYYY HH:mm:ss A')
}

async function getGameServerDetails() {
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
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
.file-empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--xy-space-3xl) var(--xy-space-md);
  text-align: center;
}

.file-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) 0;
}

.file-toolbar-primary {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
}

.file-toolbar-selection {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding-left: var(--xy-space-sm);
  border-left: 1px solid var(--xy-border);
}

.file-list-header {
  font-weight: bold;
  align-items: center;
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
  background-color: var(--xy-surface-2);
}

.file-list-container {
  font-family: var(--xy-font-mono);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
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
  overflow-x: auto;
}

.file-div {
  min-width: 0;
}

.file-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-div:hover {
  cursor: pointer;
  font-weight: 700;
}
</style>
