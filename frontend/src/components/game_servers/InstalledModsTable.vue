<template>
  <div class="installed-mods-table">
    <!-- Toolbar -->
    <div class="mods-toolbar">
      <q-input
        v-model="filterText"
        dense
        outlined
        placeholder="Filter mods..."
        aria-label="Filter installed mods"
        class="mods-filter-input"
        clearable
        @clear="filterText = ''">
        <template #prepend>
          <q-icon name="search" size="xs" class="text-xy-muted" aria-hidden="true" />
        </template>
      </q-input>
      <q-btn
        v-if="updatesAvailable > 0"
        color="primary"
        label="Update All"
        icon="system_update_alt"
        size="sm"
        no-caps
        @click="emit('update-all')" />
    </div>

    <!-- Loading state -->
    <div v-if="loading" class="mods-loading">
      <q-spinner color="primary" size="2rem" />
      <span class="text-xy-muted">Loading mods...</span>
    </div>

    <!-- Empty state -->
    <div v-else-if="installedMods.length === 0" class="mods-empty">
      <q-icon name="extension_off" size="3rem" class="text-xy-muted" aria-hidden="true" />
      <div class="mods-empty-title text-xy-secondary">No mods installed</div>
      <div class="mods-empty-subtitle text-xy-muted">Browse available mods to get started.</div>
    </div>

    <!-- No filter results -->
    <div v-else-if="filteredMods.length === 0" class="mods-empty">
      <div class="mods-empty-title text-xy-secondary">No mods match "{{ filterText }}"</div>
    </div>

    <!-- Mod table -->
    <div v-else class="mods-table-scroll">
      <table class="mods-table">
        <thead>
          <tr>
            <th class="col-mod">Mod</th>
            <th class="col-source">Source</th>
            <th class="col-version">Version</th>
            <th class="col-status">Status</th>
            <th class="col-auto">Auto</th>
            <th class="col-actions">
              <span class="sr-only">Actions</span>
            </th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="mod in filteredMods"
            :key="mod.id"
            class="mod-row"
            :class="{
              'mod-row--update': mod.updateAvailable,
              'mod-row--disabled': !mod.enabled,
            }">
            <!-- Mod name + author -->
            <td class="col-mod">
              <div class="mod-identity">
                <div
                  class="mod-icon"
                  :style="{ background: iconGradient(mod.modName) }"
                  aria-hidden="true">
                  {{ mod.modName.charAt(0).toUpperCase() }}
                </div>
                <div class="mod-info">
                  <span class="mod-name">{{ mod.modName }}</span>
                  <span class="mod-author text-xy-muted">{{ mod.modAuthor }}</span>
                </div>
              </div>
            </td>

            <!-- Source badge -->
            <td class="col-source">
              <span class="source-badge" :style="sourceBadgeStyle(mod.source)">
                {{ sourceLabel(mod.source) }}
              </span>
              <span class="source-name text-xy-muted">{{ sourceDisplayName(mod.source) }}</span>
            </td>

            <!-- Version -->
            <td class="col-version">
              <span
                class="version-text font-mono"
                :class="{ 'version-text--update': mod.updateAvailable }">
                {{ mod.installedVersion }}
              </span>
            </td>

            <!-- Status -->
            <td class="col-status">
              <span v-if="!mod.enabled" class="status-disabled text-xy-muted">
                <q-icon name="block" size="xs" aria-hidden="true" />
                Disabled
              </span>
              <q-btn
                v-else-if="mod.updateAvailable"
                color="primary"
                size="sm"
                dense
                no-caps
                :label="`Update to ${mod.latestVersion}`"
                icon="upgrade"
                @click="emit('update', mod.id)" />
              <span v-else class="status-up-to-date">
                <q-icon name="check_circle" size="xs" color="positive" aria-hidden="true" />
                Up to date
              </span>
            </td>

            <!-- Auto-update toggle -->
            <td class="col-auto">
              <label class="q-toggle auto-update-toggle" role="switch" :aria-checked="Boolean(mod.autoUpdate)">
                <input
                  type="checkbox"
                  :checked="Boolean(mod.autoUpdate)"
                  :aria-label="`Auto-update ${mod.modName}`"
                  @change="emit('toggle-auto-update', mod.id, !mod.autoUpdate)" />
                <span class="toggle-track" :class="{ 'toggle-track--active': mod.autoUpdate }">
                  <span class="toggle-thumb" />
                </span>
              </label>
            </td>

            <!-- Actions overflow menu -->
            <td class="col-actions">
              <q-btn
                flat
                dense
                round
                icon="more_vert"
                size="sm"
                :aria-label="`Actions for ${mod.modName}`">
                <q-menu>
                  <q-list dense style="min-width: 160px">
                    <q-item
                      v-close-popup
                      clickable
                      @click="emit('toggle-enabled', mod.id, !mod.enabled)">
                      <q-item-section side>
                        <q-icon :name="mod.enabled ? 'visibility_off' : 'visibility'" size="xs" />
                      </q-item-section>
                      <q-item-section>
                        {{ mod.enabled ? 'Disable' : 'Enable' }}
                      </q-item-section>
                    </q-item>
                    <q-item v-close-popup clickable @click="emit('pin-version', mod.id)">
                      <q-item-section side>
                        <q-icon name="push_pin" size="xs" />
                      </q-item-section>
                      <q-item-section>Pin Version</q-item-section>
                    </q-item>
                    <q-separator />
                    <q-item
                      v-close-popup
                      clickable
                      class="text-negative"
                      @click="emit('uninstall', mod.id)">
                      <q-item-section side>
                        <q-icon name="delete" size="xs" color="negative" />
                      </q-item-section>
                      <q-item-section>Uninstall</q-item-section>
                    </q-item>
                  </q-list>
                </q-menu>
              </q-btn>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { InstalledMod } from '@/proto/shared_pb'

interface Props {
  installedMods: InstalledMod[]
  loading: boolean
}

const props = defineProps<Props>()

const emit = defineEmits<{
  update: [modId: string]
  uninstall: [modId: string]
  'toggle-auto-update': [modId: string, enabled: boolean]
  'toggle-enabled': [modId: string, enabled: boolean]
  'pin-version': [modId: string]
  'update-all': []
}>()

const filterText = ref('')

const filteredMods = computed((): InstalledMod[] => {
  if (!filterText.value) return props.installedMods
  const query = filterText.value.toLowerCase()
  return props.installedMods.filter(
    (mod) =>
      mod.modName.toLowerCase().includes(query) || mod.modAuthor.toLowerCase().includes(query),
  )
})

const updatesAvailable = computed((): number => {
  return props.installedMods.filter((mod) => mod.updateAvailable && mod.enabled).length
})

const SOURCE_BADGES: Record<string, { bg: string; letter: string; name: string }> = {
  modrinth: { bg: '#1BD96A', letter: 'M', name: 'Modrinth' },
  hangar: { bg: '#2196F3', letter: 'H', name: 'Hangar' },
  thunderstore: { bg: '#0066FF', letter: 'T', name: 'Thunderstore' },
  steam_workshop: { bg: '#1B2838', letter: 'S', name: 'Steam Workshop' },
  papermc: { bg: '#2196F3', letter: 'P', name: 'PaperMC' },
}

function sourceBadgeStyle(source: string): Record<string, string> {
  const config = SOURCE_BADGES[source]
  if (!config) return { backgroundColor: 'var(--xy-surface-3)', color: 'var(--xy-text-primary)' }
  return { backgroundColor: config.bg, color: '#FFFFFF' }
}

function sourceLabel(source: string): string {
  return SOURCE_BADGES[source]?.letter ?? source.charAt(0).toUpperCase()
}

function sourceDisplayName(source: string): string {
  return SOURCE_BADGES[source]?.name ?? source
}

function iconGradient(name: string): string {
  // Simple hash-based gradient from the mod name
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  const hue1 = Math.abs(hash) % 360
  const hue2 = (hue1 + 40) % 360
  return `linear-gradient(135deg, hsl(${hue1}, 60%, 40%), hsl(${hue2}, 60%, 30%))`
}
</script>

<style scoped>
.installed-mods-table {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

/* ---- Toolbar ---- */
.mods-toolbar {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background-color: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  flex-shrink: 0;
}

.mods-filter-input {
  flex: 1;
  max-width: 320px;
}

/* ---- Loading / Empty states ---- */
.mods-loading,
.mods-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-2xl) var(--xy-space-md);
}

.mods-empty-title {
  font-size: 0.9rem;
  font-weight: 500;
}

.mods-empty-subtitle {
  font-size: 0.8rem;
}

/* ---- Table scroll ---- */
.mods-table-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background-color: var(--xy-base);
}

.mods-table-scroll::-webkit-scrollbar {
  width: 6px;
}

.mods-table-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.mods-table-scroll::-webkit-scrollbar-thumb {
  background: var(--xy-surface-4);
  border-radius: 3px;
}

/* ---- Table ---- */
.mods-table {
  width: 100%;
  border-collapse: collapse;
}

.mods-table thead th {
  position: sticky;
  top: 0;
  z-index: 5;
  background-color: var(--xy-surface-1);
  padding: 0.5rem 1rem;
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  text-align: left;
  border-bottom: 1px solid var(--xy-border);
  white-space: nowrap;
}

.mod-row {
  transition: background-color var(--xy-transition-fast);
}

.mod-row:nth-child(even) {
  background-color: color-mix(in srgb, var(--xy-surface-0) 50%, transparent);
}

.mod-row:hover {
  background-color: var(--xy-surface-1);
}

.mod-row td {
  padding: 0.45rem 1rem;
  border-bottom: 1px solid var(--xy-border);
  vertical-align: middle;
}

/* Update-available row: left amber border */
.mod-row--update {
  border-left: 2px solid var(--xy-warning);
}

.mod-row--update td:first-child {
  padding-left: calc(1rem - 2px);
}

/* Disabled row: reduced opacity */
.mod-row--disabled {
  opacity: 0.5;
}

/* ---- Column widths ---- */
.col-mod {
  min-width: 200px;
}

.col-source {
  width: 130px;
  white-space: nowrap;
}

.col-version {
  width: 120px;
}

.col-status {
  width: 180px;
  white-space: nowrap;
}

.col-auto {
  width: 60px;
  text-align: center;
}

.col-actions {
  width: 48px;
  text-align: center;
}

/* ---- Mod identity (icon + name) ---- */
.mod-identity {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mod-icon {
  width: 32px;
  height: 32px;
  border-radius: 6px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.85rem;
  font-weight: 700;
  color: #fff;
  flex-shrink: 0;
}

.mod-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.mod-name {
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--xy-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.mod-author {
  font-size: 0.7rem;
}

/* ---- Source badge ---- */
.source-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 4px;
  font-size: 0.65rem;
  font-weight: 700;
  flex-shrink: 0;
  vertical-align: middle;
}

.source-name {
  font-size: 0.75rem;
  margin-left: var(--xy-space-xs);
  vertical-align: middle;
}

/* ---- Version ---- */
.version-text {
  font-size: 0.8rem;
  color: var(--xy-text-primary);
}

.version-text--update {
  color: var(--xy-warning);
  font-weight: 600;
}

/* ---- Status ---- */
.status-up-to-date {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  font-size: 0.8rem;
  color: var(--xy-success);
}

.status-disabled {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  font-size: 0.8rem;
}

/* ---- Screen reader only ---- */
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

/* ---- Mobile ---- */
@media (max-width: 767px) {
  .mods-toolbar {
    flex-wrap: wrap;
    padding: var(--xy-space-xs) var(--xy-space-sm);
  }

  .mods-filter-input {
    max-width: none;
    flex: 1 1 100%;
  }

  .col-source,
  .col-auto {
    display: none;
  }
}

/* ---- Custom auto-update toggle ---- */
.auto-update-toggle {
  display: inline-flex;
  align-items: center;
  cursor: pointer;
  position: relative;
}

.auto-update-toggle input[type='checkbox'] {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-track {
  display: inline-block;
  width: 36px;
  height: 14px;
  border-radius: 7px;
  background-color: var(--xy-surface-4);
  position: relative;
  transition: background-color 0.2s ease;
}

.toggle-track--active {
  background-color: color-mix(in srgb, var(--q-primary) 50%, transparent);
}

.toggle-thumb {
  position: absolute;
  top: -3px;
  left: 0;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  background-color: var(--xy-text-muted);
  transition:
    left 0.2s ease,
    background-color 0.2s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.3);
}

.toggle-track--active .toggle-thumb {
  left: 16px;
  background-color: var(--q-primary);
}
</style>
