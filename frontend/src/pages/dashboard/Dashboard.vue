<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header" style="margin-bottom: var(--xy-space-xl)">
      <div>
        <div class="xy-page-title">Federation Health</div>
        <div class="text-caption text-xy-secondary" style="margin-top: 2px">
          Real-time overview of all connected nodes
        </div>
      </div>
    </div>

    <div v-if="!selectedNode">
      <div class="stat-row">
        <div class="stat-card">
          <div class="stat-value">{{ nodeSummaries.length }}</div>
          <div class="stat-label">Nodes</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ totalServers }}</div>
          <div class="stat-label">Servers</div>
        </div>
        <div class="stat-card">
          <div class="stat-value text-success">{{ runningServers }}</div>
          <div class="stat-label">Running</div>
        </div>
        <div class="stat-card">
          <div class="stat-value">{{ totalUsers }}</div>
          <div class="stat-label">Users</div>
        </div>
      </div>
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
.stat-row {
  display: flex;
  gap: var(--xy-space-md);
  margin-bottom: var(--xy-space-2xl);
}

.stat-card {
  flex: 1;
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  padding: var(--xy-space-md) var(--xy-space-lg);
}

.stat-value {
  font-family: var(--xy-font-display);
  font-size: 2rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  line-height: 1;
}

.stat-label {
  font-size: 0.7rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--xy-text-muted);
  margin-top: 6px;
}

.nodes-section {
  padding-top: var(--xy-space-sm);
}

.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: var(--xy-space-3xl) var(--xy-space-md);
}

@media (max-width: 599px) {
  .stat-row {
    flex-wrap: wrap;
  }

  .stat-card {
    flex: 1 1 calc(50% - var(--xy-space-sm));
    min-width: calc(50% - var(--xy-space-sm));
  }
}
</style>
