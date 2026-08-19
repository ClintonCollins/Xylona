<template>
  <form class="config-editor" @submit.prevent>
    <!-- Sticky header -->
    <div class="editor-header-sticky">
      <div class="editor-header">
        <div class="editor-header-info">
          <div class="editor-file-name font-mono">{{ filePath }}</div>
          <div class="editor-meta">
            <q-badge
              v-if="category"
              :label="category"
              :style="{ borderColor: categoryColor, color: categoryColor }"
              class="editor-category-badge"
              outline />
            <span class="text-xy-muted editor-meta-text">{{ format }}</span>
            <span v-if="editedValues.size > 0" class="editor-modified-count">
              <span aria-hidden="true" class="modified-dot"></span>
              {{ editedValues.size }} modified
            </span>
          </div>
        </div>
        <div class="editor-header-actions">
          <q-btn
            v-if="isMissing"
            :loading="generating"
            color="warning"
            icon="note_add"
            label="Generate File"
            outline
            size="sm"
            type="button"
            @click="$emit('generate')" />
          <q-btn
            v-else
            :class="{ 'save-success': saveSuccess }"
            :color="saveSuccess ? 'positive' : 'primary'"
            :disable="!hasChanges && !saveSuccess"
            :icon="saveSuccess ? 'check' : 'save'"
            :label="saveSuccess ? 'Saved' : 'Save'"
            :loading="saving"
            class="save-btn"
            size="sm"
            type="button"
            @click.prevent.stop="handleSave">
            <q-tooltip>{{ hasChanges ? 'Save changes (Ctrl+S)' : 'No changes to save' }}</q-tooltip>
          </q-btn>
        </div>
      </div>
      <q-separator class="editor-divider" />

      <!-- Scrollspy group tabs -->
      <div v-if="allGroups.length > 1" ref="groupTabsRef" class="group-tabs" role="tablist">
        <button
          v-for="group in allGroups"
          :key="group.name"
          :ref="(el) => setTabRef(group.name, el as HTMLElement | null)"
          :aria-selected="activeGroup === group.name"
          :class="{
            active: activeGroup === group.name,
            dimmed: searchQuery && !filteredGroupCounts.has(group.name),
          }"
          :style="{ '--tab-accent': groupAccentColor(group.name) }"
          class="group-tab"
          role="tab"
          @click="scrollToGroup(group.name)">
          {{ group.displayName }}
          <span class="tab-count">{{
            searchQuery ? (filteredGroupCounts.get(group.name) ?? 0) : group.fields.length
          }}</span>
        </button>
        <div
          ref="tabIndicatorRef"
          :style="{ background: groupAccentColor(activeGroup) }"
          aria-hidden="true"
          class="tab-indicator" />
      </div>

      <!-- Validation errors -->
      <Transition name="validation-slide">
        <q-banner v-if="validationErrors.length > 0" class="validation-banner" dense role="alert">
          <template #avatar>
            <q-icon aria-hidden="true" color="negative" name="error_outline" size="sm" />
          </template>
          <div class="validation-errors">
            <div v-for="(error, i) in validationErrors" :key="i" class="validation-error-item">
              <strong>{{ error.field }}:</strong> {{ error.message }}
            </div>
          </div>
        </q-banner>
      </Transition>

      <!-- Search controls -->
      <div v-if="fields.length > 0" class="editor-controls">
        <q-input
          v-model="searchQuery"
          :placeholder="`Filter ${fields.length} settings...`"
          aria-label="Search configuration fields"
          class="search-input"
          clearable
          dense
          outlined
          @clear="searchQuery = ''"
          @keydown.enter.prevent>
          <template #prepend>
            <q-icon aria-hidden="true" class="text-xy-muted" name="search" size="xs" />
          </template>
        </q-input>
      </div>
    </div>

    <!-- Compact settings table -->
    <div ref="tableScrollRef" class="table-scroll">
      <div
        v-if="fields.length === 0 && advancedFields.length === 0"
        class="no-fields text-xy-muted">
        No fields defined for this file. A superuser can add fields in the game's config schema
        editor.
      </div>

      <div v-else-if="filteredFields.length === 0 && searchQuery" class="no-fields text-xy-muted">
        <q-icon class="q-mb-sm" name="search_off" size="28px" />
        <div>
          No settings match "<strong class="text-xy-secondary">{{ searchQuery }}</strong
          >"
        </div>
        <q-btn
          class="q-mt-sm"
          dense
          flat
          label="Clear filter"
          size="sm"
          @click="searchQuery = ''" />
      </div>

      <table v-else class="settings-table">
        <template v-for="group in displayGroups" :key="group.name">
          <!-- Sentinel for IntersectionObserver (invisible) -->
          <tr class="group-sentinel-row">
            <td colspan="2">
              <div :id="`sentinel-${group.name}`" :data-group="group.name" class="group-sentinel" />
            </td>
          </tr>
          <!-- Sticky group header -->
          <tr :style="{ '--group-accent': groupAccentColor(group.name) }" class="group-header-row">
            <td colspan="2">
              <div class="group-header-inner">
                <span class="group-header-title">{{ group.displayName }}</span>
                <span class="group-header-count">{{ group.fields.length }}</span>
              </div>
            </td>
          </tr>
          <!-- Field rows -->
          <tr
            v-for="field in group.fields"
            :key="field.key"
            :class="{
              'setting-edited': editedValues.has(field.key),
              'setting-managed': field.isManaged,
            }"
            :data-test="`config-row-${field.key}`"
            class="setting-row">
            <!-- Setting name -->
            <td class="setting-key">
              <div class="setting-key-label">
                {{ field.title || field.key }}
                <q-badge v-if="field.isManaged" class="managed-badge" color="accent">
                  <q-icon class="q-mr-xs" name="lock" size="10px" />
                  Managed
                </q-badge>
                <span
                  v-if="field.required && !field.isManaged"
                  aria-label="required"
                  class="field-required"
                  >*</span
                >
              </div>
              <div v-if="field.description" class="setting-description">
                {{ field.description }}
              </div>
            </td>
            <!-- Setting value -->
            <td class="setting-value">
              <!-- Managed: read-only with lock icon and source label -->
              <div
                v-if="field.isManaged"
                class="managed-field-display"
                data-test="managed-field-display">
                <div class="managed-state-label">
                  <q-icon class="q-mr-xs" color="accent" name="admin_panel_settings" size="12px" />
                  Managed by server settings
                </div>
                <span class="managed-value font-mono">
                  {{ field.value || field.defaultValue }}
                  <q-icon class="q-ml-xs" color="accent" name="lock" size="xs">
                    <q-tooltip
                      >Automatically set from server settings — edit it there instead</q-tooltip
                    >
                  </q-icon>
                </span>
                <div class="managed-source-hint text-xy-muted">
                  <span class="managed-source-label">
                    Source: {{ getManagedSourceLabel(field.managedSource) }}
                  </span>
                </div>
              </div>

              <!-- Boolean toggle -->
              <div v-else-if="field.fieldType === 'boolean'" class="inline-toggle">
                <q-toggle
                  :id="fieldId(field.key)"
                  :aria-label="field.title || field.key"
                  :color="getFieldValue(field) === 'true' ? 'positive' : 'primary'"
                  :model-value="getFieldValue(field) === 'true'"
                  dense
                  @update:model-value="(val: boolean) => setFieldValue(field.key, String(val))" />
                <span
                  :class="getFieldValue(field) === 'true' ? 'toggle-on' : ''"
                  class="toggle-label">
                  {{ getFieldValue(field) === 'true' ? 'Enabled' : 'Disabled' }}
                </span>
              </div>

              <!-- Enum dropdown -->
              <q-select
                v-else-if="field.enumOptions.length > 0"
                :id="fieldId(field.key)"
                :aria-label="field.title || field.key"
                :model-value="getFieldValue(field)"
                :options="enumFilteredOptions(field)"
                class="inline-input"
                dense
                emit-value
                fill-input
                hide-selected
                hint="Select or type a custom value"
                input-debounce="0"
                map-options
                new-value-mode="add"
                outlined
                use-input
                @filter="
                  (val: string, update: (fn: () => void) => void) => enumFilter(field, val, update)
                "
                @input-value="(val: string) => (enumInputValues[field.key] = val)"
                @update:model-value="(val: string) => setFieldValue(field.key, val)" />

              <!-- Number input -->
              <q-input
                v-else-if="field.fieldType === 'integer' || field.fieldType === 'number'"
                :id="fieldId(field.key)"
                :aria-label="field.title || field.key"
                :hint="getNumberHint(field)"
                :max="field.maximum ?? undefined"
                :min="field.minimum ?? undefined"
                :model-value="getFieldValue(field)"
                :rules="getNumberRules(field)"
                class="inline-input"
                dense
                input-class="font-mono"
                outlined
                type="number"
                @keydown.enter.prevent
                @update:model-value="
                  (val: string | number | null) => setFieldValue(field.key, String(val ?? ''))
                " />

              <!-- String input (default) -->
              <q-input
                v-else
                :id="fieldId(field.key)"
                :aria-label="field.title || field.key"
                :maxlength="field.maxLength ?? undefined"
                :model-value="getFieldValue(field)"
                :rules="getStringRules(field)"
                class="inline-input"
                dense
                input-class="font-mono"
                outlined
                @keydown.enter.prevent
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
  </form>
</template>

<script lang="ts" setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import type { AdvancedField, ConfigFieldData, ConfigValidationError } from '@/proto/xylona_pb'
import { getManagedSourceLabel } from '@/components/shared/placeholder-definitions'
import ConfigAdvancedFields from './ConfigAdvancedFields.vue'
import { filterFields, groupFields } from './config-field-helpers'

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
const saveSuccess = ref(false)
let saveSuccessTimer = 0

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

// Reset activeGroup only when the group structure actually changes (e.g., file switch),
// not when the same groups reload with updated field values after save.
watch(
  () => allGroups.value.map((g) => g.name).join(','),
  (groupKey, oldGroupKey) => {
    const groups = allGroups.value
    if (groups.length === 0) return

    // If the current active group still exists, keep it
    if (oldGroupKey && groups.some((g) => g.name === activeGroup.value)) return

    activeGroup.value = groups[0].name
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

// Stable accent colors for group headers — derived from the existing palette
const GROUP_ACCENT_COLORS = [
  'var(--xy-primary)', // Blue
  'var(--xy-accent)', // Cyan
  'var(--xy-success)', // Green
  'var(--xy-warning)', // Amber
  'var(--xy-secondary)', // Indigo
  'var(--xy-info)', // Teal
]

const groupAccentMap = computed(() => {
  const map = new Map<string, string>()
  for (let i = 0; i < allGroups.value.length; i++) {
    map.set(allGroups.value[i].name, GROUP_ACCENT_COLORS[i % GROUP_ACCENT_COLORS.length])
  }
  return map
})

function groupAccentColor(groupName: string): string {
  return groupAccentMap.value.get(groupName) || GROUP_ACCENT_COLORS[0]
}

// ---- Spring-physics tab indicator ----
const groupTabsRef = ref<HTMLElement | null>(null)
const tabIndicatorRef = ref<HTMLElement | null>(null)
const tabRefs = new Map<string, HTMLElement>()
const prefersReducedMotion = ref(false)

function setTabRef(name: string, el: HTMLElement | null) {
  if (el) tabRefs.set(name, el)
  else tabRefs.delete(name)
}

// Spring solver state
let springLeftPos = 0
let springLeftVel = 0
let springWidthPos = 0
let springWidthVel = 0
let springAnimId = 0
let indicatorInitialized = false

function solveSpring(
  pos: number,
  vel: number,
  target: number,
  stiffness: number,
  damping: number,
): [number, number] {
  const force = -stiffness * (pos - target)
  const dampForce = -damping * vel
  const accel = force + dampForce
  const dt = 1 / 60
  const newVel = vel + accel * dt
  const newPos = pos + newVel * dt
  return [newPos, newVel]
}

function moveTabIndicator(groupName: string) {
  const tab = tabRefs.get(groupName)
  const container = groupTabsRef.value
  const indicator = tabIndicatorRef.value
  if (!tab || !container || !indicator) return

  const containerRect = container.getBoundingClientRect()
  const tabRect = tab.getBoundingClientRect()
  const targetLeft = tabRect.left - containerRect.left + container.scrollLeft
  const targetWidth = tabRect.width

  // First render: snap instantly, no spring
  if (!indicatorInitialized) {
    indicatorInitialized = true
    springLeftPos = targetLeft
    springWidthPos = targetWidth
    indicator.style.opacity = '1'
    indicator.style.transform = `translateX(${targetLeft}px)`
    indicator.style.width = `${targetWidth}px`
    return
  }

  // Reduced motion: snap
  if (prefersReducedMotion.value) {
    springLeftPos = targetLeft
    springWidthPos = targetWidth
    indicator.style.transform = `translateX(${targetLeft}px)`
    indicator.style.width = `${targetWidth}px`
    return
  }

  cancelAnimationFrame(springAnimId)

  const stiffness = 300
  const damping = 26

  function step() {
    ;[springLeftPos, springLeftVel] = solveSpring(
      springLeftPos,
      springLeftVel,
      targetLeft,
      stiffness,
      damping,
    )
    ;[springWidthPos, springWidthVel] = solveSpring(
      springWidthPos,
      springWidthVel,
      targetWidth,
      stiffness,
      damping,
    )

    if (!indicator) return

    indicator.style.transform = `translateX(${springLeftPos}px)`
    indicator.style.width = `${springWidthPos}px`

    const settled =
      Math.abs(springLeftPos - targetLeft) < 0.3 &&
      Math.abs(springLeftVel) < 0.3 &&
      Math.abs(springWidthPos - targetWidth) < 0.3 &&
      Math.abs(springWidthVel) < 0.3

    if (settled) {
      springLeftPos = targetLeft
      springLeftVel = 0
      springWidthPos = targetWidth
      springWidthVel = 0
      indicator.style.transform = `translateX(${targetLeft}px)`
      indicator.style.width = `${targetWidth}px`
    } else {
      springAnimId = requestAnimationFrame(step)
    }
  }

  springAnimId = requestAnimationFrame(step)
}

// Animate indicator when active group changes
watch(activeGroup, (groupName) => {
  if (groupName) {
    void nextTick(() => moveTabIndicator(groupName))
  }
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
  cancelAnimationFrame(scrollRafId)
  scrollRafId = requestAnimationFrame(() => {
    const scrollRoot = tableScrollRef.value
    if (!scrollRoot) return

    if (scrollSpySuppressed) return

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
    } else if (displayGroups.value.length > 0) {
      activeGroup.value = displayGroups.value[0].name
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
    // Reset tab indicator so it snaps to new position
    indicatorInitialized = false
    springLeftVel = 0
    springWidthVel = 0
  },
)

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  prefersReducedMotion.value = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  void nextTick(() => {
    setupScrollspy()
    // Initialize tab indicator position
    if (activeGroup.value) {
      moveTabIndicator(activeGroup.value)
    }
  })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
  cancelAnimationFrame(springAnimId)
  clearTimeout(saveSuccessTimer)
  if (tableScrollRef.value) {
    tableScrollRef.value.removeEventListener('scroll', updateActiveGroupFromScroll)
  }
})

function enumSelectOptions(field: ConfigFieldData) {
  if (field.enumLabels.length > 0) {
    return field.enumOptions.map((val, i) => ({
      label: field.enumLabels[i] ?? val,
      value: val,
    }))
  }
  return field.enumOptions.map((val) => ({ label: val, value: val }))
}

const enumInputValues: Record<string, string> = reactive({})
const enumFilteredCache: Record<string, { label: string; value: string }[]> = reactive({})

function enumFilteredOptions(field: ConfigFieldData) {
  return enumFilteredCache[field.key] ?? enumSelectOptions(field)
}

function enumFilter(field: ConfigFieldData, val: string, update: (fn: () => void) => void) {
  update(() => {
    const allOptions = enumSelectOptions(field)
    if (!val) {
      enumFilteredCache[field.key] = allOptions
      return
    }
    const needle = val.toLowerCase()
    enumFilteredCache[field.key] = allOptions.filter(
      (opt) => opt.label.toLowerCase().includes(needle) || opt.value.toLowerCase().includes(needle),
    )
  })
}

function getFieldValue(field: ConfigFieldData): string {
  const edited = editedValues.get(field.key)
  if (edited !== undefined) {
    return edited
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

function getNumberHint(field: ConfigFieldData): string | undefined {
  const hasMin = field.minimum !== undefined
  const hasMax = field.maximum !== undefined
  if (hasMin && hasMax) {
    return `${field.minimum} – ${field.maximum}`
  }
  if (hasMin) {
    return `Min: ${field.minimum}`
  }
  if (hasMax) {
    return `Max: ${field.maximum}`
  }
  return undefined
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

let savedScrollTop = 0

function handleSave() {
  // Save scroll position before the async save cycle replaces the fields prop
  savedScrollTop = tableScrollRef.value?.scrollTop ?? 0

  const fieldValues = new Map<string, string>()
  for (const [key, value] of editedValues) {
    fieldValues.set(key, value)
  }
  emit('save', fieldValues)
  editedValues.clear()
  advancedChanged.value = false
}

// Suppress scrollspy while saving to prevent the active group from resetting
// when the fields prop is replaced and the table DOM re-renders.
watch(
  () => props.saving,
  (saving, wasSaving) => {
    if (saving && !wasSaving) {
      scrollSpySuppressed = true
    }
    if (wasSaving && !saving) {
      // Restore scroll position and re-enable scrollspy after Vue re-renders
      void nextTick(() => {
        const scrollRoot = tableScrollRef.value
        if (scrollRoot && savedScrollTop > 0) {
          scrollRoot.scrollTop = savedScrollTop
        }
        // Also re-position the tab indicator since tab refs may have been recreated
        if (activeGroup.value) {
          moveTabIndicator(activeGroup.value)
        }
        setTimeout(() => {
          scrollSpySuppressed = false
        }, 100)
      })
      // Flash success state
      if (props.validationErrors.length === 0) {
        clearTimeout(saveSuccessTimer)
        saveSuccess.value = true
        saveSuccessTimer = window.setTimeout(() => {
          saveSuccess.value = false
        }, 2000)
      }
    }
  },
)
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
  padding: var(--xy-space-md) var(--xy-space-md) var(--xy-space-sm);
  gap: var(--xy-space-md);
}

.editor-header-info {
  min-width: 0;
  flex: 1;
}

.editor-file-name {
  font-size: 1rem;
  font-weight: 700;
  color: var(--xy-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  letter-spacing: 0.01em;
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
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 0.75rem;
  font-weight: 700;
  color: var(--xy-warning);
  background: color-mix(in srgb, var(--xy-warning) 10%, transparent);
  padding: 0.15rem 0.5rem;
  border-radius: 4px;
}

.modified-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background-color: var(--xy-warning);
  flex-shrink: 0;
  animation: pulse-dot 2s ease-in-out infinite;
}

@keyframes pulse-dot {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0.4;
  }
}

/* ---- Save button success flash ---- */
.save-btn {
  transition:
    background-color 200ms cubic-bezier(0.25, 1, 0.5, 1),
    transform 150ms cubic-bezier(0.25, 1, 0.5, 1);
}

.save-success {
  animation: save-pop 300ms cubic-bezier(0.25, 1, 0.5, 1);
}

@keyframes save-pop {
  0% {
    transform: scale(1);
  }
  40% {
    transform: scale(1.08);
  }
  100% {
    transform: scale(1);
  }
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
  background: var(--xy-surface-0);
  position: relative;
}

.group-tabs::-webkit-scrollbar {
  display: none;
}

.group-tab {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.65rem 1.5rem;
  font-family: var(--xy-font-display);
  font-size: 0.8rem;
  font-weight: 500;
  color: var(--xy-text-muted);
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  white-space: nowrap;
  flex-shrink: 0;
  letter-spacing: 0.02em;
  transition:
    color var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.group-tab:hover {
  color: var(--xy-text-secondary);
  background-color: var(--xy-surface-2);
}

.group-tab:focus-visible {
  outline: 2px solid var(--xy-primary);
  outline-offset: -2px;
  border-radius: 2px;
}

.group-tab.active {
  color: var(--xy-text-primary);
  border-bottom-color: var(--tab-accent, var(--xy-primary));
  background-color: color-mix(in srgb, var(--tab-accent, var(--xy-primary)) 6%, transparent);
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
  font-weight: 700;
  background: var(--xy-surface-3);
  color: var(--xy-text-muted);
  padding: 0.15rem 0.5rem;
  border-radius: 8px;
  min-width: 1.4rem;
  text-align: center;
}

.group-tab.active .tab-count {
  background: var(--tab-accent, var(--xy-primary));
  color: var(--xy-base);
}

/* ---- Spring-physics tab indicator ---- */
.tab-indicator {
  position: absolute;
  bottom: 0;
  left: 0;
  height: 2px;
  width: 0;
  background: var(--xy-primary);
  opacity: 0;
  pointer-events: none;
  will-change: transform, width;
  border-radius: 1px 1px 0 0;
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

/* Validation banner entrance/exit animation */
.validation-slide-enter-active {
  animation: validation-enter 300ms cubic-bezier(0.25, 1, 0.5, 1);
}

.validation-slide-leave-active {
  animation: validation-enter 200ms cubic-bezier(0.25, 1, 0.5, 1) reverse;
}

@keyframes validation-enter {
  from {
    opacity: 0;
    transform: translateY(-8px);
    max-height: 0;
    margin-top: 0;
  }
  to {
    opacity: 1;
    transform: translateY(0);
    max-height: 200px;
    margin-top: var(--xy-space-sm);
  }
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

.search-input :deep(.q-field__control) {
  background-color: var(--xy-surface-0);
}

.search-input :deep(.q-field--focused .q-field__control) {
  border-color: var(--xy-primary);
}

/* ---- Table scroll container (darker bg for contrast against header) ---- */
.table-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  background-color: var(--xy-base);
  /* Extra bottom padding ensures last rows are reachable even if
     the container height is slightly clipped by Quasar's layout. */
  padding-bottom: 5rem;
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

/* Section divider: add visual gap above every group header that follows a setting row.
   The sentinel row sits between groups, so target it when preceded by a setting row. */
.setting-row + .group-sentinel-row td {
  padding-top: 24px;
  background: var(--xy-base);
}

.group-header-row td {
  padding: 0;
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  position: sticky;
  top: 0;
  z-index: 5;
}

.group-header-inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.65rem 1rem 0.5rem;
  border: 1px solid color-mix(in srgb, var(--group-accent, var(--xy-primary)) 28%, var(--xy-border));
  background-color: color-mix(in srgb, var(--group-accent, var(--xy-primary)) 5%, transparent);
}

.group-header-title {
  font-family: var(--xy-font-display);
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--xy-text-primary);
}

.group-header-count {
  font-family: var(--xy-font-mono);
  font-size: 0.65rem;
  font-weight: 600;
  color: var(--group-accent, var(--xy-text-muted));
  opacity: 0.7;
  padding: 0.1rem 0.45rem;
  border-radius: 4px;
}

.setting-row {
  transition:
    background-color var(--xy-transition-fast),
    outline-color var(--xy-transition-fast);
  outline: 1px solid transparent;
  outline-offset: -1px;
}

/* Subtle alternating rows for scan-ability.
   Uses :nth-child(… of .setting-row) to count only setting rows,
   ignoring sentinel/header rows interleaved in the table. */
.setting-row:nth-child(even of .setting-row) {
  background-color: color-mix(in srgb, var(--xy-surface-0) 60%, transparent);
}

.setting-row:hover {
  background-color: var(--xy-surface-1);
  outline-color: color-mix(in srgb, var(--xy-primary) 30%, transparent);
}

.setting-row:focus-within {
  background-color: var(--xy-surface-1);
  outline-color: var(--xy-primary);
}

.setting-row td {
  padding: 0.5rem 1rem;
  border-bottom: 1px solid var(--xy-border);
  vertical-align: middle;
}

/* When a description is present, top-align so the value stays paired with the label */
.setting-row:has(.setting-description) td {
  vertical-align: top;
  padding-top: 0.6rem;
}

.setting-edited {
  background-color: color-mix(in srgb, var(--xy-warning) 4%, transparent);
  outline-color: var(--xy-warning-border);
}

.setting-edited:hover {
  outline-color: var(--xy-warning);
}

.setting-managed {
  background-color: color-mix(in srgb, var(--xy-accent) 4%, transparent);
}

.setting-key {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  width: 40%;
  min-width: 160px;
  overflow-wrap: break-word;
  word-break: break-word;
}

.setting-description {
  font-size: 0.7rem;
  color: var(--xy-text-muted);
  line-height: 1.4;
  margin-top: 3px;
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

.managed-field-display {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding: 0.55rem 0.7rem;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 35%, transparent);
  border-radius: 8px;
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-0));
}

.managed-state-label {
  display: inline-flex;
  align-items: center;
  font-size: 0.7rem;
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.managed-value {
  font-size: 0.8rem;
  color: var(--xy-text-primary);
  overflow-wrap: break-word;
  word-break: break-word;
}

.managed-source-hint {
  font-size: 0.72rem;
}

.inline-toggle {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.toggle-label {
  font-size: 0.7rem;
  color: var(--xy-text-muted);
  transition: color var(--xy-transition-fast);
}

.toggle-on {
  color: var(--xy-success);
}

.inline-input {
  max-width: 300px;
}

/* Give inputs a visible background so empty fields are clearly interactive */
.inline-input :deep(.q-field__control) {
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
}

.inline-input :deep(.q-field__control:hover) {
  border-color: color-mix(in srgb, var(--xy-primary) 50%, var(--xy-border));
}

.inline-input :deep(.q-field--focused .q-field__control) {
  border-color: var(--xy-primary);
}

.managed-field-display {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.managed-source-hint {
  font-size: 0.65rem;
}

.managed-source-label {
  color: var(--xy-accent);
  font-style: normal;
  font-weight: 500;
}

/* ---- Reduced motion ---- */
@media (prefers-reduced-motion: reduce) {
  .setting-row,
  .group-tab,
  .file-item,
  .inline-input :deep(.q-field__control) {
    transition-duration: 0.01ms !important;
  }

  .validation-slide-enter-active,
  .validation-slide-leave-active {
    animation-duration: 0.01ms !important;
  }

  .modified-dot,
  .save-success {
    animation: none;
  }
}

/* ---- Mobile ---- */
@media (max-width: 767px) {
  .editor-header {
    flex-wrap: wrap;
    padding: var(--xy-space-xs) var(--xy-space-sm);
    gap: var(--xy-space-sm);
  }

  .editor-file-name {
    font-size: 0.8rem;
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
