<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(15%)"
    persistent>
    <q-card class="game-delete-dialog">
      <q-card-section>
        <div id="dialog-title" class="text-h6 font-display text-negative">Delete Game</div>
      </q-card-section>
      <q-card-section class="game-delete-dialog__body">
        <p>Are you sure you want to delete {{ game.name }}?</p>
        <q-banner
          v-if="usedBy.length > 0"
          class="xy-banner-warning"
          data-test="game-in-use"
          dense
          rounded>
          <template #avatar>
            <q-icon name="dns" />
          </template>
          Used by {{ usedBy.length }} game server{{ usedBy.length === 1 ? '' : 's' }}:
          {{ usedBy.join(', ') }}. Delete or move those servers to another game first.
        </q-banner>
        <p v-else-if="game.xylonaOfficial" data-test="official-note">
          This is an official game, so it will be restored the next time the controller starts.
        </p>
        <p v-else class="text-bold">This action cannot be undone.</p>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn color="neutral" flat label="Cancel" @click="showDialog = false" />
        <q-btn
          :disable="checkingUsage || usedBy.length > 0"
          :loading="checkingUsage"
          class="bg-error"
          label="Delete"
          @click="deleteGame" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ref, watch } from 'vue'
import { QBtn, QCard, QCardSection, QDialog } from 'quasar'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import { ListGameServersRequestSchema, RemoveGameRequestSchema } from '@/proto/xylona_pb'
import { GameSchema } from '@/proto/shared_pb'
import { getXylonaClient } from '@/api/connect-client'

const props = defineProps({
  game: {
    type: create(GameSchema),
    required: true,
  },
})

const emit = defineEmits<{
  submit: [error: boolean]
}>()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

// Names of the game servers built on this game; the controller refuses to delete it while any exist.
const usedBy = ref<string[]>([])
const checkingUsage = ref(false)

watch(
  [showDialog, () => props.game.id],
  async ([open]) => {
    usedBy.value = []
    if (!open) {
      return
    }

    checkingUsage.value = true
    try {
      const response = await getXylonaClient().listGameServers(
        create(ListGameServersRequestSchema, {}),
      )
      usedBy.value = response.gameServers
        .filter((server) => server.gameId === props.game.id)
        .map((server) => server.name)
    } catch (error: unknown) {
      // The controller still refuses a delete that would orphan servers, so the check can fail open.
      notifyConnectError(error, 'Could not check which game servers use this game')
    } finally {
      checkingUsage.value = false
    }
  },
  { immediate: true },
)

async function deleteGame() {
  try {
    const request = create(RemoveGameRequestSchema, { gameId: props.game.id })
    await getXylonaClient().removeGame(request)
    notifySuccess(`${props.game.name} deleted successfully`, { timeout: 5000 })
    showDialog.value = false
    emit('submit', false)
  } catch (error: unknown) {
    notifyConnectError(error, 'Error deleting game')
    emit('submit', true)
  }
}
</script>

<style scoped>
.game-delete-dialog {
  width: min(480px, calc(100vw - 2rem));
}

.game-delete-dialog__body {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
  padding-top: 0;
}

.game-delete-dialog__body p {
  margin: 0;
}
</style>
