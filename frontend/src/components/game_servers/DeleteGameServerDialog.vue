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
        <q-btn
          :disable="deleting"
          color="neutral"
          flat
          label="Cancel"
          @click="showDialog = false" />
        <q-btn
          :disable="deleting || gameServers.length === 0"
          :loading="deleting"
          class="bg-error"
          label="Delete"
          @click="deleteGameServers" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { GetXylonaClient } from '@/utils/shared'
import { PropType, ref } from 'vue'
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

interface DeleteFailure {
  id: string
  name: string
  error: string
}

interface DeleteResult {
  succeeded: Array<{ id: string; name: string }>
  failed: DeleteFailure[]
}

const emit = defineEmits<{
  submit: [result: DeleteResult]
}>()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

const deleting = ref(false)

async function deleteGameServers() {
  if (deleting.value) {
    return
  }

  deleting.value = true
  const result: DeleteResult = {
    succeeded: [],
    failed: [],
  }

  for (const gameServer of props.gameServers) {
    const request: RemoveGameServerRequest = create(RemoveGameServerRequestSchema, {})
    request.serverId = gameServer.id
    try {
      await GetXylonaClient().removeGameServer(request)
      result.succeeded.push({ id: gameServer.id, name: gameServer.name })
    } catch (unknownError: unknown) {
      result.failed.push({
        id: gameServer.id,
        name: gameServer.name,
        error: deleteFailureMessage(unknownError),
      })
    }
  }

  const summary = [
    ...result.succeeded.map((server) => `Deleted: ${server.name}`),
    ...result.failed.map((failure) => `Failed: ${failure.name} — ${failure.error}`),
  ].join('\n')

  $q.notify({
    message:
      result.failed.length === 0
        ? `Deleted ${result.succeeded.length} game server${result.succeeded.length === 1 ? '' : 's'}.`
        : `Deleted ${result.succeeded.length}; ${result.failed.length} failed.`,
    caption: summary,
    type: result.failed.length === 0 ? 'xylona-success' : 'xylona-error',
    position: 'top',
    timeout: result.failed.length === 0 ? 5000 : 0,
    multiLine: true,
    actions: result.failed.length === 0 ? undefined : [{ icon: 'close' }],
  })

  deleting.value = false
  showDialog.value = false
  emit('submit', result)
}

function deleteFailureMessage(error: unknown): string {
  if (error instanceof Error && error.message !== '') {
    return error.message
  }
  if (typeof error === 'string' && error !== '') {
    return error
  }
  return 'Unknown error'
}
</script>

<style scoped></style>
