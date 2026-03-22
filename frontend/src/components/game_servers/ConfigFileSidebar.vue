<template>
  <div class="config-sidebar" :class="{ collapsed: isCollapsed && !isMobile }">
    <!-- Expanded view -->
    <div class="sidebar-expanded" :inert="isCollapsed && !isMobile ? true : undefined">
      <div class="sidebar-header">
        <div class="sidebar-title font-display">Config Files</div>
        <q-btn
          v-if="!isMobile"
          flat
          dense
          round
          icon="chevron_left"
          size="sm"
          class="collapse-btn"
          aria-label="Collapse sidebar"
          @click="isCollapsed = true">
          <q-tooltip>Collapse sidebar</q-tooltip>
        </q-btn>
      </div>

      <q-separator class="sidebar-divider" />

      <div class="sidebar-content">
        <div v-if="configFiles.length === 0" class="no-files text-xy-muted q-pa-md">
          No config files defined for this game.
        </div>

        <div v-for="(files, category) in groupedFiles" :key="category" class="category-group">
          <div v-if="categoryCount > 1" class="category-header">
            <span
              class="category-dot"
              :style="{ backgroundColor: getCategoryColor(String(category)) }"></span>
            <span class="category-name">{{ category }}</span>
          </div>

          <q-list dense class="category-files">
            <q-item
              v-for="file in files"
              :key="file.path"
              clickable
              :active="selectedPath === file.path"
              active-class="file-active"
              class="file-item"
              :class="{ 'file-missing': !file.existsOnDisk }"
              @click="$emit('select', file.path, !file.existsOnDisk)">
              <q-item-section side class="file-icon-section">
                <q-icon
                  :name="file.existsOnDisk ? 'description' : 'note_add'"
                  :color="file.existsOnDisk ? undefined : 'warning'"
                  size="xs" />
              </q-item-section>
              <q-item-section>
                <q-item-label class="file-name font-mono">{{
                  getFileName(file.path)
                }}</q-item-label>
                <q-item-label caption class="file-meta">
                  {{ file.format }} &middot; {{ file.fieldCount }} fields
                </q-item-label>
              </q-item-section>
              <q-item-section side>
                <q-badge
                  v-if="!file.existsOnDisk"
                  color="warning"
                  text-color="dark"
                  label="Missing"
                  class="file-badge" />
                <q-badge
                  v-else-if="file.managedFieldCount > 0"
                  outline
                  color="accent"
                  :label="`${file.managedFieldCount} managed`"
                  class="file-badge" />
              </q-item-section>
            </q-item>
          </q-list>
        </div>
      </div>
    </div>

    <!-- Collapsed view (always in DOM for smooth transition) -->
    <div class="sidebar-collapsed" :inert="!isCollapsed || isMobile ? true : undefined">
      <q-btn
        flat
        dense
        round
        icon="chevron_right"
        size="sm"
        class="expand-btn"
        aria-label="Expand sidebar"
        @click="isCollapsed = false">
        <q-tooltip>Expand sidebar</q-tooltip>
      </q-btn>

      <q-separator class="sidebar-divider" />

      <div class="collapsed-files">
        <q-btn
          v-for="file in configFiles"
          :key="file.path"
          flat
          dense
          class="collapsed-file-btn"
          :class="{
            'collapsed-active': selectedPath === file.path,
            'collapsed-missing': !file.existsOnDisk,
          }"
          @click="$emit('select', file.path, !file.existsOnDisk)">
          <span class="collapsed-abbr font-mono">{{ getAbbreviation(file.path) }}</span>
          <q-tooltip>{{ file.path }}</q-tooltip>
        </q-btn>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useQuasar } from 'quasar'
import type { ConfigFileInfo } from '@/proto/xylona_pb'
import { CATEGORY_COLORS, buildCategoryColorMap } from './config-field-helpers'

const $q = useQuasar()
const isMobile = computed(() => $q.screen.lt.md)

const props = defineProps<{
  configFiles: ConfigFileInfo[]
  selectedPath: string
}>()

defineEmits<{
  select: [path: string, isMissing: boolean]
}>()

const isCollapsed = ref(false)

const categoryColorMap = computed(() => buildCategoryColorMap(props.configFiles))

const categoryCount = computed(() => categoryColorMap.value.size)

const groupedFiles = computed(() => {
  const groups: Record<string, ConfigFileInfo[]> = {}
  for (const file of props.configFiles) {
    const cat = file.category || 'Uncategorized'
    if (!groups[cat]) {
      groups[cat] = []
    }
    groups[cat].push(file)
  }
  return groups
})

function getCategoryColor(category: string): string {
  return categoryColorMap.value.get(category) || CATEGORY_COLORS[0]
}

function getFileName(path: string): string {
  const parts = path.split('/')
  return parts[parts.length - 1]
}

function getAbbreviation(path: string): string {
  const name = getFileName(path)
  const dotIndex = name.indexOf('.')
  const base = dotIndex > 0 ? name.substring(0, dotIndex) : name
  return base.substring(0, 2).toUpperCase()
}
</script>

<style scoped>
.config-sidebar {
  background-color: var(--xy-surface-1);
  border-right: 1px solid var(--xy-border);
  height: 100%;
  display: flex;
  flex-direction: column;
  width: 260px;
  overflow: hidden;
  position: relative;
  flex-shrink: 0;
  transition: width 300ms cubic-bezier(0.25, 1, 0.5, 1);
}

.config-sidebar.collapsed {
  width: 48px;
}

.sidebar-expanded {
  width: 260px;
  min-width: 260px;
  display: flex;
  flex-direction: column;
  height: 100%;
  opacity: 1;
  transition: opacity 200ms cubic-bezier(0.25, 1, 0.5, 1);
}

.collapsed .sidebar-expanded {
  opacity: 0;
  pointer-events: none;
  position: absolute;
  inset: 0;
}

.sidebar-collapsed {
  width: 48px;
  min-width: 48px;
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  padding-top: var(--xy-space-sm);
  opacity: 0;
  pointer-events: none;
  position: absolute;
  inset: 0;
  transition: opacity 200ms cubic-bezier(0.25, 1, 0.5, 1) 100ms;
}

.collapsed .sidebar-collapsed {
  opacity: 1;
  pointer-events: auto;
  position: static;
}

.sidebar-header {
  padding: var(--xy-space-sm) var(--xy-space-md);
  position: relative;
}

.sidebar-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  letter-spacing: 0.02em;
}

.collapse-btn {
  position: absolute;
  top: var(--xy-space-xs);
  right: var(--xy-space-xs);
  color: var(--xy-text-muted);
}

.sidebar-divider {
  background-color: var(--xy-border);
}

.sidebar-content {
  flex: 1;
  overflow-y: auto;
  padding: var(--xy-space-xs) 0;
}

.category-group {
  margin-bottom: var(--xy-space-xs);
}

.category-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) var(--xy-space-md);
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.category-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
}

.file-item {
  padding: var(--xy-space-xs) var(--xy-space-md);
  min-height: 44px;
  border-left: 2px solid transparent;
  transition:
    background-color var(--xy-transition-fast),
    border-left-color var(--xy-transition-fast);
}

.file-item:hover {
  background-color: var(--xy-surface-2);
}

.file-active {
  background-color: color-mix(in srgb, var(--xy-primary) 8%, var(--xy-surface-2));
  border-left-color: var(--xy-primary);
}

.file-active .file-name {
  color: var(--xy-primary);
  font-weight: 600;
}

.file-missing {
  opacity: 0.7;
}

.file-icon-section {
  min-width: 24px;
  padding-right: var(--xy-space-xs);
}

.file-name {
  font-size: 0.8rem;
  color: var(--xy-text-primary);
}

.file-meta {
  font-size: 0.75rem;
  color: var(--xy-text-muted);
}

.file-badge {
  font-size: 0.75rem;
}

.no-files {
  font-size: 0.8rem;
  text-align: center;
}

/* Collapsed state */
.expand-btn {
  color: var(--xy-text-muted);
  margin-bottom: var(--xy-space-xs);
}

.collapsed-files {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) 0;
}

.collapsed-file-btn {
  width: 44px;
  height: 44px;
  min-height: 44px;
  border-radius: 6px;
  color: var(--xy-text-secondary);
  border: 1px solid transparent;
  transition:
    background-color var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.collapsed-file-btn:hover {
  background-color: var(--xy-surface-2);
}

.collapsed-active {
  background-color: color-mix(in srgb, var(--xy-primary) 12%, var(--xy-surface-2));
  border-color: var(--xy-primary);
  color: var(--xy-primary);
}

.collapsed-missing {
  opacity: 0.5;
}

.collapsed-abbr {
  font-size: 0.75rem;
  font-weight: 600;
}

/* Reduced motion */
@media (prefers-reduced-motion: reduce) {
  .config-sidebar,
  .sidebar-expanded,
  .sidebar-collapsed {
    transition-duration: 0.01ms !important;
  }
}

/* Mobile: horizontal scrollable file strip */
@media (max-width: 767px) {
  .config-sidebar {
    border-right: none;
    border-bottom: 1px solid var(--xy-border);
    height: auto;
    width: 100%;
    transition: none;
  }

  .sidebar-expanded {
    width: 100%;
    min-width: unset;
    opacity: 1;
    position: static;
    pointer-events: auto;
  }

  .sidebar-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--xy-space-xs) var(--xy-space-md);
  }

  .collapse-btn {
    position: static;
  }

  .sidebar-content {
    overflow-x: auto;
    overflow-y: hidden;
    padding: 0 var(--xy-space-xs) var(--xy-space-xs);
  }

  .category-group {
    margin-bottom: 0;
  }

  .category-header {
    display: none;
  }

  .category-files {
    display: flex;
    flex-direction: row;
    gap: var(--xy-space-xs);
  }

  .file-item {
    flex-shrink: 0;
    min-width: 140px;
    max-width: 200px;
    border-left: none;
    border-bottom: 2px solid transparent;
    border-radius: 6px;
    background-color: var(--xy-surface-0);
  }

  .file-active {
    border-left-color: transparent;
    border-bottom-color: var(--xy-primary);
    background-color: var(--xy-surface-2);
  }

  /* Hide collapsed state on mobile — always show expanded strip */
  .sidebar-collapsed {
    display: none;
  }

  .config-sidebar.collapsed .sidebar-expanded {
    display: flex;
    opacity: 1;
    position: static;
    pointer-events: auto;
  }
}
</style>
