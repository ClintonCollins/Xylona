<template>
  <q-card class="full-width">
    <q-card-section>
      <div class="row">
        <div class="text-h6" v-text="existingNodeId ? 'Update Node' : 'Add Node'"></div>
      </div>
    </q-card-section>
    <q-card-section>
      <q-form class="q-pa-lg">
        <div class="row wrap q-col-gutter-md justify-between">
          <template v-if="existingNodeId">
            <q-input
              v-model="node.name"
              class="col-12 col-xl-6"
              outlined
              type="text"
              autofocus
              :label="nameLabel"
              :hint="remoteNameHint"></q-input>
            <template v-if="!isRemote">
              <q-input
                v-model="node.host"
                class="col-12 col-xl-6"
                outlined
                type="text"
                label="Host"></q-input>
              <q-input
                v-model.number="port"
                class="col-12 col-xl-6"
                outlined
                type="text"
                label="Port"></q-input>
              <q-input
                v-model="node.baseUrl"
                class="col-12 col-xl-6"
                outlined
                type="url"
                label="Base URL"
                placeholder="https://panel.example.com"
                hint="Public URL used for node pairing"></q-input>
              <div class="col-12">
                <q-toggle
                  v-model="node.allowInsecureTls"
                  label="Allow insecure TLS for this panel endpoint"></q-toggle>
              </div>
            </template>
            <template v-else>
              <q-input
                v-model="node.baseUrl"
                class="col-12 col-xl-6"
                outlined
                type="url"
                label="Base URL"
                placeholder="http://192.168.1.100:8080"
                hint="Full URL including protocol and port"></q-input>
              <q-input
                v-model="node.secretKey"
                class="col-12 col-xl-6"
                outlined
                type="text"
                label="Secret Key"
                hint="The secret key generated on the remote node"></q-input>
              <div class="col-12">
                <q-toggle
                  v-model="node.allowInsecureTls"
                  label="Allow insecure TLS for this remote node"></q-toggle>
              </div>
            </template>
          </template>
          <template v-else>
            <div class="col-12">
              <q-btn-toggle
                v-model="addMode"
                spread
                no-caps
                unelevated
                color="grey-3"
                text-color="dark"
                toggle-color="primary"
                :options="addModeOptions"></q-btn-toggle>
            </div>
            <template v-if="addMode === 'copy'">
              <div class="col-12 text-subtitle2">Copy Node Details</div>
              <q-input
                class="col-12"
                outlined
                type="textarea"
                autogrow
                label="Node Details JSON"
                :model-value="generatedPairingPayload"
                readonly></q-input>
              <div class="col-12 row q-gutter-sm">
                <q-btn
                  outline
                  color="primary"
                  :label="generatedPairingKey ? 'Regenerate JSON' : 'Generate JSON'"
                  :loading="pairingKeySubmitting"
                  @click="generatePairingKey(false)"></q-btn>
                <q-btn
                  outline
                  color="primary"
                  label="Copy JSON"
                  :disable="generatedPairingPayload === ''"
                  @click="copyPairingPayload"></q-btn>
              </div>
            </template>
            <template v-else>
              <div class="col-12 text-subtitle2">Paste Node Details</div>
              <q-input
                v-model="node.name"
                class="col-12 col-xl-6"
                outlined
                type="text"
                autofocus
                label="Remote Name (Optional)"
                hint="Leave blank to use the remote node's name"></q-input>
              <q-input
                v-model="pairingPayloadInput"
                class="col-12"
                outlined
                type="textarea"
                autogrow
                label="Remote Panel Pairing JSON"
                hint="Paste the JSON copied from the remote panel"></q-input>
              <div class="col-12">
                <q-toggle
                  v-model="pairingRemoteAllowInsecureTLS"
                  label="Allow insecure TLS for this remote node"></q-toggle>
              </div>
              <div class="col-12 row q-gutter-sm">
                <q-btn
                  outline
                  color="primary"
                  label="Validate JSON"
                  @click="validatePairingPayload(true)"></q-btn>
              </div>
              <div v-if="parsedPairingPayload" class="col-12 text-caption">
                Remote panel URL: {{ parsedPairingPayload.base_url }}
              </div>
            </template>
          </template>
        </div>
        <div v-if="errorMessage" class="q-mt-md">
          <q-banner dense class="bg-red-2 text-red-9">
            {{ errorMessage }}
          </q-banner>
        </div>
      </q-form>
    </q-card-section>
    <q-separator></q-separator>
    <q-card-actions class="q-pa-md" align="right">
      <q-btn flat label="Cancel" @click="cancel"></q-btn>
      <q-btn
        v-if="showSaveButton"
        :label="submitLabel"
        color="primary"
        :loading="formSubmitting"
        @click="submitNode"></q-btn>
    </q-card-actions>
    <q-inner-loading
      :showing="formSubmitting"
      label="Saving..."
      label-class="text-primary"></q-inner-loading>
    <q-dialog v-model="showPairConfirmDialog" aria-labelledby="dialog-title-confirm">
      <q-card style="min-width: min(360px, 90vw); max-width: 560px">
        <q-card-section>
          <div id="dialog-title-confirm" class="text-h6">Confirm Node Pairing</div>
        </q-card-section>
        <q-card-section>
          <div>
            This will add <strong>{{ parsedPairingPayload?.base_url }}</strong> to this panel.
          </div>
          <div class="q-mt-sm">
            It will also ask the remote panel to add this panel back automatically.
          </div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat label="Cancel"></q-btn>
          <q-btn
            color="primary"
            label="Pair Nodes"
            :loading="formSubmitting"
            @click="confirmPairNode"></q-btn>
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-card>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useClipboard } from '@vueuse/core'
import { useQuasar } from 'quasar'
import { GetXylonaClient } from '@/utils/shared'
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NodeSchema } from '@/proto/shared_pb'
import {
  createNodePairingPayload,
  normalizeNodePairingBaseURL,
  parseNodePairingPayload,
  type NodePairingPayload,
} from '@/utils/node-pairing'
import {
  EditNodeRequest,
  EditNodeRequestSchema,
  GenerateNodePairingObjectRequest,
  GenerateNodePairingObjectRequestSchema,
  GetNodeRequest,
  GetNodeRequestSchema,
  ListNodesRequestSchema,
  PairNodeRequest,
  PairNodeRequestSchema,
} from 'src/proto/xylona_pb'
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
const port = ref(8080)
const isRemote = ref(true)
const addMode = ref<'copy' | 'paste'>('copy')
const errorMessage = ref('')
const pairingLocalBaseUrl = ref('')
const pairingLocalMTLSPort = ref(8443)
const pairingLocalAllowInsecureTLS = ref(false)
const pairingRemoteAllowInsecureTLS = ref(false)
const generatedPairingKey = ref('')
const pairingKeySubmitting = ref(false)
const pairingPayloadInput = ref('')
const parsedPairingPayload = ref<NodePairingPayload | null>(null)
const showPairConfirmDialog = ref(false)

const { copy } = useClipboard()
const addModeOptions = [
  { label: 'Copy Node Details', value: 'copy' },
  { label: 'Paste Node Details', value: 'paste' },
]

const nameLabel = computed(() => {
  if (props.existingNodeId && !isRemote.value) {
    return 'Name'
  }
  return 'Remote Name (Optional)'
})

const remoteNameHint = computed(() => {
  if (props.existingNodeId && !isRemote.value) {
    return ''
  }
  return "Leave blank to use the remote node's name"
})

const submitLabel = computed(() => {
  if (!props.existingNodeId) {
    return 'Pair Node'
  }
  return 'Save'
})

const showSaveButton = computed(() => props.existingNodeId || addMode.value === 'paste')

const generatedPairingPayload = computed(() => {
  if (generatedPairingKey.value === '') {
    return ''
  }
  try {
    return createNodePairingPayload(
      pairingLocalBaseUrl.value,
      generatedPairingKey.value,
      pairingLocalMTLSPort.value,
    )
  } catch {
    return ''
  }
})

onMounted(async () => {
  if (props.existingNodeId) {
    await getNodeDetails()
    return
  }
  const localBaseURLLoaded = await loadLocalPairingBaseURL()
  if (localBaseURLLoaded) {
    await generatePairingKey(true)
  }
})

watch(port, (newVal) => {
  node.value.port = BigInt(newVal)
})

watch(pairingPayloadInput, () => {
  parsedPairingPayload.value = null
})

watch(addMode, async (newMode) => {
  errorMessage.value = ''
  if (newMode === 'copy' && generatedPairingKey.value === '' && pairingLocalBaseUrl.value !== '') {
    await generatePairingKey(true)
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
    isRemote.value = !response.node.local
    port.value = Number(response.node.port)
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to load node details'
    }
    console.error(e)
  }
}

async function loadLocalPairingBaseURL(): Promise<boolean> {
  try {
    const response = await GetXylonaClient().listNodes(create(ListNodesRequestSchema, {}))
    const localNode = response.nodes.find((currentNode) => currentNode.local)
    if (!localNode) {
      errorMessage.value = 'Local node configuration was not found'
      return false
    }

    try {
      pairingLocalBaseUrl.value = normalizeNodePairingBaseURL(localNode.baseUrl)
      pairingLocalAllowInsecureTLS.value = localNode.allowInsecureTls
      return true
    } catch {
      pairingLocalBaseUrl.value = ''
      errorMessage.value =
        'Local node Base URL is not configured. Edit the local node and set Base URL before pairing.'
      return false
    }
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to load local node configuration'
    }
    console.error(e)
    return false
  }
}

async function updateNode() {
  errorMessage.value = ''
  formSubmitting.value = true
  const request: EditNodeRequest = create(EditNodeRequestSchema, {})
  request.node = node.value
  if (!isRemote.value) {
    request.node.port = BigInt(port.value)
  }
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

async function generatePairingKey(silent: boolean = false) {
  errorMessage.value = ''
  pairingKeySubmitting.value = true
  try {
    const request: GenerateNodePairingObjectRequest = create(
      GenerateNodePairingObjectRequestSchema,
      {},
    )
    const response = await GetXylonaClient().generateNodePairingObject(request)

    generatedPairingKey.value = response.pairingToken
    pairingLocalBaseUrl.value = normalizeNodePairingBaseURL(response.baseUrl)

    const mtlsPortFromResponse = Number(response.mtlsPort)
    if (
      Number.isInteger(mtlsPortFromResponse) &&
      mtlsPortFromResponse > 0 &&
      mtlsPortFromResponse <= 65535
    ) {
      pairingLocalMTLSPort.value = mtlsPortFromResponse
    }
    if (!silent) {
      $q.notify({
        type: 'positive',
        message: 'Pairing JSON generated',
      })
    }
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to generate pairing key'
    }
    console.error(e)
  } finally {
    pairingKeySubmitting.value = false
  }
}

async function copyPairingPayload() {
  if (generatedPairingPayload.value === '') {
    if (pairingLocalBaseUrl.value === '') {
      errorMessage.value =
        'Local node Base URL is not configured. Edit the local node and set Base URL before pairing.'
    } else {
      errorMessage.value = 'Generate pairing JSON first'
    }
    return
  }

  await copy(generatedPairingPayload.value)
  $q.notify({
    type: 'positive',
    message: 'Copied to clipboard',
  })
}

function validatePairingPayload(showNotification: boolean): boolean {
  try {
    parsedPairingPayload.value = parseNodePairingPayload(pairingPayloadInput.value)
    errorMessage.value = ''
    if (showNotification) {
      $q.notify({
        type: 'positive',
        message: 'Pairing JSON looks valid',
      })
    }
    return true
  } catch (errParsePayload) {
    parsedPairingPayload.value = null
    if (errParsePayload instanceof Error) {
      errorMessage.value = errParsePayload.message
    } else {
      errorMessage.value = 'Pairing JSON is invalid'
    }
    return false
  }
}

async function pairNode() {
  errorMessage.value = ''
  if (!validatePairingPayload(false)) {
    return
  }

  const payload = parsedPairingPayload.value
  if (payload === null) {
    errorMessage.value = 'Pairing JSON is required'
    return
  }

  let localBaseURL: string
  try {
    localBaseURL = normalizeNodePairingBaseURL(pairingLocalBaseUrl.value)
  } catch (errNormalizeLocalURL) {
    if (errNormalizeLocalURL instanceof Error) {
      errorMessage.value = `Local node Base URL is invalid: ${errNormalizeLocalURL.message}`
    } else {
      errorMessage.value = 'Local node Base URL is invalid'
    }
    return
  }

  showPairConfirmDialog.value = false
  formSubmitting.value = true
  const request: PairNodeRequest = create(PairNodeRequestSchema, {})
  request.remoteBaseUrl = payload.base_url
  request.remoteSecretKey = payload.secret_key
  request.remoteMtlsPort = BigInt(payload.mtls_port)
  request.localBaseUrl = localBaseURL
  request.remoteName = node.value.name.trim()
  request.remoteAllowInsecureTls = pairingRemoteAllowInsecureTLS.value
  request.localAllowInsecureTls = pairingLocalAllowInsecureTLS.value
  try {
    await GetXylonaClient().pairNode(request)
    $q.notify({
      type: 'positive',
      message: 'Nodes paired successfully',
    })
    await router.push('/nodes')
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to pair nodes'
    }
    console.error(e)
  } finally {
    formSubmitting.value = false
  }
}

async function confirmPairNode() {
  await pairNode()
}

async function submitNode() {
  if (props.existingNodeId) {
    await updateNode()
    return
  }
  if (addMode.value !== 'paste') {
    return
  }
  if (!validatePairingPayload(false)) {
    return
  }
  showPairConfirmDialog.value = true
}
</script>

<style scoped></style>
