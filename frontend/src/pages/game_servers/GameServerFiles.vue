<template>
  <q-card-section>
    <div class="row">
      <div class="text-h6">Your Game Server</div>
    </div>
  </q-card-section>
  <q-card-section>
    <div class="row q-px-md justify-center">
      <div v-on:drop="fileContainerDrop" v-on:dragover="fileContainerDrag"
           class="col-xs-12 file-list-container bg-neutral-glass-4 q-pa-sm" style="border-radius: .5rem">
        <div class="row q-px-sm">
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
        <div class="row file-list-body-row q-px-sm" v-for="directory in directories">
          <div class="col-xs-2 col-md-2 col-lg-1">
            <q-checkbox :val="directory" v-model="selectedFiles"></q-checkbox>
          </div>
          <div class="col-xs-4 file-div" @click="clickDirectory(directory)">
            <q-icon size="xs" color="amber" :name="tabFolderFilled" left></q-icon>
            {{ directory.name }}
          </div>
          <div class="col-xs-3">{{ bytesToSize(Number(directory.size)) }}</div>
          <div class="col-xs-3">{{ toTimestamp(directory.lastModified) }}</div>
        </div>
        <div class="row file-list-body-row q-px-sm" v-for="file in files">
          <div class="col-xs-2 col-md-2 col-lg-1">
            <q-checkbox :val="file" v-model="selectedFiles"></q-checkbox>
          </div>
          <div class="col-xs-4 file-div" @click="clickFile(file)">
            <q-icon size="xs" :style="'color:'+ getColorFromFilenameExtension(file.name)"
                    :name="getIconFromFilenameExtension(file.name)" left></q-icon>
            <span class="file-name">{{ file.name }}</span>
          </div>
          <div class="col-xs-3">{{ bytesToSize(Number(file.size)) }}</div>
          <div class="col-xs-3">{{ toTimestamp(file.lastModified) }}</div>
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
import {computed, onMounted, Ref, ref, watch} from "vue";
import dayjs from "dayjs";
import {Timestamp} from "@bufbuild/protobuf";
import {
  tabArchive,
  tabFileFilled,
  tabFileSettings,
  tabFileTypeTxt,
  tabFilterSearch,
  tabFolderFilled,
  tabJson
} from "quasar-extras-svg-icons/tabler-icons-v2";
import {Code, ConnectError} from "@connectrpc/connect";
import {QMenu} from "quasar";

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

onMounted(async () => {
  await listDirectoryFiles(path.value)
  await getGameServerDetails()
})

watch(selectAllFiles, (newValue) => {
  if (newValue) {
    selectedFiles.value = directories.value.concat(files.value)
    return
  }
  if (selectedFiles.value.length === directories.value.length + files.value.length) {
    selectedFiles.value = []
  }
})

watch(selectedFiles, (newValue) => {
  if (selectAllFiles) {
    selectAllFiles.value = newValue.length === directories.value.length + files.value.length;
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
  console.log(file.name)
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
  const fileRequest = new GetFileRequest()
  fileRequest.gameServerId = gameServerId.value
  fileRequest.path = filePath
  try {
    console.log(fileRequest.toJsonString())
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
  return dayjs(date.toDate()).format("M/D/YYYY H:m:ss A")
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

function fileContainerDrop(event: DragEvent) {
  event.preventDefault()
  console.log(event)
  if (event.dataTransfer.items) {
    const items = event.dataTransfer.items
    for (let i = 0; i < items.length; i++) {
      if (items[i].kind === 'file') {
        const file = items[i].getAsFile()
        console.log(file)
      }

    }
  }
}

function fileContainerDrag(event: DragEvent) {
  event.preventDefault()
  // console.log(event)
}

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
  min-width: 720px;
  height: 2.5rem;
  font-size: 1rem;
}

.file-list-body-row:hover {
  background-color: var(--bg-neutral-glass-3);
}

.file-list-container {
  max-height: 80vh;
  overflow: scroll;
  font-family: "Oxygen Mono", monospace;
}

.file-div {
  height: 100% !important;
  display: flex;
  align-items: center;
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
</style>
