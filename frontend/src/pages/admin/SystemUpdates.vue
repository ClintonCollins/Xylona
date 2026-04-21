<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div>
        <h1 class="xy-page-title">System Updates</h1>
        <div class="text-caption text-xy-secondary" style="margin-top: 2px">
          {{ updates.length }} {{ updates.length === 1 ? 'target' : 'targets' }}
          <template v-if="pendingJobCount > 0"> &middot; {{ pendingJobCount }} active </template>
        </div>
      </div>
      <div class="xy-page-actions">
        <q-btn
          :loading="loading"
          color="primary"
          icon="refresh"
          label="Check"
          @click="refreshAll" />
      </div>
    </div>

    <div class="system-updates__grid">
      <section class="system-updates__section">
        <div class="section-title">Available Targets</div>
        <q-table
          :columns="updateColumns"
          :loading="loading"
          :rows="updates"
          class="xy-standalone-table"
          flat
          row-key="targetKey">
          <template #body-cell-target="props">
            <q-td :props="props">
              <div class="text-weight-medium">{{ targetName(props.row) }}</div>
              <div class="text-caption text-xy-muted">
                {{ props.row.os || 'unknown' }} / {{ props.row.architecture || 'unknown' }}
              </div>
            </q-td>
          </template>
          <template #body-cell-version="props">
            <q-td :props="props">
              <div>{{ props.row.currentVersion || 'Unknown' }}</div>
              <div class="text-caption text-xy-muted">
                Latest {{ props.row.latestVersion || '-' }}
              </div>
            </q-td>
          </template>
          <template #body-cell-status="props">
            <q-td :props="props">
              <q-badge
                :color="availabilityBadge(props.row).color"
                :label="availabilityBadge(props.row).label" />
              <div v-if="props.row.reason" class="text-caption text-xy-muted q-mt-xs">
                {{ props.row.reason }}
              </div>
            </q-td>
          </template>
          <template #body-cell-artifact="props">
            <q-td :props="props">
              <div>{{ props.row.artifactName || '-' }}</div>
              <div v-if="props.row.artifactSha256" class="text-caption text-xy-muted">
                {{ props.row.artifactSha256.slice(0, 12) }}
              </div>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <q-btn
                :disable="!props.row.updateable"
                color="primary"
                dense
                flat
                icon="system_update_alt"
                @click="openConfirm(props.row)">
                <q-tooltip>{{
                  props.row.updateable ? 'Start update' : 'Not updateable'
                }}</q-tooltip>
              </q-btn>
            </q-td>
          </template>
        </q-table>
      </section>

      <section class="system-updates__section">
        <div class="section-title">Recent Jobs</div>
        <q-table
          :columns="jobColumns"
          :loading="jobsLoading"
          :rows="jobs"
          class="xy-standalone-table"
          flat
          row-key="id">
          <template #body-cell-target="props">
            <q-td :props="props">
              <div class="text-weight-medium">{{ jobTargetName(props.row) }}</div>
              <div class="text-caption text-xy-muted">{{ props.row.targetVersion }}</div>
            </q-td>
          </template>
          <template #body-cell-progress="props">
            <q-td :props="props">
              <q-linear-progress
                :color="jobProgressColor(props.row)"
                :value="props.row.progressPercent / 100"
                rounded
                size="8px" />
              <div class="text-caption text-xy-muted q-mt-xs">
                {{ statusLabel(props.row.status) }} &middot; {{ props.row.progressPercent }}%
              </div>
            </q-td>
          </template>
          <template #body-cell-error="props">
            <q-td :props="props">
              <span v-if="props.row.error" class="text-negative">{{ props.row.error }}</span>
              <span v-else class="text-xy-muted">-</span>
            </q-td>
          </template>
          <template #body-cell-created="props">
            <q-td :props="props">{{ formatTimestamp(props.row.createdAt) }}</q-td>
          </template>
        </q-table>
      </section>
    </div>

    <q-dialog v-model="confirmOpen">
      <q-card class="system-updates__dialog">
        <q-card-section>
          <div class="text-h6">Confirm Update</div>
        </q-card-section>
        <q-card-section>
          <div class="q-mb-sm">
            Updating {{ selectedUpdate ? targetName(selectedUpdate) : 'target' }} to
            {{ selectedUpdate?.latestVersion }} will stop these game servers. They remain offline
            after the update.
          </div>
          <q-list bordered separator>
            <q-item v-for="server in affectedServers" :key="server.id">
              <q-item-section>
                <q-item-label>{{ server.name }}</q-item-label>
                <q-item-label caption>{{
                  server.gameName || server.game?.name || server.id
                }}</q-item-label>
              </q-item-section>
            </q-item>
            <q-item v-if="affectedServers.length === 0">
              <q-item-section>
                <q-item-label class="text-xy-muted">No game servers on this target</q-item-label>
              </q-item-section>
            </q-item>
          </q-list>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup flat label="Cancel" />
          <q-btn color="primary" flat label="Start Update" @click="startSelectedUpdate" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Notify } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import type { GameServer, Node } from '@/proto/shared_pb'
import {
  CheckSystemUpdatesRequestSchema,
  ListGameServersRequestSchema,
  ListNodesRequestSchema,
  ListSystemUpdateJobsRequestSchema,
  StartSystemUpdateRequestSchema,
  SystemUpdateAvailability,
  SystemUpdateComponent,
  SystemUpdateJob,
  SystemUpdateJobStatus,
  SystemUpdateProgress,
} from '@/proto/xylona_pb'
import {
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'

type UpdateRow = SystemUpdateAvailability & { targetKey: string }

const loading = ref(false)
const jobsLoading = ref(false)
const updates = ref<UpdateRow[]>([])
const jobs = ref<SystemUpdateJob[]>([])
const nodes = ref<Node[]>([])
const gameServers = ref<GameServer[]>([])
const confirmOpen = ref(false)
const selectedUpdate = ref<UpdateRow | null>(null)
const route = useRoute()

const pendingJobCount = computed(
  () =>
    jobs.value.filter(
      (job) =>
        job.status !== SystemUpdateJobStatus.SUCCEEDED &&
        job.status !== SystemUpdateJobStatus.FAILED,
    ).length,
)

const affectedServers = computed(() => {
  const update = selectedUpdate.value
  if (!update) return []
  const nodeId = nodeIdForUpdate(update)
  if (!nodeId) return []
  return gameServers.value.filter((server) => server.nodeId === nodeId)
})

const updateColumns = [
  { name: 'target', label: 'Target', align: 'left' as const, field: 'targetKey', sortable: true },
  { name: 'version', label: 'Version', align: 'left' as const, field: 'latestVersion' },
  { name: 'status', label: 'Updateability', align: 'left' as const, field: 'updateable' },
  { name: 'artifact', label: 'Artifact', align: 'left' as const, field: 'artifactName' },
  { name: 'actions', label: '', align: 'center' as const, field: 'targetKey' },
]

const jobColumns = [
  { name: 'target', label: 'Target', align: 'left' as const, field: 'nodeName' },
  { name: 'progress', label: 'Progress', align: 'left' as const, field: 'progressPercent' },
  { name: 'error', label: 'Error', align: 'left' as const, field: 'error' },
  { name: 'created', label: 'Created', align: 'left' as const, field: 'createdAt' },
]

onMounted(async () => {
  GetOrCreateXylonaWebsocketClient()
  XylonaEventBus.on('systemUpdateProgress', onSystemUpdateProgress)
  await refreshAll()
})

onBeforeUnmount(() => {
  XylonaEventBus.off('systemUpdateProgress', onSystemUpdateProgress)
})

async function refreshAll() {
  loading.value = true
  jobsLoading.value = true
  try {
    const client = GetXylonaClient()
    const [updatesResp, jobsResp, nodesResp, serversResp] = await Promise.all([
      client.checkSystemUpdates(create(CheckSystemUpdatesRequestSchema, { includeNodes: true })),
      client.listSystemUpdateJobs(create(ListSystemUpdateJobsRequestSchema, { limit: 25 })),
      client.listNodes(create(ListNodesRequestSchema, {})),
      client.listGameServers(create(ListGameServersRequestSchema, {})),
    ])
    const focusedNodeId = String(route.query.nodeId ?? '')
    updates.value = updatesResp.updates
      .map((update) => ({
        ...update,
        targetKey: `${update.component}:${update.nodeId || 'controller'}`,
      }))
      .sort((a, b) => {
        if (!focusedNodeId) return 0
        if (a.nodeId === focusedNodeId) return -1
        if (b.nodeId === focusedNodeId) return 1
        return 0
      })
    jobs.value = jobsResp.jobs
    nodes.value = nodesResp.nodes
    gameServers.value = serversResp.gameServers
  } catch (unknownError: unknown) {
    notifyError(unknownError)
  } finally {
    loading.value = false
    jobsLoading.value = false
  }
}

function openConfirm(update: UpdateRow) {
  selectedUpdate.value = update
  confirmOpen.value = true
}

async function startSelectedUpdate() {
  const update = selectedUpdate.value
  if (!update) return
  try {
    const response = await GetXylonaClient().startSystemUpdate(
      create(StartSystemUpdateRequestSchema, {
        component: update.component,
        nodeId: update.nodeId,
        targetVersion: update.latestVersion,
        confirmedDrain: true,
      }),
    )
    if (response.job) {
      jobs.value = [response.job, ...jobs.value.filter((job) => job.id !== response.job?.id)]
    }
    confirmOpen.value = false
  } catch (unknownError: unknown) {
    notifyError(unknownError)
  }
}

function onSystemUpdateProgress(progress: SystemUpdateProgress) {
  jobs.value = jobs.value.map((job) => {
    if (job.id !== progress.jobId) return job
    return {
      ...job,
      status: progress.status,
      phase: progress.phase,
      progressPercent: progress.progressPercent,
      error: progress.error || job.error,
    }
  })
}

function targetName(update: SystemUpdateAvailability): string {
  if (update.component === SystemUpdateComponent.CONTROLLER) return 'Controller'
  return update.nodeName || update.nodeId || 'Node'
}

function jobTargetName(job: SystemUpdateJob): string {
  if (job.component === SystemUpdateComponent.CONTROLLER) return 'Controller'
  return job.nodeName || job.nodeId || 'Node'
}

function nodeIdForUpdate(update: SystemUpdateAvailability): string {
  if (update.component === SystemUpdateComponent.NODE) return update.nodeId
  return nodes.value.find((node) => node.local)?.id ?? ''
}

function availabilityBadge(update: SystemUpdateAvailability): { color: string; label: string } {
  if (!update.updateAvailable) return { color: 'grey-6', label: 'Current' }
  if (update.updateable) return { color: 'positive', label: 'Ready' }
  return { color: 'warning', label: 'Blocked' }
}

function statusLabel(status: SystemUpdateJobStatus): string {
  return SystemUpdateJobStatus[status] ?? 'UNKNOWN'
}

function jobProgressColor(job: SystemUpdateJob): string {
  if (job.status === SystemUpdateJobStatus.FAILED) return 'negative'
  if (job.status === SystemUpdateJobStatus.SUCCEEDED) return 'positive'
  return 'primary'
}

function formatTimestamp(ts: { seconds: bigint } | undefined): string {
  if (!ts?.seconds) return '-'
  return new Date(Number(ts.seconds) * 1000).toLocaleString()
}

function notifyError(unknownError: unknown) {
  const err = ConnectError.from(unknownError)
  Notify.create({
    type: 'xylona-error',
    position: 'top',
    caption: ConnectErrorToString(err),
    timeout: 0,
    closeBtn: 'Dismiss',
    icon: 'report_problem',
  })
}
</script>

<style scoped>
.system-updates__grid {
  display: grid;
  gap: var(--xy-space-lg);
}

.system-updates__section {
  min-width: 0;
}

.section-title {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-heading);
  font-size: 0.95rem;
  margin: 0 0 var(--xy-space-sm);
}

.system-updates__dialog {
  width: min(620px, 92vw);
}
</style>
