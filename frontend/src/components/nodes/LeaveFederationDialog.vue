<template>
  <q-dialog v-model="dialogOpen" aria-labelledby="leave-dialog-title">
    <q-card style="min-width: min(400px, 90vw)">
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
          :loading="leaving"
          color="negative"
          flat
          label="Leave Federation"
          @click="confirmLeave" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Notify } from 'quasar'

import { getXylonaClient } from '@/api/connect-client'
import { useApiCall } from '@/composables/useApiCall'
import { LeaveFederationRequestSchema } from '@/proto/xylona_pb'

defineProps<{
  peerCount: number
}>()

const emit = defineEmits<{
  (e: 'left'): void
}>()

const dialogOpen = defineModel<boolean>({ default: false })

const { loading: leaving, execute: executeLeave } = useApiCall(
  () => getXylonaClient().leaveFederation(create(LeaveFederationRequestSchema, {})),
  {
    notify: (opts) =>
      Notify.create({
        ...opts,
        timeout: 0,
        closeBtn: 'Dismiss',
        icon: 'report_problem',
      }),
  },
)

async function confirmLeave() {
  const result = await executeLeave()
  if (result !== undefined) {
    dialogOpen.value = false
    Notify.create({
      type: 'positive',
      position: 'top',
      message: 'Successfully left the federation.',
      icon: 'check_circle',
    })
    emit('left')
  }
}
</script>

<style scoped></style>
