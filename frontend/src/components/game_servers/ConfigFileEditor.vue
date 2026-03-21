<template>
  <div class="config-editor">
    <!-- Sticky header -->
    <div class="editor-header-sticky">
      <div class="editor-header">
        <div class="editor-header-info">
          <div class="editor-file-name font-mono">{{ filePath }}</div>
          <div class="editor-meta">
            <q-badge
              v-if="category"
              outline
              :style="{ borderColor: categoryColor, color: categoryColor }"
              :label="category"
              class="editor-category-badge" />
            <span class="text-xy-muted editor-meta-text">{{ format }}</span>
            <span v-if="editedValues.size > 0" class="editor-modified-count">
              {{ editedValues.size }} modified
            </span>
          </div>
        </div>
        <div class="editor-header-actions">
          <q-btn
            v-if="isMissing"
            outline
            color="warning"
            label="Generate File"
            icon="note_add"
            size="sm"
            :loading="generating"
            @click="$emit('generate')" />
          <q-btn
            v-else
            color="primary"
            label="Save"
            icon="save"
            size="sm"
            :loading="saving"
            :disable="!hasChanges"
            @click="handleSave" />
        </div>
      </div>
      <q-separator class="editor-divider" />

      <!-- Scrollspy group tabs -->
      <div v-if="allGroups.length > 1" class="group-tabs" role="tablist">
        <button
          v-for="group in allGroups"
          :key="group.name"
          role="tab"
          :aria-selected="activeGroup === group.name"
          class="group-tab"
          :class="{
            active: activeGroup === group.name,
            dimmed: searchQuery && !filteredGroupCounts.has(group.name),
          }"
          @click="scrollToGroup(group.name)">
          {{ group.displayName }}
          <span class="tab-count">{{
            searchQuery
              ? (filteredGroupCounts.get(group.name) ?? 0)
              : group.fields.length
          }}</span>
        </button>
      </div>

      <!-- Validation errors -->
      <q-banner
        v-if="validationErrors.length > 0"
        role="alert"
        dense
        class="validation-banner">
        <template #avatar>
          <q-icon name="error_outline" color="negative" size="sm" aria-hidden="true" />
        </template>
        <div class="validation-errors">
          <div v-for="(error, i) in validationErrors" :key="i" class="validation-error-item">
            <strong>{{ error.field }}:</strong> {{ error.message }}
          </div>
        </div>
      </q-banner>

      <!-- Search controls -->
      <div v-if="fields.length > 0" class="editor-controls">
        <q-input
          v-model="searchQuery"
          dense
          outlined
          placeholder="Search settings..."
          aria-label="Search configuration fields"
          class="search-input"
          clearable
          @clear="searchQuery = ''">
          <template #prepend>
            <q-icon name="search" size="xs" class="text-xy-muted" aria-hidden="true" />
          </template>
        </q-input>
      </div>
    </div>

    <!-- Compact settings table -->
    <div ref="tableScrollRef" class="table-scroll">
      <div
        v-if="fields.length === 0 && advancedFields.length === 0"
        class="no-fields text-xy-muted">
        No fields defined in the schema for this file.
      </div>

      <div
        v-else-if="filteredFields.length === 0 && searchQuery"
        class="no-fields text-xy-muted">
        No fields match "{{ searchQuery }}"
      </div>

      <table v-else class="settings-table">
        <template v-for="group in displayGroups" :key="group.name">
          <!-- Sentinel for IntersectionObserver (invisible) -->
          <tr class="group-sentinel-row">
            <td colspan="2">
              <div
                :id="`sentinel-${group.name}`"
                :data-group="group.name"
                class="group-sentinel" />
            </td>
          </tr>
          <!-- Sticky group header -->
          <tr class="group-header-row">
            <td colspan="2">
              <span class="group-header-title">{{ group.displayName }}</span>
              <span class="group-header-count">{{ group.fields.length }}</span>
            </td>
          </tr>
          <!-- Field rows -->
          <tr
            v-for="field in group.fields"
            :key="field.key"
            class="setting-row"
            :class="{ 'setting-edited': editedValues.has(field.key) }">
            <!-- Setting name -->
            <td class="setting-key">
              {{ field.title || field.key }}
              <q-badge
                v-if="field.isManaged"
                color="accent"
                label="Managed"
                class="managed-badge" />
              <span v-if="field.required && !field.isManaged" class="field-required" aria-label="required">*</span>
            </td>
            <!-- Setting value -->
            <td class="setting-value">
              <!-- Managed: read-only -->
              <span v-if="field.isManaged" class="managed-value font-mono">
                {{ field.value || field.defaultValue }}
              </span>

              <!-- Boolean toggle -->
              <div v-else-if="field.fieldType === 'boolean'" class="inline-toggle">
                <q-toggle
                  :id="fieldId(field.key)"
                  :model-value="getFieldValue(field) === 'true'"
                  dense
                  color="primary"
                  :aria-label="field.title || field.key"
                  @update:model-value="(val: boolean) => setFieldValue(field.key, String(val))" />
                <span class="toggle-label">{{
                  getFieldValue(field) === 'true' ? 'Enabled' : 'Disabled'
                }}</span>
              </div>

              <!-- Enum dropdown -->
              <q-select
                v-else-if="field.enumOptions.length > 0"
                :id="fieldId(field.key)"
                :model-value="getFieldValue(field)"
                :options="field.enumOptions"
                :aria-label="field.title || field.key"
                dense
                outlined
                emit-value
                map-options
                class="inline-input"
                @update:model-value="(val: string) => setFieldValue(field.key, val)" />

              <!-- Number input -->
              <q-input
                v-else-if="field.fieldType === 'integer' || field.fieldType === 'number'"
                :id="fieldId(field.key)"
                :model-value="getFieldValue(field)"
                :aria-label="field.title || field.key"
                dense
                outlined
                type="number"
                :rules="getNumberRules(field)"
                class="inline-input"
                input-class="font-mono"
                @update:model-value="
                  (val: string | number | null) => setFieldValue(field.key, String(val ?? ''))
                " />

              <!-- String input (default) -->
              <q-input
                v-else
                :id="fieldId(field.key)"
                :model-value="getFieldValue(field)"
                :aria-label="field.title || field.key"
                dense
                outlined
                :maxlength="field.maxLength ?? undefined"
                :rules="getStringRules(field)"
                class="inline-input"
                input-class="font-mono"
                @update:model-value="
                  (val: string | number | null) => setFieldValue(field.key, String(val ?? ''))
                " />
            </td>
          </tr>
        </template>
      </table>

      <!-- Advanced fields (below table) -->
      <config-advanced-fields :fields="advancedFields" @update="handleAdvancedUpdate" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import type { ConfigFieldData, ConfigValidationError, AdvancedField } from '@/proto/xylona_pb'
import ConfigAdvancedFields from './ConfigAdvancedFields.vue'
import { groupFields, filterFields } from './config-field-helpers'

const props = defineProps<{
  filePath: string
  format: string
  category: string
  categoryColor: string
  fields: ConfigFieldData[]
  advancedFields: AdvancedField[]
  validationErrors: ConfigValidationError[]
  isMissing: boolean
  saving: boolean
  generating: boolean
}>()

const emit = defineEmits<{
  save: [fieldValues: Map<string, string>]
  generate: []
  updateAdvanced: [fields: AdvancedField[]]
}>()

// Track local edits as key → value overrides
const editedValues = reactive(new Map<string, string>())
const advancedChanged = ref(false)

// Ctrl+S / Cmd+S keyboard shortcut
function onKeyDown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (hasChanges.value && !props.saving) {
      handleSave()
    }
  }
}

// Search state
const searchQuery = ref('')

const hasChanges = computed(() => editedValues.size > 0 || advancedChanged.value)

defineExpose({ hasChanges })

const filteredFields = computed(() => {
  return filterFields([...props.fields], searchQuery.value)
})

const displayGroups = computed(() => {
  return groupFields([...filteredFields.value])
})

const allGroups = computed(() => {
  return groupFields([...props.fields])
})

const activeGroup = ref('')

// Reset activeGroup when the groups change (e.g., file switch)
watch(
  allGroups,
  (groups) => {
    if (groups.length > 0) {
      activeGroup.value = groups[0].name
    }
  },
  { immediate: true },
)

const filteredGroupCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const group of displayGroups.value) {
    counts.set(group.name, group.fields.length)
  }
  return counts
})

// Scrollspy
const tableScrollRef = ref<HTMLElement | null>(null)
let scrollSpySuppressed = false
let scrollRafId = 0

function scrollToGroup(groupName: string) {
  const scrollRoot = tableScrollRef.value
  const sentinel = document.getElementById(`sentinel-${groupName}`)
  if (!scrollRoot || !sentinel) return

  // Set active immediately and suppress scrollspy during smooth scroll
  activeGroup.value = groupName
  scrollSpySuppressed = true

  const containerTop = scrollRoot.getBoundingClientRect().top
  const sentinelTop = sentinel.getBoundingClientRect().top
  const offset = sentinelTop - containerTop + scrollRoot.scrollTop

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  scrollRoot.scrollTo({ top: offset, behavior: reducedMotion ? 'instant' : 'smooth' })

  // Re-enable scrollspy after scroll settles
  if (reducedMotion) {
    scrollSpySuppressed = false
  } else {
    // Wait for smooth scroll to finish (~400ms typical)
    setTimeout(() => {
      scrollSpySuppressed = false
    }, 500)
  }
}

function updateActiveGroupFromScroll() {
  if (scrollSpySuppressed) return

  // Use rAF to avoid thrashing during fast scrolling
  cancelAnimationFrame(scrollRafId)
  scrollRafId = requestAnimationFrame(() => {
    const scrollRoot = tableScrollRef.value
    if (!scrollRoot) return

    const containerTop = scrollRoot.getBoundingClientRect().top
    let currentGroup = ''

    const sentinels = scrollRoot.querySelectorAll('.group-sentinel')
    for (const sentinel of sentinels) {
      const sentinelTop = sentinel.getBoundingClientRect().top
      if (sentinelTop <= containerTop + 10) {
        const groupName = sentinel.getAttribute('data-group')
        if (groupName) {
          currentGroup = groupName
        }
      }
    }

    if (currentGroup) {
      activeGroup.value = currentGroup
    }
  })
}

function setupScrollspy() {
  const scrollRoot = tableScrollRef.value
  if (!scrollRoot) return

  scrollRoot.addEventListener('scroll', updateActiveGroupFromScroll, { passive: true })
}


// Reset edits when file changes
watch(
  () => props.filePath,
  () => {
    editedValues.clear()
    advancedChanged.value = false
    searchQuery.value = ''
    // Reset scroll position
    if (tableScrollRef.value) {
      tableScrollRef.value.scrollTop = 0
    }
  },
)

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  nextTick(() => setupScrollspy())
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
  if (tableScrollRef.value) {
    tableScrollRef.value.removeEventListener('scroll', updateActiveGroupFromScroll)
  }
})

function getFieldValue(field: ConfigFieldData): string {
  if (editedValues.has(field.key)) {
    return editedValues.get(field.key)!
  }
  return field.value || field.defaultValue
}

function setFieldValue(key: string, value: string) {
  const original = props.fields.find((f) => f.key === key)
  const originalValue = original ? original.value || original.defaultValue : ''
  if (value === originalValue) {
    editedValues.delete(key)
  } else {
    editedValues.set(key, value)
  }
}

function getNumberRules(field: ConfigFieldData): ((val: string) => true | string)[] {
  const rules: ((val: string) => true | string)[] = []
  if (field.required) {
    rules.push((val: string) => (val !== '' && val !== undefined && val !== null) || 'Required')
  }
  if (field.minimum !== undefined) {
    const min = Number(field.minimum)
    rules.push((val: string) => Number(val) >= min || `Minimum: ${min}`)
  }
  if (field.maximum !== undefined) {
    const max = Number(field.maximum)
    rules.push((val: string) => Number(val) <= max || `Maximum: ${max}`)
  }
  return rules
}

function getStringRules(field: ConfigFieldData): ((val: string) => true | string)[] {
  const rules: ((val: string) => true | string)[] = []
  if (field.required) {
    rules.push((val: string) => (val !== '' && val !== undefined && val !== null) || 'Required')
  }
  if (field.maxLength) {
    const max = field.maxLength
    rules.push((val: string) => !val || val.length <= max || `Maximum ${max} characters`)
  }
  return rules
}

function fieldId(key: string): string {
  return `cfg-${key.replace(/[^a-zA-Z0-9-_]/g, '-')}`
}

function handleAdvancedUpdate(fields: AdvancedField[]) {
  advancedChanged.value = true
  emit('updateAdvanced', fields)
}

function handleSave() {
  const fieldValues = new Map<string, string>()
  for (const field of props.fields) {
    if (field.isManaged) continue
    fieldValues.set(field.key, getFieldValue(field))
  }
  emit('save', fieldValues)
  editedValues.clear()
  advancedChanged.value = false
}
</script>

<style scoped>
.config-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

/* ---- Fixed header area (flex-shrink: 0 keeps it in place while table scrolls) ---- */
.editor-header-sticky {
  flex-shrink: 0;
  background-color: var(--xy-surface-1);
}

.editor-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: var(--xy-space-sm) var(--xy-space-md);
  gap: var(--xy-space-md);
}

.editor-file-name {
  font-size: 0.9rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.editor-meta {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-xs);
}

.editor-meta-text {
  font-size: 0.75rem;
}

.editor-category-badge {
  font-size: 0.75rem;
}

.editor-modified-count {
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--xy-warning);
}

.editor-divider {
  background-color: var(--xy-border);
}

/* ---- Scrollspy group tabs ---- */
.group-tabs {
  display: flex;
  overflow-x: auto;
  scrollbar-width: none;
  border-bottom: 1px solid var(--xy-border);
}

.group-tabs::-webkit-scrollbar {
  display: none;
}

.group-tab {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.55rem 1.25rem;
  font-family: var(--xy-font-body);
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--xy-text-muted);
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  transition:
    color var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
}

.group-tab:hover {
  color: var(--xy-text-secondary);
  background-color: var(--xy-surface-2);
}

.group-tab.active {
  color: var(--xy-text-primary);
  border-bottom-color: var(--xy-primary);
}

.group-tab.dimmed {
  opacity: 0.4;
  cursor: default;
}

.group-tab.dimmed:hover {
  background-color: transparent;
  color: var(--xy-text-muted);
}

.tab-count {
  font-size: 0.65rem;
  font-weight: 600;
  background: var(--xy-surface-3);
  color: var(--xy-text-muted);
  padding: 0.1rem 0.4rem;
  border-radius: 8px;
  min-width: 1.3rem;
  text-align: center;
}

.group-tab.active .tab-count {
  background: var(--xy-primary-muted);
  color: var(--xy-primary);
}

/* ---- Validation banner ---- */
.validation-banner {
  margin: var(--xy-space-sm) var(--xy-space-md) 0;
  background-color: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: 6px;
  font-size: 0.75rem;
}

.validation-error-item {
  color: var(--xy-text-secondary);
  padding: 2px 0;
}

/* ---- Search controls ---- */
.editor-controls {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm) var(--xy-space-md);
}

.search-input {
  flex: 1;
  max-width: 400px;
}

/* ---- Table scroll container (darker bg for contrast against header) ---- */
.table-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background-color: var(--xy-base);
}

.table-scroll::-webkit-scrollbar {
  width: 6px;
}

.table-scroll::-webkit-scrollbar-track {
  background: transparent;
}

.table-scroll::-webkit-scrollbar-thumb {
  background: var(--xy-surface-4);
  border-radius: 3px;
}

.no-fields {
  text-align: center;
  padding: var(--xy-space-xl);
  font-size: 0.85rem;
}

/* ---- Settings table ---- */
.settings-table {
  width: 100%;
  border-collapse: collapse;
}

.group-sentinel-row {
  height: 0;
  border: none;
}

.group-sentinel-row td {
  padding: 0;
  border: none;
}

.group-sentinel {
  height: 1px;
}

.group-header-row td {
  padding: 0.5rem 1rem 0.35rem;
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  background: var(--xy-surface-0);
  border-bottom: 1px solid var(--xy-border);
  position: sticky;
  top: 0;
  z-index: 5;
}

.group-header-count {
  float: right;
  font-weight: 400;
  font-size: 0.65rem;
  color: var(--xy-text-muted);
  text-transform: none;
  letter-spacing: 0;
}

.setting-row {
  transition: background-color 0.1s ease;
}

.setting-row:hover {
  background-color: var(--xy-surface-1);
}

.setting-row:focus-within {
  background-color: var(--xy-surface-1);
}

.setting-row td {
  padding: 0.4rem 1rem;
  border-bottom: 1px solid rgba(255, 255, 255, 0.04);
  vertical-align: middle;
}

.setting-edited {
  border-left: 2px solid var(--xy-warning);
}

.setting-edited td:first-child {
  padding-left: calc(1rem - 2px);
}

.setting-key {
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--xy-text-primary);
  width: 40%;
  min-width: 160px;
}

.managed-badge {
  font-size: 0.6rem;
  margin-left: 0.4rem;
  vertical-align: middle;
}

.field-required {
  color: var(--xy-danger);
  margin-left: 0.15rem;
}

.setting-value {
  width: 60%;
}

.managed-value {
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  opacity: 0.6;
}

.inline-toggle {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.toggle-label {
  font-size: 0.7rem;
  color: var(--xy-text-muted);
}

.inline-input {
  max-width: 300px;
}

/* ---- Mobile ---- */
@media (max-width: 767px) {
  .editor-header {
    flex-wrap: wrap;
    padding: var(--xy-space-xs) var(--xy-space-sm);
    gap: var(--xy-space-sm);
  }

  .editor-header-info {
    min-width: 0;
    flex: 1;
  }

  .editor-file-name {
    font-size: 0.8rem;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .editor-controls {
    padding: var(--xy-space-xs) var(--xy-space-sm) 0;
  }

  .search-input {
    max-width: none;
  }

  .validation-banner {
    margin: var(--xy-space-xs) var(--xy-space-sm) 0;
  }

  .setting-row td {
    display: block;
    width: 100%;
  }

  .setting-key {
    width: 100%;
    padding-bottom: 0.15rem;
  }

  .setting-value {
    width: 100%;
    padding-top: 0;
  }

  .inline-input {
    max-width: none;
  }
}
</style>
