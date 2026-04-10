<template>
  <q-dialog
    v-model="showDialog"
    aria-labelledby="dialog-title"
    backdrop-filter="brightness(15%)"
    persistent>
    <q-card v-if="!showKey" class="full-width">
      <q-card-section>
        <div class="row">
          <div id="dialog-title" class="text-h6">Create secret key</div>
        </div>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-input
              v-model="keyName"
              autofocus
              class="col-12"
              label="Name"
              outlined
              type="text"></q-input>
          </div>
        </q-form>
      </q-card-section>
      <q-separator></q-separator>
      <q-card-actions align="right" class="q-pa-md">
        <q-btn flat label="Cancel" @click="cancel"></q-btn>
        <q-btn color="primary" label="Save" @click="createSecretKey"></q-btn>
      </q-card-actions>
      <q-inner-loading
        :showing="formSubmitting"
        label="Adding secret key..."
        label-class="text-primary"></q-inner-loading>
    </q-card>
    <q-card v-else>
      <q-card-section>
        <div class="row">
          <div class="text-h6">New Generated Secret key</div>
        </div>
      </q-card-section>
      <q-card-section>
        <q-form class="q-pa-lg">
          <div class="row wrap q-col-gutter-md justify-between">
            <q-input
              v-model="keyName"
              class="col-12"
              disable
              label="Name"
              outlined
              type="text"></q-input>
            <q-input
              v-model="key"
              autogrow
              class="col-12"
              label="Secret Key"
              outlined
              readonly
              type="text">
              <template #append>
                <q-btn
                  :icon="ionClipboard"
                  class="cursor-pointer"
                  flat
                  size="small"
                  @click="copy(key)"></q-btn>
              </template>
            </q-input>
          </div>
        </q-form>
      </q-card-section>
      <q-separator></q-separator>
      <q-card-actions align="right" class="q-pa-md">
        <q-btn flat label="Close" @click="showDialog = false"></q-btn>
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ionClipboard } from '@quasar/extras/ionicons-v7'
import { QBtn, QCard, QCardSection, QDialog, useQuasar } from 'quasar'
import { ConnectError } from '@connectrpc/connect'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { Ref, ref } from 'vue'
import { CreateLocalSecretKeyRequest, CreateLocalSecretKeyRequestSchema } from '@/proto/xylona_pb'
import { useClipboard } from '@vueuse/core'

const formSubmitting = ref(false)
const showKey = ref(false)

const $q = useQuasar()
const emit = defineEmits<{
  submit: [error: boolean]
}>()
const keyName: Ref<string> = ref('')
const key: Ref<string> = ref('')

const { copy } = useClipboard({ key })

const showDialog = defineModel('showDialog', {
  type: Boolean,
  default: false,
})

async function cancel() {
  showDialog.value = false
}

async function createSecretKey() {
  const request: CreateLocalSecretKeyRequest = create(CreateLocalSecretKeyRequestSchema, {})
  request.name = keyName.value
  try {
    const response = await GetXylonaClient().createLocalSecretKey(request)
    key.value = response.secretKey
    showKey.value = true
    emit('submit', false)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to create secret key: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}
</script>

<style scoped></style>
