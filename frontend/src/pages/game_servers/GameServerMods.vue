<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  ListInstalledModsRequestSchema,
  UpdateModRequestSchema,
  UninstallModRequestSchema,
  SetModAutoUpdateRequestSchema,
  SetModEnabledRequestSchema,
  PinModVersionRequestSchema,
  InstallModRequestSchema,
  GetModVersionsRequestSchema,
} from '@/proto/xylona_pb'
import type { InstalledMod, ModVersion } from '@/proto/shared_pb'
import InstalledModsTable from '@/components/game_servers/InstalledModsTable.vue'
import ModBrowse from '@/components/game_servers/ModBrowse.vue'
import ModDetailDialog from '@/components/game_servers/ModDetailDialog.vue'
import ModInstallDialog from '@/components/game_servers/ModInstallDialog.vue'

const $q = useQuasar()
const route = useRoute()
const gameServerId = route.params.id as string

const activeTab = ref<'installed' | 'browse'>('installed')
const loading = ref(true)
const installedMods = ref<InstalledMod[]>([])

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

// Mod sources for the browse tab filter chips.
// Derived from unique sources present in installed mods as a fallback
// until the server software selector (Task 18) provides proper source discovery.
const modSources = computed(() => {
  const seen = new Set<string>()
  const sources: { id: string; searchParams: Record<string, unknown> }[] = []
  for (const mod of installedMods.value) {
    if (!seen.has(mod.source)) {
      seen.add(mod.source)
      sources.push({ id: mod.source, searchParams: {} })
    }
  }
  return sources
})

const detailIsInstalled = computed(() => {
  return installedMods.value.some(
    (mod) => mod.source === detailSource.value && mod.sourceId === detailSourceId.value,
  )
})

onMounted(async () => {
  await loadInstalledMods()
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loading.value = false
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
    $q.notify({
      type: 'xylona-success',
      caption: 'Mod updated successfully',
      position: 'top',
      timeout: 3000,
    })
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
      $q.notify({
        type: 'xylona-success',
        caption: `${modName} uninstalled`,
        position: 'top',
        timeout: 3000,
      })
      await loadInstalledMods()
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: ConnectErrorToString(err),
        position: 'top',
        timeout: 5000,
      })
    }
  })
}

async function handleToggleAutoUpdate(modId: string, enabled: boolean): Promise<void> {
  // Optimistically replace the mod object to trigger Vue reactivity.
  const idx = installedMods.value.findIndex((m) => m.id === modId)
  if (idx !== -1) {
    const updated = Object.assign(Object.create(Object.getPrototypeOf(installedMods.value[idx])), installedMods.value[idx])
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
    $q.notify({
      type: 'xylona-success',
      caption: `Mod ${enabled ? 'enabled' : 'disabled'}`,
      position: 'top',
      timeout: 3000,
    })
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
      $q.notify({
        type: 'xylona-success',
        caption: 'Version unpinned',
        position: 'top',
        timeout: 3000,
      })
      await loadInstalledMods()
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: ConnectErrorToString(err),
        position: 'top',
        timeout: 5000,
      })
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
    $q.notify({
      type: 'xylona-success',
      caption: `Pinned to version ${mod.installedVersion}`,
      position: 'top',
      timeout: 3000,
    })
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
    $q.notify({
      type: 'xylona-success',
      caption: `${successCount} mod${successCount === 1 ? '' : 's'} updated`,
      position: 'top',
      timeout: 3000,
    })
  }
  if (failCount > 0) {
    $q.notify({
      type: 'xylona-error',
      caption: `${failCount} mod${failCount === 1 ? '' : 's'} failed to update`,
      position: 'top',
      timeout: 5000,
    })
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
      $q.notify({
        type: 'xylona-error',
        caption: 'No versions available for this mod',
        position: 'top',
        timeout: 5000,
      })
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
    $q.notify({
      type: 'xylona-success',
      caption: 'Mod installed successfully',
      position: 'top',
      timeout: 3000,
    })
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
        const err = ConnectError.from(depErr)
        $q.notify({
          type: 'xylona-error',
          caption: `Failed to install dependency: ${ConnectErrorToString(err)}`,
          position: 'top',
          timeout: 5000,
        })
      }
    }

    $q.notify({
      type: 'xylona-success',
      caption: 'Mod installed successfully',
      position: 'top',
      timeout: 3000,
    })
    await loadInstalledMods()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    pendingInstall.value = null
  }
}
</script>

<template>
  <div class="mods-page">
    <q-tabs
      v-model="activeTab"
      dense
      class="mods-tabs"
      active-color="primary"
      indicator-color="primary"
      align="left"
      narrow-indicator>
      <q-tab name="installed" label="Installed" />
      <q-tab name="browse" label="Browse" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" animated class="mods-panels">
      <q-tab-panel name="installed" class="mods-panel">
        <installed-mods-table
          :installed-mods="installedMods"
          :loading="loading"
          @update="handleUpdate"
          @uninstall="handleUninstall"
          @toggle-auto-update="handleToggleAutoUpdate"
          @toggle-enabled="handleToggleEnabled"
          @pin-version="handlePinVersion"
          @update-all="handleUpdateAll" />
      </q-tab-panel>

      <q-tab-panel name="browse" class="mods-panel">
        <mod-browse
          :game-server-id="gameServerId"
          :installed-mods="installedMods"
          :sources="modSources"
          @view-details="handleViewDetails"
          @install="handleInstallFromBrowse" />
      </q-tab-panel>
    </q-tab-panels>

    <!-- Detail dialog -->
    <mod-detail-dialog
      v-model:show="showDetailDialog"
      :game-server-id="gameServerId"
      :source="detailSource"
      :source-id="detailSourceId"
      :is-installed="detailIsInstalled"
      @install="handleInstallFromDetail" />

    <!-- Install confirmation dialog -->
    <mod-install-dialog
      v-model:show="showInstallDialog"
      :mod-name="installModName"
      :mod-version="installModVersion"
      :file-size="installFileSize"
      :dependencies="installDeps"
      :installed-mods="installedMods"
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

.mods-tabs {
  background-color: var(--xy-surface-1);
  flex-shrink: 0;
}

.mods-panels {
  flex: 1;
  min-height: 0;
  background-color: transparent;
}

.mods-panel {
  padding: 0;
  display: flex;
  flex-direction: column;
  height: 100%;
}
</style>
