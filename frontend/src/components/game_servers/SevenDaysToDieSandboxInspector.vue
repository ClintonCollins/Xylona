<template>
  <section class="sandbox-inspector" aria-labelledby="sandbox-inspector-title">
    <button
      class="sandbox-summary"
      type="button"
      :aria-expanded="expanded"
      aria-controls="sandbox-inspector-content"
      @click="toggleExpanded">
      <span class="sandbox-summary-title">
        <q-icon name="fact_check" size="20px" />
        <span id="sandbox-inspector-title">Effective sandbox settings</span>
      </span>
      <span class="sandbox-status" :class="`sandbox-status--${status.tone}`">
        <q-spinner v-if="loading" size="14px" />
        <q-icon v-else :name="status.icon" size="16px" />
        {{ status.label }}
      </span>
      <q-icon :name="expanded ? 'expand_less' : 'expand_more'" size="20px" />
    </button>

    <div v-if="expanded" id="sandbox-inspector-content" class="sandbox-content">
      <div class="sandbox-toolbar">
        <p class="sandbox-guidance" role="status">{{ status.message }}</p>
        <button class="sandbox-refresh" type="button" :disabled="loading" @click="loadSettings">
          <q-icon name="refresh" size="18px" />
          Refresh
        </button>
      </div>

      <template v-if="hasObservation">
        <div
          class="sandbox-code-grid"
          :aria-label="isStale ? 'Sandbox observations' : 'Sandbox code comparison'">
          <div>
            <span class="sandbox-code-label">{{ configuredCodeLabel }}</span>
            <code>{{ response?.configuredCode || 'Empty' }}</code>
          </div>
          <div>
            <span class="sandbox-code-label">{{ effectiveCodeLabel }}</span>
            <code>{{ response?.effectiveCode || 'Not reported' }}</code>
          </div>
        </div>

        <div class="sandbox-filters">
          <label class="sandbox-search">
            <q-icon name="search" size="18px" />
            <span class="sr-only">Filter sandbox settings</span>
            <input v-model="filter" type="search" placeholder="Filter settings" />
          </label>
        </div>

        <div v-if="groupedSettings.length === 0" class="sandbox-empty">
          {{
            response?.settings.length === 0
              ? 'The game reported no sandbox settings.'
              : 'No settings match this filter.'
          }}
        </div>
        <section v-for="group in groupedSettings" :key="group.name" class="sandbox-group">
          <h3>{{ group.name }}</h3>
          <div class="sandbox-table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col">Setting</th>
                  <th scope="col">{{ runningValueLabel }}</th>
                  <th scope="col">Result</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="setting in group.settings" :key="setting.key">
                  <th scope="row">
                    <span>{{ setting.label || setting.key }}</span>
                    <small v-if="setting.description">{{ setting.description }}</small>
                  </th>
                  <td>{{ displayValue(setting.effectiveLabel, setting.effectiveValue) }}</td>
                  <td>
                    <span
                      class="sandbox-row-status"
                      :class="settingsMatch ? 'is-match' : 'is-uncompared'">
                      <q-icon :name="settingsMatch ? 'check_circle' : 'history'" size="16px" />
                      {{ settingsMatch ? 'Matches' : 'Not compared' }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>
      </template>
    </div>
  </section>
</template>

<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'

import {
  GetSevenDaysToDieSandboxSettingsRequestSchema,
  type GetSevenDaysToDieSandboxSettingsResponse,
  type SevenDaysToDieSandboxSetting,
  SevenDaysToDieSandboxComparisonState,
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const props = defineProps<{
  gameServerId: string
  refreshKey: number
}>()

const expanded = ref(false)
const loading = ref(false)
const unauthorized = ref(false)
const failed = ref(false)
const staleSinceSave = ref(false)
const response = ref<GetSevenDaysToDieSandboxSettingsResponse>()
const filter = ref('')
let latestRequestGeneration = 0

const availableState =
  SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
const serverOfflineState =
  SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE
const authenticationDeniedState =
  SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED

const hasObservation = computed(() => response.value?.state === availableState)
const isStale = computed(
  () =>
    staleSinceSave.value ||
    response.value?.comparisonState === SevenDaysToDieSandboxComparisonState.STALE,
)
const settingsMatch = computed(
  () =>
    !isStale.value &&
    response.value?.comparisonState === SevenDaysToDieSandboxComparisonState.MATCH,
)
const configuredCodeLabel = computed(() =>
  staleSinceSave.value ? 'Previously saved SandboxCode' : 'Saved SandboxCode',
)
const effectiveCodeLabel = computed(() =>
  staleSinceSave.value ? 'Previously observed effective code' : 'Observed effective code',
)
const runningValueLabel = computed(() =>
  staleSinceSave.value ? 'Previously observed running' : 'Observed running',
)

const status = computed(() => {
  if (loading.value) {
    return {
      label: 'Loading',
      icon: 'hourglass_empty',
      tone: 'neutral',
      message: 'Reading the running game…',
    }
  }
  if (unauthorized.value) {
    return {
      label: 'Unauthorized',
      icon: 'lock',
      tone: 'warning',
      message: 'You do not have permission to inspect these configuration settings.',
    }
  }
  if (failed.value) {
    return {
      label: 'Failed',
      icon: 'error',
      tone: 'negative',
      message: 'The settings request failed. Retry when the controller and node are available.',
    }
  }
  if (!response.value) {
    return {
      label: 'Not checked',
      icon: 'info',
      tone: 'neutral',
      message: 'Expand this inspector to compare the saved code with the running game.',
    }
  }
  if (response.value.connectionState === serverOfflineState) {
    return {
      label: 'Offline',
      icon: 'power_off',
      tone: 'neutral',
      message: 'Start the game server before inspecting its effective settings.',
    }
  }
  if (
    response.value.connectionState === authenticationDeniedState ||
    response.value.state ===
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED
  ) {
    return {
      label: 'Unauthorized',
      icon: 'lock',
      tone: 'warning',
      message:
        'The game WebAPI denied access to sandbox settings. Check its read token permissions.',
    }
  }
  if (
    response.value.state ===
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED
  ) {
    return {
      label: 'Unsupported',
      icon: 'block',
      tone: 'neutral',
      message: 'This running game does not advertise the native sandbox-settings endpoint.',
    }
  }
  if (response.value.state !== availableState) {
    return {
      label: 'Unavailable',
      icon: 'cloud_off',
      tone: 'negative',
      message:
        'Effective sandbox settings are currently unavailable. Retry after checking the node and game WebAPI.',
    }
  }
  if (staleSinceSave.value) {
    return {
      label: 'Stale',
      icon: 'history',
      tone: 'warning',
      message:
        'These observations predate the current editor value and are not a comparison. Refresh after the game reloads the saved code. Nothing was changed or restarted.',
    }
  }
  if (response.value.comparisonState === SevenDaysToDieSandboxComparisonState.STALE) {
    return {
      label: 'Stale',
      icon: 'history',
      tone: 'warning',
      message:
        'The saved SandboxCode was not recognized, so running observations are not compared with it. Review the code in the editor. Nothing was changed or restarted.',
    }
  }
  if (response.value.comparisonState === SevenDaysToDieSandboxComparisonState.MATCH) {
    return {
      label: 'Match',
      icon: 'check_circle',
      tone: 'positive',
      message:
        'The saved SandboxCode matches the running game. Editing this file does not apply changes to the running server.',
    }
  }
  if (response.value.comparisonState === SevenDaysToDieSandboxComparisonState.MISMATCH) {
    return {
      label: 'Mismatch',
      icon: 'warning',
      tone: 'warning',
      message:
        'The saved code differs from the running game. The running settings below are observations, not per-setting comparisons; nothing was changed or restarted.',
    }
  }
  return {
    label: 'Unavailable',
    icon: 'cloud_off',
    tone: 'negative',
    message: 'The game returned settings without a recognizable comparison state.',
  }
})

const groupedSettings = computed(() => {
  const query = filter.value.trim().toLocaleLowerCase()
  const groups = new Map<string, SevenDaysToDieSandboxSetting[]>()
  for (const setting of response.value?.settings ?? []) {
    const searchable = [setting.key, setting.label, setting.description, setting.group]
      .join(' ')
      .toLocaleLowerCase()
    if (query !== '' && !searchable.includes(query)) continue
    const group = setting.group || 'Sandbox'
    const entries = groups.get(group) ?? []
    entries.push(setting)
    groups.set(group, entries)
  }
  return [...groups].map(([name, settings]) => ({ name, settings }))
})

watch(
  () => props.refreshKey,
  () => {
    staleSinceSave.value = true
  },
)

async function toggleExpanded(): Promise<void> {
  expanded.value = !expanded.value
  if (expanded.value && !response.value && !loading.value) await loadSettings()
}

async function loadSettings(): Promise<void> {
  const requestGeneration = ++latestRequestGeneration
  const refreshKeyAtRequest = props.refreshKey
  loading.value = true
  unauthorized.value = false
  failed.value = false
  try {
    const request = create(GetSevenDaysToDieSandboxSettingsRequestSchema, {
      gameServerId: props.gameServerId,
    })
    const nextResponse = await GetXylonaClient().getSevenDaysToDieSandboxSettings(request)
    if (requestGeneration !== latestRequestGeneration) return
    response.value = nextResponse
    staleSinceSave.value = refreshKeyAtRequest !== props.refreshKey
  } catch (unknownErr: unknown) {
    if (requestGeneration !== latestRequestGeneration) return
    response.value = undefined
    const err = ConnectError.from(unknownErr)
    unauthorized.value = err.code === Code.PermissionDenied || err.code === Code.Unauthenticated
    failed.value = !unauthorized.value
  } finally {
    if (requestGeneration === latestRequestGeneration) loading.value = false
  }
}

function displayValue(label: string, value: string): string {
  return label || value || 'Not reported'
}
</script>

<style scoped>
.sandbox-inspector {
  margin: 0 var(--xy-space-md) var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  background: var(--xy-surface-1);
  overflow: hidden;
}

.sandbox-summary {
  width: 100%;
  min-height: 44px;
  padding: 0 var(--xy-space-md);
  border: 0;
  background: transparent;
  color: inherit;
  display: grid;
  grid-template-columns: 1fr auto auto;
  align-items: center;
  gap: var(--xy-space-sm);
  cursor: pointer;
  text-align: left;
}

.sandbox-summary:hover,
.sandbox-summary:focus-visible {
  background: var(--xy-surface-2);
}
.sandbox-summary:focus-visible {
  outline: 2px solid var(--q-primary);
  outline-offset: -2px;
}
.sandbox-summary-title,
.sandbox-status,
.sandbox-row-status,
.sandbox-refresh,
.sandbox-search {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.sandbox-summary-title {
  font-weight: 600;
}
.sandbox-status {
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}
.sandbox-status--positive,
.is-match {
  color: var(--q-positive);
}
.sandbox-status--warning {
  color: var(--q-warning);
}
.sandbox-status--negative {
  color: var(--q-negative);
}
.sandbox-status--neutral {
  color: var(--xy-text-secondary);
}

.sandbox-content {
  max-height: min(46vh, 520px);
  padding: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
  overflow: auto;
}
.sandbox-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: var(--xy-space-md);
}
.sandbox-guidance {
  margin: 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}
.sandbox-refresh {
  flex: none;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  padding: 5px 10px;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.sandbox-refresh:disabled {
  opacity: 0.55;
  cursor: default;
}
.sandbox-code-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-md);
}
.sandbox-code-grid > div {
  padding: var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  background: var(--xy-surface-2);
}
.sandbox-code-label {
  display: block;
  margin-bottom: 4px;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-2xs);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}
.sandbox-code-grid code {
  overflow-wrap: anywhere;
}
.sandbox-filters {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
  margin: var(--xy-space-md) 0;
}
.sandbox-search {
  flex: 1;
  min-width: 180px;
  padding: 6px 10px;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
}
.sandbox-search:focus-within {
  outline: 2px solid var(--q-primary);
  outline-offset: 1px;
}
.sandbox-search input {
  width: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  color: inherit;
}
.sandbox-group + .sandbox-group {
  margin-top: var(--xy-space-md);
}
.sandbox-group h3 {
  margin: 0 0 var(--xy-space-xs);
  font-size: var(--xy-font-size-sm);
}
.sandbox-table-wrap {
  overflow-x: auto;
}
table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--xy-font-size-xs);
}
th,
td {
  padding: 8px;
  border-bottom: 1px solid var(--xy-border);
  text-align: left;
  vertical-align: top;
}
th[scope='row'] {
  min-width: 220px;
  font-weight: 500;
}
th small {
  display: block;
  margin-top: 2px;
  color: var(--xy-text-secondary);
  font-weight: 400;
}
.sandbox-row-status {
  white-space: nowrap;
  font-weight: 600;
}
.is-uncompared {
  color: var(--xy-text-secondary);
}
.sandbox-empty {
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
}
.sr-only {
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

@media (max-width: 599px) {
  .sandbox-inspector {
    margin-inline: var(--xy-space-sm);
  }
  .sandbox-summary {
    padding-inline: var(--xy-space-sm);
  }
  .sandbox-toolbar,
  .sandbox-filters {
    align-items: stretch;
    flex-direction: column;
  }
  .sandbox-refresh {
    align-self: flex-start;
  }
  .sandbox-code-grid {
    grid-template-columns: 1fr;
  }
}
</style>
