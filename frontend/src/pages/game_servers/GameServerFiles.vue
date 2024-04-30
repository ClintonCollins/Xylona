<template>
  <q-card-section>
    <div class="row q-px-md justify-center">
      <div ref="fileListContainer"
           v-on:dragenter="fileContainerDragEnter"
           v-on:dragleave="fileContainerDragLeave"
           v-on:drop="fileContainerDrop"
           v-on:dragover="$event.preventDefault()"
           class="col-xs-12 file-list-container bg-neutral-glass-4 q-pa-sm" style="border-radius: .5rem">
        <div class="row q-py-sm justify-end q-gutter-x-md">
          <q-btn v-show="deleteButtonEnabled" class="bg-error">Delete</q-btn>
          <q-btn v-show="downloadButtonEnabled" class="bg-blue">Download</q-btn>
          <q-btn v-show="zipButtonEnabled" class="bg-teal">Zip/Archive</q-btn>
          <q-btn v-show="extractButtonEnabled" class="bg-green">Extract</q-btn>
          <q-btn class="bg-alert" label="Upload from URL" @click="fileUploaderDialog = true"></q-btn>
          <q-btn class="bg-success" label="Upload" @click="fileUploaderDialog = true"></q-btn>
        </div>
        <div class="row q-py-sm">
          <div class="col-xs-12">
            <q-input @keydown.prevent.enter="updatePathFromInput" :prefix="gameServer.directory + pathSeparator"
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
          <div class="row file-list-body-row q-px-sm" :class="fileIsSelectedClass(directory)" v-for="directory in directories">
            <div class="col-xs-2 col-md-2 col-lg-1 file-list-cell">
              <q-checkbox v-if="directory.name !== '..'" :val="directory" v-model="selectedFiles"></q-checkbox>
            </div>
            <div class="col-xs-4 file-div file-list-cell" @click="clickDirectory(directory)">
              <q-icon size="xs" color="amber" :name="tabFolderFilled" left></q-icon>
              {{ directory.name }}
            </div>
            <div class="col-xs-3 file-list-cell">{{ bytesToSize(Number(directory.size)) }}</div>
            <div class="col-xs-3 file-list-cell">{{ toTimestamp(directory.lastModified) }}</div>
          </div>
          <div class="row file-list-body-row q-px-sm" :class="fileIsSelectedClass(file)" v-for="file in files" :data-file-name="file.name"
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
        <q-menu ref="contextMenu" touch-position context-menu @before-show="contextMenuClick">
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
    </div>
  </q-card-section>
  <q-dialog v-model="editorModal" backdrop-filter="blur(6px) brightness(15%)">
    <q-card class="file-editor">
      <q-card-section>
        <div class="text-h6">{{ editingFilename }}</div>
      </q-card-section>

      <q-card-section>
        <q-input v-model="editingFileContent" type="textarea" rows="40" outlined dense></q-input>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="Cancel" color="primary" v-close-popup/>
        <q-btn flat label="Save" color="primary" v-close-popup/>
      </q-card-actions>
    </q-card>
  </q-dialog>
  <q-dialog v-model="fileUploaderDialog" backdrop-filter="brightness(25%)" @hide="uploader.close()">
    <q-card id="fileUploaderCard" class="fileUploaderDialogCard"
            v-on:dragenter="fileContainerDragEnter"
            v-on:dragleave="fileContainerDragLeave"
            v-on:drop="fileContainerDrop"
            v-on:dragover="$event.preventDefault()"
    >
      <q-card-section>
        <div class="text-h6">File Upload (You can drag and drop files below)</div>
        <q-toolbar class="bg-primary q-py-sm">
          <q-btn v-if="uploader.files.size > 0" icon="clear_all" @click="uploader.removeQueuedFiles()" round dense flat>
            <q-tooltip>Clear All</q-tooltip>
          </q-btn>
          <q-spinner v-if="uploader.isUploading" class="q-uploader__spinner"/>
          <div class="col">
            <div class="q-uploader__title">Upload your files</div>
            <div class="q-uploader__subtitle">{{ uploader.uploadedSizeSoFarLabel }} uploaded out of
              {{ uploader.uploadSizeLabel }} / {{ uploader.uploadProgressLabel }}
            </div>
            <div class="q-uploader__subtitle">{{ uploader.queuedFilesCount }} files in queue /
              {{ uploader.uploadedFilesCount }} files uploaded
            </div>
          </div>
          <q-btn v-if="uploader.queuedFilesCount > 0 && uploader.canUpload" icon="cloud_upload"
                 @click="uploader.upload()" round dense flat>
            <q-tooltip>Upload Files</q-tooltip>
          </q-btn>

          <q-btn v-if="uploader.isUploading" icon="clear" @click="uploader.abort()" round dense flat>
            <q-tooltip>Abort Upload</q-tooltip>
          </q-btn>
        </q-toolbar>

        <div class="file-uploader">
          <q-list separator>
            <q-item v-for="file in uploader.files.values()" :key="file.key">
              <q-item-section avatar v-if="!file.error && !file.success">
                <q-icon size="lg" :name="tabDots" class="text-primary-brighter"/>
              </q-item-section>
              <q-item-section avatar v-if="file.error">
                <q-icon size="lg" :name="tabAlertTriangle" class="text-error-brighter"/>
              </q-item-section>
              <q-item-section avatar v-if="file.success">
                <q-icon size="lg" :name="tabCheck" class="text-success-brighter"/>
              </q-item-section>
              <q-item-section>
                <q-item-label class="full-width ellipsis">
                  {{ file.file.name }}
                </q-item-label>
                <q-item-label caption>
                  Status: {{ file.status }}
                </q-item-label>

                <q-item-label caption>
                  {{ file.uploadedSizeSoFarLabel }} uploaded out of {{ file.sizeLabel }} / {{ file.progressLabel }}
                </q-item-label>
              </q-item-section>

              <q-item-section top side>
                <q-btn
                    class="gt-xs"
                    size="1rem"
                    flat
                    round
                    color="negative"
                    :icon="file.error ? tabX : tabTrash"
                    @click="uploader.removeFile(file)"
                />
              </q-item-section>
            </q-item>

          </q-list>
        </div>
      </q-card-section>
      <q-card-actions align="right" class="">
        <q-btn flat label="Close" color="grey" @click="uploader.close()"/>
        <q-btn label="Upload" class="bg-success" @click="uploader.upload()"
               :disable="!uploader.canUpload || uploader.queuedFilesCount <= 0"/>
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import {
  File as xylonaFile,
  GameServer,
  GetFileRequest,
  GetGameServerRequest,
  ListDirectoryFilesRequest,
  ListDirectoryFilesResponse
} from "src/proto/xylona_pb";
import {bytesToSize, GetXylonaClient} from "src/utils/shared";
import {useRoute} from "vue-router";
import {computed, onMounted, ref, Ref, watch} from "vue";
import dayjs from "dayjs";
import {Timestamp} from "@bufbuild/protobuf";
import {
  tabArchive,
  tabCheck,
  tabDots,
  tabFileFilled,
  tabFileSettings,
  tabFileTypeTxt,
  tabFilterSearch,
  tabFolderFilled,
  tabJson,
  tabTrash,
  tabX
} from "quasar-extras-svg-icons/tabler-icons-v2";
import {Code, ConnectError} from "@connectrpc/connect";
import {QCard, QMenu, useQuasar} from "quasar";
import axios, {AxiosRequestConfig} from "axios";
import {tabAlertTriangle} from "quasar-extras-svg-icons/tabler-icons";

const $q = useQuasar()
const uploadURL: Ref<string> = ref('/api/file/upload')

const route = useRoute()
const gameServerId: Ref<string> = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
const gameServer: Ref<GameServer> = ref(new GameServer()) as Ref<GameServer>
const files: Ref<Array<xylonaFile>> = ref([])
const directories: Ref<Array<xylonaFile>> = ref([])
const selectedFiles: Ref<Array<xylonaFile>> = ref([])
const selectAllFiles: Ref<boolean> = ref(false)
const path: Ref<string> = ref("")
const editorModal: Ref<boolean> = ref(false)
const editingFilename: Ref<string> = ref("")
const editingFileContent: Ref<string> = ref("")
const contextMenu: Ref<QMenu | null> = ref(null)
const fileListContainer: Ref<HTMLElement | null> = ref(null)
const dragInFileContainerChildren: Ref<boolean> = ref(false)
const fileUploaderDialog: Ref<boolean> = ref(false)
const addingFilesToUploaderViaFileContainerDrop: Ref<boolean> = ref(false) // This is for overriding the default uploader adding files. When this is false, we intercept the file add event and add files manually.


function fileIsSelectedClass(file) {
  if (selectedFiles.value.includes(file)) {
    return "bg-neutral-glass-4"
  }
  return ""
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
  if (selected.length <= 0) {
    return false
  }
  for (let i = 0; i < selected.length; i++) {
    if (!selected[i].name.endsWith(".zip")) {
      return false
    }
  }
  return true
})

function sanitizeSelectedFiles(): xylonaFile[] {
  const sanitizedFiles: xylonaFile[] = []
  selectedFiles.value.forEach((file) => {
    if (file.name === "..") {
      return
    }
    sanitizedFiles.push(file)
  })
  return sanitizedFiles
}

type uploaderFile = {
  file: File
  path: string
  key: string
  status: string
  uploadedSizeSoFarLabel: string
  sizeLabel: string
  progressLabel: string
  error: boolean
  success: boolean
}

class GameServerFileUploader {
  files: Map<string, uploaderFile> = new Map<string, uploaderFile>()
  uploadSize: number = 0
  uploadSizeLabel: string = '0.00 MB'
  uploadedSizeSoFarLabel: string = '0.00 MB'
  uploadedBytes: number = 0
  uploadProgressLabel: string = '0.00%'
  isUploading: boolean = false
  canUpload: boolean = true
  queuedFilesCount: number = 0
  uploadedFilesCount: number = 0
  finishedUpload: boolean = false
  gotError: boolean = false
  abortController: AbortController | null = null

  checkFilesLeftToUpload() {
    if (this.queuedFilesCount === 0 || this.gotError) {
      this.isUploading = false
      this.canUpload = true
      this.finishedUpload = true
      listDirectoryFiles(path.value)
    }
  }

  removeFile(file: uploaderFile) {
    this.files.delete(file.key)
    this.uploadSize -= file.file.size
    this.queuedFilesCount--
    this.calculateLabels()
  }

  removeQueuedFiles() {
    this.reset()
  }

  reset() {
    this.files.clear()
    this.uploadSize = 0
    this.uploadedBytes = 0
    this.queuedFilesCount = 0
    this.uploadedFilesCount = 0
    this.canUpload = true
    this.finishedUpload = false
    this.gotError = false
    this.calculateLabels()
  }

  calculateLabels() {
    this.uploadSizeLabel = bytesToSize(this.uploadSize)
    const uploadedBytes = this.uploadedBytes <= this.uploadSize ? this.uploadedBytes : this.uploadSize
    this.uploadedSizeSoFarLabel = bytesToSize(uploadedBytes)
    let progress = this.uploadedBytes / this.uploadSize
    if (isNaN(progress)) {
      progress = 0
    }
    let calculatedProgress = (progress * 100)
    if (calculatedProgress > 100) {
      calculatedProgress = 100
    }
    this.uploadProgressLabel = calculatedProgress.toFixed(2) + "%"
  }

  abort() {
    if (this.abortController !== null) {
      this.abortController.abort()
    }
  }

  close() {
    this.reset()
    fileUploaderDialog.value = false
  }

  upload() {
    this.isUploading = true
    this.canUpload = false
    this.files.forEach((file) => {
      this.uploadFile(file)
    })
  }

  uploadFile(file: uploaderFile) {
    file.error = false
    file.success = false
    const formData = new FormData()
    const abortController = new AbortController()
    this.abortController = abortController
    formData.append('file', file.file)
    formData.append('fileName', file.file.name)
    formData.append('path', file.path)
    formData.append('gameServerId', gameServerId.value)
    const axiosConfig: AxiosRequestConfig = {
      headers: {
        'Content-Type': 'multipart/form-data'
      },
      signal: abortController.signal,
      onUploadProgress: (progressEvent) => {
        const progress = (progressEvent.loaded / progressEvent.total)
        file.progressLabel = (progress * 100).toFixed(2) + "%"
        file.uploadedSizeSoFarLabel = bytesToSize(progressEvent.loaded)
        this.uploadedBytes += progressEvent.bytes
        this.calculateLabels()
      }
    }

    axios.post(uploadURL.value, formData, axiosConfig).then((response) => {
      file.status = "Uploaded"
      file.success = true
      this.uploadedFilesCount++
      this.queuedFilesCount--
    }).catch((error) => {
      // console.error(error)
      file.status = "Error"
      file.progressLabel = "0.00%"
      file.error = true
      this.gotError = true
    }).finally(() => {
      this.checkFilesLeftToUpload()
      this.calculateLabels()
    })
  }

  addFile(file: File) {
    if (this.finishedUpload) {
      this.reset()
    }
    const updatedFile = new File([file], getUploaderFileRelativePath(file), {type: file.type})
    if (this.files.has(updatedFile.name)) {
      const oldFile = this.files.get(updatedFile.name)
      this.files.delete(updatedFile.name)
      if (oldFile !== undefined) {
        this.uploadSize -= oldFile.file.size
        this.queuedFilesCount--
      }
    }
    // console.log(getUploaderFilePath(file))
    this.files.set(updatedFile.name, {
      file: updatedFile,
      path: getUploaderFilePath(file),
      key: updatedFile.name,
      status: "Queued",
      error: false,
      success: false,
      sizeLabel: bytesToSize(updatedFile.size),
      uploadedSizeSoFarLabel: "0.00 MB",
      progressLabel: "0.00%"
    })
    this.queuedFilesCount++
    this.uploadSize += updatedFile.size
    this.calculateLabels()
  }
}

const uploader: Ref<GameServerFileUploader> = ref(new GameServerFileUploader())

onMounted(async () => {
  await listDirectoryFiles(path.value)
  await getGameServerDetails()
  const draggableElements = document.querySelectorAll('div[draggable="true"]');

  draggableElements.forEach((draggableElement: HTMLElement) => {
    draggableElement.addEventListener("dragstart", (event: DragEvent) => {
      const fileName = (event.target as HTMLElement).dataset.fileName
      event.dataTransfer.setData("filename", fileName)
    })
  })

})

watch(selectAllFiles, (newValue) => {
  const sanitizedDirectories = directories.value.filter((directory) => {
    return directory.name !== ".."
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
      return directory.name !== ".."
    })
    selectAllFiles.value = newValue.length === sanitizedDirectories.length + files.value.length;
  }
})

const pathSeparator = computed(() => {
  if (gameServer.value.directory.indexOf('\\') !== -1) {
    return "\\"
  }
  return "/"
})

async function clickDirectory(directory: xylonaFile) {
  if (directory.name === "..") {
    const pathSplit = path.value.split(pathSeparator.value)
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
  await listDirectoryFiles(path.value)
}

async function clickFile(file: xylonaFile) {
  // If file is too large, don't directly read it.
  if (file.size > BigInt(1024 * 1024 * 5)) {
    alert("File is too large to read directly.")
    return
  }
  if (path.value === "") {
    await readFileOctetStream(file.name)
    return
  }
  await readFileOctetStream(path.value + pathSeparator.value + file.name)
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
    upDirectory.name = ".."
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
        path.value = ""
        setTimeout(() => {
          listDirectoryFiles("")
        }, 100)
        alert("Directory does not exist.")
        return
      }
      console.error(`Error listing directory files: ${err.code} ${err.message}`)
      return
    }
    console.error(err)
  } finally {
  }
}

function contextMenuClick(event: MouseEvent) {
  const el = event.target as HTMLElement
  // console.log(el.children)
  // console.log(el.innerHTML)
  const f = directories.value.concat(files.value)
  if (f.length === 0) {
    return
  }
  let foundFileName: string | null = null
  f.forEach((file) => {
    if (el.innerHTML == file.name) {
      foundFileName = file.name
    }
  })
  if (foundFileName === null) {
    if (contextMenu.value === null) {
      return
    }
    contextMenu.value?.hide()
    return
  }
  console.log("foundFileName: " + foundFileName)
}

async function readFileOctetStream(filePath: string) {
  $q.loading.show({
    message: "Reading file...",
    delay: 100
  })
  const fileRequest = new GetFileRequest()
  fileRequest.gameServerId = gameServerId.value
  fileRequest.path = filePath
  try {
    const response = await fetch('/api/file/get', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json'
      },
      body: fileRequest.toJsonString()
    })
    const data = await response.text()
    const fileNameSplit = filePath.split(pathSeparator.value)
    if (fileNameSplit.length <= 0) {
      return
    }
    editingFilename.value = fileNameSplit[fileNameSplit.length - 1]
    editingFileContent.value = data
    editorModal.value = true
  } catch (e) {
    console.error(e)
  } finally {
    $q.loading.hide()
  }
}

function getIconFromFilenameExtension(fileName: string): string {
  const fileNameSplit = fileName.split('.')
  if (fileNameSplit.length <= 1) {
    return tabFileFilled
  }
  const extension = fileNameSplit[fileNameSplit.length - 1]
  switch (extension) {
    case "json":
      return tabJson
    case "txt":
      return tabFileTypeTxt
    case "log":
      return tabFilterSearch
    case "settings":
      return tabFileSettings
    case "jar":
      return tabArchive
    default:
      return tabFileFilled
  }
}

function getColorFromFilenameExtension(fileName: string): string {
  const fileNameSplit = fileName.split('.')
  if (fileNameSplit.length <= 1) {
    return "whitesmoke"
  }
  const extension = fileNameSplit[fileNameSplit.length - 1]
  switch (extension) {
    case "json":
      return "#74c639"
    case "txt":
      return "#94c2e6"
    case "log":
      return "#818181"
    case "settings":
      return "orange"
    case "jar":
      return "#f0db4f"
    default:
      return "whitesmoke"
  }
}

function toTimestamp(date: Timestamp) {
  return dayjs(date.toDate()).format("MM/DD/YYYY HH:mm:ss A")
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

async function processDirectoryEntry(webkitDirEntry: any) {
  return new Promise((resolveDirectory) => {
    webkitDirEntry.createReader().readEntries(async (entries: any[]) => {
      for (let i = 0; i < entries.length; i++) {
        const entry = entries[i];
        if (entry.isFile) {
          await processFileEntry(entry);
        } else if (entry.isDirectory) {
          await processDirectoryEntry(entry);
        }
      }
      resolveDirectory();
    })
  })
}


async function processFileEntry(fileEntry: any) {
  return new Promise((resolveFile) => {
    fileEntry.file((file: File) => {
      uploader.value.addFile(file);
      // fileUploader.value?.addFiles([updatedFile]);
      resolveFile();
    });
  });
}

async function fileContainerDrop(event: DragEvent) {
  event.preventDefault()
  if (event.dataTransfer.files.length <= 0) {
    return
  }
  const fileUploaderCard = document.getElementById("fileUploaderCard")
  fileListContainer.value?.classList.remove("file-container-drag-over")
  fileUploaderCard?.classList.remove("file-container-drag-over")
  dragInFileContainerChildren.value = false
  if (event.dataTransfer.items) {
    const items = event.dataTransfer.items
    fileUploaderDialog.value = true
    setTimeout(async () => {
      addingFilesToUploaderViaFileContainerDrop.value = true
      for (let i = 0; i < items.length; i++) {
        const webkitEntry = items[i].webkitGetAsEntry()
        if (webkitEntry.isDirectory) {
          await processDirectoryEntry(webkitEntry);
        } else if (webkitEntry.isFile) {
          await processFileEntry(webkitEntry);
        }
      }
      addingFilesToUploaderViaFileContainerDrop.value = false
    }, 100)
  }
}

function fileContainerDragEnter(event: DragEvent) {
  const target = event.target as HTMLElement
  if (event.dataTransfer.files.length <= 0) {
    // Check if the drag event is a file list file. Potentially add the ability to move files to directories by drag and drop...
    let fileListFile = false
    event.dataTransfer.types.forEach((type) => {
      if (type === "filename") {
        fileListFile = true
      }
    })
    return
  }
  event.preventDefault()
  // event.stopImmediatePropagation()
  const fileUploaderCard = document.getElementById("fileUploaderCard")
  if (target != fileListContainer.value && target != fileUploaderCard) {
    dragInFileContainerChildren.value = true
    return
  }
  dragInFileContainerChildren.value = false
  if (target === fileListContainer.value) {
    fileListContainer.value?.classList.add("file-container-drag-over")
  }
  if (target === fileUploaderCard) {
    fileUploaderCard.classList.add("file-container-drag-over")
  }
}

function fileContainerDragLeave(event: DragEvent) {
  const target = event.target as HTMLElement
  event.preventDefault()
  // event.stopImmediatePropagation()
  const fileUploaderCard = document.getElementById("fileUploaderCard")
  if (dragInFileContainerChildren.value) {
    return
  }
  if (target != fileListContainer.value && target != fileUploaderCard) {
    return
  }
  if (fileListContainer.value === null && fileUploaderCard === null) {
    return
  }
  if (target === fileListContainer.value) {
    fileListContainer.value?.classList.remove("file-container-drag-over")
  }
  if (target === fileUploaderCard) {
    fileUploaderCard.classList.remove("file-container-drag-over")
  }
  dragInFileContainerChildren.value = false
}

function getUploaderFileRelativePath(file: File): string {
  if (file.webkitRelativePath === "") {
    return file.name
  }
  return file.webkitRelativePath
}

function getUploaderFilePath(file: File): string {
  let joinedRelativePath = path.value
  if (joinedRelativePath.length > 0) {
    joinedRelativePath += pathSeparator.value
  }
  joinedRelativePath += file.webkitRelativePath
  return joinedRelativePath.slice(0, joinedRelativePath.lastIndexOf("/"))
}

// @media screen and (min-height: 2160px), screen {
//   height: 70dvh;
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

.file-div {
  //height: 100% !important;
  //display: flex;
  //align-items: center;
}

.file-div:hover {
  cursor: pointer;
  font-weight: 700;
}

.file-editor {
  min-width: 60vw !important;
  min-height: 70vh !important;
  font-family: "Oxygen Mono", monospace !important;
}

.file-container-drag-over {
  border: dashed 2px var(--q-primary);
  background-color: var(--bg-neutral-opaque);
}

.file-uploader {
  //border: solid 1px var(--bg-neutral);
  height: 50vh;
  overflow: scroll;
}

.fileUploaderDialogCard {
  min-width: 40vw !important;
  //min-height: 60vh !important;
  font-family: "Oxygen Mono", monospace !important;
  border: 2px solid var(--bg-neutral-opaque);
}
</style>
