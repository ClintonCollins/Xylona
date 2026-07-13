<template>
  <q-card-section>
    <file-uploader-drop
      v-model:file-uploader-dialog="fileUploaderDialog"
      :game-server-id="gameServerId"
      :path="loadedPath"
      :path-separator="pathSeparator"
      :target-element="directoryActionsEnabled ? fileListContainer : null"
      :upload-u-r-l="uploadURL"
      @uploaded-files="listDirectoryFiles(loadedPath)">
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
              :disable="!directoryActionsEnabled"
              icon="upload"
              label="Upload"
              @click="fileUploaderDialog = true" />
            <q-btn
              v-if="downloadButtonEnabled"
              :disable="!downloadButtonEnabled"
              :loading="downloadingSelected"
              flat
              icon="download"
              label="Download"
              @click="downloadSelectedFiles" />
          </div>
          <div v-if="selectedFiles.length > 0" class="file-toolbar-selection">
            <span class="text-caption text-xy-secondary">
              {{ selectedFiles.length }} selected
            </span>
            <q-btn
              v-if="renameButtonEnabled"
              dense
              flat
              icon="edit"
              label="Rename"
              @click="renameFilesDialog = true" />
            <q-btn
              v-if="moveButtonEnabled"
              dense
              flat
              icon="drive_file_move"
              label="Move"
              @click="moveFilesDialog = true" />
            <q-btn
              v-if="zipButtonEnabled"
              dense
              flat
              icon="archive"
              label="Archive"
              @click="archiveFilesDialog = true" />
            <q-btn
              v-if="extractButtonEnabled"
              dense
              flat
              icon="unarchive"
              label="Extract"
              @click="extractFilesDialog = true" />
            <q-btn
              v-if="deleteButtonEnabled"
              color="negative"
              dense
              flat
              icon="delete"
              label="Delete"
              @click="deleteFilesDialog = true" />
          </div>
          <q-space />
          <q-btn
            class="gt-xs"
            :disable="!directoryActionsEnabled"
            dense
            flat
            icon="link"
            label="URL Upload"
            @click="fileUploaderDialog = true" />
        </div>
        <div class="row q-py-sm">
          <div class="col-xs-12">
            <q-input
              v-model="path"
              :prefix="gameServer.directory + pathSeparator"
              aria-label="File path"
              dense
              :loading="directoryLoading"
              outlined
              @update:model-value="clearSelection"
              @keydown.prevent.enter="updatePathFromInput"></q-input>
          </div>
        </div>
        <div class="row file-list-header q-px-sm">
          <div class="col-xs-2 col-md-2 col-lg-1">
            <q-checkbox
              v-model="selectAllFiles"
              :disable="!directoryActionsEnabled"
              label="All"></q-checkbox>
          </div>
          <div class="col-xs-5 col-sm-4">Name</div>
          <div class="col-xs-5 col-sm-3">Size</div>
          <div class="col-xs-3 gt-sm">Modified</div>
        </div>
        <q-separator class="q-my-sm"></q-separator>
        <div v-if="directoryLoading" aria-live="polite" class="file-directory-state" role="status">
          <q-spinner color="primary" size="2rem" />
          <div class="text-subtitle1 text-xy-secondary">Loading directory…</div>
        </div>
        <div
          v-else-if="directoryError"
          aria-live="assertive"
          class="file-directory-state"
          role="alert">
          <q-icon class="text-error" name="folder_off" size="2.5rem" />
          <div class="text-subtitle1 text-xy-primary">Could not load this directory</div>
          <div class="file-directory-error text-caption text-xy-secondary">
            {{ directoryError }}
          </div>
          <div class="row q-gutter-sm q-mt-sm">
            <q-btn color="primary" icon="refresh" label="Retry" @click="retryDirectoryLoad" />
            <q-btn
              v-if="path !== loadedPath"
              flat
              icon="undo"
              label="Return to loaded directory"
              @click="returnToLoadedDirectory" />
          </div>
        </div>
        <div v-else-if="directories.length === 0 && files.length === 0" class="file-empty-state">
          <q-icon class="text-xy-muted q-mb-sm" name="folder_open" size="3rem" />
          <div class="text-subtitle1 text-xy-secondary">This directory is empty</div>
          <div class="text-caption text-xy-muted q-mt-xs">
            Upload files or create new ones using the toolbar above.
          </div>
        </div>
        <div v-if="!directoryLoading && !directoryError" id="file-list" ref="filesList">
          <div
            v-for="directory in directories"
            :key="directory.name"
            :class="fileIsSelectedClass(directory)"
            class="row file-list-body-row q-px-sm">
            <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
              <q-checkbox
                v-if="directory.name !== '..'"
                v-model="selectedFiles"
                :disable="!directoryActionsEnabled"
                :val="directory"></q-checkbox>
            </div>
            <div
              class="col-xs-5 col-sm-4 file-div file-list-cell"
              role="button"
              tabindex="0"
              @click="clickDirectory(directory)"
              @keydown.enter="clickDirectory(directory)"
              @keydown.space.prevent="clickDirectory(directory)">
              <q-icon :name="tabFolderFilled" color="amber" left size="xs"></q-icon>
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
            :class="fileIsSelectedClass(file)"
            :data-file-name="file.name"
            class="row file-list-body-row q-px-sm"
            draggable="false">
            <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
              <q-checkbox
                v-model="selectedFiles"
                :disable="!directoryActionsEnabled"
                :val="file"></q-checkbox>
            </div>
            <div
              class="col-xs-5 col-sm-4 file-div file-list-cell"
              role="button"
              tabindex="0"
              @click="clickFile(file)"
              @keydown.enter="clickFile(file)"
              @keydown.space.prevent="clickFile(file)">
              <q-icon
                :name="getIconFromFilenameExtension(file.name)"
                :style="'color:' + getColorFromFilenameExtension(file.name)"
                left
                size="xs"></q-icon>
              <span class="file-name">{{ file.name }}</span>
            </div>
            <div class="col-xs-5 col-sm-3 file-list-cell">{{ bytesToSize(Number(file.size)) }}</div>
            <div class="col-xs-3 file-list-cell gt-sm">{{ toTimestamp(file.lastModified) }}</div>
          </div>
        </div>
        <q-menu ref="contextMenu" context-menu touch-position>
          <q-list>
            <q-item
              v-ripple
              clickable
              :disable="!directoryActionsEnabled"
              @click="selectAllFiles = true">
              <q-item-section> Select All</q-item-section>
            </q-item>
            <q-item
              v-ripple
              clickable
              :disable="!directoryActionsEnabled"
              @click="selectAllFiles = false">
              <q-item-section> Deselect All</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </div>
    </file-uploader-drop>
  </q-card-section>
  <q-dialog v-model="editorModal" backdrop-filter="blur(6px) brightness(15%)" no-shake persistent>
    <editor
      v-model:code-input="editorFileContent"
      :file-name="editorFilename"
      :full-file-path="editorFilePath"
      :game-server-id="gameServerId"
      @submit="editorSaved"></editor>
  </q-dialog>
  <archive-files
    v-model:archive-name="archiveName"
    v-model:show-dialog="archiveFilesDialog"
    :game-server-id="gameServerId"
    :path="loadedPath"
    :path-separator="pathSeparator"
    :selected-files="selectedFiles"
    @cancel="archiveFilesDialog = false"
    @submit="refreshFileList">
  </archive-files>
  <extract-files
    v-model:show-dialog="extractFilesDialog"
    :full-archive-path="
      GetRelativeFilePath(gameServer.directory, loadedPath, selectedFiles[0]?.name)
    "
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :path="loadedPath"
    @cancel="extractFilesDialog = false"
    @submit="refreshFileList">
  </extract-files>
  <create
    v-model:show-dialog="createFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :path="loadedPath"
    @submit="createFilesDialogSubmitted">
  </create>
  <rename-file
    v-model:show-dialog="renameFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :old-file-name="selectedFiles[0]?.name"
    :path="loadedPath"
    @submit="refreshFileList">
  </rename-file>
  <move-files
    v-model:show-dialog="moveFilesDialog"
    :game-server-id="gameServerId"
    :game-server-path="gameServer.directory"
    :neighboring-directories-in-path="directories.map((f) => f.name)"
    :path="loadedPath"
    :selected-files="selectedFiles"
    @submit="refreshFileList">
  </move-files>
  <delete-game-server-files-dialog
    v-model:show-dialog="deleteFilesDialog"
    :current-path="loadedPath"
    :files-to-delete="selectedFiles"
    :game-server-i-d="gameServerId"
    :path-separator="pathSeparator"
    @files-deleted="refreshFileList()">
  </delete-game-server-files-dialog>
</template>

<script lang="ts" setup>
import { create, toJsonString } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, Ref, watch } from 'vue'
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
import { GameServer, GameServerSchema } from '@/proto/shared_pb'
import {
  DownloadFileRequest,
  DownloadFileRequestSchema,
  File as xylonaFile,
  FileSchema,
  ListDirectoryFilesRequest,
  ListDirectoryFilesRequestSchema,
  ListDirectoryFilesResponse,
} from '@/proto/gameserver_files_operations_pb'
import {
  bytesToSize,
  getColorFromFilenameExtension,
  getIconFromFilenameExtension,
  GetRelativeFilePath,
  GetXylonaClient,
} from '@/utils/shared'
import { useRoute } from 'vue-router'
import { Timestamp, timestampDate, TimestampSchema } from '@bufbuild/protobuf/wkt'
import { GetGameServerRequest, GetGameServerRequestSchema } from '@/proto/xylona_pb'

const Editor = defineAsyncComponent(() => import('@/components/Editor.vue'))

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
const loadedPath: Ref<string> = ref('')
const directoryLoading: Ref<boolean> = ref(true)
const directoryError: Ref<string> = ref('')
const downloadingSelected: Ref<boolean> = ref(false)
const editorModal: Ref<boolean> = ref(false)
const editorFilename: Ref<string> = ref('')
const editorFilePath: Ref<string> = ref('')
const editorFileContent: Ref<string> = ref('')
const contextMenu: Ref<QMenu | null> = ref(null)
const fileListContainer: Ref<HTMLElement | null> = ref(null)
const fileUploaderDialog: Ref<boolean> = ref(false)
const createFileName: Ref<string> = ref('')
let directoryRequestSequence = 0
let componentUnmounted = false

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
  await listDirectoryFiles(loadedPath.value)
}

async function editorSaved() {
  editorModal.value = false
  await refreshFileList()
}

async function createFilesDialogSubmitted(
  success: boolean,
  data: {
    fileName: string
    fullFilePath: string
    isDir: boolean
  } | null,
) {
  await listDirectoryFiles(loadedPath.value)
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
  return directoryActionsEnabled.value && selected.length > 0
})

const downloadButtonEnabled = computed(() => {
  if (!directoryActionsEnabled.value) {
    return false
  }
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
  return directoryActionsEnabled.value && selected.length > 0
})

const createButtonEnabled = computed(() => {
  return directoryActionsEnabled.value && selectedFiles.value.length === 0
})

const renameButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return directoryActionsEnabled.value && selected.length === 1
})

const moveButtonEnabled = computed(() => {
  const selected = sanitizeSelectedFiles()
  return directoryActionsEnabled.value && selected.length > 0
})

const extractButtonEnabled = computed(() => {
  if (!directoryActionsEnabled.value) {
    return false
  }
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

const directoryActionsEnabled = computed(
  () => !directoryLoading.value && directoryError.value === '' && path.value === loadedPath.value,
)

function clearSelection() {
  selectAllFiles.value = false
  selectedFiles.value = []
}

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
  await getGameServerDetails()
  if (componentUnmounted) {
    return
  }
  window.addEventListener('hashchange', handleHashNavigation)
  const hashedPath = window.location.hash
  if (hashedPath.length > 0) {
    const deHashed = hashedPath.substring(1)
    path.value = deHashed.replaceAll('/', pathSeparator.value)
  }
  await listDirectoryFiles(path.value)
  const draggableElements = document.querySelectorAll('div[draggable="true"]')
  for (let i = 0; i < draggableElements.length; i++) {
    attachDragStartEvent(draggableElements[i] as HTMLElement)
  }
})

onBeforeUnmount(() => {
  componentUnmounted = true
  window.removeEventListener('hashchange', handleHashNavigation)
})

function handleHashNavigation() {
  const hashPath = normalizeDirectoryPath(
    window.location.hash.substring(1).replaceAll('/', pathSeparator.value),
  )
  if (hashPath === path.value && hashPath === loadedPath.value) {
    return
  }

  clearSelection()
  path.value = hashPath
  void listDirectoryFiles(hashPath)
}

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
  if (!directoryActionsEnabled.value) {
    return
  }

  let nextPath = loadedPath.value
  if (directory.name === '..') {
    const pathSplit = nextPath.lastIndexOf('/') !== -1 ? nextPath.split('/') : nextPath.split('\\')
    pathSplit.pop()
    nextPath = pathSplit.join(pathSeparator.value)
  } else {
    nextPath = GetRelativeFilePath(gameServer.value.directory, nextPath, directory.name)
  }

  clearSelection()
  path.value = normalizeDirectoryPath(nextPath)
  await listDirectoryFiles(path.value)
}

async function clickFile(file: xylonaFile) {
  if (!directoryActionsEnabled.value) {
    return
  }
  // If file is an allowed file type for editing, open the editor.
  const fileExtension = file.name.substring(file.name.lastIndexOf('.')).toLowerCase()
  if (allowedFileEditExtensions.includes(fileExtension)) {
    await readFileOctetStream(file.name)
    return
  }
  // If file is not an allowed file type for editing, download the file.
  await downloadGameServerFile(file.name)
}

function updatePathFromInput() {
  clearSelection()
  path.value = normalizeDirectoryPath(path.value)
  void listDirectoryFiles(path.value)
}

async function listDirectoryFiles(directoryPath: string) {
  const normalizedPath = normalizeDirectoryPath(directoryPath)
  const requestSequence = ++directoryRequestSequence
  clearSelection()
  fileUploaderDialog.value = false
  directoryLoading.value = true
  directoryError.value = ''

  const request: ListDirectoryFilesRequest = create(ListDirectoryFilesRequestSchema, {})
  try {
    request.gameServerId = gameServerId.value
    request.path = normalizedPath
    const response: ListDirectoryFilesResponse = await GetXylonaClient().listDirectoryFiles(request)
    if (requestSequence !== directoryRequestSequence) {
      return
    }

    directories.value = []
    files.value = []
    if (normalizedPath !== '') {
      const upDirectory: xylonaFile = create(FileSchema)
      upDirectory.name = '..'
      upDirectory.size = BigInt(0)
      upDirectory.isDirectory = true
      upDirectory.lastModified = create(TimestampSchema, {})
      directories.value.push(upDirectory)
    }
    response.files.forEach((file) => {
      if (file.isDirectory) {
        directories.value.push(file)
      } else {
        files.value.push(file)
      }
    })
    loadedPath.value = normalizedPath
    path.value = normalizedPath
    window.location.hash = normalizedPath.replaceAll(pathSeparator.value, '/')
  } catch (err) {
    if (requestSequence !== directoryRequestSequence) {
      return
    }

    if (err instanceof ConnectError) {
      if (err.code === Code.NotFound) {
        directoryError.value = 'The requested directory was not found.'
        return
      }
      console.error(`Error listing directory files: ${err.code} ${err.message}`)
      directoryError.value = err.message || 'The directory listing request failed.'
      return
    }
    console.error(err)
    directoryError.value = err instanceof Error ? err.message : 'The directory listing failed.'
  } finally {
    if (requestSequence === directoryRequestSequence) {
      directoryLoading.value = false
    }
  }
}

function normalizeDirectoryPath(directoryPath: string): string {
  const segments = directoryPath.split(/[\\/]+/)
  const normalizedSegments: string[] = []

  for (const segment of segments) {
    if (segment === '' || segment === '.') {
      continue
    }
    if (segment === '..') {
      normalizedSegments.pop()
      continue
    }
    normalizedSegments.push(segment)
  }

  return normalizedSegments.join(pathSeparator.value)
}

function retryDirectoryLoad() {
  void listDirectoryFiles(path.value)
}

function returnToLoadedDirectory() {
  clearSelection()
  path.value = loadedPath.value
  directoryError.value = ''
  directoryLoading.value = false
  window.location.hash = loadedPath.value.replaceAll(pathSeparator.value, '/')
}

async function readFileOctetStream(fileName: string) {
  $q.loading.show({
    message: 'Reading file...',
    delay: 100,
  })
  const operationPath = loadedPath.value
  const fullFilePath = GetRelativeFilePath(gameServer.value.directory, operationPath, fileName)
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
    if (!response.ok) {
      throw new Error(response.statusText || `File request failed with status ${response.status}`)
    }
    const data = await response.text()
    if (loadedPath.value !== operationPath || path.value !== operationPath) {
      return
    }
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

async function downloadSelectedFiles() {
  if (downloadingSelected.value || !downloadButtonEnabled.value) {
    return
  }

  const operationPath = loadedPath.value
  const filesToDownload = sanitizeSelectedFiles()
  downloadingSelected.value = true
  try {
    for (const file of filesToDownload) {
      await downloadGameServerFile(file.name, operationPath)
    }
  } finally {
    downloadingSelected.value = false
  }
}

async function downloadGameServerFile(fileName: string, operationPath = loadedPath.value) {
  const fullFilePath = GetRelativeFilePath(gameServer.value.directory, operationPath, fileName)
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
      caption: `Error downloading file ${fileName}.`,
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

.file-directory-state {
  display: flex;
  min-height: 16rem;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xl) var(--xy-space-md);
  text-align: center;
}

.file-directory-error {
  max-width: 65ch;
  overflow-wrap: anywhere;
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
