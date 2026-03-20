<template>
  <section
    ref="dragAndDropZone"
    @dragenter="props.targetElement && dragEnterEvent($event, props.targetElement)"
    @dragleave="props.targetElement && dragLeaveEvent($event, props.targetElement)"
    @drop="props.targetElement && dragDropEvent($event, props.targetElement)"
    @dragover="$event.preventDefault()">
    <slot></slot>
  </section>
  <q-dialog
    v-model="fileUploaderDialog"
    backdrop-filter="brightness(25%)"
    aria-labelledby="dialog-title"
    @hide="uploader.close()">
    <q-card
      id="fileUploaderCard"
      class="fileUploaderDialogCard q-px-lg"
      @dragenter="dragEnterEvent($event, 'fileUploaderCard')"
      @dragleave="dragLeaveEvent($event, 'fileUploaderCard')"
      @drop="dragDropEvent($event, 'fileUploaderCard')"
      @dragover="$event.preventDefault()">
      <q-card-section>
        <div id="dialog-title" class="text-h6">File Upload (You can drag and drop files below)</div>
        <q-toolbar class="bg-xy-surface-2 q-py-sm">
          <q-btn
            v-if="uploader.files.size > 0"
            :icon="tabClearAll"
            class="text-white bg-alert-brighter"
            aria-label="Clear all files"
            @click="uploader.removeQueuedFiles()">
            <q-tooltip>Clear All</q-tooltip>
          </q-btn>
          <q-spinner v-if="uploader.isUploading" class="q-uploader__spinner q-ml-md" />
          <div class="col q-ml-md">
            <div class="q-uploader__title">Upload your files</div>
            <div class="q-uploader__subtitle">
              {{ uploader.uploadedSizeSoFarLabel }} uploaded out of {{ uploader.uploadSizeLabel }} /
              {{ uploader.uploadProgressLabel }}
            </div>
            <div class="q-uploader__subtitle">
              {{ uploader.queuedFilesCount }} files in queue /
              {{ uploader.uploadedFilesCount }} files uploaded
            </div>
          </div>
          <q-btn
            v-if="uploader.queuedFilesCount > 0 && uploader.canUpload"
            class="text-white bg-success-darker"
            :icon="tabOutlineUpload"
            aria-label="Upload all files"
            @click="uploader.upload()">
            <q-tooltip>Upload Files</q-tooltip>
          </q-btn>

          <q-btn
            v-if="uploader.isUploading"
            icon="clear"
            aria-label="Abort upload"
            round
            dense
            flat
            @click="uploader.abort()">
            <q-tooltip>Abort Upload</q-tooltip>
          </q-btn>
        </q-toolbar>

        <div class="file-uploader">
          <q-list separator>
            <q-item
              v-for="file in uploader.files.values()"
              v-show="uploader.files.size <= maxNumberOfFilesToDisplay"
              :key="file.key">
              <q-item-section v-if="file.status === FileStatus.Queued" avatar>
                <q-icon size="lg" :name="tabDots" class="text-primary-brighter" />
              </q-item-section>
              <q-item-section v-else-if="file.status === FileStatus.Error" avatar>
                <q-icon size="lg" :name="tabAlertTriangle" class="text-error-brighter" />
              </q-item-section>
              <q-item-section v-else-if="file.status === FileStatus.Uploaded" avatar>
                <q-icon size="lg" :name="tabCheck" class="text-success-brighter" />
              </q-item-section>
              <q-item-section v-else-if="file.status === FileStatus.Aborted" avatar>
                <q-icon size="lg" :name="tabBarrierBlock" class="text-alert-brighter" />
              </q-item-section>
              <q-item-section>
                <q-item-label class="full-width ellipsis">
                  {{ file.file.name }}
                </q-item-label>
                <q-item-label caption> Status: {{ file.status.toString() }} </q-item-label>

                <q-item-label caption>
                  {{ file.uploadedSizeSoFarLabel }} uploaded out of {{ file.sizeLabel }} /
                  {{ file.progressLabel }}
                </q-item-label>
              </q-item-section>

              <q-item-section top side>
                <q-btn
                  class="gt-xs"
                  size="1rem"
                  flat
                  round
                  color="negative"
                  :icon="file.status === FileStatus.Error ? tabX : tabTrash"
                  :aria-label="file.status === FileStatus.Error ? 'Retry upload' : 'Remove file'"
                  @click="uploader.removeFile(file)" />
              </q-item-section>
            </q-item>
          </q-list>
          <div v-if="addingFiles">
            <div class="q-pa-md">
              <q-item v-for="i in 4" :key="i">
                <q-item-section>
                  <q-item-label>
                    <q-skeleton type="text" width="50%"></q-skeleton>
                  </q-item-label>
                  <q-item-label caption>
                    <q-skeleton type="text" width="80%"></q-skeleton>
                  </q-item-label>
                </q-item-section>
              </q-item>
            </div>
          </div>
          <div
            v-if="uploader.files.size > maxNumberOfFilesToDisplay"
            class="flex column justify-center items-center full-height">
            <div>
              You've selected <span class="text-bold">{{ uploader.files.size }}</span> files and/or
              directories. For performance reasons, we only display up to
              <span class="text-bold">{{ maxNumberOfFilesToDisplay }}</span
              >.
            </div>
            <div class="text-info text-bold">
              You can still upload your files by clicking the upload button.
            </div>
          </div>
        </div>
      </q-card-section>
      <q-card-actions align="right" class="">
        <q-btn flat label="Close" class="text-xy-secondary" @click="uploader.close()" />
        <q-btn
          label="Upload"
          class="bg-success"
          :disable="!uploader.canUpload || uploader.queuedFilesCount <= 0"
          @click="uploader.upload()" />
      </q-card-actions>
      <q-inner-loading :showing="addingFiles">
        <div class="text-info">Generating upload preview</div>
      </q-inner-loading>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { ref, Ref } from 'vue'
import { bytesToSize } from '@/utils/shared'
import axios, { AxiosRequestConfig } from 'axios'
import {
  tabBarrierBlock,
  tabCheck,
  tabClearAll,
  tabDots,
  tabTrash,
  tabX,
} from 'quasar-extras-svg-icons/tabler-icons-v2'
import { tabAlertTriangle } from 'quasar-extras-svg-icons/tabler-icons'
import { QCard } from 'quasar'
import { tabOutlineUpload } from 'quasar-extras-svg-icons/tabler-icons-v3'

const props = defineProps({
  uploadURL: {
    type: String,
    required: true,
  },
  path: {
    type: String,
    required: true,
  },
  pathSeparator: {
    type: String,
    required: true,
  },
  gameServerId: {
    type: String,
    required: true,
  },
  targetElement: {
    type: HTMLElement,
    default: null,
  },
})

const emits = defineEmits(['uploadedFiles'])
const maxNumberOfFilesToDisplay = 1000
const addingFilesToUploaderViaFileContainerDrop: Ref<boolean> = ref(false) // This is for overriding the default uploader adding files. When this is false, we intercept the file add event and add files manually.
const dragEventInTargetMap: Map<HTMLElement, boolean> = new Map<HTMLElement, boolean>()
const addingFiles: Ref<boolean> = ref(false)
// const worker = new Worker()
//
// worker.onmessage = (event) => {
//   if (event.data === "done") {
//     addingFiles.value = false
//   }
// }

const fileUploaderDialog: Ref<boolean> = defineModel('fileUploaderDialog', {
  type: Boolean,
  default: false,
})

type uploaderFile = {
  file: File
  path: string
  key: string
  status: FileStatus
  uploadedSizeSoFarLabel: string
  sizeLabel: string
  progressLabel: string
}

enum FileStatus {
  Queued = 'Queued',
  Uploaded = 'Uploaded',
  Error = 'Error',
  Aborted = 'Aborted',
}

class FileUploader {
  files: Map<string, uploaderFile> = new Map<string, uploaderFile>()
  queuedFiles: uploaderFile[] = []
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
  concurrentPool = 5
  timeBetweenUploads = 10
  aborted: boolean = false

  constructor() {
    this.setFileProgressDetails = this.setFileProgressDetails.bind(this)
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
    this.abortController = null
    this.files.clear()
    this.uploadSize = 0
    this.uploadedBytes = 0
    this.queuedFilesCount = 0
    this.uploadedFilesCount = 0
    this.canUpload = true
    this.finishedUpload = false
    this.gotError = false
    this.aborted = false
    this.calculateLabels()
  }

  calculateLabels() {
    this.uploadSizeLabel = bytesToSize(this.uploadSize)
    const uploadedBytes =
      this.uploadedBytes <= this.uploadSize ? this.uploadedBytes : this.uploadSize
    this.uploadedSizeSoFarLabel = bytesToSize(uploadedBytes)
    let progress = this.uploadedBytes / this.uploadSize
    if (isNaN(progress)) {
      progress = 0
    }
    let calculatedProgress = progress * 100
    if (calculatedProgress > 100) {
      calculatedProgress = 100
    }
    this.uploadProgressLabel = calculatedProgress.toFixed(2) + '%'
  }

  abort() {
    if (this.abortController !== null) {
      this.abortController.abort()
      this.aborted = true
      this.files.forEach((file) => {
        if (file.status === FileStatus.Queued) {
          this.changeFileStatus(file, FileStatus.Aborted)
        }
      })
    }
  }

  close() {
    this.reset()
    fileUploaderDialog.value = false
  }

  async upload() {
    this.isUploading = true
    this.canUpload = false
    this.abortController = new AbortController()
    this.queuedFiles = Array.from(this.files.values()).filter(
      (file) => file.status !== FileStatus.Uploaded,
    )
    this.queuedFilesCount = this.queuedFiles.length
    if (this.queuedFiles.length === 0) {
      console.debug('No files to upload')
      this.uploadFinish()
      return
    }

    const uploadPromises = []
    for (let i = 0; i < this.concurrentPool; i++) {
      const file = this.queuedFiles.shift()
      if (file) {
        uploadPromises.push(this.uploadFile(file))
      }
    }

    await Promise.all(uploadPromises)
    this.uploadFinish()
  }

  async uploadFile(file: uploaderFile) {
    file.status = FileStatus.Queued
    const formData = new FormData()
    formData.append('gameServerId', props.gameServerId)
    formData.append('path', file.path)
    formData.append('file', file.file)
    if (this.abortController === null) {
      return
    }
    const axiosConfig: AxiosRequestConfig = {
      headers: {
        'Content-Type': 'multipart/form-data',
      },
      signal: this.abortController.signal,
      onUploadProgress: (progressEvent) => {
        this.setFileProgressDetails.bind(this)(file, progressEvent)
        this.calculateLabels()
      },
    }

    await axios
      .post(props.uploadURL, formData, axiosConfig)
      .then(() => {
        this.changeFileStatus(file, FileStatus.Uploaded)
        this.uploadedFilesCount++
        this.queuedFilesCount--
      })
      .catch((error: Error) => {
        if (axios.isCancel(error)) {
          this.changeFileStatus(file, FileStatus.Aborted)
          return
        }
        this.changeFileStatus(file, FileStatus.Error)
        file.progressLabel = '0.00%'
      })
      .finally(async () => {
        if (this.aborted) {
          return
        }
        await this.delay(this.timeBetweenUploads)
        const nextFile = this.queuedFiles.shift()
        if (nextFile) {
          await this.uploadFile(nextFile)
        }
      })
  }

  async delay(time: number) {
    return new Promise((resolve) => setTimeout(resolve, time))
  }

  changeFileStatus(file: uploaderFile, status: FileStatus) {
    file.status = status
  }

  uploadFinish() {
    this.isUploading = false
    this.canUpload = true
    this.finishedUpload = true
    this.abortController = null
    this.aborted = false
    emits('uploadedFiles', this.uploadedFilesCount)
  }

  setFileProgressDetails(file: uploaderFile, progressEvent: any) {
    if (progressEvent.total === undefined) {
      return
    }
    const progress = progressEvent.loaded / progressEvent.total
    file.progressLabel = (progress * 100).toFixed(2) + '%'
    file.uploadedSizeSoFarLabel = bytesToSize(progressEvent.loaded)
    this.uploadedBytes += progressEvent.bytes
  }

  async addFiles(files: FileSystemFileEntry[]) {
    await Promise.all(
      files.map(
        (fileEntry: FileSystemFileEntry) =>
          new Promise<void>((resolve, reject) => {
            fileEntry.file((file: File) => {
              this.addFile(file, fileEntry)
              resolve()
            }, reject)
          }),
      ),
    )
  }

  addFile(file: File, fileEntry: FileSystemFileEntry) {
    if (!file) {
      console.error('File is null')
      return
    }
    if (this.finishedUpload) {
      this.reset()
    }
    if (this.abortController === null) {
      this.abortController = new AbortController()
    }
    const updatedFile = new File([file], getUploaderFileRelativePath(fileEntry), {
      type: file.type,
    })
    if (this.files.has(updatedFile.name)) {
      this.removeOldFile(updatedFile.name)
    }
    const uploaderFile = this.prepareUploadingFile(updatedFile)
    this.files.set(updatedFile.name, uploaderFile)
    this.queuedFilesCount++
    this.uploadSize += updatedFile.size
    this.calculateLabels()
  }

  removeOldFile(fileName: string) {
    const oldFile = this.files.get(fileName)
    this.files.delete(fileName)

    if (oldFile !== undefined) {
      this.uploadSize -= oldFile.file.size
      this.queuedFilesCount--
    }
  }

  prepareUploadingFile(file: File): uploaderFile {
    return {
      file: file,
      path: getUploaderFilePath(file),
      key: file.name,
      status: FileStatus.Queued,
      sizeLabel: bytesToSize(file.size),
      uploadedSizeSoFarLabel: '0.00 MB',
      progressLabel: '0.00%',
    }
  }
}

const uploader = ref(new FileUploader())

// TODO add directories as well. A user might upload an empty directory. We need to handle that.
async function processDirectoryEntry(
  webkitDirEntry: FileSystemDirectoryEntry,
): Promise<FileSystemFileEntry[]> {
  return new Promise((resolveDirectory) => {
    webkitDirEntry.createReader().readEntries(async (entries: FileSystemEntry[]) => {
      const allPromises = entries
        .map((entry: FileSystemEntry) => {
          if (entry.isFile) {
            return processFileEntry(entry as FileSystemFileEntry)
          }
          if (entry.isDirectory) {
            return processDirectoryEntry(entry as FileSystemDirectoryEntry)
          }
        })
        .filter(Boolean) // Filter out any undefined values

      const results = await Promise.allSettled(allPromises)
      const allFilesCombined = results
        .filter((result) => result.status === 'fulfilled')
        .map((result) => (result as PromiseFulfilledResult<FileSystemFileEntry[]>).value)
        .flat()

      resolveDirectory(allFilesCombined)
    })
  })
}

async function processFileEntry(fileEntry: FileSystemFileEntry): Promise<FileSystemFileEntry> {
  return fileEntry
}

async function handleDropEvent(event: DragEvent) {
  if (!event.dataTransfer) {
    return
  }
  if (event.dataTransfer.items) {
    const items = [...event.dataTransfer.items]
    fileUploaderDialog.value = true
    addingFiles.value = true
    const directoryProcessors: (Promise<FileSystemFileEntry[]> | Promise<FileSystemFileEntry>)[] =
      items.map((item: DataTransferItem) => {
        // All the items should finally map to a Promise<File>
        const webkitEntry = item.webkitGetAsEntry()
        if (webkitEntry && webkitEntry.isDirectory) {
          // If it's a directory, process it (this could include nested directories)
          return processDirectoryEntry(webkitEntry as FileSystemDirectoryEntry)
        }
        if (webkitEntry && webkitEntry.isFile) {
          // If it's a file, process it directly
          return processFileEntry(webkitEntry as FileSystemFileEntry)
        }
        return item.webkitGetAsEntry() as unknown as Promise<FileSystemFileEntry>
      })

    const allFiles = (await Promise.all(directoryProcessors)).flat()
    addingFilesToUploaderViaFileContainerDrop.value = false
    await uploader.value.addFiles(allFiles)
    addingFiles.value = false
  }
}

/**
 * Handles the drag enter event.
 *
 * This function is triggered when a dragged item enters a drop target.
 * It prevents the default behavior of the event and adds a specific class to the target element if certain conditions are met.
 * targetElement can be an HTMLElement or a string representing the id of the HTMLElement.
 *
 * @param {DragEvent} event - The drag event object.
 * @param {HTMLElement|string} targetElement - The target element of the drag event. It can be an HTMLElement or a string representing the id of the HTMLElement.
 *
 * @async
 */
async function dragEnterEvent(event: DragEvent, targetElement: HTMLElement | string) {
  event.preventDefault()
  if (typeof targetElement === 'string') {
    const t: HTMLElement | null = document.getElementById(targetElement as string)
    if (t === null) {
      return
    }
    targetElement = t
  }
  const element = targetElement as HTMLElement

  const eventTarget = event.target as HTMLElement
  if (event.dataTransfer === null) {
    return
  }
  if (event.dataTransfer.items.length <= 0) {
    return
  }

  if (event.target != element) {
    dragEventInTargetMap.set(element as HTMLElement, true)
    return
  }
  dragEventInTargetMap.set(element, false)

  if (eventTarget === element) {
    element.classList.add('file-container-drag-over')
  }
}

/**
 * Handles the drag leave event.
 *
 * This function is triggered when a dragged item leaves a drop target.
 * It prevents the default behavior of the event and removes a specific class from the target element if certain conditions are met.
 * targetElement can be an HTMLElement or a string representing the id of the HTMLElement.
 *
 * @param {DragEvent} event - The drag event object.
 * @param {HTMLElement|string} targetElement - The target element of the drag event. It can be an HTMLElement or a string representing the id of the HTMLElement.
 *
 * @async
 */
async function dragLeaveEvent(event: DragEvent, targetElement: HTMLElement | string) {
  event.preventDefault()
  if (typeof targetElement === 'string') {
    const t: HTMLElement | null = document.getElementById(targetElement as string)
    if (t === null) {
      return
    }
    targetElement = t
  }
  const element = targetElement as HTMLElement

  const eventTarget = event.target as HTMLElement
  if (eventTarget !== element) {
    return
  }

  if (dragEventInTargetMap.get(element) === true) {
    return
  }
  dragEventInTargetMap.set(element, false)

  element.classList.remove('file-container-drag-over')
}

/**
 * Handles the drag drop event.
 *
 * This function is triggered when a dragged item is dropped on a drop target.
 * It prevents the default behavior of the event and removes a specific class from the target element if certain conditions are met.
 * targetElement can be an HTMLElement or a string representing the id of the HTMLElement.
 *
 * @param {DragEvent} event - The drag event object.
 * @param {HTMLElement|string} targetElement - The target element of the drag event. It can be an HTMLElement or a string representing the id of the HTMLElement.
 *
 * @async
 */
async function dragDropEvent(event: DragEvent, targetElement: HTMLElement | string) {
  event.preventDefault()
  if (typeof targetElement === 'string') {
    const t: HTMLElement | null = document.getElementById(targetElement as string)
    if (t === null) {
      return
    }
    targetElement = t
  }
  const element = targetElement as HTMLElement
  element.classList.remove('file-container-drag-over')
  dragEventInTargetMap.set(element, false)
  void handleDropEvent(event)
}

function getUploaderFileRelativePath(file: FileSystemFileEntry): string {
  if (!file) {
    console.error('File is null')
    return ''
  }
  let fullPath = file.fullPath
  if (fullPath === '') {
    return file.name
  }
  if (fullPath.startsWith('/') || fullPath.startsWith('\\')) {
    fullPath = fullPath.slice(1)
  }
  return fullPath
}

function getUploaderFilePath(file: File): string {
  let joinedRelativePath = props.path
  if (joinedRelativePath.length > 0) {
    joinedRelativePath += props.pathSeparator
  }
  let relativePath = file.webkitRelativePath
  if (file.webkitRelativePath === '') {
    const lastIndexOfPath = file.name.lastIndexOf('/')
    if (lastIndexOfPath !== -1) {
      relativePath = file.name.slice(0, lastIndexOfPath) + props.pathSeparator
    }
  }
  joinedRelativePath += relativePath
  return joinedRelativePath.slice(0, joinedRelativePath.lastIndexOf(props.pathSeparator))
}
</script>

<style>
.file-uploader {
  height: 50vh;
  overflow: scroll;
}

.fileUploaderDialogCard {
  min-width: 40vw !important;
  font-family: var(--xy-font-mono) !important;
  border: 2px solid var(--xy-surface-1);
}

.file-container-drag-over {
  border: 2px solid var(--q-primary) !important;
}
</style>
