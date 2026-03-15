<template>
    <q-page>
        <div class="row justify-center q-pa-md">
            <q-card class="full-width">
                <q-card-section>
                    <div class="text-h6">Edit Peer Node</div>
                </q-card-section>
                <q-card-section v-if="peerNode">
                    <q-form class="q-pa-lg">
                        <div class="row wrap q-col-gutter-md justify-between">
                            <q-input class="col-12 col-xl-6" outlined type="text" autofocus label="Name"
                                     v-model="peerNode.name"/>
                            <q-input class="col-12 col-xl-6" outlined type="url" label="Base URL"
                                     v-model="peerNode.baseUrl"/>
                            <q-input class="col-12 col-xl-6" outlined type="text" label="Secret Key (leave blank to keep)"
                                     v-model="newSecretKey"
                                     hint="Only fill in to change the secret key"/>
                            <q-toggle class="col-12 col-xl-6" v-model="peerNode.enabled" label="Enabled"/>
                        </div>
                        <div class="q-mt-lg">
                            <div class="text-subtitle2 text-grey">Peer Info</div>
                            <div class="q-gutter-sm q-mt-sm">
                                <div><strong>Node ID:</strong> {{ peerNode.nodeId || 'Unknown' }}</div>
                                <div><strong>Health:</strong>
                                    <q-badge :color="healthColor(peerNode.healthStatus)" :label="peerNode.healthStatus || 'unknown'" class="q-ml-sm"/>
                                </div>
                                <div><strong>Version:</strong> {{ peerNode.version || 'Unknown' }}</div>
                                <div><strong>Protocol:</strong> v{{ peerNode.protocolVersion }}</div>
                                <div><strong>Last Sync:</strong> {{ formatTimestamp(peerNode.lastSyncAt) }}
                                    <q-badge v-if="peerNode.lastSyncStatus" :color="peerNode.lastSyncStatus === 'success' ? 'green' : 'red'" :label="peerNode.lastSyncStatus" class="q-ml-sm"/>
                                </div>
                            </div>
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
                    <q-btn label="Save" color="primary" @click="savePeer" :loading="submitting"/>
                </q-card-actions>
            </q-card>
        </div>
    </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { GetXylonaClient } from '@/utils/shared'
import { PeerNode, EditPeerNodeRequestSchema, GetPeerNodeRequestSchema } from '@/proto/federation_pb'
import { ConnectError } from '@connectrpc/connect'

const router = useRouter()
const route = useRoute()
const peerNode = ref<PeerNode | null>(null)
const newSecretKey = ref('')
const submitting = ref(false)
const errorMessage = ref('')

onMounted(async () => {
  await loadPeerNode()
})

async function loadPeerNode() {
  try {
    const response = await GetXylonaClient().getPeerNode(create(GetPeerNodeRequestSchema, {
      peerNodeId: route.params.id as string
    }))
    peerNode.value = response.peerNode ?? null
  } catch (e) {
    console.error(e)
  }
}

function cancel() {
  router.back()
}

function healthColor(status: string): string {
  switch (status) {
    case 'healthy': return 'green'
    case 'offline': return 'red'
    default: return 'grey'
  }
}

function formatTimestamp(ts: { seconds: bigint } | undefined): string {
  if (!ts || !ts.seconds) return 'Never'
  const date = new Date(Number(ts.seconds) * 1000)
  return date.toLocaleString()
}

async function savePeer() {
  if (!peerNode.value) return
  errorMessage.value = ''
  submitting.value = true
  try {
    await GetXylonaClient().editPeerNode(create(EditPeerNodeRequestSchema, {
      peerNodeId: peerNode.value.id,
      name: peerNode.value.name,
      baseUrl: peerNode.value.baseUrl,
      secretKey: newSecretKey.value,
      enabled: peerNode.value.enabled
    }))
    await router.push('/peer-nodes')
  } catch (e) {
    if (e instanceof ConnectError) {
      errorMessage.value = e.message
    } else {
      errorMessage.value = 'Failed to update peer node'
    }
    console.error(e)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
</style>
