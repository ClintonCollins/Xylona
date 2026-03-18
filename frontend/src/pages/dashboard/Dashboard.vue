<template>
  <q-page :padding="true">
    <div class="text-h5 q-mb-md">Federation Health Dashboard</div>

    <div v-if="!selectedNode">
      <div class="row q-col-gutter-md">
        <div
          v-for="summary in nodeSummaries"
          :key="summary.node?.id"
          class="col-xs-12 col-sm-6 col-lg-4 col-xl-3">
          <NodeOverviewCard
            :node="summary.node!"
            :system-info="summary.systemInfo"
            :snapshot="summary.snapshot"
            @select="selectedNode = summary" />
        </div>
      </div>
      <div v-if="!loading && nodeSummaries.length === 0" class="text-grey text-center q-mt-xl">
        No nodes found.
      </div>
    </div>

    <NodeDetailPanel
      v-if="selectedNode?.node"
      :node="selectedNode.node"
      :system-info="selectedNode.systemInfo"
      @close="selectedNode = null" />

    <q-inner-loading :showing="loading" />
  </q-page>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { DashboardNodeSummary } from 'src/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import NodeOverviewCard from '@/components/dashboard/NodeOverviewCard.vue'
import NodeDetailPanel from '@/components/dashboard/NodeDetailPanel.vue'

const loading = ref(false)
const nodeSummaries = ref<DashboardNodeSummary[]>([])
const selectedNode = ref<DashboardNodeSummary | null>(null)

async function fetchDashboard() {
  loading.value = true
  try {
    const resp = await GetXylonaClient().getDashboardOverview({})
    nodeSummaries.value = resp.nodes
  } catch (err) {
    const connectErr = ConnectError.from(err)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(connectErr),
      timeout: 0,
      closeBtn: 'Dismiss',
    })
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await fetchDashboard()
})
</script>
