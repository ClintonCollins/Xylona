<template>
  <section ref="dragAndDropZone"
           v-on:dragenter="dragEnterEvent($event, props.targetElement)"
           v-on:dragleave="dragLeaveEvent($event, props.targetElement)"
           v-on:drop="dragDropEvent($event, props.targetElement)"
           v-on:dragover="$event.preventDefault()">
    <slot
    ></slot>
  </section>
  <q-dialog v-model="fileUploaderDialog" backdrop-filter="brightness(25%)" @hide="uploader.close()">
    <q-card id="fileUploaderCard" class="fileUploaderDialogCard q-px-lg"
            v-on:dragenter="dragEnterEvent($event, 'fileUploaderCard')"
            v-on:dragleave="dragLeaveEvent($event, 'fileUploaderCard')"
            v-on:drop="dragDropEvent($event, 'fileUploaderCard')"
            v-on:dragover="$event.preventDefault()">
      <q-card-section>
        <div class="text-h6">File Upload (You can drag and drop files below)</div>
        <q-toolbar class="bg-blue-grey-10 q-py-sm">
          <q-btn v-if="uploader.files.size > 0" :icon="tabClearAll" class="text-white bg-alert-brighter"
                 @click="uploader.removeQueuedFiles()">
            <q-tooltip>Clear All</q-tooltip>
          </q-btn>
          <q-spinner v-if="uploader.isUploading" class="q-uploader__spinner"/>
          <div class="col q-ml-md">
            <div class="q-uploader__title">Upload your files</div>
            <div class="q-uploader__subtitle">{{ uploader.uploadedSizeSoFarLabel }} uploaded out of
              {{ uploader.uploadSizeLabel }} / {{ uploader.uploadProgressLabel }}
            </div>
            <div class="q-uploader__subtitle">{{ uploader.queuedFilesCount }} files in queue /
              {{ uploader.uploadedFilesCount }} files uploaded
            </div>
          </div>
          <q-btn v-if="uploader.queuedFilesCount > 0 && uploader.canUpload" class="text-white bg-success-darker"
                 :icon="tabOutlineUpload"
                 @click="uploader.upload()">
            <q-tooltip>Upload Files</q-tooltip>
          </q-btn>

          <q-btn v-if="uploader.isUploading" icon="clear" @click="uploader.abort()" round dense flat>
            <q-tooltip>Abort Upload</q-tooltip>
          </q-btn>
        </q-toolbar>

        <div class="file-uploader">
          <q-list separator>
            <q-item v-if="uploader.files.size <= maxNumberOfFilesToDisplay" v-for="file in uploader.files.values()" :key="file.key">
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
          <div v-if="uploader.files.size > maxNumberOfFilesToDisplay" class="flex column justify-center items-center full-height">
            <div>You've selected <span class="text-bold">{{ uploader.files.size }}</span> files and/or directories. For performance reasons, we only display up to <span class="text-bold">{{ maxNumberOfFilesToDisplay }}</span>.</div>
            <div class="text-info text-bold">You can still upload your files by clicking the upload button.</div>
          </div>
        </div>
      </q-card-section>
      <q-card-actions align="right" class="">
        <q-btn flat label="Close" color="grey" @click="uploader.close()"/>
        <q-btn label="Upload" class="bg-success" @click="uploader.upload()"
               :disable="!uploader.canUpload || uploader.queuedFilesCount <= 0"/>
      </q-card-actions>
      <q-inner-loading :showing="addingFiles">
        <div class="text-info">Generating upload preview</div>
      </q-inner-loading>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import {ref, Ref} from "vue";
import {bytesToSize} from "src/utils/shared";
import axios, {AxiosRequestConfig} from "axios";
import {tabCheck, tabClearAll, tabDots, tabTrash, tabX} from "quasar-extras-svg-icons/tabler-icons-v2";
import {tabAlertTriangle} from "quasar-extras-svg-icons/tabler-icons";
import {QCard} from "quasar";
import {tabOutlineUpload} from "quasar-extras-svg-icons/tabler-icons-v3";

const props = defineProps({
  uploadURL: {
    type: String,
    required: true
  },
  path: {
    type: String,
    required: true
  },
  pathSeparator: {
    type: String,
    required: true
  },
  gameServerId: {
    type: String,
    required: true
  },
  targetElement: {
    type: HTMLElement,
    required: true
  }
})

const emits = defineEmits(['uploadedFiles'])
const maxNumberOfFilesToDisplay = 1000
const addingFilesToUploaderViaFileContainerDrop: Ref<boolean> = ref(false) // This is for overriding the default uploader adding files. When this is false, we intercept the file add event and add files manually.
const dragEventInTargetMap: Map<HTMLElement, boolean> = new Map<HTMLElement, boolean>()
const addingFiles: Ref<boolean> = ref(false)

const fileUploaderDialog: Ref<boolean> = defineModel('fileUploaderDialog', {
  type: Boolean,
  default: false
})

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

class FileUploader {
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
      this.abortController = null
      emits('uploadedFiles', this.uploadedFilesCount)
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
    this.abortController = null
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

  async upload() {
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
    formData.append('file', file.file)
    formData.append('fileName', file.file.name)
    formData.append('path', file.path)
    formData.append('gameServerId', props.gameServerId)
    const axiosConfig: AxiosRequestConfig = {
      headers: {
        'Content-Type': 'multipart/form-data'
      },
      signal: this.abortController.signal,
      onUploadProgress: (progressEvent) => {
        if (progressEvent.total === undefined) {
          return
        }
        const progress = (progressEvent.loaded / progressEvent.total)
        file.progressLabel = (progress * 100).toFixed(2) + "%"
        file.uploadedSizeSoFarLabel = bytesToSize(progressEvent.loaded)
        this.uploadedBytes += progressEvent.bytes
        this.calculateLabels()
      }
    }

    axios.post(props.uploadURL, formData, axiosConfig).then(() => {
      file.status = "Uploaded"
      file.success = true
      this.uploadedFilesCount++
      this.queuedFilesCount--
    }).catch((error) => {
      console.error(error)
      file.status = "Error"
      file.progressLabel = "0.00%"
      file.error = true
      this.gotError = true
    }).finally(() => {
      this.checkFilesLeftToUpload()
      this.calculateLabels()
    })
  }

  async addFiles(files: File[]) {
    await Promise.all(files.map(file => this.addFile(file)));
  }

  addFile(file: File) {
    if (this.finishedUpload) {
      this.reset();
    }
    if (this.abortController === null) {
      this.abortController = new AbortController();
    }

    const updatedFile = new File([file], getUploaderFileRelativePath(file), {type: file.type});
    if (this.files.has(updatedFile.name)) {
      this.removeOldFile(updatedFile.name);
    }

    const uploaderFile = this.prepareUploadingFile(updatedFile);
    this.files.set(updatedFile.name, uploaderFile);
    this.queuedFilesCount++;
    this.uploadSize += updatedFile.size;
    this.calculateLabels();
  }

  removeOldFile(fileName: string) {
    const oldFile = this.files.get(fileName);
    this.files.delete(fileName);

    if (oldFile !== undefined) {
      this.uploadSize -= oldFile.file.size;
      this.queuedFilesCount--;
    }
  }

  prepareUploadingFile(file: File): uploaderFile {
    return {
      file: file,
      path: getUploaderFilePath(file),
      key: file.name,
      status: "Queued",
      error: false,
      success: false,
      sizeLabel: bytesToSize(file.size),
      uploadedSizeSoFarLabel: "0.00 MB",
      progressLabel: "0.00%"
    };
  }
}

const uploader = ref(new FileUploader())

// TODO add directories as well. A user might upload an empty directory. We need to handle that.
async function processDirectoryEntry(webkitDirEntry: any): Promise<any> {
  return new Promise((resolveDirectory) => {
    webkitDirEntry.createReader().readEntries(async (entries: any[]) => {
      const allPromises = entries.map(async (entry) => {
        if (entry.isFile) {
          return processFileEntry(entry);
        }
        else if (entry.isDirectory) {
          // This is a directory. It's handled recursively. It could return multiple files as an array
          return await processDirectoryEntry(entry);
        }
      });

      // Collect all the file promises (including nested file promises), then flatten them into a single array.
      const allFilesCombined = (await Promise.all(allPromises)).flat();
      resolveDirectory(allFilesCombined);
    });
  });
}

async function processFileEntry(fileEntry: any): Promise<File> {
  return new Promise((resolveFile) => {
    fileEntry.file((file: File) => {
      resolveFile(file) // The promise is resolved with the file
    });
  });
}

async function handleDropEvent(event: DragEvent) {
  if (event.dataTransfer?.items) {
    const items = [...event.dataTransfer.items];
    fileUploaderDialog.value = true
    addingFiles.value = true
    const directoryProcessors: Promise<File>[] = items.map(item => { // All the items should finally map to a Promise<File>
      const webkitEntry = item.webkitGetAsEntry() as FileSystemFileEntry;

      if (webkitEntry.isDirectory) {
        // If it's a directory, process it (this could include nested directories)
        return processDirectoryEntry(webkitEntry);
      }
      else if (webkitEntry.isFile) {
        // If it's a file, process it directly
        return processFileEntry(webkitEntry);
      }
    });

    const allFiles = (await Promise.all(directoryProcessors)).flat();
    addingFilesToUploaderViaFileContainerDrop.value = false;
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
    element.classList.add("file-container-drag-over")
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

  element.classList.remove("file-container-drag-over")
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
  element.classList.remove("file-container-drag-over")
  dragEventInTargetMap.set(element, false)
  void handleDropEvent(event)
}

function getUploaderFileRelativePath(file: File): string {
  if (file.webkitRelativePath === "") {
    return file.name
  }
  return file.webkitRelativePath
}

function getUploaderFilePath(file: File): string {
  let joinedRelativePath = props.path
  if (joinedRelativePath.length > 0) {
    joinedRelativePath += props.pathSeparator
  }
  let relativePath = file.webkitRelativePath
  if (file.webkitRelativePath === "") {
    const lastIndexOfPath = file.name.lastIndexOf("/")
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
  font-family: "Oxygen Mono", monospace !important;
  border: 2px solid var(--bg-neutral-opaque);
}

.file-container-drag-over {
  border: 2px solid var(--q-primary) !important;
}
</style>
