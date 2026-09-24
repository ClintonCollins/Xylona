<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(15%)"
    persistent>
    <q-card class="delete-server-dialog">
      <q-card-section>
        <div id="dialog-title" class="text-h6 text-negative">
          {{
            gameServers.length === 1
              ? 'Delete Game Server'
              : `Delete ${gameServers.length} Game Servers`
          }}
        </div>
      </q-card-section>
      <q-card-section class="delete-server-body">
        <p>
          This permanently deletes the {{ gameServers.length === 1 ? 'server' : 'servers' }} and
          {{ gameServers.length === 1 ? 'its folder' : 'their folders' }} on the node, including
          worlds, saves, configs and mods:
        </p>
        <ul class="delete-server-targets">
          <li v-for="gameServer in gameServers" :key="gameServer.id">
            <strong>{{ gameServer.name }}</strong>
            <span v-if="gameServer.nodeName"> on {{ gameServer.nodeName }}</span>
            <code v-if="gameServer.directory">{{ gameServer.directory }}</code>
          </li>
        </ul>
        <ul class="delete-server-consequences">
          <li>
            Xylona stops {{ gameServers.length === 1 ? 'the server' : 'each server' }} first if it
            is running, which disconnects its players.
          </li>
          <li>
            Backup archives stay on disk, but Xylona stops listing them. To keep one, download it
            from
            <router-link
              v-if="gameServers.length === 1 && gameServers[0]"
              :to="`/game-servers/${gameServers[0].id}/backups`"
              @click="showDialog = false"
              >Backups</router-link
            ><template v-else>each server's Backups tab</template> first.
          </li>
          <li>DNS records stay at the provider after their local bindings are removed.</li>
        </ul>
        <p class="text-weight-bold">This cannot be undone.</p>
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
          :label="
            gameServers.length === 1 ? 'Delete server' : `Delete ${gameServers.length} servers`
          "
          :loading="deleting"
          color="negative"
          unelevated
          @click="deleteGameServers" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { GetXylonaClient } from '@/utils/shared'
import { connectErrorMessage } from '@/api/connect-errors'
import { PropType, ref } from 'vue'
import { RemoveGameServerRequest, RemoveGameServerRequestSchema } from '@/proto/shared_pb'

export interface DeleteGameServerTarget {
  id: string
  name: string
  nodeName?: string
  directory?: string
}

const props = defineProps({
  gameServers: {
    type: Array as PropType<DeleteGameServerTarget[]>,
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
    return connectErrorMessage(error)
  }
  if (typeof error === 'string' && error !== '') {
    return error
  }
  return 'Unknown error'
}
</script>

<style scoped>
.delete-server-dialog {
  width: min(34rem, 100%);
}

.delete-server-body p {
  margin: 0;
}

.delete-server-body ul {
  margin: var(--xy-space-xs) 0 var(--xy-space-md);
  padding-left: var(--xy-space-lg);
}

.delete-server-body li + li {
  margin-top: var(--xy-space-2xs);
}

.delete-server-targets code {
  display: block;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  overflow-wrap: anywhere;
}

.delete-server-consequences {
  color: var(--xy-text-secondary);
}

.delete-server-consequences a {
  color: var(--xy-primary);
}
</style>
