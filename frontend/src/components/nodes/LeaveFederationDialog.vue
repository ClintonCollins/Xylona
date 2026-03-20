<template>
  <q-dialog v-model="dialogOpen" aria-labelledby="leave-dialog-title">
    <q-card style="min-width: 400px">
      <q-card-section>
        <div id="leave-dialog-title" class="text-h6">Leave Federation</div>
      </q-card-section>
      <q-card-section>
        <p>
          This will broadcast a departure notice to
          <strong>{{ peerCount }}</strong>
          {{ peerCount === 1 ? 'peer node' : 'peer nodes' }} and remove all federation pairings.
        </p>
        <p class="text-warning">
          All connected peers will be notified and will remove this node from their federation.
        </p>
        <p class="text-xy-secondary" style="margin-bottom: 0">
          Your local game servers and data will not be affected.
        </p>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn v-close-popup flat label="Cancel" />
        <q-btn
          flat
          label="Leave Federation"
          color="negative"
          :loading="leaving"
          @click="confirmLeave" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { ref } from 'vue'
import { LeaveFederationRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

defineProps<{
  peerCount: number
}>()

const emit = defineEmits<{
  (e: 'left'): void
}>()

const dialogOpen = defineModel<boolean>({ default: false })
const leaving = ref(false)

async function confirmLeave() {
  leaving.value = true
  try {
    await GetXylonaClient().leaveFederation(create(LeaveFederationRequestSchema, {}))
    dialogOpen.value = false
    Notify.create({
      type: 'positive',
      position: 'top',
      message: 'Successfully left the federation.',
      icon: 'check_circle',
    })
    emit('left')
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
    console.error(err.message)
  } finally {
    leaving.value = false
  }
}
</script>

<style scoped></style>
