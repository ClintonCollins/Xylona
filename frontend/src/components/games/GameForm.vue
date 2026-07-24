<template>
  <div class="game-form-wrapper full-width">
    <!-- Sentinel for sticky detection -->
    <div ref="stickySentinel" class="sticky-sentinel"></div>
    <!-- Header -->
    <div :class="{ 'is-stuck': isStuck }" class="game-form-header">
      <div class="game-form-header-left">
        <nav aria-label="Breadcrumb" class="game-form-breadcrumbs">
          <ol class="breadcrumb-list">
            <li>
              <router-link class="breadcrumb-link" to="/games">Games</router-link>
            </li>
            <li aria-current="page">
              <span class="breadcrumb-current">{{ breadcrumbLabel }}</span>
            </li>
          </ol>
        </nav>
        <h1 class="game-form-title font-display">{{ formTitle }}</h1>
      </div>
      <div class="game-form-header-actions">
        <router-link
          v-if="!existingGame && !copyGame"
          class="game-form-guided-link text-caption"
          to="/games/new">
          Use guided setup
        </router-link>
        <q-btn
          v-if="existingGame && !copyGame"
          aria-label="Export game JSON"
          :disable="loading || submitting"
          flat
          icon="file_download"
          round
          @click="exportCurrentGame">
          <q-tooltip>Export game JSON</q-tooltip>
        </q-btn>
        <q-btn :disable="submitting" flat label="Cancel" @click="handleCancel" />
        <q-btn
          :disable="loading"
          :loading="submitting"
          color="primary"
          label="Save"
          @click="submit" />
      </div>
    </div>

    <!-- Diverged official definition banner -->
    <div
      v-if="showDivergedBanner"
      class="game-form-diverged-banner"
      data-testid="game-form-diverged-banner">
      <q-icon class="diverged-banner-icon" name="difference" size="20px" />
      <div class="diverged-banner-text">
        <div class="diverged-banner-title">Modified from official definition</div>
        <div class="diverged-banner-caption">
          This game has local edits, so it no longer receives official definition updates.
        </div>
      </div>
      <q-btn
        :loading="restoringOfficial"
        class="diverged-banner-btn"
        color="primary"
        dense
        label="Restore official definition"
        no-caps
        outline
        @click="confirmRestoreOfficial" />
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="game-form-loading">
      <q-spinner-dots color="primary" size="40px" />
      <div class="text-xy-secondary q-mt-sm">Loading game details...</div>
    </div>

    <div v-else class="game-form-body">
      <q-form ref="formRef">
        <div class="game-form-tabs-panel">
          <div class="game-form-tabs-shell">
            <div aria-label="Game editor sections" class="game-form-tabs" role="tablist">
              <button
                v-for="tab in formTabs"
                :id="formTabID(tab.id)"
                :key="tab.id"
                :aria-controls="formTabPanelID(tab.id)"
                :aria-selected="activeFormTab === tab.id"
                :class="{ 'game-form-tab--active': activeFormTab === tab.id }"
                :data-testid="`game-form-tab-${tab.id}`"
                :tabindex="activeFormTab === tab.id ? 0 : -1"
                :title="tab.copy"
                class="game-form-tab"
                role="tab"
                type="button"
                @click="activeFormTab = tab.id"
                @keydown="handleFormTabKeydown($event, tab.id)">
                <span class="game-form-tab__label font-display">{{ tab.label }}</span>
              </button>
            </div>
          </div>

          <!-- Tab descriptions available via tooltip on tab buttons -->
        </div>

        <div
          v-show="activeFormTab === 'overview'"
          :id="formTabPanelID('overview')"
          :aria-hidden="activeFormTab !== 'overview'"
          :aria-labelledby="formTabID('overview')"
          :inert="activeFormTab !== 'overview'"
          class="game-form-tab-panel"
          data-testid="game-form-tab-panel-overview"
          role="tabpanel">
          <game-form-overview-tab />
        </div>

        <div
          v-show="activeFormTab === 'runtime'"
          :id="formTabPanelID('runtime')"
          :aria-hidden="activeFormTab !== 'runtime'"
          :aria-labelledby="formTabID('runtime')"
          :inert="activeFormTab !== 'runtime'"
          class="game-form-tab-panel"
          data-testid="game-form-tab-panel-runtime"
          role="tabpanel">
          <game-form-runtime-tab />
        </div>

        <div
          v-show="activeFormTab === 'mods'"
          :id="formTabPanelID('mods')"
          :aria-hidden="activeFormTab !== 'mods'"
          :aria-labelledby="formTabID('mods')"
          :inert="activeFormTab !== 'mods'"
          class="game-form-tab-panel"
          data-testid="game-form-tab-panel-mods"
          role="tabpanel">
          <game-form-mods-tab />
        </div>

        <div
          v-show="activeFormTab === 'console-commands'"
          :id="formTabPanelID('console-commands')"
          :aria-hidden="activeFormTab !== 'console-commands'"
          :aria-labelledby="formTabID('console-commands')"
          :inert="activeFormTab !== 'console-commands'"
          class="game-form-tab-panel"
          data-testid="game-form-tab-panel-console-commands"
          role="tabpanel">
          <game-form-console-commands-tab />
        </div>

        <div
          v-show="activeFormTab === 'config'"
          :id="formTabPanelID('config')"
          :aria-hidden="activeFormTab !== 'config'"
          :aria-labelledby="formTabID('config')"
          :inert="activeFormTab !== 'config'"
          class="game-form-tab-panel"
          data-testid="game-form-tab-panel-config"
          role="tabpanel">
          <game-form-config-tab />
        </div>
      </q-form>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { QForm, useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, provide, ref, Ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CommandType,
  EnvironmentValidationIssue,
  EnvironmentVariable,
  EnvironmentVariableSchema,
  Game,
  GameSchema,
  UpdateProviderConfigSchema,
} from '@/proto/shared_pb'
import {
  GetGameEnvironmentRequestSchema,
  ResetGameToOfficialDefinitionRequestSchema,
  UpdateGameEnvironmentRequestSchema,
} from '@/proto/xylona_pb'
import type { ConfigSchemaEntry } from './config-schema-types'
import GameFormOverviewTab from './GameFormOverviewTab.vue'
import GameFormRuntimeTab from './GameFormRuntimeTab.vue'
import GameFormModsTab from './GameFormModsTab.vue'
import GameFormConsoleCommandsTab from './GameFormConsoleCommandsTab.vue'
import GameFormConfigTab from './GameFormConfigTab.vue'
import { type GameFormContext, gameFormContextKey } from './GameFormTypes'
import {
  applySimpleGameConfig,
  getCommandProcessorOptions,
  getCommandTypeOptions,
  isCommandTypeCommand,
  isManagedGameConfig,
} from './game-form-provider-fields'
import { useGameFormModProfile } from './useGameFormModProfile'
import { useGameFormDirtyState } from './useGameFormDirtyState'
import { useGameFormPlatform } from './useGameFormPlatform'
import { useGameFormRuntimePanels } from './useGameFormRuntimePanels'
import { useGameFormStartArgsState } from './useGameFormStartArgsState'
import { useGameFormTabs } from './useGameFormTabs'
import { useGameFormNewGameSetup } from './useGameFormNewGameSetup'
import { useGameFormPersistence } from './useGameFormPersistence'
import { exportGameDefinitionJSON } from './game-definition-json'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const router = useRouter()
const $q = useQuasar()
const formRef = ref<QForm | null>(null)
const stickySentinel = ref<HTMLElement | null>(null)
const isStuck = ref(false)
let stickyObserver: IntersectionObserver | null = null
const defaultPort: Ref<number | null> = ref(null)
const defaultQueryPort: Ref<number | null> = ref(null)
const savedSuccessfully = ref(false)

const formTitle = computed(() => {
  if (existingGame.value) {
    return `Editing ${game.value.name || 'Game'}`
  }
  return 'Add Game'
})

const breadcrumbLabel = computed(() => {
  if (existingGame.value) {
    return game.value.name || 'Game'
  }
  if (copyGame.value) {
    return 'New Game (Copy)'
  }
  return 'New Game'
})

// --- Validation rules ---

const idRules = [
  (val: string) => (val && val.trim().length > 0) || 'Unique ID is required',
  (val: string) =>
    /^[a-z0-9_-]+$/.test(val) || 'Only lowercase letters, numbers, hyphens, and underscores',
]

const nameRules = [(val: string) => (val && val.trim().length > 0) || 'Name is required']

const portRules = [
  (val: number | null | string) =>
    (val !== null && val !== '' && val !== undefined) || 'Port is required',
  (val: number | null | string) => {
    const num = Number(val)
    return (Number.isInteger(num) && num >= 1 && num <= 65535) || 'Must be 1-65535'
  },
]

const commandTypeOptions = getCommandTypeOptions()
const linuxCommandProcessorOptions = getCommandProcessorOptions('linux')
const windowsCommandProcessorOptions = getCommandProcessorOptions('windows')

const props = defineProps({
  existingGameId: {
    type: String,
    required: false,
    default: '',
  },
  copyGameId: {
    type: String,
    required: false,
    default: '',
  },
})

const game: Ref<Game> = ref(create(GameSchema)) as Ref<Game>
const { formTabs, activeFormTab, formTabID, formTabPanelID, handleFormTabKeydown } =
  useGameFormTabs()
const { activePlatform, activePlatformResolved, syncActivePlatformFromGame } =
  useGameFormPlatform(game)
const {
  runtimeSequenceExpanded,
  runtimePolicyExpanded,
  toggleRuntimePolicy,
  updateRuntimeSequenceExpanded,
} = useGameFormRuntimePanels()
const existingGame = ref(false)
const copyGame = ref(false)
const gameID = ref('')
const configSchemas = ref<ConfigSchemaEntry[]>([])
const defaultEnvRows = ref<EnvironmentVariable[]>([])
const defaultEnvIssues = ref<EnvironmentValidationIssue[]>([])
const defaultEnvLoading = ref(false)
const defaultEnvSaving = ref(false)
const {
  linuxStartArgsTemplate,
  windowsStartArgsTemplate,
  startArgBlocklist,
  baselineLinuxBaseCommand,
  baselineWindowsBaseCommand,
  baselineLinuxStartArgsTemplate,
  baselineWindowsStartArgsTemplate,
  syncStructuredStartArgsFromGame,
  captureRuntimeBaselineFromCurrentState,
  syncStructuredStartArgsToGame,
} = useGameFormStartArgsState(game)
const downstreamImpactServers = ref<Array<{ name: string; patchCount: number }>>([])
const { isDirty, commitSnapshot, commitFormSnapshot, commitDefaultEnvSnapshot } =
  useGameFormDirtyState({
    game,
    defaultPort,
    defaultQueryPort,
    configSchemas,
    defaultEnvRows,
    linuxStartArgsTemplate,
    windowsStartArgsTemplate,
    startArgBlocklist,
    downstreamImpactServers,
  })
const managedTypedConfig = computed(() => isManagedGameConfig(game.value))
const {
  managedModConfig,
  variantModSummaries,
  modSourceOptions,
  activeModSourceLabel,
  ensureModProfileSources,
  addGameModProfile,
  clearGameModProfile,
  onModSourceProviderChanged,
  readModSourceDisplayValue,
  updateModSourceDisplayValue,
  getModSourceConfig,
} = useGameFormModProfile(game)
const { initializeNewGameForm } = useGameFormNewGameSetup({
  game,
  downstreamImpactServers,
  ensureTypedGameConfig,
  syncSimpleGameConfig,
  syncStructuredStartArgsFromGame,
  captureRuntimeBaselineFromCurrentState,
  syncActivePlatformFromGame,
  commitSnapshot,
})
const { loading, submitting, loadGameDetails, navigateToSchemaEditor, submit } =
  useGameFormPersistence({
    formRef,
    game,
    gameID,
    existingGame,
    copyGame,
    defaultPort,
    defaultQueryPort,
    configSchemas,
    downstreamImpactServers,
    savedSuccessfully,
    ensureTypedGameConfig,
    syncSimpleGameConfig,
    syncStructuredStartArgsFromGame,
    captureRuntimeBaselineFromCurrentState,
    syncStructuredStartArgsToGame,
    syncActivePlatformFromGame,
    commitFormSnapshot,
  })
const restoringOfficial = ref(false)

const showDivergedBanner = computed(() => {
  return (
    existingGame.value &&
    !copyGame.value &&
    game.value.xylonaOfficial &&
    game.value.officialDefinitionDiverged
  )
})

function confirmRestoreOfficial() {
  $q.dialog({
    title: 'Restore official definition?',
    message:
      'Local edits to this game will be replaced with the bundled official definition. Existing game servers keep their own settings.',
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'primary', label: 'Restore' },
    persistent: true,
  }).onOk(() => {
    void restoreOfficialDefinition()
  })
}

async function restoreOfficialDefinition() {
  restoringOfficial.value = true
  try {
    const request = create(ResetGameToOfficialDefinitionRequestSchema, {
      gameId: gameID.value,
    })
    await GetXylonaClient().resetGameToOfficialDefinition(request)
    await loadGameDetails()
    $q.notify({
      type: 'xylona-success',
      position: 'top',
      caption: 'Official definition restored.',
      icon: 'check_circle',
    })
  } catch (unknownError: unknown) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption:
        'Failed to restore official definition: ' +
        ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  } finally {
    restoringOfficial.value = false
  }
}

const runtimePolicySummary = computed(() => {
  const summary = [
    game.value.allowStartArgEditing ? 'Owners on' : 'Owners off',
    `${startArgBlocklist.value.length} reserved`,
  ]

  if (existingGame.value && !copyGame.value) {
    summary.push(`${downstreamImpactServers.value.length} affected`)
  }

  return summary
})

const runtimePolicyAssistiveSummary = computed(
  () => `Runtime guardrails. ${runtimePolicySummary.value.join('. ')}.`,
)

// Provide form context to tab child components
provide(gameFormContextKey, {
  game,
  existingGame,
  copyGame,
  activeFormTab,
  defaultPort,
  defaultQueryPort,
  activePlatform,
  activePlatformResolved,
  idRules,
  nameRules,
  portRules,
  commandTypeOptions,
  linuxCommandProcessorOptions,
  windowsCommandProcessorOptions,
  isCommandTypeCommand,
  commandTypeSummary,
  highlightCommand,
  linuxStartArgsTemplate,
  windowsStartArgsTemplate,
  startArgBlocklist,
  baselineLinuxBaseCommand,
  baselineWindowsBaseCommand,
  baselineLinuxStartArgsTemplate,
  baselineWindowsStartArgsTemplate,
  runtimeSequenceExpanded,
  runtimePolicyExpanded,
  runtimePolicySummary,
  runtimePolicyAssistiveSummary,
  toggleRuntimePolicy,
  updateRuntimeSequenceExpanded,
  downstreamImpactServers,
  defaultEnvRows,
  defaultEnvIssues,
  defaultEnvLoading,
  defaultEnvSaving,
  addDefaultEnvRow,
  removeDefaultEnvRow,
  saveDefaultEnvironment,
  modSourceOptions,
  managedModConfig,
  variantModSummaries,
  activeModSourceLabel,
  addGameModProfile,
  clearGameModProfile,
  onModSourceProviderChanged,
  readModSourceDisplayValue,
  updateModSourceDisplayValue,
  getModSourceConfig,
  configSchemas,
  navigateToSchemaEditor,
  managedTypedConfig,
} satisfies GameFormContext)

function ensureTypedGameConfig(): void {
  if (!game.value.updateProvider) {
    game.value.updateProvider = create(UpdateProviderConfigSchema, {})
  }
  if (!Array.isArray(game.value.variants)) {
    game.value.variants = []
  }
  ensureModProfileSources()
}

function syncSimpleGameConfig(): void {
  ensureTypedGameConfig()
  if (managedTypedConfig.value) {
    return
  }
  applySimpleGameConfig(game.value)
}

function commandTypeSummary(commandType: CommandType, operation: 'install' | 'update'): string {
  switch (commandType) {
    case CommandType.NONE:
      return `No ${operation} step will run for this platform.`
    case CommandType.STEAMCMD:
      return `Xylona will generate the SteamCMD ${operation} command automatically from the Steam App ID.`
    case CommandType.PAPERMC:
      return `Xylona will handle the PaperMC ${operation} flow internally.`
    case CommandType.MOJANG:
      return `Xylona will handle the Mojang ${operation} flow internally.`
    default:
      return `Use a shell command for this ${operation} step.`
  }
}

// --- Command syntax highlighting ---

function escapeHTML(str: string): string {
  return str.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}

function highlightCommand(cmd: string): string {
  if (!cmd) return ''
  const escaped = escapeHTML(cmd)
  return escaped.replace(/(\S+)/g, (token) => {
    // Flags: starts with - or + (JVM flags like -XX:+UseZGC)
    if (/^[-+]/.test(token)) {
      return `<span class="cmd-hl-flag">${token}</span>`
    }
    // Known binaries / jar files
    if (
      /\.(jar|exe|sh|bat)$/i.test(token) ||
      /^(java|steamcmd|python|node|bash|sh|cmd)$/i.test(token)
    ) {
      return `<span class="cmd-hl-binary">${token}</span>`
    }
    return token
  })
}

// --- Expose dirty state for route-level navigation guards ---

defineExpose({
  isDirty,
  savedSuccessfully,
})

onMounted(async () => {
  // Sticky header detection
  if (stickySentinel.value) {
    stickyObserver = new IntersectionObserver(
      ([entry]) => {
        isStuck.value = !entry.isIntersecting
      },
      { threshold: 0 },
    )
    stickyObserver.observe(stickySentinel.value)
  }

  if (props.existingGameId !== '') {
    existingGame.value = true
    gameID.value = props.existingGameId
  }
  if (props.copyGameId !== '') {
    copyGame.value = true
    gameID.value = props.copyGameId
  }
  if (existingGame.value || copyGame.value) {
    await loadGameDetails()
    await initializeGameDefaultEnvironment()
    commitDefaultEnvSnapshot()
  } else {
    await initializeNewGameForm()
    resetDefaultEnvironment()
    commitSnapshot()
  }
})

onBeforeUnmount(() => {
  stickyObserver?.disconnect()
})

function handleCancel() {
  // The onBeforeRouteLeave guard handles the unsaved changes prompt
  router.back()
}

async function exportCurrentGame(): Promise<void> {
  if (gameID.value === '') {
    return
  }

  try {
    const fileName = await exportGameDefinitionJSON(gameID.value)
    $q.notify({
      type: 'xylona-success',
      position: 'top',
      caption: `Exported ${fileName}.`,
      icon: 'check_circle',
    })
  } catch (unknownError: unknown) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to export game: ' + ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  }
}

function resetDefaultEnvironment(): void {
  defaultEnvRows.value = []
  defaultEnvIssues.value = []
}

async function initializeGameDefaultEnvironment(): Promise<void> {
  if (!existingGame.value || copyGame.value || gameID.value === '') {
    resetDefaultEnvironment()
    return
  }

  defaultEnvLoading.value = true
  try {
    const response = await GetXylonaClient().getGameEnvironment(
      create(GetGameEnvironmentRequestSchema, {
        gameId: gameID.value,
      }),
    )
    defaultEnvRows.value = cloneEnvironmentVariables(response.defaultEnv)
    defaultEnvIssues.value = response.validationIssues
  } catch (unknownError: unknown) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption:
        'Failed to load default environment: ' +
        ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  } finally {
    defaultEnvLoading.value = false
  }
}

function cloneEnvironmentVariables(variables: EnvironmentVariable[]): EnvironmentVariable[] {
  return variables.map((variable) =>
    create(EnvironmentVariableSchema, {
      name: variable.name,
      value: variable.value,
    }),
  )
}

function addDefaultEnvRow(): void {
  defaultEnvRows.value.push(create(EnvironmentVariableSchema))
}

function removeDefaultEnvRow(index: number): void {
  defaultEnvRows.value.splice(index, 1)
}

async function saveDefaultEnvironment(): Promise<void> {
  if (gameID.value === '') {
    return
  }

  defaultEnvSaving.value = true
  try {
    const defaultEnv = defaultEnvRows.value.map((row) =>
      create(EnvironmentVariableSchema, {
        name: row.name.trim(),
        value: row.value,
      }),
    )
    const response = await GetXylonaClient().updateGameEnvironment(
      create(UpdateGameEnvironmentRequestSchema, {
        gameId: gameID.value,
        defaultEnv,
      }),
    )
    defaultEnvRows.value = cloneEnvironmentVariables(response.defaultEnv)
    defaultEnvIssues.value = response.validationIssues
    commitDefaultEnvSnapshot()
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Default environment saved.',
      icon: 'task_alt',
    })
  } catch (unknownError: unknown) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption:
        'Failed to save default environment: ' +
        ConnectErrorToString(ConnectError.from(unknownError)),
      icon: 'report_problem',
    })
  } finally {
    defaultEnvSaving.value = false
  }
}
</script>

<!-- Unscoped so child tab components inherit styles. Class names are well-namespaced. -->
<style>
.game-form-wrapper {
  width: 100%;
  --game-form-sticky-stack-offset: calc(var(--xy-header-stack-height, 50px) + 4rem);
}

/* ---- Header ---- */

.game-form-header {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-md);
  padding: calc(var(--xy-space-sm) + 2px) var(--xy-space-lg) calc(var(--xy-space-xs) + 2px);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  border-radius: 8px 8px 0 0;
  position: sticky;
  top: var(--xy-toolbar-height);
  z-index: 10;
  transition:
    border-color 0.2s ease,
    box-shadow 0.2s ease;
}

.game-form-header.is-stuck {
  border-bottom: 2px solid var(--xy-accent);
  box-shadow: var(--xy-shadow-md);
  border-radius: 0;
}

.sticky-sentinel {
  height: 0;
  overflow: hidden;
}

.game-form-header-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.structured-start-stack {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-lg);
}

.runtime-policy-panel {
  display: flex;
  flex-direction: column;
  gap: 0;
  border-radius: 18px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-gradient-subtle), var(--xy-surface-1);
  box-shadow: var(--xy-shadow-md);
  overflow: hidden;
}

.runtime-policy-toggle {
  width: 100%;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-lg);
  padding: 1rem 1rem 0.9rem;
  border: none;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
}

.runtime-policy-toggle-copy {
  display: flex;
  flex-direction: column;
  gap: 0.22rem;
  min-width: 0;
}

.runtime-policy-eyebrow {
  font-size: 0.74rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.runtime-policy-summary-line {
  max-width: 38rem;
  font-size: 0.84rem;
  line-height: 1.45;
  color: color-mix(in srgb, var(--xy-accent) 12%, var(--xy-text-secondary) 88%);
}

.runtime-policy-header-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 0.65rem;
}

.runtime-policy-toggle-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.42rem;
  min-height: 2.15rem;
  padding: 0.38rem 0.42rem;
  border-radius: 999px;
  border: none;
  background: transparent;
  color: var(--xy-accent);
  font-size: 0.82rem;
  cursor: pointer;
  transition: color var(--xy-transition-fast);
}

.runtime-policy-toggle:hover .runtime-policy-toggle-indicator {
  color: var(--xy-accent-hover);
}

.runtime-policy-toggle-indicator .q-icon {
  transition: transform 180ms ease-out;
}

.runtime-policy-toggle:hover .runtime-policy-toggle-indicator .q-icon {
  transform: scale(1.12);
}

@media (prefers-reduced-motion: reduce) {
  .runtime-policy-panel-enter-active,
  .runtime-policy-panel-leave-active,
  .runtime-policy-toggle-indicator,
  .runtime-policy-toggle-indicator .q-icon {
    transition: none;
  }
}

.runtime-policy-toggle:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--xy-accent) 62%, transparent);
  outline-offset: 2px;
}

.runtime-policy-sr-only {
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

.runtime-policy-content {
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
  padding: 0 clamp(16px, 1.7vw, 22px) clamp(16px, 1.7vw, 22px);
  border-top: 1px solid color-mix(in srgb, var(--xy-border) 82%, transparent);
}

.runtime-policy-panel-enter-active,
.runtime-policy-panel-leave-active {
  transition:
    opacity 180ms cubic-bezier(0.25, 1, 0.5, 1),
    transform 240ms cubic-bezier(0.22, 1, 0.36, 1);
}

.runtime-policy-panel-enter-from,
.runtime-policy-panel-leave-to {
  opacity: 0;
  transform: translateY(-10px);
}

.runtime-policy-layout {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(240px, 292px);
  gap: 0.8rem;
  padding-top: 0.85rem;
}

.runtime-policy-rail {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.runtime-policy-subsection {
  display: flex;
  flex-direction: column;
  gap: 0.62rem;
  padding: 0.82rem 0;
}

.runtime-policy-subsection + .runtime-policy-subsection {
  border-top: 1px solid var(--xy-border);
}

.runtime-policy-subsection--reserved {
  min-width: 0;
}

.game-default-env-panel {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  padding: 1rem;
  border: 1px solid var(--xy-border);
  border-radius: 10px;
  background: var(--xy-surface-1);
}

.game-default-env-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}

.game-default-env-title {
  font-size: 0.78rem;
  letter-spacing: 0.16em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.game-default-env-empty {
  padding: 0.5rem 0;
}

.game-default-env-row {
  display: grid;
  grid-template-columns: minmax(130px, 0.75fr) minmax(180px, 1fr) auto;
  gap: 0.65rem;
  align-items: center;
}

.game-default-env-actions {
  display: flex;
  justify-content: flex-end;
}

.runtime-policy-subsection--impact {
  /* no override needed -- inherits flat style */
}

.runtime-policy-subhead {
  color: var(--xy-text-primary);
  font-size: 0.88rem;
}

.runtime-policy-subcopy {
  font-size: 0.8rem;
  line-height: 1.4;
}

.runtime-policy-switch-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.9rem;
  padding: 0.8rem 0.85rem;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 16%, var(--xy-border) 84%);
  background: color-mix(in srgb, var(--xy-accent) 3%, var(--xy-surface-1) 97%);
}

.runtime-policy-rail-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.9rem;
}

.runtime-policy-switch-copy {
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
  min-width: 0;
}

.runtime-policy-switch-title {
  color: var(--xy-text-primary);
  font-size: 0.92rem;
  font-weight: 600;
}

.runtime-policy-switch-note {
  font-size: 0.82rem;
  line-height: 1.4;
}

.runtime-policy-mini-note {
  font-size: 0.75rem;
  line-height: 1.4;
}

.runtime-policy-card-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.9rem;
}

.runtime-policy-card-head--stacked {
  align-items: center;
}

.runtime-policy-card-copy {
  display: flex;
  flex-direction: column;
  gap: 0.22rem;
  min-width: 0;
}

.runtime-policy-quantity {
  display: inline-flex;
  align-items: center;
  min-height: 1.75rem;
  padding: 0.16rem 0.58rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 32%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-1) 92%);
  color: color-mix(in srgb, var(--xy-accent) 24%, var(--xy-text-primary) 76%);
  font-size: 0.72rem;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .runtime-policy-toggle {
    flex-direction: column;
    gap: 0.85rem;
  }

  .runtime-policy-header-actions {
    width: 100%;
    align-items: flex-start;
  }

  .runtime-policy-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .game-default-env-row {
    grid-template-columns: minmax(0, 1fr);
  }

  .runtime-policy-switch-row,
  .runtime-policy-rail-head,
  .runtime-policy-card-head {
    flex-direction: column;
    align-items: flex-start;
  }
}

.game-form-breadcrumbs {
  font-size: 0.76rem;
  line-height: 1;
}

.breadcrumb-list {
  display: flex;
  align-items: center;
  gap: 4px;
  list-style: none;
  margin: 0;
  padding: 0;
}

.breadcrumb-list li + li::before {
  content: '/';
  color: var(--xy-text-muted);
  opacity: 0.5;
  margin-right: 4px;
}

.breadcrumb-link {
  color: var(--xy-text-muted);
  text-decoration: none;
  transition: color var(--xy-transition-fast);
}

.breadcrumb-link:hover {
  color: var(--xy-accent);
}

.breadcrumb-current {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.game-form-title {
  margin: 0;
  font-size: 1.12rem;
  font-weight: 600;
  line-height: 1.1;
  color: var(--xy-text-primary);
  letter-spacing: 0.02em;
}

.game-form-guided-link {
  color: var(--xy-accent);
  margin-right: 8px;
  text-decoration: none;
}

.game-form-header-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.5rem;
  flex-shrink: 0;
  padding-top: 0;
}

/* ---- Loading ---- */

.game-form-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 200px;
}

/* ---- Diverged official definition banner ---- */

.game-form-diverged-banner {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  margin: 0.75rem 0;
  padding: 0.6rem 0.9rem;
  border: 1px solid var(--xy-warning);
  border-radius: 6px;
  background: var(--xy-surface-1);
}

.diverged-banner-icon {
  color: var(--xy-warning);
  flex-shrink: 0;
}

.diverged-banner-text {
  flex: 1;
  min-width: 0;
}

.diverged-banner-title {
  font-size: 0.82rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.diverged-banner-caption {
  font-size: 0.72rem;
  color: var(--xy-text-muted);
}

.diverged-banner-btn {
  flex-shrink: 0;
}

@media (max-width: 640px) {
  .game-form-diverged-banner {
    flex-wrap: wrap;
  }
}

/* ---- Body ---- */

.game-form-body {
  padding: var(--xy-space-xs) var(--xy-space-lg) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border-radius: 0 0 8px 8px;
}

.game-form-tabs-panel {
  display: flex;
  flex-direction: column;
  gap: 0.45rem;
  margin-bottom: var(--xy-space-sm);
  padding-top: 0;
}

.game-form-tabs-shell {
  overflow-x: auto;
  border-bottom: 1px solid var(--xy-border);
}

.game-form-tabs {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
  min-width: max-content;
}

.game-form-tab {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 2px;
  border: none;
  border-bottom: 2px solid transparent;
  background: transparent;
  color: var(--xy-text-muted);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.game-form-tab:hover {
  color: var(--xy-text-secondary);
}

.game-form-tab--active {
  color: var(--xy-text-primary);
  border-bottom-color: var(--xy-primary);
}

.game-form-tab__label {
  font-size: 0.8rem;
  letter-spacing: 0.04em;
  text-transform: none;
}

.game-form-tab-panel {
  min-width: 0;
}

.game-form-sr-only {
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

/* ---- Sections ---- */

.game-form-wrapper .form-section {
  padding-top: var(--xy-space-lg);
  border-bottom: 1px solid var(--xy-border);
  padding-bottom: var(--xy-space-lg);
}

.game-form-wrapper .form-section:first-child {
  padding-top: var(--xy-space-sm);
}

.game-form-wrapper .form-section--compact {
  padding-top: var(--xy-space-sm);
  padding-bottom: var(--xy-space-md);
}

.game-form-wrapper .form-section--last {
  border-bottom: none;
  padding-bottom: 0;
}

/* ---- Overview Metadata Strip ---- */

.overview-metadata {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  gap: var(--xy-space-lg);
}

@media (max-width: 900px) {
  .overview-metadata {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--xy-space-md);
  }
}

.game-form-wrapper .section-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
}

.game-form-wrapper .section-bar {
  width: 3px;
  height: 16px;
  border-radius: 2px;
  flex-shrink: 0;
}

.game-form-wrapper .section-title {
  margin: 0;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--xy-text-secondary);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  white-space: nowrap;
}

.game-form-wrapper .section-line {
  flex: 1;
  height: 1px;
  background: var(--xy-border);
  margin-left: var(--xy-space-xs);
}

/* ---- Feature Chips ---- */

.feature-groups {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-lg);
}

.feature-group {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.feature-group-label {
  font-size: 0.62rem;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.feature-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: stretch;
  gap: var(--xy-space-sm);
}

.feature-chip {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 44px;
  padding: 10px 14px;
  border-radius: 6px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  cursor: pointer;
  transition:
    background var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast),
    opacity var(--xy-transition-fast);
  color: var(--xy-text-muted);
  font-size: 0.8rem;
  font-family: inherit;
  line-height: 1.2;
  opacity: 0.7;
}

.feature-chip:hover {
  border-color: var(--xy-border-hover);
  opacity: 1;
}

.feature-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--xy-text-muted);
  opacity: 0.4;
  transition:
    background var(--xy-transition-fast),
    opacity var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
  flex-shrink: 0;
}

.feature-chip--active {
  background: var(--xy-primary-bg-subtle);
  border-color: var(--xy-primary-border-soft);
  color: var(--xy-text-primary);
  opacity: 1;
}

.feature-chip--active .feature-dot {
  background: var(--xy-primary);
  opacity: 1;
  box-shadow: 0 0 6px var(--xy-primary-glow-soft);
}

/* ---- Platform Tabs ---- */

.platform-empty {
  font-size: 0.82rem;
  padding: var(--xy-space-md);
  text-align: center;
  background: var(--xy-surface-0);
  border-radius: 8px;
  border: 1px solid var(--xy-border);
}

.platform-tabs {
  display: inline-flex;
  background: var(--xy-surface-0);
  border-radius: 8px;
  padding: 3px;
  gap: 2px;
  margin-bottom: var(--xy-space-md);
}

.platform-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 44px;
  padding: 6px 16px;
  border-radius: 6px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 0.75rem;
  color: var(--xy-text-muted);
  transition:
    background var(--xy-transition-fast),
    color var(--xy-transition-fast);
  font-family: inherit;
}

.platform-tab:hover {
  color: var(--xy-text-secondary);
}

.platform-tab--active {
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
}

.platform-icon-inactive {
  color: var(--xy-text-muted);
}

.platform-icon-windows-active {
  color: var(--xy-platform-windows);
}

.platform-icon-linux-active {
  color: var(--xy-platform-linux);
}

/* ---- Command Blocks ---- */

.platform-commands {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.cmd-block {
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  overflow: hidden;
  transition: border-color var(--xy-transition-fast);
}

.cmd-block--windows {
  border-color: var(--xy-platform-windows);
}

.cmd-block--windows:hover {
  border-color: var(--xy-platform-windows-hover);
}

.cmd-block--linux {
  border-color: var(--xy-platform-linux);
}

.cmd-block--linux:hover {
  border-color: var(--xy-platform-linux-hover);
}

.cmd-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding: 6px 12px;
  border-bottom: 1px solid var(--xy-border);
}

.cmd-label {
  font-size: 0.72rem;
  color: var(--xy-text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  font-weight: 500;
}

.cmd-badge {
  font-size: 0.6rem;
  letter-spacing: 0.04em;
  border: 1px solid var(--xy-border);
  padding: 1px 6px;
  border-radius: 3px;
}

.cmd-type-select {
  appearance: none;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  color: var(--xy-text-secondary);
  font-size: 0.65rem;
  min-height: 32px;
  padding: 2px 24px 2px 8px;
  cursor: pointer;
  outline: none;
  background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E%3Cpath d='M1 1l4 4 4-4' stroke='%23979b9e' stroke-width='1.5' fill='none' stroke-linecap='round' stroke-linejoin='round'/%3E%3C/svg%3E");
  background-repeat: no-repeat;
  background-position: right 8px center;
  transition:
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
}

.cmd-type-select:hover {
  border-color: var(--xy-border-hover);
}

.cmd-type-select:focus {
  border-color: var(--xy-primary);
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--xy-primary) 24%, transparent);
}

.cmd-type-select option {
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
}

.cmd-type-group {
  display: flex;
  align-items: center;
  gap: 6px;
}

.cmd-type-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.cmd-type-label {
  font-size: 0.6rem;
  color: var(--xy-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  font-weight: 500;
}

.cmd-input-wrap {
  position: relative;
}

.cmd-highlight {
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  padding: 10px 12px;
  font-size: 0.82rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-all;
  pointer-events: none;
  overflow: hidden;
}

.cmd-textarea {
  display: block;
  width: 100%;
  background: transparent;
  border: none;
  color: transparent;
  caret-color: var(--xy-text-primary);
  font-size: 0.82rem;
  padding: 10px 12px;
  resize: vertical;
  outline: none;
  line-height: 1.5;
  position: relative;
  z-index: 1;
  transition: box-shadow var(--xy-transition-fast);
}

.cmd-textarea:focus-visible {
  box-shadow: inset 0 0 0 1px var(--xy-primary);
}

.cmd-textarea::placeholder {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

/* When textarea is empty, show placeholder text (not the highlight layer) */
.cmd-textarea:placeholder-shown {
  color: transparent;
}

.cmd-internal {
  padding: 10px 12px;
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  font-style: italic;
}

/* Working directory doesn't need highlighting — show text normally */
.cmd-block .cmd-textarea:only-child {
  color: var(--xy-text-primary);
}

/* ---- Section Help Text ---- */

.section-help {
  font-size: 0.78rem;
  margin-top: calc(var(--xy-space-md) * -0.5);
  margin-bottom: var(--xy-space-md);
  line-height: 1.5;
}

/* ---- Mods Layout ---- */

.mods-layout {
  display: grid;
  grid-template-columns: minmax(220px, 280px) minmax(0, 1fr);
  gap: clamp(16px, 2vw, 26px);
  align-items: start;
}

.mods-layout-single {
  display: block;
}

.mods-rail {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.mods-rail-intro,
.mods-rail-signal {
  padding: clamp(14px, 1.6vw, 18px);
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--xy-info) 15%, var(--xy-border) 85%);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-info) 7%, transparent), transparent 64%),
    var(--xy-surface-0);
}

.mods-rail-eyebrow,
.mods-rail-signal-label {
  display: inline-flex;
  align-items: center;
  font-size: 0.7rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--xy-info) 58%, var(--xy-text-secondary) 42%);
}

.mods-rail-title {
  margin-top: 0.5rem;
  color: var(--xy-text-primary);
  font-size: 0.98rem;
  line-height: 1.28;
}

.mods-rail-copy,
.mods-rail-signal-copy {
  margin-top: 0.45rem;
  font-size: 0.8rem;
  line-height: 1.55;
}

.mods-rail-signals {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.mods-workspace {
  min-width: 0;
}

.mods-workspace-card {
  min-height: 100%;
  border-radius: 16px;
  border-color: color-mix(in srgb, var(--xy-info) 12%, var(--xy-border) 88%);
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-info) 4%, transparent), transparent 36%),
    var(--xy-surface-0);
}

.mods-workspace-header {
  gap: var(--xy-space-md);
}

.mods-status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.55rem;
}

.mods-status-chip {
  display: inline-flex;
  align-items: center;
  min-height: 1.95rem;
  padding: 0.28rem 0.72rem;
  border-radius: 999px;
  border: 1px solid var(--xy-border);
  background: color-mix(in srgb, var(--xy-surface-1) 86%, transparent);
  color: var(--xy-text-secondary);
  font-size: 0.72rem;
  line-height: 1;
}

.variant-mod-list {
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  margin-top: 0.75rem;
}

.variant-mod-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  padding: 0.4rem 0;
  border-top: 1px solid var(--xy-border);
}

.variant-mod-name {
  min-width: 6rem;
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.variant-mod-none {
  font-size: 0.72rem;
  font-style: italic;
}

.mods-status-chip--active {
  border-color: color-mix(in srgb, var(--xy-info) 28%, var(--xy-border) 72%);
  background: color-mix(in srgb, var(--xy-info) 8%, var(--xy-surface-1) 92%);
  color: color-mix(in srgb, var(--xy-info) 32%, var(--xy-text-primary) 68%);
}

.mods-quickstart {
  display: flex;
  flex-direction: column;
  gap: 0.65rem;
  padding: 0.9rem 1rem;
  border: 1px dashed color-mix(in srgb, var(--xy-info) 22%, var(--xy-border) 78%);
  border-radius: 14px;
  background: color-mix(in srgb, var(--xy-info) 4%, var(--xy-surface-1) 96%);
}

.mods-quickstart-title {
  font-size: 0.8rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--xy-info) 42%, var(--xy-text-primary) 58%);
}

.mods-quickstart-copy,
.mods-next-step {
  font-size: 0.78rem;
  line-height: 1.55;
}

.mods-quickstart-steps {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.mods-quickstart-step {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 0.7rem;
  align-items: start;
}

.mods-quickstart-step-index {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 2.1rem;
  min-height: 2.1rem;
  padding: 0 0.45rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-info) 22%, var(--xy-border) 78%);
  background: color-mix(in srgb, var(--xy-surface-0) 84%, transparent);
  font-size: 0.74rem;
  color: color-mix(in srgb, var(--xy-info) 48%, var(--xy-text-primary) 52%);
}

.mods-quickstart-step-copy {
  padding-top: 0.25rem;
  font-size: 0.78rem;
  line-height: 1.5;
  color: var(--xy-text-secondary);
}

.mods-next-step {
  padding: 0.82rem 0.95rem;
  border-radius: 12px;
  border: 1px solid color-mix(in srgb, var(--xy-info) 18%, var(--xy-border) 82%);
  background: color-mix(in srgb, var(--xy-info) 5%, var(--xy-surface-1) 95%);
}

/* ---- Typed Game Config ---- */

.typed-config-grid {
  display: grid;
  gap: var(--xy-space-md);
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}

.typed-config-card {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: 10px;
}

.typed-config-header {
  display: flex;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  align-items: flex-start;
}

.typed-config-title {
  font-size: 0.94rem;
  color: var(--xy-text-primary);
}

.typed-config-copy {
  font-size: 0.78rem;
  line-height: 1.45;
  margin-top: 0.15rem;
}

.typed-config-copy--secondary {
  max-width: 34rem;
}

.typed-config-fields {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.typed-config-managed {
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border: 1px dashed var(--xy-border);
  border-radius: 10px;
  line-height: 1.55;
}

.typed-config-fields--nested {
  padding: var(--xy-space-sm);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
}

.typed-subtitle {
  font-size: 0.78rem;
  font-weight: 600;
  color: var(--xy-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

/* ---- Preset Selector ---- */

.preset-selector {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  margin-bottom: var(--xy-space-md);
}

.preset-select {
  max-width: 400px;
}

.preset-description {
  font-size: 0.78rem;
  line-height: 1.4;
  padding-left: 2px;
}

@media (max-width: 700px) {
  .game-form-header {
    grid-template-columns: minmax(0, 1fr);
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm) var(--xy-space-md) calc(var(--xy-space-xs) + 2px);
  }

  .game-form-header-left {
    width: 100%;
  }

  .game-form-header-actions {
    width: 100%;
    padding-top: 0;
  }

  .game-form-title {
    font-size: 1.04rem;
    line-height: 1.15;
  }

  .game-form-body {
    padding: var(--xy-space-xs) var(--xy-space-md) var(--xy-space-md);
  }

  .game-form-tabs-shell {
    margin-inline: calc(var(--xy-space-xs) * -1);
    padding-inline: var(--xy-space-xs);
  }

  .game-form-tabs {
    gap: var(--xy-space-sm);
  }

  .game-form-tab {
    min-height: 44px;
  }

  .game-form-tab__label {
    font-size: 0.76rem;
  }

  .feature-groups {
    flex-direction: column;
    gap: var(--xy-space-md);
  }

  .feature-chips {
    display: grid;
    grid-template-columns: 1fr;
  }

  .feature-chip {
    width: 100%;
    justify-content: flex-start;
  }

  .mods-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .mods-workspace {
    order: -1;
  }

  .platform-tabs {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
  }

  .platform-tab {
    width: 100%;
  }

  .cmd-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .cmd-type-row {
    width: 100%;
    gap: var(--xy-space-sm);
  }

  .cmd-type-select {
    min-height: 40px;
    font-size: 0.72rem;
    padding-inline: 10px;
  }
}
</style>

<style>
.game-form-wrapper .cmd-hl-flag {
  color: var(--xy-accent);
}

.game-form-wrapper .cmd-hl-binary {
  color: var(--xy-warning);
}
</style>
