<template>
  <section class="diagnosis" aria-label="Latest server diagnosis">
    <template v-if="diagnosis">
      <div class="diagnosis-heading">
        <div>
          <strong>{{ summary }}</strong>
          <div class="diagnosis-meta">
            <span>{{ statusLabel }}</span>
            <time v-if="occurredAt" :datetime="occurredAt.toISOString()">{{
              occurredAt.toLocaleString()
            }}</time>
            <span v-if="stale" class="diagnosis-stale" role="status"
              >Report may be out of date</span
            >
          </div>
        </div>
        <button
          v-if="diagnosis.category === 'incomplete_setup' && readinessVisible"
          class="diagnosis-link"
          type="button"
          @click="$emit('showReadiness')">
          Review setup
        </button>
        <router-link
          v-else-if="action"
          :to="`/game-servers/${serverId}/${action.path}`"
          class="diagnosis-link"
          >{{ action.label }}</router-link
        >
      </div>
      <details>
        <summary>Evidence and next steps</summary>
        <div class="diagnosis-details">
          <p>{{ guidance }}</p>
          <p v-if="diagnosis.stage === 'unknown_outcome'">
            The start request did not return a confirmed result. Check the current status and
            console before trying again.
          </p>
          <dl>
            <div>
              <dt>Stage</dt>
              <dd>{{ stageLabel }}</dd>
            </div>
            <div v-if="diagnosis.exitCode !== undefined">
              <dt>Process exit code</dt>
              <dd>{{ diagnosis.exitCode }}</dd>
            </div>
          </dl>
          <p v-if="diagnosis.evidenceRestricted">
            Console permission is required to view error details and evidence.
          </p>
          <template v-else>
            <pre v-if="diagnosis.error">{{ diagnosis.error }}</pre>
            <template v-if="diagnosis.matchedEvidence"
              ><strong>Supporting evidence</strong>
              <pre>{{ diagnosis.matchedEvidence }}</pre>
            </template>
            <template v-if="diagnosis.evidence"
              ><strong>Final console output</strong>
              <pre>{{ diagnosis.evidence }}</pre>
            </template>
            <p v-else-if="!diagnosis.evidenceAvailable">
              Final console evidence was not available for this attempt.
            </p>
            <p v-else>No console output was recorded for this attempt.</p>
            <p v-if="diagnosis.truncated">
              Earlier output was omitted. Only the final 200 lines, up to 32 KiB, are retained.
            </p>
          </template>
          <template v-if="permissions.includes('game_server.metrics') && occurredAt">
            <router-link
              class="diagnosis-link"
              :to="{
                path: `/game-servers/${serverId}/metrics`,
                query: { at: occurredAt.toISOString(), range: '1h' },
              }"
              >View metrics around this failure</router-link
            >
            <p>
              Metrics sampling can miss brief spikes. Resource usage alone does not identify the
              cause.
            </p>
          </template>
          <router-link
            v-if="permissions.includes('game_server.backup')"
            class="diagnosis-link"
            :to="`/game-servers/${serverId}/backups`"
            >Review available backups</router-link
          >
        </div>
      </details>
    </template>
    <div v-else class="diagnosis-empty" role="status">
      <span v-if="stale">{{
        loaded
          ? 'No failure was recorded at the last check. Diagnosis is currently unavailable.'
          : 'Diagnosis is currently unavailable.'
      }}</span>
      <span v-else-if="loading && !loaded">Checking latest diagnosis…</span>
      <span v-else>No failure recorded.</span>
      <button v-if="stale" type="button" class="diagnosis-link" @click="refresh">Retry</button>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, toRef } from 'vue'
import { Status } from '@/proto/shared_pb'
import { useGameServerDiagnosis } from '@/pages/game_servers/useGameServerDiagnosis'

const props = defineProps<{
  serverId: string
  status: Status
  permissions: string[]
  readinessVisible: boolean
}>()
defineEmits<{ showReadiness: [] }>()
const { diagnosis, loading, stale, loaded, refresh } = useGameServerDiagnosis(
  toRef(props, 'serverId'),
  toRef(props, 'status'),
)
defineExpose({ refresh })

const occurredAt = computed(() => {
  const timestamp = diagnosis.value?.occurredAt
  return timestamp ? new Date(Number(timestamp.seconds) * 1000 + timestamp.nanos / 1_000_000) : null
})
const categoryCopy: Record<
  string,
  { summary: string; guidance: string; path: string; label: string; permission: string }
> = {
  missing_executable: {
    summary: 'Server executable could not be found',
    guidance:
      'Check the executable path and confirm the server software is installed on its assigned node.',
    path: 'start-command',
    label: 'Review executable settings',
    permission: 'game_server.settings',
  },
  permission_denied: {
    summary: 'Access to a required file or resource was denied',
    guidance:
      'Check file ownership and permissions for the account running the node, including access to the server directory and executable.',
    path: 'settings',
    label: 'Review server settings',
    permission: 'game_server.settings',
  },
  port_in_use: {
    summary: 'A required port is already in use',
    guidance:
      'Check the configured ports and stop the conflicting process or choose an available port before starting again.',
    path: 'settings',
    label: 'Review server ports',
    permission: 'game_server.settings',
  },
  disk_full: {
    summary: 'The server ran out of disk space',
    guidance:
      'Review storage usage on the assigned node. Make room without deleting files needed to restore your server.',
    path: 'metrics',
    label: 'Review storage metrics',
    permission: 'game_server.metrics',
  },
  incomplete_setup: {
    summary: 'Server setup is incomplete',
    guidance: 'Complete the pending setup requirements in Server details before starting again.',
    path: 'configuration',
    label: 'Review configuration',
    permission: 'game_server.config',
  },
  node_unavailable: {
    summary: 'The assigned node could not be reached',
    guidance: 'Check that the assigned node is running and connected to the controller.',
    path: 'settings',
    label: 'Review assigned node',
    permission: 'game_server.settings',
  },
}
const copy = computed(() => categoryCopy[diagnosis.value?.category ?? ''])
const summary = computed(() =>
  diagnosis.value?.stage === 'unknown_outcome'
    ? 'Start outcome unknown'
    : copy.value
      ? `${diagnosis.value?.inferred ? 'Possible cause: ' : ''}${copy.value.summary}`
      : 'Cause not identified',
)
const guidance = computed(
  () =>
    copy.value?.guidance ??
    'Review the captured error and final console output for the next step. The available evidence does not identify a specific cause.',
)
const action = computed(() =>
  copy.value && props.permissions.includes(copy.value.permission) ? copy.value : null,
)
const stageLabels: Record<string, string> = {
  pre_start: 'Before process launch',
  launch: 'Process launch',
  runtime: 'Unexpected process exit',
  unknown_outcome: 'Start outcome unknown',
  unknown: 'Failure stage unavailable',
}
const stageLabel = computed(() => stageLabels[diagnosis.value?.stage ?? ''] ?? 'Not recorded')
const statusLabel = computed(() => {
  const prefix = diagnosis.value?.stage === 'unknown_outcome' ? 'Last start report' : 'Last failure'
  if (props.status === Status.ONLINE) return `${prefix} — server is now running`
  if (props.status === Status.OFFLINE) return `${prefix} — server is currently offline`
  return `${prefix} — current status ${Status[props.status]?.toLowerCase() ?? 'unknown'}`
})
</script>

<style scoped>
.diagnosis {
  flex-shrink: 0;
  padding: var(--xy-space-base) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  font-size: var(--xy-font-size-sm);
}
.diagnosis-heading,
.diagnosis-meta,
.diagnosis-empty {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-sm) var(--xy-space-md);
}
.diagnosis-heading {
  justify-content: space-between;
}
.diagnosis-meta,
.diagnosis-empty {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}
.diagnosis-meta {
  margin-top: var(--xy-space-xs);
}
.diagnosis-stale {
  color: var(--xy-warning-hover);
}
.diagnosis details {
  margin-top: var(--xy-space-sm);
}
.diagnosis summary {
  cursor: pointer;
  width: fit-content;
  color: var(--xy-text-primary);
}
.diagnosis summary:focus-visible,
.diagnosis-link:focus-visible {
  outline: 2px solid var(--xy-primary);
  outline-offset: 3px;
}
.diagnosis-details {
  display: grid;
  gap: var(--xy-space-sm);
  padding-top: var(--xy-space-md);
  max-height: 35dvh;
  overflow-y: auto;
  overscroll-behavior: contain;
}
.diagnosis p {
  margin: 0;
  color: var(--xy-text-secondary);
  max-width: 75ch;
}
.diagnosis dl {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-lg);
  margin: 0;
}
.diagnosis dt {
  color: var(--xy-text-secondary);
}
.diagnosis dd {
  margin: 0;
}
.diagnosis pre {
  margin: 0;
  padding: var(--xy-space-base);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 20rem;
  overflow: auto;
  background: var(--xy-surface-0);
  border-radius: var(--xy-radius-sm);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}
.diagnosis-link {
  color: var(--xy-primary);
  background: none;
  border: none;
  padding: 0;
  cursor: pointer;
  font: inherit;
  text-decoration: underline;
  text-underline-offset: 3px;
  width: fit-content;
}
.diagnosis-link:hover {
  color: var(--xy-text-primary);
}
@media (max-width: 599px) {
  .diagnosis {
    padding: var(--xy-space-base) var(--xy-space-md);
  }
  .diagnosis-heading {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
