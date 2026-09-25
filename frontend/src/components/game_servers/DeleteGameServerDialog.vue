<template>
  <q-dialog v-model="showDialog" aria-labelledby="dialog-title" persistent>
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
          <li v-if="deleteBackups">
            {{ gameServers.length === 1 ? 'Its' : 'Their' }} backup archives are permanently deleted
            before {{ gameServers.length === 1 ? 'its folder' : 'their folders' }}. If one can't be
            deleted, {{ gameServers.length === 1 ? 'the server' : 'its server' }} is kept but stays
            stopped, and archives already deleted stay deleted.
          </li>
          <li v-else>
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
        <div class="delete-server-backups">
          <q-checkbox
            v-model="deleteBackups"
            data-testid="delete-server-backups"
            dense
            :aria-describedby="
              !canDeleteBackups || backupSummary ? 'delete-server-backups-detail' : undefined
            "
            :disable="deleting || !canDeleteBackups"
            :label="
              gameServers.length === 1 ? 'Also delete its backups' : 'Also delete their backups'
            " />
          <div
            v-if="!canDeleteBackups"
            id="delete-server-backups-detail"
            class="delete-server-backups__detail">
            Needs the backup permission on
            {{ gameServers.length === 1 ? 'this server' : 'every selected server' }}.
          </div>
          <div
            v-else-if="backupSummary"
            id="delete-server-backups-detail"
            class="delete-server-backups__detail">
            {{ backupSummary }}
          </div>
        </div>
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
import { QBtn, QCard, QCardSection, QDialog } from 'quasar'
import { bytesToSize, GetXylonaClient } from '@/utils/shared'
import { connectErrorMessage } from '@/api/connect-errors'
import { notifyError, notifySuccess } from '@/api/notifications'
import { computed, PropType, ref, watch } from 'vue'
import { RemoveGameServerRequest, RemoveGameServerRequestSchema } from '@/proto/shared_pb'
import { ListGameServerBackupsRequestSchema } from '@/proto/xylona_pb'

export interface DeleteGameServerTarget {
  id: string
  name: string
  nodeName?: string
  directory?: string
  // Deleting backups also needs game_server.backup on the server.
  canDeleteBackups?: boolean
}

const props = defineProps({
  gameServers: {
    type: Array as PropType<DeleteGameServerTarget[]>,
    required: true,
  },
})

// Keeping backups is the safe default, so the option starts unchecked on every open.
const deleteBackups = ref(false)
const canDeleteBackups = computed(
  () =>
    props.gameServers.length > 0 &&
    props.gameServers.every((gameServer) => gameServer.canDeleteBackups === true),
)
const backupSummary = ref('')
let backupSummaryToken = 0

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

watch(
  showDialog,
  async (open) => {
    const token = ++backupSummaryToken
    deleteBackups.value = false
    backupSummary.value = ''
    if (!open || !canDeleteBackups.value) {
      return
    }
    try {
      const responses = await Promise.all(
        props.gameServers.map((gameServer) =>
          GetXylonaClient().listGameServerBackups(
            create(ListGameServerBackupsRequestSchema, { gameServerId: gameServer.id }),
          ),
        ),
      )
      if (token !== backupSummaryToken) {
        return
      }
      const backups = responses.flatMap((response) => response.backups)
      const sizeBytes = backups.reduce((total, backup) => total + backup.sizeBytes, 0n)
      backupSummary.value =
        backups.length === 0
          ? 'No backups recorded.'
          : `${backups.length} ${backups.length === 1 ? 'backup' : 'backups'} · ${bytesToSize(Number(sizeBytes))}`
    } catch (error) {
      // The count is only a hint; the option works without it.
      console.error(error)
    }
  },
  { immediate: true },
)

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
    const request: RemoveGameServerRequest = create(RemoveGameServerRequestSchema, {
      serverId: gameServer.id,
      deleteBackups: deleteBackups.value && canDeleteBackups.value,
    })
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

  if (result.failed.length === 0) {
    notifySuccess(summary, {
      message: `Deleted ${result.succeeded.length} game server${result.succeeded.length === 1 ? '' : 's'}.`,
      timeout: 5000,
      multiLine: true,
    })
  } else {
    // Failures stay until dismissed, so the operator can read every reason.
    notifyError(summary, {
      message: `Deleted ${result.succeeded.length}; ${result.failed.length} failed.`,
      timeout: 0,
      multiLine: true,
      actions: [{ icon: 'close', 'aria-label': 'Dismiss' }],
    })
  }

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
  color: var(--xy-primary-text);
}

.delete-server-backups {
  display: grid;
  gap: var(--xy-space-2xs);
  margin-bottom: var(--xy-space-md);
}

.delete-server-backups__detail {
  padding-left: var(--xy-space-lg);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}
</style>
