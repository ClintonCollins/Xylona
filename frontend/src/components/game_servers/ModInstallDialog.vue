<template>
  <q-dialog
    :model-value="show"
    persistent
    @update:model-value="(val: boolean) => emit('update:show', val)">
    <q-card class="mod-install-dialog">
      <q-card-section class="mod-install-header">
        <div class="mod-install-title">
          <q-icon aria-hidden="true" color="primary" name="download" size="sm" />
          <h3>Install Mod</h3>
        </div>
      </q-card-section>

      <q-separator />

      <q-card-section class="mod-install-body">
        <!-- Mod info summary -->
        <div class="mod-install-summary">
          <div class="mod-install-detail">
            <span class="mod-install-label text-xy-muted">Mod</span>
            <span class="mod-install-value">{{ modName }}</span>
          </div>
          <div class="mod-install-detail">
            <span class="mod-install-label text-xy-muted">Version</span>
            <span class="mod-install-value font-mono">{{ modVersion }}</span>
          </div>
          <div v-if="fileSize > 0" class="mod-install-detail">
            <span class="mod-install-label text-xy-muted">Size</span>
            <span class="mod-install-value font-mono">{{ formatBytes(fileSize) }}</span>
          </div>
        </div>

        <!-- Dependencies -->
        <div v-if="dependencies.length > 0" class="mod-install-deps">
          <div class="mod-install-deps-title text-xy-muted">
            Dependencies ({{ dependencies.length }})
          </div>

          <div class="mod-install-dep-list">
            <label
              v-for="dep in depItems"
              :key="dep.sourceId"
              :class="{
                'dep-installed': dep.isInstalled,
                'dep-required': dep.required && !dep.isInstalled,
              }"
              class="mod-install-dep-row">
              <q-checkbox
                v-model="dep.selected"
                :aria-label="`Install dependency ${dep.name}`"
                :disable="dep.isInstalled"
                color="primary"
                dense />

              <div class="dep-info">
                <span class="dep-name">{{ dep.name || dep.sourceId }}</span>
                <span v-if="dep.required" class="dep-tag dep-tag--required">required</span>
                <span v-else class="dep-tag dep-tag--optional">optional</span>
              </div>

              <span v-if="dep.isInstalled" class="dep-installed-badge">
                <q-icon aria-hidden="true" color="positive" name="check_circle" size="xs" />
                Installed
              </span>
            </label>
          </div>
        </div>
      </q-card-section>

      <q-separator />

      <q-card-actions align="right" class="mod-install-actions">
        <q-btn flat label="Cancel" no-caps @click="emit('update:show', false)" />
        <q-btn color="primary" icon="download" label="Install" no-caps @click="handleConfirm" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { ref, watch } from 'vue'
import type { InstalledMod, ModDependency } from '@/proto/shared_pb'

interface Props {
  show: boolean
  modName: string
  modVersion: string
  fileSize: number
  dependencies: ModDependency[]
  installedMods: InstalledMod[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  confirm: [selectedDeps: string[]]
}>()

interface DepItem {
  sourceId: string
  name: string
  required: boolean
  isInstalled: boolean
  selected: boolean
}

const depItems = ref<DepItem[]>([])

watch(
  () => props.show,
  (isOpen) => {
    if (isOpen) {
      depItems.value = props.dependencies.map((dep): DepItem => {
        const isInstalled = props.installedMods.some((mod) => mod.sourceId === dep.sourceId)
        return {
          sourceId: dep.sourceId,
          name: dep.name,
          required: dep.required,
          isInstalled,
          selected: isInstalled ? false : dep.required,
        }
      })
    }
  },
)

function handleConfirm(): void {
  const selectedDeps = depItems.value
    .filter((dep) => dep.selected && !dep.isInstalled)
    .map((dep) => dep.sourceId)
  emit('confirm', selectedDeps)
}

function formatBytes(bytes: number): string {
  const sizes = ['B', 'KB', 'MB', 'GB']
  if (bytes === 0) return '0 B'
  const i = Math.floor(Math.log(bytes) / Math.log(1024))
  return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${sizes[i]}`
}
</script>

<style scoped>
.mod-install-dialog {
  background-color: var(--xy-surface-0);
  width: min(520px, calc(100vw - 2rem));
  max-height: calc(100vh - 2rem);
}

.mod-install-header {
  display: flex;
  align-items: center;
}

.mod-install-title {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mod-install-title h3 {
  margin: 0;
  font-size: 1rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.mod-install-body {
  padding: var(--xy-space-md) var(--xy-space-lg);
}

/* ---- Summary ---- */
.mod-install-summary {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  margin-bottom: var(--xy-space-md);
}

.mod-install-detail {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mod-install-label {
  font-size: 0.75rem;
  min-width: 60px;
  flex-shrink: 0;
}

.mod-install-value {
  font-size: 0.85rem;
  color: var(--xy-text-primary);
}

/* ---- Dependencies ---- */
.mod-install-deps {
  padding-top: var(--xy-space-sm);
  border-top: 1px solid var(--xy-border);
}

.mod-install-deps-title {
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  margin-bottom: var(--xy-space-sm);
}

.mod-install-dep-list {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.mod-install-dep-row {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border: 1px solid transparent;
  border-radius: var(--xy-radius-sm);
  cursor: pointer;
  transition: background-color var(--xy-transition-fast);
}

.mod-install-dep-row:hover {
  background-color: var(--xy-surface-1);
}

.dep-installed {
  opacity: 0.6;
  cursor: default;
}

.dep-required {
  background-color: var(--xy-warning-bg-faint);
  border-color: var(--xy-warning-border);
}

.dep-info {
  flex: 1;
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.dep-name {
  font-size: 0.8rem;
  color: var(--xy-text-primary);
  overflow-wrap: anywhere;
}

.dep-tag {
  font-size: 0.6rem;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding: 1px 5px;
  border-radius: 3px;
}

.dep-tag--required {
  background-color: color-mix(in srgb, var(--xy-warning) 20%, transparent);
  color: var(--xy-warning);
}

.dep-tag--optional {
  background-color: var(--xy-surface-2);
  color: var(--xy-text-muted);
}

.dep-installed-badge {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 0.7rem;
  color: var(--xy-success);
  font-weight: 500;
  flex-shrink: 0;
}

/* ---- Actions ---- */
.mod-install-actions {
  padding: var(--xy-space-sm) var(--xy-space-lg);
}

/* ---- Mobile ---- */
@media (max-width: 500px) {
  .mod-install-dialog {
    width: calc(100vw - 1rem);
    max-height: calc(100vh - 1rem);
  }

  .mod-install-body {
    padding: var(--xy-space-md);
  }

  .mod-install-detail,
  .mod-install-dep-row,
  .dep-info {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .mod-install-actions {
    flex-wrap: wrap;
    padding: var(--xy-space-sm) var(--xy-space-md);
  }

  .mod-install-actions .q-btn {
    flex: 1 1 8rem;
  }
}
</style>
