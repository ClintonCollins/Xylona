<template>
  <q-dialog v-model="showDialog" aria-labelledby="dialog-title" persistent>
    <q-card class="full-width">
      <q-form @submit.prevent="moveFiles">
        <q-card-section>
          <div id="dialog-title" class="text-h6">{{ dialogTitle }}</div>
        </q-card-section>
        <q-card-section>
          <div class="q-pa-lg">
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
                :rules="[validateDestination]"
                aria-required="true"
                use-input>
                <template #prepend>
                  <q-icon name="folder" />
                </template>
              </q-select>
            </div>
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn
            color="primary"
            :disable="submitting"
            flat
            label="Cancel"
            @click="showDialog = false" />
          <q-btn color="primary" label="Move" :loading="submitting" type="submit" />
        </q-card-actions>
      </q-form>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog } from 'quasar'
import {
  File as xylonaFile,
  GameServerFilesMoveRequest,
  GameServerFilesMoveRequestSchema,
} from '@/proto/gameserver_files_operations_pb'
import { GetPathSeparator, GetRelativeFilePath, GetXylonaClient } from '@/utils/shared'
import { connectErrorMessage } from '@/api/connect-errors'
import { notifyError, notifySuccess } from '@/api/notifications'
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

const dialogTitle = computed(() => {
  const count = props.selectedFiles.length
  const items = count === 1 ? '1 item' : `${count} items`
  return `Move ${items} from ${props.path === '' ? 'the server root' : props.path}`
})

// A folder cannot be moved into itself, and '..' is the only way up.
const moveOptions = computed(() => {
  const moving = new Set(props.selectedFiles.map((file: xylonaFile) => file.name))
  const options = props.neighboringDirectoriesInPath
    .filter((neighbor: string) => neighbor !== '..' && !moving.has(neighbor))
    .sort((a: string, b: string) => a.localeCompare(b))
    .map((neighbor: string) => ({ label: neighbor, value: neighbor }))
  if (props.path !== '') {
    options.unshift({ label: '.. (parent folder)', value: '..' })
  }
  return options
})

const destinationDirectory: Ref<string> = ref('')
const submitting = ref(false)

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

function validateDestination(value: string): true | string {
  return value?.trim() ? true : 'Choose or enter a destination directory.'
}

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
  if (submitting.value || validateDestination(destinationDirectory.value) !== true) {
    return
  }
  submitting.value = true
  const request: GameServerFilesMoveRequest = create(GameServerFilesMoveRequestSchema, {})
  request.gameServerId = props.gameServerId
  request.destinationBasePath = getDestinationDirectory()
  request.fullFilePaths = props.selectedFiles.map((file: xylonaFile) => {
    return GetRelativeFilePath(props.gameServerPath, props.path, file.name)
  })
  try {
    await GetXylonaClient().gameServerFilesMove(request)
    emit('submit')
    notifySuccess(
      `Moved to ${destinationDirectory.value === '..' ? 'the parent folder' : destinationDirectory.value}.`,
    )
    showDialog.value = false
    destinationDirectory.value = ''
  } catch (err: unknown) {
    console.error(err)
    notifyError(
      err instanceof Error
        ? `Could not move the selected items. ${connectErrorMessage(err)}`
        : 'Could not move the selected items. Try again.',
    )
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped></style>
