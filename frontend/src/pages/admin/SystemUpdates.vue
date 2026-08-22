<template>
  <q-page class="xy-page-content system-updates">
    <page-header title="System Updates">
      <div class="system-updates__summary text-xy-secondary">
        {{ updates.length }} {{ updates.length === 1 ? 'target' : 'targets' }}
        <template v-if="activeJobs.length > 0"> &middot; {{ activeJobs.length }} active </template>
      </div>
      <template #actions>
        <q-btn
          :disable="!websocketBrowserOnline"
          :loading="isRefreshing"
          color="primary"
          icon="sync"
          label="Resync"
          no-caps
          @click="resyncPage" />
      </template>
    </page-header>

    <div
      :class="['system-updates__live-strip', `is-${liveStatus.tone}`]"
      aria-label="System update status"
      data-test="live-status-strip">
      <div class="system-updates__live-state">
        <q-icon :name="liveStatus.icon" size="sm" />
        <div>
          <strong>{{ liveStatus.title }}</strong>
          <span>{{ liveStatus.detail }}</span>
        </div>
      </div>
      <div class="system-updates__live-actions">
        <q-spinner
          v-if="websocketBrowserOnline && !websocketStateAuthoritative"
          aria-label="Reconnecting"
          color="primary"
          size="1.25rem" />
        <span class="system-updates__sync-time">
          {{ lastSynchronizedLabel }}
        </span>
      </div>
    </div>

    <div
      v-if="controllerReloadJobId"
      class="system-updates__reload-banner bg-xy-success-tint"
      data-test="controller-reload-prompt">
      <q-icon name="check_circle" size="sm" />
      <div>
        <strong>Controller update completed</strong>
        <span>Reload this interface to use the frontend embedded in the new controller.</span>
      </div>
      <div class="system-updates__reload-actions">
        <q-btn
          color="positive"
          flat
          icon="refresh"
          label="Reload interface"
          @click="reloadInterface" />
        <q-btn
          aria-label="Dismiss reload prompt"
          dense
          flat
          icon="close"
          round
          @click="controllerReloadJobId = null">
          <q-tooltip>Dismiss</q-tooltip>
        </q-btn>
      </div>
    </div>

    <div class="system-updates__sections">
      <section
        v-if="!jobsHasLoaded || jobsError || activeJobs.length > 0"
        class="system-updates__section"
        aria-labelledby="active-update-jobs-title">
        <div class="system-updates__section-header">
          <div>
            <h2 id="active-update-jobs-title" class="system-updates__section-title">
              Active updates
            </h2>
          </div>
          <q-badge
            v-if="activeJobs.length > 0"
            color="primary"
            :label="`${activeJobs.length} active`" />
        </div>

        <div v-if="jobsError" class="system-updates__error" role="alert">
          <q-icon name="sync_problem" size="sm" />
          <div>
            <strong>Update jobs could not be synchronized.</strong>
            <span>{{ jobsError }}</span>
          </div>
          <q-btn
            :loading="jobsLoading"
            dense
            flat
            icon="refresh"
            label="Retry"
            @click="loadJobs()" />
        </div>

        <div v-if="jobsLoading && !jobsHasLoaded" class="system-updates__loading">
          <q-spinner color="primary" size="1.5rem" />
          <span>Loading update jobs…</span>
        </div>

        <div v-else-if="activeJobs.length > 0" class="system-updates__active-list">
          <q-card
            v-for="job in activeJobs"
            :key="job.id"
            bordered
            class="system-updates__active-job"
            flat
            :data-test="`active-job-${job.id}`">
            <q-card-section class="system-updates__active-main">
              <div class="system-updates__active-copy">
                <div class="system-updates__active-heading">
                  <div>
                    <h3>{{ jobTargetName(job) }}</h3>
                    <span class="system-updates__version">
                      {{ job.currentVersion || 'Unknown' }} → {{ job.targetVersion || 'Unknown' }}
                    </span>
                  </div>
                  <q-badge :color="jobProgressColor(job)" :label="statusLabel(job.status)" />
                </div>

                <div class="system-updates__phase-line">
                  <strong>{{ phaseLabel(job.phase) }}</strong>
                  <span>{{ progressPercent(job) }}%</span>
                </div>
                <q-linear-progress
                  :aria-label="`${jobTargetName(job)} update ${progressPercent(job)} percent complete`"
                  :color="jobProgressColor(job)"
                  :value="progressPercent(job) / 100"
                  rounded
                  size="8px" />

                <p class="system-updates__job-message">
                  {{ latestJobMessage(job) }}
                </p>
                <p v-if="isExpectedControllerRestart(job)" class="system-updates__restart-context">
                  The controller may briefly disconnect while it replaces and restarts itself.
                  Persisted job state will be checked when the connection returns.
                </p>
              </div>

              <dl class="system-updates__active-meta">
                <div>
                  <dt>Started by</dt>
                  <dd>{{ requestedBy(job) }}</dd>
                </div>
                <div>
                  <dt>Last update</dt>
                  <dd>{{ formatJobLastUpdate(job) }}</dd>
                </div>
                <div>
                  <dt>Job</dt>
                  <dd class="system-updates__job-id" :title="job.id">{{ job.id }}</dd>
                </div>
              </dl>
            </q-card-section>
          </q-card>
        </div>
      </section>

      <section class="system-updates__section" aria-labelledby="available-targets-title">
        <div class="system-updates__section-header">
          <div>
            <h2 id="available-targets-title" class="system-updates__section-title">
              Available targets
            </h2>
            <p>Review compatibility and affected game servers before starting an update.</p>
          </div>
          <span v-if="availabilityLastSyncedAt" class="system-updates__section-sync">
            Checked {{ formatTime(availabilityLastSyncedAt) }}
          </span>
        </div>

        <div v-if="availabilityError" class="system-updates__error" role="alert">
          <q-icon name="cloud_off" size="sm" />
          <div>
            <strong>Update availability could not be checked.</strong>
            <span>{{ availabilityError }}</span>
          </div>
          <q-btn
            :loading="availabilityLoading"
            dense
            flat
            icon="refresh"
            label="Retry"
            @click="loadAvailability()" />
        </div>

        <div
          v-if="contextError"
          class="system-updates__error system-updates__error--warning"
          role="alert">
          <q-icon name="warning_amber" size="sm" />
          <div>
            <strong>Affected game servers could not be verified.</strong>
            <span
              >{{ contextError }} Update actions stay disabled until this context is fresh.</span
            >
          </div>
          <q-btn
            :loading="contextLoading"
            dense
            flat
            icon="refresh"
            label="Retry"
            @click="loadContext()" />
        </div>

        <div v-if="availabilityLoading && !availabilityHasLoaded" class="system-updates__loading">
          <q-spinner color="primary" size="1.5rem" />
          <span>Checking controller and node targets…</span>
        </div>

        <q-table
          v-else-if="updates.length > 0"
          :columns="updateColumns"
          :grid="mobileGrid"
          :rows="updates"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="targetKey">
          <template #item="props">
            <q-card bordered class="system-updates__mobile-card" flat>
              <q-card-section class="system-updates__mobile-card-header">
                <div class="system-updates__target-copy">
                  <div class="system-updates__mobile-title">{{ targetName(props.row) }}</div>
                  <div class="system-updates__platform text-xy-muted">
                    {{ props.row.os || 'unknown' }} / {{ props.row.architecture || 'unknown' }}
                  </div>
                </div>
                <q-badge
                  :color="availabilityBadge(props.row).color"
                  :label="availabilityBadge(props.row).label" />
              </q-card-section>

              <q-separator />

              <q-card-section class="system-updates__mobile-fields">
                <div>
                  <span>Current</span>
                  <strong class="system-updates__version">{{
                    props.row.currentVersion || 'Unknown'
                  }}</strong>
                </div>
                <div>
                  <span>Latest</span>
                  <strong class="system-updates__version">{{
                    props.row.latestVersion || 'Unknown'
                  }}</strong>
                </div>
                <div v-if="props.row.artifactName">
                  <span>Artifact</span>
                  <strong>{{ props.row.artifactName }}</strong>
                </div>
                <div v-if="targetStateReason(props.row)">
                  <span>State</span>
                  <strong>{{ targetStateReason(props.row) }}</strong>
                </div>
              </q-card-section>

              <q-card-actions align="right">
                <q-btn
                  :disable="targetActionDisabled(props.row)"
                  :loading="preflightTargetKey === props.row.targetKey"
                  color="primary"
                  flat
                  icon="system_update_alt"
                  label="Update"
                  no-caps
                  @click="openConfirm(props.row)">
                  <q-tooltip v-if="targetActionReason(props.row)">
                    {{ targetActionReason(props.row) }}
                  </q-tooltip>
                </q-btn>
              </q-card-actions>
            </q-card>
          </template>

          <template #body-cell-target="props">
            <q-td :props="props">
              <div class="text-weight-medium system-updates__wrap">
                {{ targetName(props.row) }}
              </div>
              <div class="system-updates__platform text-xy-muted">
                {{ props.row.os || 'unknown' }} / {{ props.row.architecture || 'unknown' }}
              </div>
            </q-td>
          </template>
          <template #body-cell-version="props">
            <q-td :props="props">
              <div class="system-updates__version">
                {{ props.row.currentVersion || 'Unknown' }}
              </div>
              <div class="system-updates__platform text-xy-muted">
                Latest {{ props.row.latestVersion || 'Unknown' }}
              </div>
            </q-td>
          </template>
          <template #body-cell-status="props">
            <q-td :props="props">
              <q-badge
                :color="availabilityBadge(props.row).color"
                :label="availabilityBadge(props.row).label" />
              <div
                v-if="targetStateReason(props.row)"
                class="system-updates__table-reason text-xy-muted">
                {{ targetStateReason(props.row) }}
              </div>
            </q-td>
          </template>
          <template #body-cell-artifact="props">
            <q-td :props="props">
              <div class="system-updates__wrap">{{ props.row.artifactName || '—' }}</div>
              <div
                v-if="props.row.artifactSha256"
                class="system-updates__checksum text-xy-muted"
                :title="props.row.artifactSha256">
                {{ props.row.artifactSha256.slice(0, 12) }}
              </div>
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <q-btn
                :aria-label="`Update ${targetName(props.row)}`"
                :disable="targetActionDisabled(props.row)"
                :loading="preflightTargetKey === props.row.targetKey"
                color="primary"
                dense
                flat
                icon="system_update_alt"
                @click="openConfirm(props.row)">
                <q-tooltip>
                  {{ targetActionReason(props.row) || 'Start update' }}
                </q-tooltip>
              </q-btn>
            </q-td>
          </template>
        </q-table>

        <div v-else-if="availabilityHasLoaded && !availabilityError" class="system-updates__empty">
          <q-icon name="inventory_2" size="md" />
          <div>
            <strong>No update targets were returned</strong>
            <span>Resync to check the controller and registered nodes again.</span>
          </div>
        </div>
      </section>

      <section class="system-updates__section" aria-labelledby="update-history-title">
        <div class="system-updates__section-header">
          <div>
            <h2 id="update-history-title" class="system-updates__section-title">Update history</h2>
            <p>Completed controller and node update attempts.</p>
          </div>
        </div>

        <div v-if="jobsLoading && !jobsHasLoaded" class="system-updates__loading">
          <q-spinner color="primary" size="1.5rem" />
          <span>Loading update history…</span>
        </div>

        <q-table
          v-else-if="terminalJobs.length > 0"
          :columns="historyColumns"
          :grid="mobileGrid"
          :rows="terminalJobs"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="id">
          <template #item="props">
            <q-card bordered class="system-updates__mobile-card" flat>
              <q-card-section class="system-updates__mobile-card-header">
                <div class="system-updates__target-copy">
                  <div class="system-updates__mobile-title">{{ jobTargetName(props.row) }}</div>
                  <div class="system-updates__version text-xy-muted">
                    {{ props.row.targetVersion || 'Unknown version' }}
                  </div>
                </div>
                <q-badge
                  :color="jobProgressColor(props.row)"
                  :label="statusLabel(props.row.status)" />
              </q-card-section>
              <q-card-section class="system-updates__mobile-fields">
                <div>
                  <span>Completed</span>
                  <strong>{{
                    formatTimestamp(props.row.completedAt || props.row.updatedAt)
                  }}</strong>
                </div>
                <div>
                  <span>Requested by</span>
                  <strong>{{ requestedBy(props.row) }}</strong>
                </div>
                <div>
                  <span>Result</span>
                  <strong :class="{ 'text-negative': props.row.error }">
                    {{ props.row.error || latestJobMessage(props.row) }}
                  </strong>
                </div>
              </q-card-section>
            </q-card>
          </template>

          <template #body-cell-target="props">
            <q-td :props="props">
              <div class="text-weight-medium system-updates__wrap">
                {{ jobTargetName(props.row) }}
              </div>
              <div class="system-updates__version text-xy-muted">
                {{ props.row.targetVersion || 'Unknown version' }}
              </div>
            </q-td>
          </template>
          <template #body-cell-completed="props">
            <q-td :props="props">
              {{ formatTimestamp(props.row.completedAt || props.row.updatedAt) }}
            </q-td>
          </template>
          <template #body-cell-requester="props">
            <q-td :props="props">
              <span class="system-updates__wrap">{{ requestedBy(props.row) }}</span>
            </q-td>
          </template>
          <template #body-cell-result="props">
            <q-td :props="props">
              <q-badge
                :color="jobProgressColor(props.row)"
                :label="statusLabel(props.row.status)" />
            </q-td>
          </template>
          <template #body-cell-detail="props">
            <q-td :props="props">
              <span
                :class="['system-updates__history-detail', { 'text-negative': props.row.error }]">
                {{ props.row.error || latestJobMessage(props.row) }}
              </span>
            </q-td>
          </template>
        </q-table>

        <div v-else-if="jobsHasLoaded && !jobsError" class="system-updates__empty">
          <q-icon name="history" size="md" />
          <div>
            <strong>No completed updates yet</strong>
            <span>Successful and failed update jobs will be retained here.</span>
          </div>
        </div>
      </section>
    </div>

    <q-dialog v-model="confirmOpen" aria-labelledby="system-update-dialog-title" persistent>
      <q-card class="system-updates__dialog">
        <q-card-section class="system-updates__dialog-heading">
          <div>
            <div id="system-update-dialog-title" class="system-updates__dialog-title">
              Confirm system update
            </div>
            <div class="system-updates__version text-xy-muted">
              {{ selectedUpdate?.currentVersion || 'Unknown' }} →
              {{ selectedUpdate?.latestVersion || 'Unknown' }}
            </div>
          </div>
          <q-btn
            v-close-popup
            aria-label="Close confirmation"
            :disable="startPending"
            dense
            flat
            icon="close"
            round>
            <q-tooltip>Close</q-tooltip>
          </q-btn>
        </q-card-section>
        <q-card-section>
          <p class="system-updates__dialog-copy">
            Updating {{ selectedUpdate ? targetName(selectedUpdate) : 'this target' }} stops the
            game servers below. They remain offline after the update so you can verify them before
            starting them again.
          </p>

          <div v-if="contextError" class="system-updates__error" role="alert">
            <q-icon name="sync_problem" size="sm" />
            <div>
              <strong>Affected server context is unavailable.</strong>
              <span>{{ contextError }}</span>
            </div>
          </div>

          <div v-else class="system-updates__affected">
            <div class="system-updates__affected-heading">
              <strong>Affected game servers</strong>
              <span>{{ affectedServers.length }}</span>
            </div>
            <q-list bordered separator>
              <q-item v-for="server in affectedServers" :key="server.id">
                <q-item-section>
                  <q-item-label class="system-updates__wrap">{{ server.name }}</q-item-label>
                  <q-item-label caption class="system-updates__wrap">
                    {{ server.gameName || server.game?.name || server.id }}
                  </q-item-label>
                </q-item-section>
              </q-item>
              <q-item v-if="affectedServers.length === 0">
                <q-item-section>
                  <q-item-label class="text-xy-muted">
                    No game servers are assigned to this target.
                  </q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </div>
        </q-card-section>
        <q-card-actions class="system-updates__dialog-actions" align="right">
          <q-btn v-close-popup :disable="startPending" flat label="Cancel" />
          <q-btn
            :disable="!canSubmitSelected"
            :loading="startPending"
            color="primary"
            icon="system_update_alt"
            label="Start update"
            @click="startSelectedUpdate" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <div class="system-updates__sr-only" aria-atomic="true" aria-live="polite">
      {{ liveAnnouncement }}
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { Notify, useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import PageHeader from '@/components/shared/PageHeader.vue'
import type { GameServer, Node } from '@/proto/shared_pb'
import {
  CheckSystemUpdatesRequestSchema,
  GetSystemUpdateJobRequestSchema,
  ListGameServersRequestSchema,
  ListNodesRequestSchema,
  ListSystemUpdateJobsRequestSchema,
  StartSystemUpdateRequestSchema,
  SystemUpdateAvailability,
  SystemUpdateComponent,
  SystemUpdateJob,
  SystemUpdateJobStatus,
  SystemUpdatePhase,
  SystemUpdateProgress,
} from '@/proto/xylona_pb'
import {
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  reconnectControllerWebsocket,
  XylonaEventBus,
} from '@/utils/shared'
import {
  websocketBrowserOnline,
  websocketConnectionEpoch,
  websocketConnectionStatus,
  websocketStateAuthoritative,
} from '@/utils/websocket-connection'

type UpdateRow = SystemUpdateAvailability & { targetKey: string }

type ProgressOverlay = {
  progress: SystemUpdateProgress
  receivedAt: number
  revision: number
}

type AuthoritativeVersion = {
  updatedAtMs: number
  requestOrder: number
  mergeSequence: number
}

type DetailRequest = {
  controller: AbortController
  generation: number
  promise: Promise<boolean>
}

type LiveTone = 'live' | 'recovering' | 'offline'

const JOB_POLL_INTERVAL_MS = 10_000
const JOB_LIMIT = 100
const DETAIL_REQUEST_TIMEOUT_MS = 15_000
const START_REQUEST_TIMEOUT_MS = 20_000

const route = useRoute()
const $q = useQuasar()
const mobileGrid = computed(() => $q.screen?.lt?.md ?? false)

const updates = ref<UpdateRow[]>([])
const jobs = ref<SystemUpdateJob[]>([])
const nodes = ref<Node[]>([])
const gameServers = ref<GameServer[]>([])
const progressOverlays = ref<Map<string, ProgressOverlay>>(new Map())
const latestMessages = ref<Map<string, string>>(new Map())

const availabilityLoading = ref(false)
const availabilityError = ref('')
const availabilityHasLoaded = ref(false)
const availabilityLastSyncedAt = ref<number | null>(null)
const jobsLoading = ref(false)
const jobsError = ref('')
const jobsHasLoaded = ref(false)
const jobsLastSyncedAt = ref<number | null>(null)
const contextLoading = ref(false)
const contextError = ref('')
const contextHasLoaded = ref(false)
const contextLastSyncedAt = ref<number | null>(null)
const lastLiveProgressAt = ref<number | null>(null)

const confirmOpen = ref(false)
const selectedUpdate = ref<UpdateRow | null>(null)
const preflightTargetKey = ref('')
const startPending = ref(false)
const ambiguousStartTargetKey = ref('')
const controllerReloadJobId = ref<string | null>(null)
const liveAnnouncement = ref('')
const documentVisible = ref(document.visibilityState !== 'hidden')

let mounted = false
let liveRevision = 0
let authoritativeRequestOrder = 0
let authoritativeMergeSequence = 0
let detailGeneration = 0
let availabilityGeneration = 0
let jobsGeneration = 0
let contextGeneration = 0
let refreshGeneration = 0
let availabilityAbortController: AbortController | null = null
let jobsAbortController: AbortController | null = null
let contextAbortController: AbortController | null = null
let refreshPromise: Promise<void> | null = null
let refreshQueued = false
let activePollPromise: Promise<void> | null = null
let pollTimer: ReturnType<typeof setTimeout> | null = null

const authoritativeVersions = new Map<string, AuthoritativeVersion>()
const detailRequests = new Map<string, DetailRequest>()
const handledTerminalJobs = new Set<string>()
const reloadPromptedJobs = new Set<string>()
const lastAnnouncedState = new Map<string, string>()
const liveObservedJobs = new Set<string>()

const displayJobs = computed(() =>
  jobs.value.map((job) => applyProgressOverlay(job, progressOverlays.value.get(job.id)?.progress)),
)

const activeJobs = computed(() => displayJobs.value.filter((job) => !isTerminalStatus(job.status)))

const reconciliationJobIds = computed(() => {
  const ids = new Set(
    jobs.value.filter((job) => !isTerminalStatus(job.status)).map((job) => job.id),
  )
  for (const [jobId] of progressOverlays.value) {
    const authoritativeJob = jobs.value.find((job) => job.id === jobId)
    if (!authoritativeJob || !isTerminalStatus(authoritativeJob.status)) {
      ids.add(jobId)
    }
  }
  return [...ids]
})

const terminalJobs = computed(() => displayJobs.value.filter((job) => isTerminalStatus(job.status)))

const activeTargetKeys = computed(() => {
  const targetKeys = new Set(
    jobs.value.filter((job) => !isTerminalStatus(job.status)).map((job) => targetKeyForJob(job)),
  )
  for (const [jobId, overlay] of progressOverlays.value) {
    const authoritativeJob = jobs.value.find((job) => job.id === jobId)
    if (!authoritativeJob || !isTerminalStatus(authoritativeJob.status)) {
      targetKeys.add(
        authoritativeJob
          ? targetKeyForJob(authoritativeJob)
          : systemUpdateTargetKey(overlay.progress.component, overlay.progress.nodeId),
      )
    }
  }
  return targetKeys
})

const affectedServers = computed(() => {
  const update = selectedUpdate.value
  if (!update || contextError.value || !contextHasLoaded.value) return []
  const nodeId = nodeIdForUpdate(update)
  if (!nodeId) return []
  return gameServers.value.filter((server) => server.nodeId === nodeId)
})

const canSubmitSelected = computed(() => {
  const update = selectedUpdate.value
  if (!update || startPending.value || preflightTargetKey.value) return false
  if (!websocketBrowserOnline.value) return false
  if (!availabilityHasLoaded.value || availabilityLoading.value || availabilityError.value) {
    return false
  }
  if (!contextHasLoaded.value || contextLoading.value || contextError.value) return false
  if (!jobsHasLoaded.value || jobsLoading.value || jobsError.value) return false
  if (ambiguousStartTargetKey.value === update.targetKey) return false
  const freshUpdate = updates.value.find((candidate) => candidate.targetKey === update.targetKey)
  return Boolean(freshUpdate?.updateable) && !activeTargetKeys.value.has(update.targetKey)
})

const isRefreshing = computed(
  () => availabilityLoading.value || jobsLoading.value || contextLoading.value,
)

const lastSynchronizedAt = computed(() => {
  const timestamps = [
    availabilityLastSyncedAt.value,
    jobsLastSyncedAt.value,
    contextLastSyncedAt.value,
    lastLiveProgressAt.value,
  ].filter((value): value is number => value !== null)
  return timestamps.length > 0 ? Math.max(...timestamps) : null
})

const lastSynchronizedLabel = computed(() =>
  lastSynchronizedAt.value
    ? `Last synchronized ${formatTime(lastSynchronizedAt.value)}`
    : 'Not synchronized yet',
)

const liveStatus = computed<{ title: string; detail: string; icon: string; tone: LiveTone }>(() => {
  if (!websocketBrowserOnline.value) {
    return {
      title: 'Offline — state may be stale',
      detail: 'Reconnect to resume live progress and persisted-state checks.',
      icon: 'cloud_off',
      tone: 'offline',
    }
  }
  if (websocketStateAuthoritative.value) {
    if (jobsHasLoaded.value && !jobsError.value && activeJobs.value.length === 0) {
      return {
        title: 'No updates in progress',
        detail: 'Live updates are connected. New controller and node updates will appear here.',
        icon: 'task_alt',
        tone: 'live',
      }
    }
    return {
      title: 'Live updates connected',
      detail: 'Progress events are active and persisted state is reconciled in the background.',
      icon: 'sensors',
      tone: 'live',
    }
  }
  return {
    title:
      websocketConnectionStatus.value === 'connecting'
        ? 'Connecting to live updates'
        : 'Reconnecting to live updates',
    detail: 'The page is showing last-known state with RPC fallback checks.',
    icon: 'sync_problem',
    tone: 'recovering',
  }
})

const updateColumns = [
  { name: 'target', label: 'Target', align: 'left' as const, field: 'targetKey', sortable: true },
  { name: 'version', label: 'Version', align: 'left' as const, field: 'latestVersion' },
  { name: 'status', label: 'State', align: 'left' as const, field: 'updateable' },
  { name: 'artifact', label: 'Artifact', align: 'left' as const, field: 'artifactName' },
  { name: 'actions', label: '', align: 'center' as const, field: 'targetKey' },
]

const historyColumns = [
  { name: 'target', label: 'Target', align: 'left' as const, field: 'nodeName' },
  { name: 'completed', label: 'Completed', align: 'left' as const, field: 'completedAt' },
  {
    name: 'requester',
    label: 'Requested by',
    align: 'left' as const,
    field: 'requestedByUserName',
  },
  { name: 'result', label: 'Result', align: 'left' as const, field: 'status' },
  { name: 'detail', label: 'Detail', align: 'left' as const, field: 'error' },
]

onMounted(async () => {
  mounted = true
  XylonaEventBus.on('systemUpdateProgress', onSystemUpdateProgress)
  GetOrCreateXylonaWebsocketClient()
  window.addEventListener('pageshow', onPageShow)
  document.addEventListener('visibilitychange', onVisibilityChange)
  await refreshAll()
  await reconcileActiveJobs()
  scheduleActivePoll()
})

onBeforeUnmount(() => {
  mounted = false
  XylonaEventBus.off('systemUpdateProgress', onSystemUpdateProgress)
  window.removeEventListener('pageshow', onPageShow)
  document.removeEventListener('visibilitychange', onVisibilityChange)
  supersedeSectionRefreshes()
  supersedeDetailReconciliation()
})

watch(websocketConnectionEpoch, (epoch, previousEpoch) => {
  if (mounted && epoch > previousEpoch) {
    void queueCatchUp(true)
  }
})

watch(websocketBrowserOnline, (online, wasOnline) => {
  if (!mounted) return
  if (!online) {
    supersedeSectionRefreshes()
    supersedeDetailReconciliation()
    return
  }
  if (online && !wasOnline) {
    void queueCatchUp(true)
    return
  }
  scheduleActivePoll()
})

watch(
  () => reconciliationJobIds.value.length,
  () => {
    scheduleActivePoll()
  },
)

async function loadAvailability(): Promise<boolean> {
  const generation = ++availabilityGeneration
  availabilityAbortController?.abort()
  const controller = new AbortController()
  availabilityAbortController = controller
  availabilityLoading.value = true

  try {
    const response = await GetXylonaClient().checkSystemUpdates(
      create(CheckSystemUpdatesRequestSchema, { includeNodes: true }),
      { signal: controller.signal },
    )
    if (!mounted || controller.signal.aborted || generation !== availabilityGeneration) {
      return false
    }

    const focusedNodeId = String(route.query.nodeId ?? '')
    const mappedUpdates = response.updates.map((update) => ({
      ...update,
      targetKey: targetKeyForAvailability(update),
    }))
    // Surface the node the operator navigated from without disturbing server order.
    const refreshedUpdates = focusedNodeId
      ? [
          ...mappedUpdates.filter((update) => update.nodeId === focusedNodeId),
          ...mappedUpdates.filter((update) => update.nodeId !== focusedNodeId),
        ]
      : mappedUpdates
    updates.value = refreshedUpdates
    if (selectedUpdate.value) {
      const refreshedSelection = refreshedUpdates.find(
        (update) => update.targetKey === selectedUpdate.value?.targetKey,
      )
      if (refreshedSelection) {
        selectedUpdate.value = refreshedSelection
      }
    }
    availabilityError.value = ''
    availabilityHasLoaded.value = true
    availabilityLastSyncedAt.value = Date.now()
    return true
  } catch (unknownError: unknown) {
    if (controller.signal.aborted || generation !== availabilityGeneration || !mounted) {
      return false
    }
    availabilityError.value = errorMessage(unknownError)
    return false
  } finally {
    if (generation === availabilityGeneration) {
      availabilityLoading.value = false
      if (availabilityAbortController === controller) {
        availabilityAbortController = null
      }
    }
  }
}

async function loadJobs(): Promise<boolean> {
  const generation = ++jobsGeneration
  const requestRevision = liveRevision
  const requestOrder = beginAuthoritativeRequest()
  jobsAbortController?.abort()
  const controller = new AbortController()
  jobsAbortController = controller
  jobsLoading.value = true

  try {
    const response = await GetXylonaClient().listSystemUpdateJobs(
      create(ListSystemUpdateJobsRequestSchema, { limit: JOB_LIMIT }),
      { signal: controller.signal },
    )
    if (!mounted || controller.signal.aborted || generation !== jobsGeneration) return false

    const wasHydrated = jobsHasLoaded.value
    jobsHasLoaded.value = true
    const responseIds = new Set(response.jobs.map((job) => job.id))
    for (const job of response.jobs) {
      mergeAuthoritativeJob(job, requestRevision, requestOrder, !wasHydrated)
    }
    const retainedJobs = jobs.value.filter((job) => {
      if (responseIds.has(job.id)) return true
      const version = authoritativeVersions.get(job.id)
      return (
        !isTerminalStatus(job.status) ||
        progressOverlays.value.has(job.id) ||
        (version?.requestOrder ?? 0) > requestOrder
      )
    })
    const retainedIds = new Set(retainedJobs.map((job) => job.id))
    for (const job of jobs.value) {
      if (!retainedIds.has(job.id)) {
        removeJobState(job.id)
      }
    }
    jobs.value = sortJobs(retainedJobs)
    handleAuthoritativeTerminals(jobs.value, !wasHydrated)

    jobsError.value = ''
    jobsLastSyncedAt.value = Date.now()

    if (ambiguousStartTargetKey.value) {
      ambiguousStartTargetKey.value = ''
    }

    return true
  } catch (unknownError: unknown) {
    if (controller.signal.aborted || generation !== jobsGeneration || !mounted) {
      return false
    }
    jobsError.value = errorMessage(unknownError)
    return false
  } finally {
    if (generation === jobsGeneration) {
      jobsLoading.value = false
      if (jobsAbortController === controller) {
        jobsAbortController = null
      }
    }
  }
}

async function loadContext(): Promise<boolean> {
  const generation = ++contextGeneration
  contextAbortController?.abort()
  const controller = new AbortController()
  contextAbortController = controller
  contextLoading.value = true

  try {
    const client = GetXylonaClient()
    const [nodesResponse, serversResponse] = await Promise.all([
      client.listNodes(create(ListNodesRequestSchema, {}), { signal: controller.signal }),
      client.listGameServers(create(ListGameServersRequestSchema, {}), {
        signal: controller.signal,
      }),
    ])
    if (!mounted || controller.signal.aborted || generation !== contextGeneration) return false

    nodes.value = nodesResponse.nodes
    gameServers.value = serversResponse.gameServers
    contextError.value = ''
    contextHasLoaded.value = true
    contextLastSyncedAt.value = Date.now()
    return true
  } catch (unknownError: unknown) {
    if (controller.signal.aborted || generation !== contextGeneration || !mounted) {
      return false
    }
    contextError.value = errorMessage(unknownError)
    return false
  } finally {
    if (generation === contextGeneration) {
      contextLoading.value = false
      if (contextAbortController === controller) {
        contextAbortController = null
      }
    }
  }
}

async function refreshAll(): Promise<void> {
  if (refreshPromise) {
    refreshQueued = true
    return refreshPromise
  }

  const generation = ++refreshGeneration
  const currentRefresh = (async () => {
    do {
      refreshQueued = false
      await Promise.all([loadJobs(), loadAvailability(), loadContext()])
    } while (mounted && generation === refreshGeneration && refreshQueued)
  })()
  refreshPromise = currentRefresh

  try {
    await currentRefresh
  } finally {
    if (refreshPromise === currentRefresh) {
      refreshPromise = null
    }
  }
}

async function queueCatchUp(supersedeRequests = false): Promise<void> {
  if (supersedeRequests) {
    supersedeSectionRefreshes()
    supersedeDetailReconciliation()
  }
  const refresh = refreshAll()
  const generation = refreshGeneration
  await refresh
  if (!mounted || generation !== refreshGeneration) return
  await reconcileActiveJobs()
  if (!mounted || generation !== refreshGeneration) return
  scheduleActivePoll()
}

function supersedeSectionRefreshes(): void {
  refreshGeneration += 1
  refreshQueued = false
  refreshPromise = null

  availabilityGeneration += 1
  jobsGeneration += 1
  contextGeneration += 1

  availabilityAbortController?.abort()
  jobsAbortController?.abort()
  contextAbortController?.abort()
  availabilityAbortController = null
  jobsAbortController = null
  contextAbortController = null

  availabilityLoading.value = false
  jobsLoading.value = false
  contextLoading.value = false
}

async function resyncPage(): Promise<void> {
  if (!websocketBrowserOnline.value) return
  if (!websocketStateAuthoritative.value) {
    reconnectControllerWebsocket()
  }
  await queueCatchUp(true)
}

async function openConfirm(update: UpdateRow): Promise<void> {
  if (targetActionDisabled(update)) return
  preflightTargetKey.value = update.targetKey
  const expectedTargetKey = update.targetKey

  try {
    await refreshAll()
    const refreshedUpdate = updates.value.find(
      (candidate) => candidate.targetKey === expectedTargetKey,
    )
    const contextReady =
      Boolean(refreshedUpdate) &&
      availabilityHasLoaded.value &&
      !availabilityError.value &&
      jobsHasLoaded.value &&
      !jobsError.value &&
      contextHasLoaded.value &&
      !contextError.value

    if (!contextReady || !refreshedUpdate) {
      notifyPreflightUnavailable()
      return
    }
    if (!refreshedUpdate.updateable || activeTargetKeys.value.has(expectedTargetKey)) {
      notifyPreflightUnavailable(
        activeTargetKeys.value.has(expectedTargetKey)
          ? 'An update is already active for this target.'
          : refreshedUpdate.reason || 'This target is not currently updateable.',
      )
      return
    }

    selectedUpdate.value = refreshedUpdate
    confirmOpen.value = true
  } finally {
    preflightTargetKey.value = ''
  }
}

async function startSelectedUpdate(): Promise<void> {
  const update = selectedUpdate.value
  if (!update || !canSubmitSelected.value) return
  const knownJobIds = new Set(jobs.value.map((job) => job.id))
  const requestRevision = liveRevision
  const requestOrder = beginAuthoritativeRequest()
  startPending.value = true

  try {
    const response = await GetXylonaClient().startSystemUpdate(
      create(StartSystemUpdateRequestSchema, {
        component: update.component,
        nodeId: update.nodeId,
        targetVersion: update.latestVersion,
        confirmedDrain: true,
      }),
      { timeoutMs: START_REQUEST_TIMEOUT_MS },
    )
    if (response.job) {
      mergeAuthoritativeJob(response.job, requestRevision, requestOrder)
    }
    confirmOpen.value = false
    selectedUpdate.value = null
    scheduleActivePoll()
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    if (isAmbiguousStartError(err)) {
      ambiguousStartTargetKey.value = update.targetKey
      const reconciled = await loadJobs()
      const recoveredJob = displayJobs.value.find(
        (job) =>
          targetKeyForJob(job) === update.targetKey &&
          (!knownJobIds.has(job.id) || !isTerminalStatus(job.status)),
      )
      if (recoveredJob) {
        confirmOpen.value = false
        selectedUpdate.value = null
        Notify.create({
          type: 'xylona-info',
          position: 'top',
          caption:
            'The update request reached the controller. Its persisted job state was recovered.',
          icon: 'sync',
        })
        return
      }
      if (!reconciled) {
        Notify.create({
          type: 'xylona-error',
          position: 'top',
          caption: 'The update request outcome is unknown. Resync update jobs before trying again.',
          timeout: 0,
          closeBtn: 'Dismiss',
          icon: 'report_problem',
        })
        return
      }
    }
    notifyError(err)
  } finally {
    startPending.value = false
  }
}

function onSystemUpdateProgress(progress: SystemUpdateProgress): void {
  if (!progress.jobId) return
  const currentJob = jobs.value.find((job) => job.id === progress.jobId)
  const currentDisplay = currentJob ? displayJob(currentJob) : undefined
  if (currentDisplay && isTerminalStatus(currentDisplay.status)) return

  const normalized = normalizeProgress(currentDisplay, progress)
  liveObservedJobs.add(progress.jobId)
  const nextOverlays = new Map(progressOverlays.value)
  const receivedAt = Date.now()
  liveRevision += 1
  nextOverlays.set(progress.jobId, {
    progress: normalized,
    receivedAt,
    revision: liveRevision,
  })
  progressOverlays.value = nextOverlays
  lastLiveProgressAt.value = receivedAt

  if (normalized.message) {
    setLatestMessage(progress.jobId, normalized.message)
  }
  announceJobState(progress.jobId, currentDisplay, normalized)

  if (!currentJob || isTerminalStatus(normalized.status)) {
    void reconcileJob(progress.jobId)
  }
  scheduleActivePoll()
}

function reconcileJob(jobId: string): Promise<boolean> {
  const existing = detailRequests.get(jobId)
  if (existing?.generation === detailGeneration) return existing.promise

  const generation = detailGeneration
  const requestRevision = liveRevision
  const requestOrder = beginAuthoritativeRequest()
  const controller = new AbortController()
  const entry: DetailRequest = {
    controller,
    generation,
    promise: Promise.resolve(false),
  }
  entry.promise = (async () => {
    try {
      const response = await GetXylonaClient().getSystemUpdateJob(
        create(GetSystemUpdateJobRequestSchema, { jobId }),
        {
          signal: controller.signal,
          timeoutMs: DETAIL_REQUEST_TIMEOUT_MS,
        },
      )
      if (
        !mounted ||
        controller.signal.aborted ||
        generation !== detailGeneration ||
        detailRequests.get(jobId) !== entry ||
        !response.job
      ) {
        return false
      }

      const latestEvent = [...response.events].sort(
        (a, b) => timestampMilliseconds(b.createdAt) - timestampMilliseconds(a.createdAt),
      )[0]
      const overlay = progressOverlays.value.get(jobId)
      const accepted = mergeAuthoritativeJob(response.job, requestRevision, requestOrder)
      if (!accepted) {
        jobsLastSyncedAt.value = Date.now()
        return true
      }
      if (isTerminalStatus(response.job.status)) {
        if (latestEvent?.message) {
          setLatestMessage(jobId, latestEvent.message)
        } else if (overlay && overlay.progress.status !== response.job.status) {
          deleteLatestMessage(jobId)
        }
      } else if (
        latestEvent?.message &&
        (!overlay ||
          (overlay.revision <= requestRevision &&
            authoritativeCatchesOverlay(response.job, overlay.progress)))
      ) {
        setLatestMessage(jobId, latestEvent.message)
      }
      jobsLastSyncedAt.value = Date.now()
      return true
    } catch {
      return false
    } finally {
      if (detailRequests.get(jobId) === entry) {
        detailRequests.delete(jobId)
      }
    }
  })()

  detailRequests.set(jobId, entry)
  return entry.promise
}

async function reconcileActiveJobs(): Promise<void> {
  if (activePollPromise) return activePollPromise
  if (!mounted || !documentVisible.value || !websocketBrowserOnline.value) return
  const jobIds = reconciliationJobIds.value
  if (jobIds.length === 0) return

  const pollPromise = (async () => {
    await Promise.all(jobIds.map((jobId) => reconcileJob(jobId)))
  })()
  activePollPromise = pollPromise
  try {
    await pollPromise
  } finally {
    if (activePollPromise === pollPromise) {
      activePollPromise = null
    }
  }
}

function supersedeDetailReconciliation(): void {
  detailGeneration += 1
  for (const request of detailRequests.values()) {
    request.controller.abort()
  }
  detailRequests.clear()
  activePollPromise = null
  clearActivePoll()
}

function mergeAuthoritativeJob(
  job: SystemUpdateJob,
  requestRevision: number,
  requestOrder: number,
  suppressTerminalEffects = false,
): boolean {
  const mergeSequence = ++authoritativeMergeSequence
  const existingIndex = jobs.value.findIndex((candidate) => candidate.id === job.id)
  const existingJob = existingIndex >= 0 ? jobs.value[existingIndex] : undefined
  const candidateVersion: AuthoritativeVersion = {
    updatedAtMs: timestampMilliseconds(job.updatedAt),
    requestOrder,
    mergeSequence,
  }
  const existingVersion = authoritativeVersions.get(job.id)
  if (
    existingJob &&
    !shouldAcceptAuthoritativeJob(existingJob, job, existingVersion, candidateVersion)
  ) {
    return false
  }

  const nextJobs = [...jobs.value]
  if (existingIndex >= 0) {
    nextJobs.splice(existingIndex, 1, job)
  } else {
    nextJobs.push(job)
  }
  authoritativeVersions.set(job.id, candidateVersion)
  jobs.value = sortJobs(nextJobs)
  clearCaughtUpOverlays([job], requestRevision)
  handleAuthoritativeTerminals([job], suppressTerminalEffects)
  return true
}

function shouldAcceptAuthoritativeJob(
  existingJob: SystemUpdateJob,
  candidateJob: SystemUpdateJob,
  existingVersion: AuthoritativeVersion | undefined,
  candidateVersion: AuthoritativeVersion,
): boolean {
  if (isTerminalStatus(existingJob.status) && !isTerminalStatus(candidateJob.status)) {
    return false
  }
  if (!existingVersion) return true
  if (
    candidateVersion.updatedAtMs > 0 &&
    existingVersion.updatedAtMs > 0 &&
    candidateVersion.updatedAtMs !== existingVersion.updatedAtMs
  ) {
    return candidateVersion.updatedAtMs > existingVersion.updatedAtMs
  }
  if (candidateVersion.requestOrder !== existingVersion.requestOrder) {
    return candidateVersion.requestOrder > existingVersion.requestOrder
  }
  return candidateVersion.mergeSequence > existingVersion.mergeSequence
}

function beginAuthoritativeRequest(): number {
  authoritativeRequestOrder += 1
  return authoritativeRequestOrder
}

function removeJobState(jobId: string): void {
  authoritativeVersions.delete(jobId)
  handledTerminalJobs.delete(jobId)
  reloadPromptedJobs.delete(jobId)
  lastAnnouncedState.delete(jobId)
  liveObservedJobs.delete(jobId)

  if (progressOverlays.value.has(jobId)) {
    const nextOverlays = new Map(progressOverlays.value)
    nextOverlays.delete(jobId)
    progressOverlays.value = nextOverlays
  }
  deleteLatestMessage(jobId)
}

function clearCaughtUpOverlays(
  authoritativeJobs: SystemUpdateJob[],
  requestRevision: number,
): void {
  const nextOverlays = new Map(progressOverlays.value)
  const nextMessages = new Map(latestMessages.value)
  for (const job of authoritativeJobs) {
    const overlay = nextOverlays.get(job.id)
    if (!overlay) continue
    const overlayIsNewerThanRequest = overlay.revision > requestRevision
    if (
      isTerminalStatus(job.status) ||
      (!overlayIsNewerThanRequest && authoritativeCatchesOverlay(job, overlay.progress))
    ) {
      nextOverlays.delete(job.id)
      if (isTerminalStatus(job.status) && overlay.progress.status !== job.status) {
        nextMessages.delete(job.id)
      }
    }
  }
  progressOverlays.value = nextOverlays
  latestMessages.value = nextMessages
}

function handleAuthoritativeTerminals(
  authoritativeJobs: SystemUpdateJob[],
  suppressEffects = false,
): void {
  if (!jobsHasLoaded.value) return
  for (const job of authoritativeJobs) {
    if (!isTerminalStatus(job.status) || handledTerminalJobs.has(job.id)) continue
    handledTerminalJobs.add(job.id)
    if (suppressEffects && !liveObservedJobs.has(job.id)) continue
    announceTerminalJob(job)
    void loadAvailability()

    if (
      job.status === SystemUpdateJobStatus.SUCCEEDED &&
      job.component === SystemUpdateComponent.CONTROLLER &&
      !reloadPromptedJobs.has(job.id)
    ) {
      reloadPromptedJobs.add(job.id)
      controllerReloadJobId.value = job.id
    }
  }
}

function scheduleActivePoll(): void {
  clearActivePoll()
  if (
    !mounted ||
    !documentVisible.value ||
    !websocketBrowserOnline.value ||
    reconciliationJobIds.value.length === 0
  ) {
    return
  }
  const scheduledGeneration = detailGeneration
  pollTimer = setTimeout(async () => {
    pollTimer = null
    await reconcileActiveJobs()
    if (scheduledGeneration === detailGeneration) {
      scheduleActivePoll()
    }
  }, JOB_POLL_INTERVAL_MS)
}

function clearActivePoll(): void {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

function onVisibilityChange(): void {
  documentVisible.value = document.visibilityState !== 'hidden'
  if (documentVisible.value && websocketBrowserOnline.value) {
    void queueCatchUp(true)
    return
  }
  scheduleActivePoll()
}

function onPageShow(): void {
  documentVisible.value = document.visibilityState !== 'hidden'
  if (websocketBrowserOnline.value) {
    void queueCatchUp(true)
    return
  }
  scheduleActivePoll()
}

function displayJob(job: SystemUpdateJob): SystemUpdateJob {
  return applyProgressOverlay(job, progressOverlays.value.get(job.id)?.progress)
}

function applyProgressOverlay(
  job: SystemUpdateJob,
  progress: SystemUpdateProgress | undefined,
): SystemUpdateJob {
  if (!progress || isTerminalStatus(job.status)) return job
  return {
    ...job,
    status: progress.status,
    phase: progress.phase,
    progressPercent: clampPercent(progress.progressPercent),
    targetVersion: progress.targetVersion || job.targetVersion,
    error: progress.error || job.error,
  }
}

function normalizeProgress(
  current: SystemUpdateJob | undefined,
  progress: SystemUpdateProgress,
): SystemUpdateProgress {
  if (!current) {
    return {
      ...progress,
      progressPercent: clampPercent(progress.progressPercent),
    }
  }

  const currentIsTerminal = isTerminalStatus(current.status)
  if (currentIsTerminal) {
    return {
      ...progress,
      status: current.status,
      phase: current.phase,
      progressPercent: clampPercent(current.progressPercent),
      error: current.error || progress.error,
      targetVersion: current.targetVersion || progress.targetVersion,
    }
  }

  const nextIsTerminal = isTerminalStatus(progress.status)
  return {
    ...progress,
    status: nextIsTerminal || progress.status >= current.status ? progress.status : current.status,
    phase: progress.phase >= current.phase ? progress.phase : current.phase,
    progressPercent: Math.max(
      clampPercent(current.progressPercent),
      clampPercent(progress.progressPercent),
    ),
    error: progress.error || current.error,
    targetVersion: progress.targetVersion || current.targetVersion,
  }
}

function authoritativeCatchesOverlay(
  job: SystemUpdateJob,
  progress: SystemUpdateProgress,
): boolean {
  if (isTerminalStatus(job.status)) return true
  return (
    job.status >= progress.status &&
    job.phase >= progress.phase &&
    clampPercent(job.progressPercent) >= clampPercent(progress.progressPercent)
  )
}

function announceJobState(
  jobId: string,
  previous: SystemUpdateJob | undefined,
  progress: SystemUpdateProgress,
): void {
  const stateKey = `${progress.status}:${progress.phase}`
  if (lastAnnouncedState.get(jobId) === stateKey) return
  if (
    previous &&
    previous.status === progress.status &&
    previous.phase === progress.phase &&
    !isTerminalStatus(progress.status)
  ) {
    return
  }
  lastAnnouncedState.set(jobId, stateKey)
  const target =
    progress.component === SystemUpdateComponent.CONTROLLER
      ? 'Controller'
      : jobs.value.find((job) => job.id === jobId)?.nodeName || 'Node'
  liveAnnouncement.value = isTerminalStatus(progress.status)
    ? `${target} update ${statusLabel(progress.status).toLowerCase()}.`
    : `${target} update entered ${phaseLabel(progress.phase).toLowerCase()}.`
}

function announceTerminalJob(job: SystemUpdateJob): void {
  const stateKey = `${job.status}:${job.phase}`
  if (lastAnnouncedState.get(job.id) === stateKey) return
  lastAnnouncedState.set(job.id, stateKey)
  liveAnnouncement.value = `${jobTargetName(job)} update ${statusLabel(job.status).toLowerCase()}.`
}

function setLatestMessage(jobId: string, message: string): void {
  const nextMessages = new Map(latestMessages.value)
  nextMessages.set(jobId, message)
  latestMessages.value = nextMessages
}

function deleteLatestMessage(jobId: string): void {
  const nextMessages = new Map(latestMessages.value)
  nextMessages.delete(jobId)
  latestMessages.value = nextMessages
}

function latestJobMessage(job: SystemUpdateJob): string {
  const message = latestMessages.value.get(job.id)
  if (message) return message
  if (job.error) return job.error
  if (job.status === SystemUpdateJobStatus.SUCCEEDED) return 'Update completed successfully.'
  if (job.status === SystemUpdateJobStatus.FAILED) return 'Update failed.'
  return `${phaseLabel(job.phase)} is in progress.`
}

function targetName(update: SystemUpdateAvailability): string {
  if (update.component === SystemUpdateComponent.CONTROLLER) return 'Controller'
  return update.nodeName || update.nodeId || 'Node'
}

function jobTargetName(job: SystemUpdateJob): string {
  if (job.component === SystemUpdateComponent.CONTROLLER) return 'Controller'
  return job.nodeName || job.nodeId || 'Node'
}

function targetKeyForAvailability(update: SystemUpdateAvailability): string {
  return systemUpdateTargetKey(update.component, update.nodeId)
}

function targetKeyForJob(job: SystemUpdateJob): string {
  return systemUpdateTargetKey(job.component, job.nodeId)
}

function systemUpdateTargetKey(component: SystemUpdateComponent, nodeId: string): string {
  return `${component}:${
    component === SystemUpdateComponent.CONTROLLER ? 'controller' : nodeId || 'node'
  }`
}

function nodeIdForUpdate(update: SystemUpdateAvailability): string {
  if (update.component === SystemUpdateComponent.NODE) return update.nodeId
  return nodes.value.find((node) => node.local)?.id ?? ''
}

function availabilityBadge(update: UpdateRow): { color: string; label: string } {
  if (activeTargetKeys.value.has(update.targetKey)) {
    return { color: 'primary', label: 'Update in progress' }
  }
  if (!update.updateAvailable) return { color: 'grey-6', label: 'Current' }
  if (update.updateable) return { color: 'positive', label: 'Ready' }
  return { color: 'warning', label: 'Blocked' }
}

function targetStateReason(update: UpdateRow): string {
  if (activeTargetKeys.value.has(update.targetKey)) {
    return 'An update job is already active for this target.'
  }
  return update.reason
}

function targetActionDisabled(update: UpdateRow): boolean {
  return Boolean(targetActionReason(update))
}

function targetActionReason(update: UpdateRow): string {
  if (preflightTargetKey.value || startPending.value) {
    return 'Another update action is being prepared.'
  }
  if (ambiguousStartTargetKey.value === update.targetKey) {
    return 'The previous start request must be reconciled before another attempt.'
  }
  if (activeTargetKeys.value.has(update.targetKey)) {
    return 'An update is already active for this target.'
  }
  if (!update.updateable) return update.reason || 'This target is not updateable.'
  if (!websocketBrowserOnline.value) {
    return 'Reconnect before starting a system update.'
  }
  if (!availabilityHasLoaded.value || availabilityLoading.value || availabilityError.value) {
    return 'Update availability must be synchronized first.'
  }
  if (!jobsHasLoaded.value || jobsLoading.value || jobsError.value) {
    return 'Active update jobs must be synchronized first.'
  }
  if (!contextHasLoaded.value || contextLoading.value || contextError.value) {
    return 'Affected game servers must be synchronized first.'
  }
  return ''
}

function statusLabel(status: SystemUpdateJobStatus): string {
  const labels: Record<SystemUpdateJobStatus, string> = {
    [SystemUpdateJobStatus.UNSPECIFIED]: 'Unknown',
    [SystemUpdateJobStatus.PENDING]: 'Pending',
    [SystemUpdateJobStatus.RUNNING]: 'Running',
    [SystemUpdateJobStatus.DRAINING]: 'Draining',
    [SystemUpdateJobStatus.DOWNLOADING]: 'Downloading',
    [SystemUpdateJobStatus.STAGING]: 'Staging',
    [SystemUpdateJobStatus.APPLYING]: 'Applying',
    [SystemUpdateJobStatus.RESTARTING]: 'Restarting',
    [SystemUpdateJobStatus.SUCCEEDED]: 'Succeeded',
    [SystemUpdateJobStatus.FAILED]: 'Failed',
  }
  return labels[status] ?? 'Unknown'
}

function phaseLabel(phase: SystemUpdatePhase): string {
  const labels: Record<SystemUpdatePhase, string> = {
    [SystemUpdatePhase.UNSPECIFIED]: 'Preparing',
    [SystemUpdatePhase.CHECK]: 'Checking release',
    [SystemUpdatePhase.PREFLIGHT]: 'Running preflight checks',
    [SystemUpdatePhase.DRAIN]: 'Stopping game servers',
    [SystemUpdatePhase.DOWNLOAD]: 'Downloading artifact',
    [SystemUpdatePhase.VERIFY]: 'Verifying artifact',
    [SystemUpdatePhase.STAGE]: 'Staging update',
    [SystemUpdatePhase.APPLY]: 'Applying update',
    [SystemUpdatePhase.RESTART]: 'Restarting target',
    [SystemUpdatePhase.COMPLETE]: 'Complete',
    [SystemUpdatePhase.FAILURE]: 'Failed',
  }
  return labels[phase] ?? 'Preparing'
}

function jobProgressColor(job: SystemUpdateJob): string {
  if (job.status === SystemUpdateJobStatus.FAILED) return 'negative'
  if (job.status === SystemUpdateJobStatus.SUCCEEDED) return 'positive'
  if (job.status === SystemUpdateJobStatus.RESTARTING) return 'warning'
  return 'primary'
}

function progressPercent(job: SystemUpdateJob): number {
  return clampPercent(job.progressPercent)
}

function clampPercent(value: number): number {
  return Math.min(100, Math.max(0, Math.round(Number.isFinite(value) ? value : 0)))
}

function isTerminalStatus(status: SystemUpdateJobStatus): boolean {
  return status === SystemUpdateJobStatus.SUCCEEDED || status === SystemUpdateJobStatus.FAILED
}

function isExpectedControllerRestart(job: SystemUpdateJob): boolean {
  return (
    job.component === SystemUpdateComponent.CONTROLLER &&
    (job.status === SystemUpdateJobStatus.RESTARTING || job.phase === SystemUpdatePhase.RESTART)
  )
}

function requestedBy(job: SystemUpdateJob): string {
  return job.requestedByUserName || job.requestedByUserId || 'Unknown operator'
}

function sortJobs(items: SystemUpdateJob[]): SystemUpdateJob[] {
  return [...items].sort(
    (a, b) => timestampMilliseconds(b.createdAt) - timestampMilliseconds(a.createdAt),
  )
}

function timestampMilliseconds(timestamp: { seconds: bigint; nanos?: number } | undefined): number {
  if (!timestamp) return 0
  return Number(timestamp.seconds) * 1000 + Math.floor((timestamp.nanos ?? 0) / 1_000_000)
}

function formatTimestamp(timestamp: { seconds: bigint; nanos?: number } | undefined): string {
  const milliseconds = timestampMilliseconds(timestamp)
  if (!milliseconds) return 'Unknown time'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(milliseconds)
}

function formatJobLastUpdate(job: SystemUpdateJob): string {
  const persistedAt = timestampMilliseconds(job.updatedAt || job.createdAt)
  const liveAt = progressOverlays.value.get(job.id)?.receivedAt ?? 0
  const milliseconds = Math.max(persistedAt, liveAt)
  if (!milliseconds) return 'Unknown time'
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  }).format(milliseconds)
}

function formatTime(milliseconds: number): string {
  return new Intl.DateTimeFormat(undefined, {
    hour: 'numeric',
    minute: '2-digit',
    second: '2-digit',
  }).format(milliseconds)
}

function errorMessage(unknownError: unknown): string {
  return ConnectErrorToString(ConnectError.from(unknownError))
}

function isAmbiguousStartError(error: ConnectError): boolean {
  return [Code.Canceled, Code.Unknown, Code.DeadlineExceeded, Code.Unavailable].includes(error.code)
}

function notifyError(unknownError: unknown): void {
  Notify.create({
    type: 'xylona-error',
    position: 'top',
    caption: errorMessage(unknownError),
    timeout: 0,
    closeBtn: 'Dismiss',
    icon: 'report_problem',
  })
}

function notifyPreflightUnavailable(
  message = 'Fresh target, job, and affected-server context is required before continuing.',
): void {
  Notify.create({
    type: 'xylona-error',
    position: 'top',
    caption: message,
    timeout: 0,
    closeBtn: 'Dismiss',
    icon: 'report_problem',
  })
}

function reloadInterface(): void {
  window.location.reload()
}
</script>

<style scoped>
.system-updates {
  color: var(--xy-text-primary);
}

.system-updates__target-copy,
.system-updates__active-copy {
  min-width: 0;
}

.system-updates__summary,
.system-updates__platform,
.system-updates__section-sync,
.system-updates__sync-time,
.system-updates__table-reason {
  font-size: var(--xy-font-size-xs);
}

.system-updates__summary {
  margin-top: var(--xy-space-2xs);
}

.system-updates__live-strip,
.system-updates__reload-banner,
.system-updates__error,
.system-updates__loading,
.system-updates__empty {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
  border-radius: var(--xy-radius-lg);
}

.system-updates__live-strip {
  justify-content: space-between;
  margin-bottom: var(--xy-space-lg);
  padding: var(--xy-space-base) var(--xy-space-md);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
}

.system-updates__live-strip.is-live {
  border-color: var(--xy-success-border);
}

.system-updates__live-strip.is-recovering {
  border-color: var(--xy-warning-border);
}

.system-updates__live-strip.is-offline {
  border-color: var(--xy-danger-border);
}

.system-updates__live-state,
.system-updates__live-actions,
.system-updates__reload-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.system-updates__live-state > div,
.system-updates__reload-banner > div:not(.system-updates__reload-actions),
.system-updates__error > div,
.system-updates__empty > div {
  display: grid;
  gap: var(--xy-space-2xs);
  min-width: 0;
}

.system-updates__live-state span,
.system-updates__reload-banner span,
.system-updates__error span,
.system-updates__empty span,
.system-updates__section-header p {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  overflow-wrap: anywhere;
}

.system-updates__sync-time,
.system-updates__section-sync {
  color: var(--xy-text-muted);
  white-space: nowrap;
}

.system-updates__reload-banner {
  justify-content: space-between;
  margin-bottom: var(--xy-space-lg);
  padding: var(--xy-space-base) var(--xy-space-md);
}

.system-updates__sections {
  display: grid;
  gap: var(--xy-space-xl);
}

.system-updates__section {
  min-width: 0;
}

.system-updates__section-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--xy-space-md);
  margin-bottom: var(--xy-space-base);
}

.system-updates__section-title {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  line-height: var(--xy-line-height-tight);
}

.system-updates__section-header p {
  margin: var(--xy-space-xs) 0 0;
  max-width: 70ch;
}

.system-updates__error,
.system-updates__loading,
.system-updates__empty {
  margin-bottom: var(--xy-space-base);
  padding: var(--xy-space-base) var(--xy-space-md);
}

.system-updates__error {
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
}

.system-updates__error--warning {
  background: var(--xy-warning-bg);
  border-color: var(--xy-warning-border);
}

.system-updates__error .q-btn {
  margin-inline-start: auto;
}

.system-updates__loading,
.system-updates__empty {
  min-height: var(--xy-space-3xl);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
}

.system-updates__empty > .q-icon {
  color: var(--xy-text-muted);
}

.system-updates__active-list {
  display: grid;
  gap: var(--xy-space-base);
}

.system-updates__active-job,
.system-updates__mobile-card {
  width: 100%;
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.system-updates__active-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(12rem, 0.34fr);
  gap: var(--xy-space-lg);
}

.system-updates__active-heading,
.system-updates__phase-line,
.system-updates__mobile-card-header,
.system-updates__affected-heading,
.system-updates__dialog-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.system-updates__active-heading h3 {
  margin: 0;
  font-size: var(--xy-font-size-base);
  line-height: var(--xy-line-height-tight);
  overflow-wrap: anywhere;
}

.system-updates__version,
.system-updates__checksum,
.system-updates__job-id {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.system-updates__phase-line {
  align-items: center;
  margin-top: var(--xy-space-md);
  margin-bottom: var(--xy-space-sm);
}

.system-updates__phase-line span {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
}

.system-updates__job-message,
.system-updates__restart-context {
  margin: var(--xy-space-sm) 0 0;
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.system-updates__restart-context {
  padding: var(--xy-space-sm) var(--xy-space-base);
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

.system-updates__active-meta {
  display: grid;
  align-content: start;
  gap: var(--xy-space-base);
  margin: 0;
  padding-inline-start: var(--xy-space-lg);
  border-inline-start: 1px solid var(--xy-border);
}

.system-updates__active-meta > div {
  min-width: 0;
}

.system-updates__active-meta dt,
.system-updates__mobile-fields span {
  margin-bottom: var(--xy-space-2xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.system-updates__active-meta dd {
  margin: 0;
  overflow-wrap: anywhere;
}

.system-updates__job-id {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.system-updates__mobile-card-header {
  align-items: flex-start;
}

.system-updates__mobile-title {
  color: var(--xy-text-primary);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.system-updates__mobile-fields {
  display: grid;
  gap: var(--xy-space-sm);
}

.system-updates__mobile-fields > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.system-updates__mobile-fields strong {
  color: var(--xy-text-primary);
  font-weight: 500;
  overflow-wrap: anywhere;
}

.system-updates__table-reason {
  margin-top: var(--xy-space-xs);
  max-width: 36ch;
  overflow-wrap: anywhere;
}

.system-updates__checksum {
  overflow: hidden;
  text-overflow: ellipsis;
}

.system-updates__history-detail {
  display: inline-block;
  max-width: 42ch;
  overflow-wrap: anywhere;
}

.system-updates__dialog {
  width: min(42rem, calc(100vw - (2 * var(--xy-space-lg))));
  max-width: 100%;
}

.system-updates__dialog-title {
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 700;
}

.system-updates__dialog-copy {
  margin: 0 0 var(--xy-space-md);
  max-width: 70ch;
  color: var(--xy-text-secondary);
}

.system-updates__affected {
  display: grid;
  gap: var(--xy-space-sm);
}

.system-updates__affected-heading span {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
}

.system-updates__dialog-actions {
  padding: var(--xy-space-sm) var(--xy-space-md) var(--xy-space-md);
}

.system-updates__wrap {
  overflow-wrap: anywhere;
}

.system-updates__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

@media (max-width: 1023px) {
  .system-updates__active-main {
    grid-template-columns: minmax(0, 1fr);
  }

  .system-updates__active-meta {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    padding-block-start: var(--xy-space-md);
    padding-inline-start: 0;
    border-block-start: 1px solid var(--xy-border);
    border-inline-start: 0;
  }
}

@media (max-width: 599px) {
  .system-updates__live-strip,
  .system-updates__reload-banner,
  .system-updates__section-header {
    align-items: stretch;
    flex-direction: column;
  }

  .system-updates__live-actions,
  .system-updates__reload-actions {
    justify-content: space-between;
    flex-wrap: wrap;
  }

  .system-updates__sync-time {
    flex: 1 1 auto;
  }

  .system-updates__section-sync {
    white-space: normal;
  }

  .system-updates__active-main {
    padding: var(--xy-space-base);
  }

  .system-updates__active-heading {
    align-items: flex-start;
  }

  .system-updates__active-meta {
    grid-template-columns: minmax(0, 1fr);
  }

  .system-updates__error {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .system-updates__error .q-btn {
    margin-inline-start: 0;
  }

  .system-updates__dialog {
    width: calc(100vw - (2 * var(--xy-space-md)));
  }
}
</style>
