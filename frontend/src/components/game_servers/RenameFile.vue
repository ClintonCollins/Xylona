<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(25%)"
    persistent>
    <q-card class="full-width">
      <q-form @submit.prevent="renameFile">
        <q-card-section>
          <div id="dialog-title" class="text-h6">Rename {{ oldFileName }}</div>
        </q-card-section>
        <q-card-section>
          <div class="q-pa-lg">
            <div class="row wrap q-col-gutter-md justify-between">
              <q-input
                v-model="newFileName"
                aria-autocomplete="none"
                autofocus
                class="col-12"
                label="New name"
                name="newFileName"
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
          <q-btn color="primary" label="Rename" :loading="submitting" type="submit" />
        </q-card-actions>
      </q-form>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import {
  GameServerFileRenameRequest,
  GameServerFileRenameRequestSchema,
} from '@/proto/gameserver_files_operations_pb'
import { GetRelativeFilePath, GetXylonaClient } from '@/utils/shared'
import { ref, Ref, watch } from 'vue'

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
  oldFileName: {
    type: String,
    default: '',
  },
})

const newFileName: Ref<string> = ref('')
const submitting = ref(false)

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

watch([showDialog, () => props.oldFileName], ([visible]) => {
  if (visible) {
    newFileName.value = props.oldFileName
  }
})

function validateName(value: string): true | string {
  if (value.trim() === '') {
    return 'Enter a new name.'
  }
  return value === props.oldFileName ? 'Enter a different name.' : true
}

async function renameFile() {
  if (submitting.value) {
    return
  }
  submitting.value = true
  const request: GameServerFileRenameRequest = create(GameServerFileRenameRequestSchema, {})
  request.gameServerId = props.gameServerId
  request.oldPath = GetRelativeFilePath(props.gameServerPath, props.path, props.oldFileName)
  request.newPath = GetRelativeFilePath(props.gameServerPath, props.path, newFileName.value)
  try {
    await GetXylonaClient().gameServerFileRename(request)
    emit('submit')
    $q.notify({
      caption: `${props.oldFileName} renamed to ${newFileName.value} successfully.`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    showDialog.value = false
    newFileName.value = ''
  } catch (err: unknown) {
    console.error(err)
    $q.notify({
      caption:
        err instanceof Error
          ? `Could not rename the file or directory. ${err.message}`
          : 'Could not rename the file or directory. Try again.',
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
