<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header" style="margin-bottom: var(--xy-space-xl)">
      <div>
        <div class="xy-page-title">Federation Health</div>
        <div class="text-caption text-xy-secondary" style="margin-top: 2px">
          {{ nodeSummaries.length }} {{ nodeSummaries.length === 1 ? 'node' : 'nodes' }}
          &middot; {{ totalServers }} {{ totalServers === 1 ? 'server' : 'servers' }}
          <span v-if="runningServers > 0" class="text-success">({{ runningServers }} running)</span>
          &middot; {{ totalUsers }} {{ totalUsers === 1 ? 'user' : 'users' }}
        </div>
      </div>
    </div>

    <div v-if="!selectedNode">
      <div class="nodes-section">
        <div class="xy-section-overline">Nodes</div>
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
      </div>
      <div v-if="!loading && nodeSummaries.length === 0" class="empty-state">
        <q-icon name="dns" size="3rem" class="text-xy-muted" />
        <div class="text-subtitle1 text-xy-secondary" style="margin-top: 8px">No nodes found</div>
        <div class="text-caption text-xy-muted">
          Add a remote node to see federation health data.
        </div>
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
import { computed, onMounted, ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { DashboardNodeSummary } from 'src/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import NodeOverviewCard from '@/components/dashboard/NodeOverviewCard.vue'
import NodeDetailPanel from '@/components/dashboard/NodeDetailPanel.vue'

const loading = ref(false)
const nodeSummaries = ref<DashboardNodeSummary[]>([])
const selectedNode = ref<DashboardNodeSummary | null>(null)

const totalServers = computed(() =>
  nodeSummaries.value.reduce((sum, s) => sum + (s.snapshot?.gameServerCount ?? 0), 0),
)
const runningServers = computed(() =>
  nodeSummaries.value.reduce((sum, s) => sum + (s.snapshot?.runningGameServerCount ?? 0), 0),
)
const totalUsers = computed(() =>
  nodeSummaries.value.reduce((sum, s) => sum + (s.snapshot?.userCount ?? 0), 0),
)

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

<style scoped>
.nodes-section {
  padding-top: var(--xy-space-sm);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--xy-space-3xl) var(--xy-space-md);
}

</style>
