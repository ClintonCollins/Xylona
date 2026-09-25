<template>
  <div class="node-form">
    <page-header :title="existingNodeId ? 'Edit Node' : 'Add Remote Node'" />

    <div v-if="existingNodeId">
      <q-banner
        v-if="loadError"
        class="xy-banner-negative q-mb-md"
        dense
        inline-actions
        role="alert">
        <template #avatar>
          <q-icon name="sync_problem" />
        </template>
        <strong>Node details could not be loaded.</strong> {{ loadError }}
        <template #action>
          <q-btn flat icon="refresh" label="Retry" no-caps @click="getNodeDetails" />
        </template>
      </q-banner>
      <q-form @submit.prevent="updateNode">
        <div class="row wrap q-col-gutter-md">
          <q-input
            v-model="node.name"
            autofocus
            class="col-12 col-xl-6"
            label="Name"
            outlined
            type="text"></q-input>
          <q-input
            v-if="!node.local"
            v-model="node.baseUrl"
            class="col-12 col-xl-6"
            hint="HTTPS URL the controller uses to reach this node"
            label="Listen URL"
            outlined
            placeholder="https://node.example.com:9500"
            type="url"></q-input>
          <p v-else class="col-12 col-xl-6 local-node-note">
            Runs inside the controller, so it needs no listen URL.
          </p>
        </div>
        <div v-if="errorMessage" class="text-negative q-mt-md" role="alert">
          {{ errorMessage }}
        </div>
        <div class="row q-mt-md">
          <q-btn flat label="Cancel" @click="cancel"></q-btn>
          <q-space />
          <q-btn
            :disable="!nodeLoaded"
            :loading="formSubmitting"
            color="primary"
            label="Save"
            type="submit"></q-btn>
        </div>
      </q-form>
    </div>

    <div v-else>
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
            aria-label="Node join command"
            dense
            filled
            input-class="font-mono"
            readonly
            type="textarea"
            autogrow></q-input>
          <q-btn class="q-mt-sm" flat icon="content_copy" label="Copy" @click="copyCommand"></q-btn>
        </q-card-section>
      </q-card>

      <div v-if="errorMessage" class="text-negative q-mt-md" role="alert">{{ errorMessage }}</div>
      <div class="row q-mt-md">
        <q-btn flat label="Back" @click="cancel"></q-btn>
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { copyToClipboard } from 'quasar'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { notifyError, notifySuccess } from '@/api/notifications'
import { NodeSchema } from '@/proto/shared_pb'
import {
  type EditNodeRequest,
  EditNodeRequestSchema,
  GenerateNodePairingObjectRequestSchema,
  type GetNodeRequest,
  GetNodeRequestSchema,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'
import { connectErrorToString } from '@/api/connect-errors'
import PageHeader from '@/components/shared/PageHeader.vue'

const router = useRouter()

const props = defineProps({
  existingNodeId: {
    type: String,
    required: false,
    default: undefined,
  },
})

const node = ref(create(NodeSchema, {}))
const savedNode = ref({ name: '', baseUrl: '' })
const isDirty = computed(
  () => node.value.name !== savedNode.value.name || node.value.baseUrl !== savedNode.value.baseUrl,
)

defineExpose({ isDirty })
const formSubmitting = ref(false)
const errorMessage = ref('')
// Save stays disabled until the node loads, so a failed read can't save a blank form over it.
const loadError = ref('')
const nodeLoaded = ref(false)
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
  await router.push(props.existingNodeId ? `/nodes/${props.existingNodeId}` : '/nodes')
}

async function getNodeDetails() {
  loadError.value = ''
  const request: GetNodeRequest = create(GetNodeRequestSchema, {})
  try {
    request.nodeId = props.existingNodeId
    const response = await GetXylonaClient().getNode(request)
    if (response.node === undefined) {
      loadError.value = 'The controller returned no node.'
      return
    }
    node.value = response.node
    savedNode.value = { name: response.node.name, baseUrl: response.node.baseUrl }
    nodeLoaded.value = true
  } catch (e) {
    if (e instanceof ConnectError) {
      loadError.value = connectErrorToString(e)
    } else {
      loadError.value = 'Failed to load node details'
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
    savedNode.value = { name: node.value.name, baseUrl: node.value.baseUrl }
    await router.push('/nodes')
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = connectErrorToString(e)
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
      errorMessage.value = connectErrorToString(e)
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
  try {
    await copyToClipboard(generatedJoinCommand.value)
    notifySuccess('Node join command copied to clipboard')
  } catch {
    notifyError('Could not copy the join command. Select it and copy it manually.')
  }
}
</script>

<style scoped>
.node-form {
  max-width: 720px;
}

.local-node-note {
  margin: 0;
  padding-top: var(--xy-space-md);
  color: var(--xy-text-secondary);
}
</style>
