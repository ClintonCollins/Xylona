<template>
  <q-dialog
    v-model="showDialog"
    persistent
    backdrop-filter="brightness(15%)"
    aria-labelledby="dialog-title">
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
        <q-btn label="Cancel" color="neutral" flat @click="showDialog = false" />
        <q-btn label="Delete" class="bg-error" @click="deleteGame" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { GetXylonaClient } from '@/utils/shared'
import { RemoveGameRequest, RemoveGameRequestSchema } from '@/proto/xylona_pb'
import { GameSchema } from '@/proto/shared_pb'

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

async function deleteGame() {
  const request: RemoveGameRequest = create(RemoveGameRequestSchema, {})
  request.gameId = props.game?.id
  try {
    await GetXylonaClient().removeGame(request)
    $q.notify({
      caption: `${props.game?.name} deleted successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    showDialog.value = false
    emit('submit', false)
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    $q.notify({
      caption: `Error deleting game ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
    emit('submit', true)
  }
}
</script>

<style scoped></style>
