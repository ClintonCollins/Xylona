<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(15%)"
    persistent>
    <q-card>
      <q-card-section>
        <div id="dialog-title" class="text-h6 text-error">Delete Game</div>
      </q-card-section>
      <q-card-section>
        <div class="row wrap q-col-gutter-md justify-between">
          <p>
            Are you sure you want to delete {{ game.name }}?
            <span class="text-bold">This action cannot be undone.</span>
          </p>
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn color="neutral" flat label="Cancel" @click="showDialog = false" />
        <q-btn class="bg-error" label="Delete" @click="deleteGame" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { useApiCall } from '@/composables/useApiCall'
import { RemoveGameRequest, RemoveGameRequestSchema } from '@/proto/xylona_pb'
import { GameSchema } from '@/proto/shared_pb'
import { getXylonaClient } from '@/api/connect-client'

const props = defineProps({
  game: {
    type: create(GameSchema),
    required: true,
  },
})

const $q = useQuasar()
const emit = defineEmits<{
  submit: [error: boolean]
}>()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const { execute: executeDelete } = useApiCall(
  () => {
    const request: RemoveGameRequest = create(RemoveGameRequestSchema, {})
    request.gameId = props.game?.id
    return getXylonaClient().removeGame(request)
  },
  { notify: $q.notify, errorPrefix: 'Error deleting game' },
)

async function deleteGame() {
  const result = await executeDelete()
  if (result !== undefined) {
    $q.notify({
      caption: `${props.game?.name} deleted successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    showDialog.value = false
    emit('submit', false)
  } else {
    emit('submit', true)
  }
}
</script>

<style scoped></style>
