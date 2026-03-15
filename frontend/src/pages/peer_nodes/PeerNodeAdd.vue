<template>
    <q-page>
        <div class="row justify-center q-pa-md">
            <q-card class="full-width">
                <q-card-section>
                    <div class="text-h6">Add Peer Node</div>
                </q-card-section>
                <q-card-section>
                    <q-form class="q-pa-lg">
                        <div class="row wrap q-col-gutter-md justify-between">
                            <q-input class="col-12 col-xl-6" outlined type="text" autofocus label="Name (optional)"
                                     v-model="name" hint="Leave blank to use the remote node's name"/>
                            <q-input class="col-12 col-xl-6" outlined type="url" label="Base URL"
                                     v-model="baseUrl" placeholder="http://192.168.1.100:8080"
                                     hint="Full URL including protocol and port"/>
                            <q-input class="col-12" outlined type="text" label="Secret Key"
                                     v-model="secretKey"
                                     hint="The secret key generated on the remote node"/>
                        </div>
                        <div v-if="errorMessage" class="q-mt-md">
                            <q-banner dense class="bg-red-2 text-red-9">
                                {{ errorMessage }}
                            </q-banner>
                        </div>
                    </q-form>
                </q-card-section>
                <q-separator/>
                <q-card-actions class="q-pa-md" align="right">
                    <q-btn flat label="Cancel" @click="cancel"/>
                    <q-btn label="Add Peer" color="primary" @click="submitPeer" :loading="submitting"/>
                </q-card-actions>
            </q-card>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { GetXylonaClient } from '@/utils/shared'
import { AddPeerNodeRequestSchema } from '@/proto/federation_pb'
import { ConnectError } from '@connectrpc/connect'

const router = useRouter()
const name = ref('')
const baseUrl = ref('')
const secretKey = ref('')
const submitting = ref(false)
const errorMessage = ref('')

function cancel() {
  router.back()
}

async function submitPeer() {
  errorMessage.value = ''
  if (!baseUrl.value) {
    errorMessage.value = 'Base URL is required'
    return
  }
  if (!secretKey.value) {
    errorMessage.value = 'Secret Key is required'
    return
  }

  submitting.value = true
  try {
    await GetXylonaClient().addPeerNode(create(AddPeerNodeRequestSchema, {
      name: name.value,
      baseUrl: baseUrl.value,
      secretKey: secretKey.value
    }))
    await router.push('/peer-nodes')
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to add peer node'
    }
    console.error(e)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
</style>
