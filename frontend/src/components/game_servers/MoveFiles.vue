<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="full-width">
      <q-card-section>
        <div id="dialog-title" class="text-h6">Move files</div>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-select
              v-model="destinationDirectory"
              :options="moveOptions"
              class="col-12"
              clearable
              emit-value
              fill-input
              hide-selected
              hint="Press enter after typing a new directory name to create it and move files to it."
              input-debounce="0"
              label="Destination directory"
              map-options
              new-value-mode="add-unique"
              outlined
              use-input>
              <template #prepend>
                <q-icon name="folder" />
              </template>
            </q-select>
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn color="primary" flat label="Cancel" @click="showDialog = false" />
        <q-btn color="primary" label="Submit" @click="moveFiles" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import {
  File as xylonaFile,
  GameServerFilesMoveRequest,
  GameServerFilesMoveRequestSchema,
} from '@/proto/gameserver_files_operations_pb'
import { GetPathSeparator, GetRelativeFilePath, GetXylonaClient } from '@/utils/shared'
import { computed, ref, Ref } from 'vue'

const props = defineProps({
  gameServerId: {
    type: String,
    required: true,
  },
  gameServerPath: {
    type: String,
    required: true,
  },
  path: {
    type: String,
    required: true,
  },
  selectedFiles: {
    type: Array<xylonaFile>,
    default: [],
  },
  neighboringDirectoriesInPath: {
    type: Array<string>,
    default: [],
  },
})

const moveOptions = computed(() => {
  let options = props.neighboringDirectoriesInPath.filter((neighbor: string) => {
    if (props.path === '') {
      return neighbor !== '..'
    }
    return neighbor
  })
  options = options.sort((a: string, b: string) => {
    return a.localeCompare(b)
  })
  return options
})

const destinationDirectory: Ref<string> = ref('')

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

function getDestinationDirectory() {
  if (destinationDirectory.value === '..') {
    const pathSeparator = GetPathSeparator(props.gameServerPath)
    const pathSplit = props.path.split(pathSeparator)
    pathSplit.pop()
    return pathSplit.join(pathSeparator)
  }
  return GetRelativeFilePath(props.gameServerPath, props.path, destinationDirectory.value)
}

async function moveFiles() {
  const request: GameServerFilesMoveRequest = create(GameServerFilesMoveRequestSchema, {})
  request.gameServerId = props.gameServerId
  request.destinationBasePath = getDestinationDirectory()
  request.fullFilePaths = props.selectedFiles.map((file: xylonaFile) => {
    return GetRelativeFilePath(props.gameServerPath, props.path, file.name)
  })
  try {
    await GetXylonaClient().gameServerFilesMove(request)
    emit('submit')
    $q.notify({
      caption: `Files moved to ${destinationDirectory.value} successfully.`,
      type: 'positive',
      position: 'top',
      timeout: 3000,
    })
  } catch (err) {
    console.error(err)
    emit('submit')
    $q.notify({
      caption: `Error moving files ${err}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    showDialog.value = false
    destinationDirectory.value = ''
  }
}
</script>

<style scoped></style>
