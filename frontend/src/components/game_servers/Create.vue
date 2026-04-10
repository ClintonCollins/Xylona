<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="full-width">
      <q-card-section>
        <div id="dialog-title" class="text-h6">Create new file or directory</div>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-select
              v-model="fileDirType"
              :options="options"
              class="col-12"
              emit-value
              label="File or Directory"
              map-options
              outlined>
              <template #prepend>
                <q-icon name="event" />
              </template>
            </q-select>
            <q-input
              v-model="fileName"
              aria-autocomplete="none"
              autofocus
              class="col-12"
              label="Name"
              name="fileName"
              outlined />
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn color="primary" flat label="Cancel" @click="showDialog = false" />
        <q-btn color="primary" label="Submit" @click="createFileOrDirectory" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, QInput, useQuasar } from 'quasar'
import {
  GameServerFileOrDirectoryCreateRequest,
  GameServerFileOrDirectoryCreateRequestSchema,
} from '@/proto/gameserver_files_operations_pb'
import { GetRelativeFilePath, GetXylonaClient } from '@/utils/shared'
import { ref, Ref } from 'vue'

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
})

const options = [
  { label: 'File', value: 'file' },
  { label: 'Directory', value: 'directory' },
]
const fileName: Ref<string> = ref('')
const fileDirType: Ref<string> = ref('file')

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

async function createFileOrDirectory() {
  const request: GameServerFileOrDirectoryCreateRequest = create(
    GameServerFileOrDirectoryCreateRequestSchema,
    {},
  )
  const fullPath = GetRelativeFilePath(props.gameServerPath, props.path, fileName.value)
  const isDirectory = fileDirType.value === 'directory'
  request.gameServerId = props.gameServerId
  request.fullFilePath = fullPath
  request.content = ''
  request.isDirectory = isDirectory
  try {
    await GetXylonaClient().gameServersFileOrDirectoryCreate(request)
    emit('submit', true, { fileName: fileName, fullFilePath: fullPath, isDir: isDirectory })
  } catch (err) {
    console.error(err)
    emit('submit', false, null)
    $q.notify({
      caption: `Error creating file or directory. ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 3000,
    })
  } finally {
    showDialog.value = false
    fileName.value = ''
    fileDirType.value = 'file'
  }
}
</script>

<style scoped></style>
