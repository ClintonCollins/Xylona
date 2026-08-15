<template>
  <q-card class="full-width">
    <q-card-section>
      <div class="text-h6" v-text="existingNodeId ? 'Edit Node' : 'Add Remote Node'"></div>
    </q-card-section>

    <q-card-section v-if="existingNodeId">
      <q-form class="q-pa-lg" @submit.prevent="updateNode">
        <div class="row wrap q-col-gutter-md">
          <q-input
            v-model="node.name"
            autofocus
            class="col-12 col-xl-6"
            label="Name"
            outlined
            type="text"></q-input>
          <q-input
            v-model="node.baseUrl"
            class="col-12 col-xl-6"
            hint="HTTPS URL the controller uses to reach this node"
            label="Listen URL"
            outlined
            placeholder="https://node.example.com:9500"
            type="url"></q-input>
        </div>
        <div v-if="errorMessage" class="text-negative q-mt-md">{{ errorMessage }}</div>
        <div class="row q-mt-md">
          <q-btn flat label="Cancel" @click="cancel"></q-btn>
          <q-space />
          <q-btn :loading="formSubmitting" color="primary" label="Save" type="submit"></q-btn>
        </div>
      </q-form>
    </q-card-section>

    <q-card-section v-else>
      <div class="text-body2 q-mb-md">
        Adding a node is a two-step process:
        <ol>
          <li>Generate a node join command here.</li>
          <li>Run the generated command on the target host.</li>
        </ol>
        The node contacts the controller, exchanges its self-signed cert, and shows up in the node
        list.
      </div>

      <q-btn
        :loading="pairingKeySubmitting"
        color="primary"
        label="Generate Join Command"
        @click="generateJoinToken"></q-btn>

      <q-card v-if="generatedJoinCommand !== ''" class="q-mt-md" flat bordered>
        <q-card-section>
          <div class="text-subtitle1">Node Join Command</div>
          <div class="text-caption q-mb-sm">
            One-time use. Expires in approximately 2 hours. Run this exact command on the target
            host.
          </div>
          <q-input
            :model-value="generatedJoinCommand"
            dense
            filled
            readonly
            type="textarea"
            autogrow></q-input>
          <q-btn class="q-mt-sm" flat icon="content_copy" label="Copy" @click="copyCommand"></q-btn>
        </q-card-section>
      </q-card>

      <div v-if="errorMessage" class="text-negative q-mt-md">{{ errorMessage }}</div>
      <div class="row q-mt-md">
        <q-btn flat label="Back" @click="cancel"></q-btn>
      </div>
    </q-card-section>
  </q-card>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { copyToClipboard, useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { NodeSchema } from '@/proto/shared_pb'
import {
  type EditNodeRequest,
  EditNodeRequestSchema,
  GenerateNodePairingObjectRequestSchema,
  type GetNodeRequest,
  GetNodeRequestSchema,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const router = useRouter()
const $q = useQuasar()

const props = defineProps({
  existingNodeId: {
    type: String,
    required: false,
    default: undefined,
  },
})

const node = ref(create(NodeSchema, {}))
const formSubmitting = ref(false)
const errorMessage = ref('')
const generatedPairingKey = ref('')
const generatedControllerURL = ref('')
const pairingKeySubmitting = ref(false)

const generatedJoinCommand = computed(() => {
  if (generatedControllerURL.value === '' || generatedPairingKey.value === '') {
    return ''
  }

  return `xylona-node --controller-url ${generatedControllerURL.value} --join-token ${generatedPairingKey.value}`
})

onMounted(async () => {
  if (props.existingNodeId) {
    await getNodeDetails()
  }
})

async function cancel() {
  router.back()
}

async function getNodeDetails() {
  const request: GetNodeRequest = create(GetNodeRequestSchema, {})
  try {
    request.nodeId = props.existingNodeId
    const response = await GetXylonaClient().getNode(request)
    if (response.node === undefined) {
      return
    }
    node.value = response.node
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to load node details'
    }
    console.error(e)
  }
}

async function updateNode() {
  errorMessage.value = ''
  formSubmitting.value = true
  const request: EditNodeRequest = create(EditNodeRequestSchema, {})
  request.node = node.value
  try {
    await GetXylonaClient().editNode(request)
    await router.push('/nodes')
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to update node'
    }
    console.error(e)
  } finally {
    formSubmitting.value = false
  }
}

async function generateJoinToken() {
  errorMessage.value = ''
  pairingKeySubmitting.value = true
  try {
    const panelURL = getPanelURL()
    const request = create(GenerateNodePairingObjectRequestSchema, {})
    request.targetUrl = panelURL
    const response = await GetXylonaClient().generateNodePairingObject(request)
    generatedPairingKey.value = response.pairingToken
    generatedControllerURL.value = response.baseUrl.trim() || panelURL
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to generate join token'
    }
    console.error(e)
  } finally {
    pairingKeySubmitting.value = false
  }
}

function getPanelURL() {
  return window.location.origin.trim().replace(/\/+$/, '')
}

async function copyCommand() {
  if (generatedJoinCommand.value === '') return
  await copyToClipboard(generatedJoinCommand.value)
  $q.notify({
    type: 'positive',
    message: 'Node join command copied to clipboard',
  })
}
</script>

<style scoped></style>
