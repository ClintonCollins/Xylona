<template>
  <q-dialog
    persistent
    v-model="showDialog"
    backdrop-filter="brightness(25%)"
    aria-labelledby="dialog-title">
    <q-card class="full-width">
      <q-card-section>
        <q-card-title>
          <div id="dialog-title" class="text-h6">Rename {{ oldFileName }}</div>
        </q-card-title>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-input
              name="newFileName"
              aria-autocomplete="none"
              class="col-12"
              outlined
              v-model="newFileName"
              label="New name"
              autofocus />
          </div>
        </q-form>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="Cancel" color="primary" @click="showDialog = false" flat />
        <q-btn label="Submit" color="primary" @click="renameFile" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, QInput, useQuasar } from 'quasar'
import {
  GameServerFileRenameRequest,
  GameServerFileRenameRequestSchema,
} from 'src/proto/gameserver_files_operations_pb'
import { GetRelativeFilePath, GetXylonaClient } from 'src/utils/shared'
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
  oldFileName: {
    type: String,
    required: true,
  },
})

const newFileName: Ref<string> = ref('')

const $q = useQuasar()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const emit = defineEmits(['submit'])

async function renameFile() {
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
  } catch (err) {
    console.error(err)
    emit('submit')
    $q.notify({
      caption: `Error renaming file or directory. ${err}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 3000,
    })
  } finally {
    showDialog.value = false
    newFileName.value = ''
  }
}
</script>

<style scoped></style>
