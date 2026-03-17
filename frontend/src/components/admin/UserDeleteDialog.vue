<template>
  <q-dialog persistent v-model="showDialog" backdrop-filter="brightness(15%)">
    <q-card>
      <q-card-section>
        <q-card-title>
          <div class="text-h6 text-red">Delete User</div>
        </q-card-title>
      </q-card-section>
      <q-card-section>
        <div class="row wrap q-col-gutter-md justify-between">
          <p>
            Are you sure you want to delete {{ user?.userName }}?
            <span class="text-bold">This action cannot be undone.</span>
          </p>
        </div>
      </q-card-section>
      <q-card-actions align="right">
        <q-btn label="Cancel" color="neutral" @click="showDialog = false" flat />
        <q-btn label="Delete" class="bg-error" @click="deleteUser" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { PropType } from 'vue'
import { GetXylonaClient } from '@/utils/shared'
import { User } from '@/proto/xylona_pb'
import { DeleteUserRequest, DeleteUserRequestSchema } from '@/proto/xylona_pb'

const props = defineProps({
  user: {
    type: Object as PropType<User | null>,
    default: null,
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

async function deleteUser() {
  if (!props.user) {
    emit('submit', true)
    return
  }

  const request: DeleteUserRequest = create(DeleteUserRequestSchema, {
    id: props.user.id,
  })

  try {
    await GetXylonaClient().deleteUser(request)
    $q.notify({
      caption: `${props.user.userName} deleted successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    showDialog.value = false
    emit('submit', false)
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    $q.notify({
      caption: `Error deleting user ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
    emit('submit', true)
  }
}
</script>

<style scoped></style>
