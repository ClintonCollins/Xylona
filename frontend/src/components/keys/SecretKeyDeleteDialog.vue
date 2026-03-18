<template>
  <q-dialog
    persistent
    v-model="showDialog"
    backdrop-filter="brightness(15%)"
    aria-labelledby="dialog-title">
    <q-card>
      <q-card-section>
        <q-card-title>
          <div id="dialog-title" class="text-h6 text-red">Delete Node</div>
        </q-card-title>
      </q-card-section>
      <q-card-section>
        <div class="row wrap q-col-gutter-md justify-between">
          <p>
            Are you sure you want to delete {{ secretKey.name }}?
            <span class="text-bold">This action cannot be undone.</span>
          </p>
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="Cancel" color="neutral" @click="showDialog = false" flat />
        <q-btn label="Delete" class="bg-error" @click="deleteSecretKey" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { GetXylonaClient } from '@/utils/shared'
import { DeleteLocalSecretKeyRequest, DeleteLocalSecretKeyRequestSchema } from '@/proto/xylona_pb'
import { SecretKey } from '@/proto/shared_pb'
import { PropType } from 'vue'

const props = defineProps({
  secretKey: {
    type: Object as PropType<SecretKey>,
    required: true,
  },
})

const $q = useQuasar()
const emit = defineEmits<{
  submit: [error: Boolean]
}>()

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

async function deleteSecretKey() {
  const request: DeleteLocalSecretKeyRequest = create(DeleteLocalSecretKeyRequestSchema, {})
  request.id = props.secretKey.id
  try {
    await GetXylonaClient().deleteLocalSecretKey(request)
    $q.notify({
      caption: `${props.secretKey?.name} deleted successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    showDialog.value = false
    emit('submit', false)
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    $q.notify({
      caption: `Error deleting secret key ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
    emit('submit', true)
  }
}
</script>

<style scoped></style>
