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
                    <q-input class="col-12 col-xl-6" outlined type="text" autofocus label="Name"
                             v-model="node.name"></q-input>
                    <q-input class="col-12 col-xl-6" outlined type="text" label="Host"
                             v-model="node.host"></q-input>
                    <q-input class="col-12 col-xl-6" outlined type="text" label="Port"
                             v-model.number="port"></q-input>
                    <q-input class="col-12 col-xl-6" outlined type="text" label="Secret Key"
                             v-model="node.secretKey"></q-input>
                </div>
            </q-form>
        </q-card-section>
        <q-separator></q-separator>
        <q-card-actions class="q-pa-md" align="right">
            <q-btn flat label="Cancel" @click="cancel"></q-btn>
            <q-btn label="Save" color="primary" @click="submitNode"></q-btn>
        </q-card-actions>
        <q-inner-loading
                :showing="formSubmitting"
                label="Adding node..."
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
  } catch (e) {
    console.error(e)
  }
}

async function updateNode() {
  const request: EditNodeRequest = create(EditNodeRequestSchema, {})
  request.node = node.value
  request.node.port = BigInt(port.value)
  try {
    const response = await GetXylonaClient().editNode(request)
    await router.push(`/nodes`)
  } catch (e) {
    console.error(e)
  }
}

async function createNode() {
  const request: AddNodeRequest = create(AddNodeRequestSchema, {})
  request.node = node.value
  request.node.port = BigInt(port.value)
  try {
    const response = await GetXylonaClient().addNode(request)
    await router.push(`/nodes`)
  } catch (e) {
    console.error(e)
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
