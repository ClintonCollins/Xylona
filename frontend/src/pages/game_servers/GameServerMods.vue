<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { notifyConnectError, notifyError, notifySuccess } from '@/api/notifications'
import { GetXylonaClient } from '@/utils/shared'
import {
  GetGameServerRequestSchema,
  GetModVersionsRequestSchema,
  GetSevenDaysToDieReportedModsRequestSchema,
  GetUpdateTargetsRequestSchema,
  InstallModRequestSchema,
  ListInstalledModsRequestSchema,
  PinModVersionRequestSchema,
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIValueState,
  SetModAutoUpdateRequestSchema,
  SetModEnabledRequestSchema,
  type SevenDaysToDieReportedMod,
  UninstallModRequestSchema,
  UpdateModRequestSchema,
} from '@/proto/xylona_pb'
import type { InstalledMod, ModVersion } from '@/proto/shared_pb'
import InstalledModsTable from '@/components/game_servers/InstalledModsTable.vue'
import ModBrowse from '@/components/game_servers/ModBrowse.vue'
import ModDetailDialog from '@/components/game_servers/ModDetailDialog.vue'
import ModInstallDialog from '@/components/game_servers/ModInstallDialog.vue'
import PageHeader from '@/components/shared/PageHeader.vue'

const $q = useQuasar()
const route = useRoute()
const gameServerId = route.params.id as string

const activeTab = ref<'installed' | 'browse'>('installed')
const loading = ref(true)
const installedMods = ref<InstalledMod[]>([])
const isSevenDaysToDie = ref(false)
const reportedModsLoading = ref(false)
const reportedModsConnectionState = ref(
  SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED,
)
const reportedModsState = ref(
  SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED,
)
const reportedMods = ref<SevenDaysToDieReportedMod[]>([])

// Detail dialog state
const showDetailDialog = ref(false)
const detailSource = ref('')
const detailSourceId = ref('')

// Install dialog state
const showInstallDialog = ref(false)
const installModName = ref('')
const installModVersion = ref('')
const installFileSize = ref(0)
const installDeps = ref<{ sourceId: string; name: string; required: boolean }[]>([])
const pendingInstall = ref<{
  source: string
  sourceId: string
  versionId: string
} | null>(null)

// Mod sources derived from the resolved mod profile.
const modSources = ref<{ id: string; searchParams: Record<string, unknown> }[]>([])

// Available game versions for the browse version filter.
const availableVersions = ref<string[]>([])

const detailIsInstalled = computed(() => {
  return installedMods.value.some(
    (mod) => mod.source === detailSource.value && mod.sourceId === detailSourceId.value,
  )
})

const reportedModsStateText = computed((): string => {
  if (reportedModsLoading.value) return 'Loading reported mods...'
  if (
    reportedModsConnectionState.value ===
    SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE
  ) {
    return 'The game server is offline.'
  }
  if (
    reportedModsState.value ===
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED
  ) {
    return 'This game server does not support reporting mods.'
  }
  if (
    reportedModsState.value ===
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED ||
    reportedModsConnectionState.value ===
      SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED
  ) {
    return 'The game server denied access to its reported mods.'
  }
  if (
    reportedModsState.value ===
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE &&
    reportedMods.value.length === 0
  ) {
    return 'No mods reported by the game server.'
  }
  if (
    reportedModsState.value !==
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
  ) {
    return 'Reported mods are currently unavailable.'
  }
  return ''
})

onMounted(async () => {
  const installedModsPromise = loadInstalledMods()
  await loadGameServerConfig()
  if (isSevenDaysToDie.value) {
    await loadReportedMods()
  }
  await installedModsPromise

  // Keep 7 Days to Die on Installed so its native inventory remains visible.
  if (!isSevenDaysToDie.value && installedMods.value.length === 0) {
    activeTab.value = 'browse'
  }
})

async function loadInstalledMods(): Promise<void> {
  loading.value = true
  try {
    const request = create(ListInstalledModsRequestSchema, {
      gameServerId,
    })
    const response = await GetXylonaClient().listInstalledMods(request)
    installedMods.value = response.installedMods
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    loading.value = false
  }
}

async function loadGameServerConfig(): Promise<void> {
  try {
    const request = create(GetGameServerRequestSchema, { id: gameServerId })
    const response = await GetXylonaClient().getGameServer(request)
    const gs = response.gameServer
    if (!gs) return

    isSevenDaysToDie.value = gs.gameId === '7_days_to_die'

    const resolvedModProfile = gs.resolvedModProfile
    modSources.value =
      resolvedModProfile?.sources.map((src) => ({
        id: src.id,
        searchParams: src.searchParamsJson ? JSON.parse(src.searchParamsJson) : {},
      })) ?? []

    if (gs.selectedVariantId || gs.resolvedUpdateProvider) {
      void loadAvailableVersions()
    }
  } catch (unknownErr: unknown) {
    console.error('Failed to load game server config:', unknownErr)
  }
}

async function loadReportedMods(): Promise<void> {
  reportedModsLoading.value = true
  try {
    const request = create(GetSevenDaysToDieReportedModsRequestSchema, { gameServerId })
    const response = await GetXylonaClient().getSevenDaysToDieReportedMods(request)
    reportedModsConnectionState.value = response.connectionState
    reportedModsState.value = response.state
    reportedMods.value = response.mods
  } catch {
    reportedModsConnectionState.value =
      SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_UNSPECIFIED
    reportedModsState.value =
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
    reportedMods.value = []
  } finally {
    reportedModsLoading.value = false
  }
}

async function loadAvailableVersions(): Promise<void> {
  try {
    const request = create(GetUpdateTargetsRequestSchema, {
      gameServerId,
    })
    const response = await GetXylonaClient().getUpdateTargets(request)
    availableVersions.value = response.targets.map((target) => target.label || target.id)
  } catch {
    // Non-critical — silently ignore
  }
}

async function handleUpdate(modId: string): Promise<void> {
  try {
    const request = create(UpdateModRequestSchema, {
      gameServerId,
      installedModId: modId,
      versionId: '', // empty = latest
    })
    await GetXylonaClient().updateMod(request)
    notifySuccess('Mod updated successfully')
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

async function handleUninstall(modId: string): Promise<void> {
  const mod = installedMods.value.find((m) => m.id === modId)
  const modName = mod?.modName ?? 'this mod'

  $q.dialog({
    title: 'Uninstall Mod',
    message: `Are you sure you want to uninstall ${modName}?`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Uninstall' },
    persistent: true,
  }).onOk(async () => {
    try {
      const request = create(UninstallModRequestSchema, {
        gameServerId,
        installedModId: modId,
      })
      await GetXylonaClient().uninstallMod(request)
      notifySuccess(`${modName} uninstalled`)
      await loadInstalledMods()
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr)
    }
  })
}

async function handleToggleAutoUpdate(modId: string, enabled: boolean): Promise<void> {
  // Optimistically replace the mod object to trigger Vue reactivity.
  const idx = installedMods.value.findIndex((m) => m.id === modId)
  if (idx !== -1) {
    const updated = Object.assign(
      Object.create(Object.getPrototypeOf(installedMods.value[idx])),
      installedMods.value[idx],
    )
    updated.autoUpdate = enabled
    installedMods.value.splice(idx, 1, updated)
  }

  try {
    const request = create(SetModAutoUpdateRequestSchema, {
      gameServerId,
      installedModId: modId,
      enabled,
    })
    await GetXylonaClient().setModAutoUpdate(request)
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
    // Revert on failure
    await loadInstalledMods()
  }
}

async function handleToggleEnabled(modId: string, enabled: boolean): Promise<void> {
  try {
    const request = create(SetModEnabledRequestSchema, {
      gameServerId,
      installedModId: modId,
      enabled,
    })
    await GetXylonaClient().setModEnabled(request)
    notifySuccess(`Mod ${enabled ? 'enabled' : 'disabled'}`)
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

async function handlePinVersion(modId: string): Promise<void> {
  const mod = installedMods.value.find((m) => m.id === modId)
  if (!mod) return

  const isPinned = mod.pinnedVersion !== ''

  if (isPinned) {
    // Unpin
    try {
      const request = create(PinModVersionRequestSchema, {
        gameServerId,
        installedModId: modId,
        version: '',
      })
      await GetXylonaClient().pinModVersion(request)
      notifySuccess('Version unpinned')
      await loadInstalledMods()
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr)
    }
    return
  }

  // Pin to current version
  try {
    const request = create(PinModVersionRequestSchema, {
      gameServerId,
      installedModId: modId,
      version: mod.installedVersion,
    })
    await GetXylonaClient().pinModVersion(request)
    notifySuccess(`Pinned to version ${mod.installedVersion}`)
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

async function handleUpdateAll(): Promise<void> {
  const modsToUpdate = installedMods.value.filter((mod) => mod.updateAvailable && mod.enabled)
  if (modsToUpdate.length === 0) return

  let successCount = 0
  let failCount = 0

  for (const mod of modsToUpdate) {
    try {
      const request = create(UpdateModRequestSchema, {
        gameServerId,
        installedModId: mod.id,
        versionId: '', // empty = latest
      })
      await GetXylonaClient().updateMod(request)
      successCount++
    } catch {
      failCount++
    }
  }

  if (successCount > 0) {
    notifySuccess(`${successCount} mod${successCount === 1 ? '' : 's'} updated`)
  }
  if (failCount > 0) {
    notifyError(`${failCount} mod${failCount === 1 ? '' : 's'} failed to update`)
  }

  await loadInstalledMods()
}

function handleViewDetails(source: string, sourceId: string): void {
  detailSource.value = source
  detailSourceId.value = sourceId
  showDetailDialog.value = true
}

async function handleInstallFromBrowse(source: string, sourceId: string): Promise<void> {
  // Fetch versions to get the latest version info and dependencies
  try {
    const request = create(GetModVersionsRequestSchema, {
      gameServerId,
      source,
      sourceId,
    })
    const response = await GetXylonaClient().getModVersions(request)

    if (response.versions.length === 0) {
      notifyError('No versions available for this mod')
      return
    }

    const latestVersion = response.versions[0]

    // If there are no new dependencies, install directly without showing
    // the confirmation dialog to reduce friction.
    const hasNewDeps = latestVersion.dependencies.some(
      (dep) => !installedMods.value.some((m) => m.sourceId === dep.sourceId),
    )
    if (!hasNewDeps) {
      await directInstall(source, sourceId, latestVersion.versionId)
      return
    }

    openInstallDialog(source, sourceId, latestVersion)
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

async function directInstall(source: string, sourceId: string, versionId: string): Promise<void> {
  try {
    const request = create(InstallModRequestSchema, {
      gameServerId,
      source,
      sourceId,
      versionId,
    })
    await GetXylonaClient().installMod(request)
    notifySuccess('Mod installed successfully')
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

async function handleInstallFromDetail(
  source: string,
  sourceId: string,
  versionId: string,
): Promise<void> {
  showDetailDialog.value = false

  // Fetch versions to get dependency info for the selected version
  try {
    const request = create(GetModVersionsRequestSchema, {
      gameServerId,
      source,
      sourceId,
    })
    const response = await GetXylonaClient().getModVersions(request)
    const version = response.versions.find((v) => v.versionId === versionId)
    if (version) {
      openInstallDialog(source, sourceId, version)
    }
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

function openInstallDialog(source: string, sourceId: string, version: ModVersion): void {
  installModName.value = sourceId
  installModVersion.value = version.versionString
  installFileSize.value = Number(version.fileSize)
  installDeps.value = version.dependencies.map((dep) => ({
    sourceId: dep.sourceId,
    name: dep.name,
    required: dep.required,
  }))
  pendingInstall.value = {
    source,
    sourceId,
    versionId: version.versionId,
  }
  showInstallDialog.value = true
}

async function handleInstallConfirm(selectedDeps: string[]): Promise<void> {
  if (!pendingInstall.value) return

  const { source, sourceId, versionId } = pendingInstall.value
  showInstallDialog.value = false

  try {
    // Install the main mod
    const request = create(InstallModRequestSchema, {
      gameServerId,
      source,
      sourceId,
      versionId,
    })
    await GetXylonaClient().installMod(request)

    // Install selected dependencies
    for (const depSourceId of selectedDeps) {
      try {
        const depRequest = create(InstallModRequestSchema, {
          gameServerId,
          source,
          sourceId: depSourceId,
          versionId: '', // latest
        })
        await GetXylonaClient().installMod(depRequest)
      } catch (depErr: unknown) {
        notifyConnectError(depErr, 'Failed to install dependency')
      }
    }

    notifySuccess('Mod installed successfully')
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    pendingInstall.value = null
  }
}
</script>

<template>
  <div class="mods-page">
    <page-header class="mods-page-header" title="Mods" />
    <q-tabs
      v-model="activeTab"
      active-color="primary"
      align="left"
      class="mods-tabs"
      dense
      indicator-color="primary"
      narrow-indicator>
      <q-tab label="Installed" name="installed" />
      <q-tab label="Browse" name="browse" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" animated class="mods-panels">
      <q-tab-panel name="installed">
        <div :class="{ 'installed-sources--reported': isSevenDaysToDie }" class="installed-sources">
          <section
            :aria-labelledby="isSevenDaysToDie ? 'managed-mods-heading' : undefined"
            class="managed-mods-source">
            <h2 v-if="isSevenDaysToDie" id="managed-mods-heading" class="mod-source-heading">
              Xylona-managed
            </h2>
            <installed-mods-table
              :installed-mods="installedMods"
              :loading="loading"
              @uninstall="handleUninstall"
              @update="handleUpdate"
              @toggle-auto-update="handleToggleAutoUpdate"
              @toggle-enabled="handleToggleEnabled"
              @pin-version="handlePinVersion"
              @update-all="handleUpdateAll" />
          </section>

          <section
            v-if="isSevenDaysToDie"
            aria-labelledby="reported-mods-heading"
            class="reported-mods">
            <h2 id="reported-mods-heading" class="mod-source-heading">Reported by game server</h2>
            <div v-if="reportedModsStateText" class="reported-mods-state" role="status">
              <q-spinner v-if="reportedModsLoading" color="primary" size="1.5rem" />
              <span>{{ reportedModsStateText }}</span>
            </div>
            <div v-else class="reported-mods-grid">
              <article v-for="mod in reportedMods" :key="mod.name" class="reported-mod-card">
                <div class="reported-mod-header">
                  <div>
                    <h3 class="reported-mod-title">{{ mod.displayName || mod.name }}</h3>
                    <div class="reported-mod-name font-mono">{{ mod.name }}</div>
                  </div>
                  <span v-if="mod.version" class="reported-mod-version font-mono">
                    {{ mod.version }}
                  </span>
                </div>
                <div v-if="mod.author" class="reported-mod-author text-xy-muted">
                  By {{ mod.author }}
                </div>
                <p v-if="mod.description" class="reported-mod-description">
                  {{ mod.description }}
                </p>
              </article>
            </div>
          </section>
        </div>
      </q-tab-panel>

      <q-tab-panel name="browse">
        <mod-browse
          :available-versions="availableVersions"
          :game-server-id="gameServerId"
          :installed-mods="installedMods"
          :sources="modSources"
          @install="handleInstallFromBrowse"
          @view-details="handleViewDetails" />
      </q-tab-panel>
    </q-tab-panels>

    <!-- Detail dialog -->
    <mod-detail-dialog
      v-model:show="showDetailDialog"
      :game-server-id="gameServerId"
      :is-installed="detailIsInstalled"
      :source="detailSource"
      :source-id="detailSourceId"
      @install="handleInstallFromDetail" />

    <!-- Install confirmation dialog -->
    <mod-install-dialog
      v-model:show="showInstallDialog"
      :dependencies="installDeps"
      :file-size="installFileSize"
      :installed-mods="installedMods"
      :mod-name="installModName"
      :mod-version="installModVersion"
      @confirm="handleInstallConfirm" />
  </div>
</template>

<style scoped>
.mods-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.mods-page-header {
  padding: var(--xy-space-md) var(--xy-space-md) 0;
  margin-bottom: var(--xy-space-sm);
}

.mods-tabs {
  background-color: var(--xy-surface-1);
  flex-shrink: 0;
}

.mods-panels {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  background-color: transparent;
  display: flex;
  flex-direction: column;
}

/* Quasar QTabPanels internal wrapper chain: .mods-panels > .q-panel.scroll > .q-tab-panel.
   Each layer must constrain height so only browse-grid-scroll scrolls. */
.mods-panels :deep(.q-panel.scroll) {
  flex: 1;
  min-height: 0;
  overflow: hidden !important;
  display: flex;
  flex-direction: column;
}

.mods-panels :deep(.q-tab-panel) {
  padding: 0;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.installed-sources,
.managed-mods-source {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.installed-sources--reported {
  display: block;
  overflow-y: auto;
}

.installed-sources--reported .managed-mods-source {
  height: min(45vh, 28rem);
  min-height: 16rem;
}

.mod-source-heading {
  margin: 0;
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
  color: var(--xy-text-secondary);
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.reported-mods {
  border-top: 1px solid var(--xy-border);
}

.reported-mods-state {
  min-height: 8rem;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xl) var(--xy-space-md);
  color: var(--xy-text-muted);
  text-align: center;
}

.reported-mods-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(20rem, 100%), 1fr));
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
}

.reported-mod-card {
  min-width: 0;
  padding: var(--xy-space-md);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  background: var(--xy-surface-1);
}

.reported-mod-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.reported-mod-title {
  margin: 0;
  color: var(--xy-text-primary);
  font-size: 0.9rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.reported-mod-name,
.reported-mod-version,
.reported-mod-author {
  font-size: 0.72rem;
}

.reported-mod-name {
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-muted);
  overflow-wrap: anywhere;
}

.reported-mod-version {
  flex-shrink: 0;
  color: var(--xy-text-secondary);
}

.reported-mod-author {
  margin-top: var(--xy-space-sm);
}

.reported-mod-description {
  margin: var(--xy-space-sm) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: 1.5;
  overflow-wrap: anywhere;
}

@media (max-width: 599px) {
  .reported-mods-grid {
    padding: var(--xy-space-sm);
  }

  .reported-mod-card {
    padding: var(--xy-space-sm);
  }
}
</style>
