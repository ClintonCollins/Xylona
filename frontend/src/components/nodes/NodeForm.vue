<template>
    <q-card class="full-width">
        <q-card-section>
            <div class="row">
                <div class="text-h6" v-text="existingNodeId ? 'Update Node': 'Add Node'"></div>
            </div>
        </q-card-section>
        <q-card-section>
            <q-form class="q-pa-lg">
                <div class="row wrap q-col-gutter-md justify-between">
                    <q-toggle v-if="!existingNodeId" class="col-12" v-model="isRemote" label="Remote node (federation)"/>
                    <q-input class="col-12 col-xl-6" outlined type="text" autofocus label="Name"
                             v-model="node.name"
                             :hint="isRemote ? 'Leave blank to use the remote node\'s name' : ''"></q-input>
                    <template v-if="!isRemote">
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Host"
                                 v-model="node.host"></q-input>
                        <q-input class="col-12 col-xl-6" outlined type="text" label="Port"
                                 v-model.number="port"></q-input>
                    </template>
                    <template v-if="isRemote">
                        <q-input class="col-12 col-xl-6" outlined type="url" label="Base URL"
                                 v-model="node.baseUrl" placeholder="http://192.168.1.100:8080"
                                 hint="Full URL including protocol and port"></q-input>
                    </template>
                    <q-input class="col-12 col-xl-6" outlined type="text" label="Secret Key"
                             v-model="node.secretKey"
                             :hint="isRemote ? 'The secret key generated on the remote node' : ''"></q-input>
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
            <q-btn label="Save" color="primary" @click="submitNode" :loading="formSubmitting"></q-btn>
        </q-card-actions>
        <q-inner-loading
                :showing="formSubmitting"
                label="Saving..."
                label-class="text-primary"
        ></q-inner-loading>
    </q-card>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { GetXylonaClient } from '@/utils/shared'
import { onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { NodeSchema } from '@/proto/shared_pb'
import { ConnectError } from '@connectrpc/connect'
import {
  AddNodeRequest, AddNodeRequestSchema,
  EditNodeRequest, EditNodeRequestSchema,
  GetNodeRequest, GetNodeRequestSchema
} from 'src/proto/xylona_pb'
const router = useRouter()

const props = defineProps({
  existingNodeId: {
    type: String,
    required: false,
    default: undefined
  }
})

const node = ref(create(NodeSchema, {}))
const formSubmitting = ref(false)
const port = ref(8080)
const isRemote = ref(false)
const errorMessage = ref('')

onMounted(async () => {
  if (props.existingNodeId) {
    await getNodeDetails()
  }
})

watch(port, (newVal) => {
  node.value.port = BigInt(newVal)
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
    console.error(e)
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

async function createNode() {
  errorMessage.value = ''
  formSubmitting.value = true
  const request: AddNodeRequest = create(AddNodeRequestSchema, {})
  request.node = node.value
  if (!isRemote.value) {
    request.node.port = BigInt(port.value)
  }
  try {
    await GetXylonaClient().addNode(request)
    await router.push('/nodes')
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to add node'
    }
    console.error(e)
  } finally {
    formSubmitting.value = false
  }
}

async function submitNode() {
  if (props.existingNodeId) {
    await updateNode()
  } else {
    await createNode()
  }
}

</script>

<style scoped>

</style>
