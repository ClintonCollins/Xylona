<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="full-width">
      <q-form @submit.prevent="createFileOrDirectory">
        <q-card-section>
          <div id="dialog-title" class="text-h6">Create new file or directory</div>
        </q-card-section>
        <q-card-section>
          <div class="q-pa-lg">
            <div class="row wrap q-col-gutter-md justify-between">
              <q-select
                v-model="fileDirType"
                :options="options"
                class="col-12"
                emit-value
                label="File or directory"
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
                outlined
                :rules="[validateName]" />
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
          <q-btn color="primary" label="Create" :loading="submitting" type="submit" />
        </q-card-actions>
      </q-form>
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
const submitting = ref(false)

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

function validateName(value: string): true | string {
  return value.trim() === '' ? 'Enter a name.' : true
}

async function createFileOrDirectory() {
  if (submitting.value) {
    return
  }
  submitting.value = true
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
    emit('submit', true, { fileName: fileName.value, fullFilePath: fullPath, isDir: isDirectory })
    showDialog.value = false
    fileName.value = ''
    fileDirType.value = 'file'
  } catch (err: unknown) {
    console.error(err)
    $q.notify({
      caption:
        err instanceof Error
          ? `Could not create the file or directory. ${err.message}`
          : 'Could not create the file or directory. Try again.',
      type: 'xylona-error',
      position: 'top',
      timeout: 3000,
    })
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped></style>
