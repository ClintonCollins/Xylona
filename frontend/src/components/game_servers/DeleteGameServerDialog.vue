<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(15%)"
    persistent>
    <q-card>
      <q-card-section>
        <div id="dialog-title" class="text-h6 text-error">Delete Game Server</div>
      </q-card-section>
      <q-card-section>
        <div class="row wrap q-col-gutter-md justify-between">
          <p>
            Are you sure you want to delete
            {{ gameServers.length === 1 ? 'this game server' : 'these game servers' }}?
            <br />
            <span class="text-info">{{ gameServers.map((gs) => gs.name).join(', ') }}</span>
            <br />
            <br />
            <span class="text-bold">This action cannot be undone.</span>
          </p>
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn color="neutral" flat label="Cancel" @click="showDialog = false" />
        <q-btn class="bg-error" label="Delete" @click="deleteGameServers" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { GetXylonaClient } from '@/utils/shared'
import { PropType } from 'vue'
import {
  GameServer,
  RemoveGameServerRequest,
  RemoveGameServerRequestSchema,
} from '@/proto/shared_pb'

const props = defineProps({
  gameServers: {
    type: Array as PropType<GameServer[]>,
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

async function deleteGameServers() {
  for (const gameServer of props.gameServers) {
    const request: RemoveGameServerRequest = create(RemoveGameServerRequestSchema, {})
    request.serverId = gameServer?.id
    try {
      await GetXylonaClient().removeGameServer(request)
      $q.notify({
        caption: `${gameServer?.name} deleted successfully`,
        type: 'xylona-success',
        position: 'top',
        timeout: 5000,
      })
      emit('submit', false)
      return
    } catch (unknownError: unknown) {
      const err = unknownError as Error
      $q.notify({
        caption: `Error deleting ${gameServer.name} ${err.message}`,
        type: 'xylona-error',
        position: 'top',
        timeout: 5000,
      })
      emit('submit', true)
    }
  }
}
</script>

<style scoped></style>
