<template>
  <form
    ref="rootRef"
    :style="{ '--editor-header-height': `${headerHeight}px` }"
    class="config-editor"
    @submit.prevent>
    <!-- Sticky header: file, search, actions, group tabs -->
    <div ref="headerRef" class="editor-header-sticky">
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
            <q-badge v-if="isMissing" color="warning" label="Missing" text-color="dark" />
            <span v-if="editedValues.size > 0" class="editor-modified-count">
              <span aria-hidden="true" class="modified-dot"></span>
              {{ editedValues.size }} modified
            </span>
          </div>
        </div>
        <q-input
          v-if="fields.length > 0 || advancedFields.length > 0"
          v-model="searchQuery"
          :placeholder="`Filter ${fields.length + advancedFields.length} settings...`"
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
        <div class="editor-header-actions">
          <q-btn
            v-if="isMissing"
            :loading="generating"
            color="warning"
            icon="note_add"
            label="Generate File"
            outline
            type="button"
            @click="$emit('generate')" />
          <template v-else>
            <q-btn
              v-if="hasChanges"
              :disable="saving"
              class="discard-btn"
              flat
              label="Discard changes"
              type="button"
              @click="discardChanges" />
            <q-btn
              :class="{ 'save-success': saveSuccess }"
              :color="saveSuccess ? 'positive' : 'primary'"
              :disable="!canSave && !saveSuccess"
              :icon="saveSuccess ? 'check' : 'save'"
              :label="saveSuccess ? 'Saved' : 'Save'"
              :loading="saving"
              class="save-btn"
              type="button"
              @click.prevent.stop="handleSave">
              <q-tooltip>{{ saveTooltip }}</q-tooltip>
            </q-btn>
          </template>
        </div>
      </div>

      <!-- Scrollspy group tabs -->
      <div
        v-if="allGroups.length > 1"
        ref="groupTabsRef"
        :class="{ 'fade-start': tabsFadeStart, 'fade-end': tabsFadeEnd }"
        class="group-tabs"
        role="tablist"
        @scroll.passive="updateTabsOverflow"
        @wheel="scrollTabsWithWheel">
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
          type="button"
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
              <strong>{{ fieldLabel(error.field) }}:</strong> {{ error.message }}
            </div>
          </div>
        </q-banner>
      </Transition>
    </div>

    <!-- Settings list -->
    <div class="settings-body">
      <div
        v-if="fields.length === 0 && advancedFields.length === 0"
        class="no-fields text-xy-muted">
        No fields defined for this file. A superuser can add fields in the game's config schema
        editor.
      </div>

      <div v-else-if="filteredFields.length === 0 && searchQuery" class="no-fields text-xy-muted">
        <q-icon class="q-mb-sm" name="search_off" size="28px" />
        <div>
          No {{ advancedMatchCount > 0 ? 'schema ' : '' }}settings match "<strong
            class="text-xy-secondary"
            >{{ searchQuery }}</strong
          >"
        </div>
        <div v-if="advancedMatchCount > 0" class="q-mt-xs" data-test="advanced-match-count">
          {{ advancedMatchCount }} match{{ advancedMatchCount === 1 ? '' : 'es' }} in Advanced
          Fields below
        </div>
        <q-btn
          class="q-mt-sm"
          dense
          flat
          label="Clear filter"
          type="button"
          @click="searchQuery = ''" />
      </div>

      <div v-else class="settings-list">
        <section
          v-for="group in displayGroups"
          :key="group.name"
          :ref="(el) => setSectionRef(group.name, el as HTMLElement | null)"
          :style="{ '--group-accent': groupAccentColor(group.name) }"
          class="settings-group">
          <div class="group-header">
            <span class="group-header-title">{{ group.displayName }}</span>
            <span class="group-header-count">{{ group.fields.length }}</span>
          </div>
          <div
            v-for="field in group.fields"
            :key="field.key"
            :class="{
              'setting-edited': editedValues.has(field.key),
              'setting-invalid': serverError(field.key) !== undefined,
              'setting-managed': field.isManaged,
            }"
            :data-test="`config-row-${field.key}`"
            class="setting-row">
            <!-- Setting name -->
            <div class="setting-key">
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
            </div>
            <!-- Setting value -->
            <div class="setting-value">
              <!-- Managed: read-only with lock icon and source label -->
              <div
                v-if="field.isManaged"
                class="managed-field-display"
                data-test="managed-field-display">
                <div class="managed-state-label">
                  <q-icon class="q-mr-xs" color="accent" name="admin_panel_settings" size="12px" />
                  Managed by server settings
                </div>
                <span class="managed-value font-mono" data-test="managed-value">
                  {{ managedDisplayValue(field) }}
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
                :error="serverError(field.key) !== undefined ? true : undefined"
                :error-message="serverError(field.key)"
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
                v-else-if="isNumberField(field)"
                :id="fieldId(field.key)"
                :aria-label="field.title || field.key"
                :aria-required="field.required"
                :error="serverError(field.key) !== undefined ? true : undefined"
                :error-message="serverError(field.key)"
                :hint="getNumberHint(field)"
                :max="field.maximum ?? undefined"
                :min="field.minimum ?? undefined"
                :model-value="getFieldValue(field)"
                :rules="fieldRules(field)"
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
                :aria-required="field.required"
                :autocomplete="isSecretConfigKey(field.key) ? 'new-password' : 'off'"
                :error="serverError(field.key) !== undefined ? true : undefined"
                :error-message="serverError(field.key)"
                :maxlength="field.maxLength ?? undefined"
                :model-value="getFieldValue(field)"
                :rules="fieldRules(field)"
                :type="
                  isSecretConfigKey(field.key) && !revealedKeys.has(field.key) ? 'password' : 'text'
                "
                class="inline-input"
                dense
                input-class="font-mono"
                outlined
                @keydown.enter.prevent
                @update:model-value="
                  (val: string | number | null) => setFieldValue(field.key, String(val ?? ''))
                ">
                <template v-if="isSecretConfigKey(field.key)" #append>
                  <q-btn
                    :aria-label="revealedKeys.has(field.key) ? 'Hide value' : 'Show value'"
                    :aria-pressed="revealedKeys.has(field.key)"
                    :icon="revealedKeys.has(field.key) ? 'visibility_off' : 'visibility'"
                    dense
                    flat
                    round
                    type="button"
                    @click="toggleReveal(field.key)">
                    <q-tooltip>{{
                      revealedKeys.has(field.key) ? 'Hide value' : 'Show value'
                    }}</q-tooltip>
                  </q-btn>
                </template>
              </q-input>

              <div
                v-if="field.fieldType === 'boolean' && serverError(field.key) !== undefined"
                class="setting-error">
                {{ serverError(field.key) }}
              </div>
            </div>
          </div>
        </section>
      </div>

      <!-- Advanced fields (below the schema fields) -->
      <config-advanced-fields
        :fields="advancedFields"
        :search="searchQuery"
        @update="handleAdvancedUpdate" />
    </div>
  </form>
</template>

<script lang="ts" setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import type { AdvancedField, ConfigFieldData, ConfigValidationError } from '@/proto/xylona_pb'
import { getManagedSourceLabel } from '@/components/shared/placeholder-definitions'
import ConfigAdvancedFields from './ConfigAdvancedFields.vue'
import {
  filterFields,
  groupFields,
  isSecretConfigKey,
  trimNumberPadding,
} from './config-field-helpers'

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
  discard: []
  generate: []
  updateAdvanced: [fields: AdvancedField[]]
}>()

// Track local edits as key → value overrides
const editedValues = reactive(new Map<string, string>())
// The values of the last save attempt; server errors describe these values.
const submittedValues = reactive(new Map<string, string>())
const revealedKeys = reactive(new Set<string>())
const advancedChanged = ref(false)
const saveSuccess = ref(false)
let saveSuccessTimer = 0

// Ctrl+S / Cmd+S keyboard shortcut
function onKeyDown(e: KeyboardEvent) {
  if ((e.ctrlKey || e.metaKey) && e.key === 's') {
    e.preventDefault()
    if (canSave.value) {
      handleSave()
    }
  }
}

// Search state
const searchQuery = ref('')

const hasChanges = computed(() => editedValues.size > 0 || advancedChanged.value)

const filteredFields = computed(() => {
  return filterFields([...props.fields], searchQuery.value)
})

const advancedMatchCount = computed(() =>
  searchQuery.value.trim() ? filterFields([...props.advancedFields], searchQuery.value).length : 0,
)

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

    activeGroup.value = groups[0]?.name ?? ''
    void nextTick(updateTabsOverflow)
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

// Stable accent colors for group headers. No status colours: a green or amber
// group header would read as "OK" or "modified".
const GROUP_ACCENT_COLORS = [
  'var(--xy-primary)', // Blue
  'var(--xy-accent)', // Cyan
  'var(--xy-purple)', // Violet
  'var(--xy-info)', // Teal
]

const groupAccentMap = computed(() => {
  const map = new Map<string, string>()
  for (let i = 0; i < allGroups.value.length; i++) {
    map.set(
      allGroups.value[i]?.name ?? '',
      GROUP_ACCENT_COLORS[i % GROUP_ACCENT_COLORS.length] ?? '',
    )
  }
  return map
})

function groupAccentColor(groupName: string): string {
  return groupAccentMap.value.get(groupName) || GROUP_ACCENT_COLORS[0] || ''
}

// ---- Validation ----
const serverErrors = computed(() => {
  const messages = new Map<string, string>()
  for (const error of props.validationErrors) {
    if (!messages.has(error.field)) messages.set(error.field, error.message)
  }
  return messages
})

/** The server's error for a field, until the operator changes the value it was about. */
function serverError(key: string): string | undefined {
  const message = serverErrors.value.get(key)
  if (message === undefined || editedValues.get(key) !== submittedValues.get(key)) {
    return undefined
  }
  return message
}

function fieldLabel(key: string): string {
  return props.fields.find((field) => field.key === key)?.title || key
}

const invalidFieldCount = computed(
  () =>
    props.fields.filter(
      (field) =>
        editedValues.has(field.key) &&
        fieldRules(field).some((rule) => rule(getFieldValue(field)) !== true),
    ).length,
)

const canSave = computed(() => hasChanges.value && !props.saving && invalidFieldCount.value === 0)

const saveTooltip = computed(() => {
  if (invalidFieldCount.value > 0) {
    const count = invalidFieldCount.value
    return `Fix ${count} invalid setting${count === 1 ? '' : 's'} before saving`
  }
  return hasChanges.value ? 'Save changes (Ctrl+S)' : 'No changes to save'
})

// ---- Sticky header height ----
const rootRef = ref<HTMLElement | null>(null)
const headerRef = ref<HTMLElement | null>(null)
const headerHeight = ref(0)
let headerObserver: ResizeObserver | undefined

// The page scrolls inside the server layout's content area, not the window.
let scroller: HTMLElement | Window = window

function findScroller(el: HTMLElement): HTMLElement | Window {
  for (let node = el.parentElement; node; node = node.parentElement) {
    const { overflowY } = getComputedStyle(node)
    if (overflowY === 'auto' || overflowY === 'scroll') return node
  }
  return window
}

function scrollingElement(): Element | null {
  return scroller instanceof Window ? document.scrollingElement : scroller
}

// ---- Group tabs: spring indicator, overflow fades, reveal active tab ----
const groupTabsRef = ref<HTMLElement | null>(null)
const tabIndicatorRef = ref<HTMLElement | null>(null)
const tabRefs = new Map<string, HTMLElement>()
const tabsFadeStart = ref(false)
const tabsFadeEnd = ref(false)

function setTabRef(name: string, el: HTMLElement | null) {
  if (el) tabRefs.set(name, el)
  else tabRefs.delete(name)
}

function prefersReducedMotion(): boolean {
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

function updateTabsOverflow() {
  const tabs = groupTabsRef.value
  if (!tabs) return
  tabsFadeStart.value = tabs.scrollLeft > 1
  tabsFadeEnd.value = tabs.scrollLeft + tabs.clientWidth < tabs.scrollWidth - 1
}

// A vertical wheel over the strip scrolls it sideways until it reaches an end.
function scrollTabsWithWheel(event: WheelEvent) {
  const tabs = groupTabsRef.value
  if (!tabs || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return
  const maxLeft = tabs.scrollWidth - tabs.clientWidth
  const nextLeft = Math.min(maxLeft, Math.max(0, tabs.scrollLeft + event.deltaY))
  if (nextLeft === tabs.scrollLeft) return
  event.preventDefault()
  tabs.scrollLeft = nextLeft
}

function revealTab(groupName: string) {
  const tab = tabRefs.get(groupName)
  const tabs = groupTabsRef.value
  if (!tab || !tabs) return
  const left = tab.offsetLeft
  const right = left + tab.offsetWidth
  if (left < tabs.scrollLeft) {
    tabs.scrollLeft = left
  } else if (right > tabs.scrollLeft + tabs.clientWidth) {
    tabs.scrollLeft = right - tabs.clientWidth
  }
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
  const indicator = tabIndicatorRef.value
  if (!tab || !indicator) return

  const targetLeft = tab.offsetLeft
  const targetWidth = tab.offsetWidth

  // First render or reduced motion: snap, no spring
  if (!indicatorInitialized || prefersReducedMotion()) {
    indicatorInitialized = true
    springLeftPos = targetLeft
    springWidthPos = targetWidth
    indicator.style.opacity = '1'
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

// Animate the indicator and keep the active tab in view when the group changes
watch(activeGroup, (groupName) => {
  if (groupName) {
    void nextTick(() => {
      revealTab(groupName)
      moveTabIndicator(groupName)
    })
  }
})

// ---- Scrollspy ----
const sectionRefs = new Map<string, HTMLElement>()
let scrollSpySuppressed = false
let scrollSpyTimer = 0
let scrollRafId = 0

function setSectionRef(name: string, el: HTMLElement | null) {
  if (el) sectionRefs.set(name, el)
  else sectionRefs.delete(name)
}

function scrollToGroup(groupName: string) {
  const section = sectionRefs.get(groupName)
  if (!section) return

  // Set active immediately and suppress scrollspy during smooth scroll
  activeGroup.value = groupName
  scrollSpySuppressed = true

  const reducedMotion = prefersReducedMotion()
  // scroll-margin-top keeps the section clear of the sticky header
  section.scrollIntoView({ block: 'start', behavior: reducedMotion ? 'instant' : 'smooth' })

  clearTimeout(scrollSpyTimer)
  scrollSpyTimer = window.setTimeout(
    () => {
      scrollSpySuppressed = false
    },
    reducedMotion ? 0 : 500,
  )
}

function scrolledToEnd(): boolean {
  const el = scrollingElement()
  return el !== null && el.scrollTop > 0 && el.scrollTop + el.clientHeight >= el.scrollHeight - 2
}

function updateActiveGroupFromScroll() {
  cancelAnimationFrame(scrollRafId)
  scrollRafId = requestAnimationFrame(() => {
    const header = headerRef.value
    const groups = displayGroups.value
    if (scrollSpySuppressed || !header || groups.length === 0) return

    // A group is current once its section reaches the bottom of the sticky
    // header, where its own header pins. At the end of the page the last group
    // wins, since short final groups can never reach the top.
    let current = groups[0]?.name ?? ''
    if (scrolledToEnd()) {
      current = groups[groups.length - 1]?.name ?? current
    } else {
      const threshold = header.getBoundingClientRect().bottom + 1
      for (const group of groups) {
        const section = sectionRefs.get(group.name)
        if (section && section.getBoundingClientRect().top <= threshold) {
          current = group.name
        }
      }
    }
    activeGroup.value = current
  })
}

function measureHeader() {
  headerHeight.value = headerRef.value?.offsetHeight ?? 0
  updateTabsOverflow()
}

// Reset edits when file changes
watch(
  () => props.filePath,
  () => {
    resetEdits()
    revealedKeys.clear()
    searchQuery.value = ''
    // Start the new file at the top of the page
    const el = scrollingElement()
    if (el) el.scrollTop = 0
    // Reset tab indicator so it snaps to new position
    indicatorInitialized = false
    springLeftVel = 0
    springWidthVel = 0
  },
)

onMounted(() => {
  window.addEventListener('keydown', onKeyDown)
  if (rootRef.value) scroller = findScroller(rootRef.value)
  scroller.addEventListener('scroll', updateActiveGroupFromScroll, { passive: true })
  if (typeof ResizeObserver !== 'undefined' && headerRef.value) {
    headerObserver = new ResizeObserver(measureHeader)
    headerObserver.observe(headerRef.value)
  }
  void nextTick(() => {
    measureHeader()
    if (activeGroup.value) {
      moveTabIndicator(activeGroup.value)
    }
  })
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeyDown)
  scroller.removeEventListener('scroll', updateActiveGroupFromScroll)
  headerObserver?.disconnect()
  cancelAnimationFrame(springAnimId)
  cancelAnimationFrame(scrollRafId)
  clearTimeout(saveSuccessTimer)
  clearTimeout(scrollSpyTimer)
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

function isNumberField(field: ConfigFieldData): boolean {
  return field.fieldType === 'integer' || field.fieldType === 'number'
}

function originalValue(field: ConfigFieldData): string {
  const value = field.value || field.defaultValue
  return isNumberField(field) ? trimNumberPadding(value) : value
}

function getFieldValue(field: ConfigFieldData): string {
  return editedValues.get(field.key) ?? originalValue(field)
}

function setFieldValue(key: string, value: string) {
  const field = props.fields.find((f) => f.key === key)
  const original = field ? originalValue(field) : ''
  const unchanged =
    value === original ||
    (field !== undefined &&
      isNumberField(field) &&
      value !== '' &&
      original !== '' &&
      Number(value) === Number(original))
  if (unchanged) {
    editedValues.delete(key)
  } else {
    editedValues.set(key, value)
  }
}

const maskedValue = '••••••••'

function managedDisplayValue(field: ConfigFieldData): string {
  if (isSecretConfigKey(field.key) || isSecretConfigKey(field.managedSource)) {
    return maskedValue
  }
  const value = field.value || field.defaultValue
  if (field.fieldType === 'boolean') {
    return value === 'true' ? 'Enabled' : 'Disabled'
  }
  return value
}

function toggleReveal(key: string) {
  if (!revealedKeys.delete(key)) revealedKeys.add(key)
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

type FieldRule = (val: string) => true | string

function fieldRules(field: ConfigFieldData): FieldRule[] {
  if (field.isManaged || field.fieldType === 'boolean' || field.enumOptions.length > 0) {
    return []
  }
  const rules: FieldRule[] = []
  if (field.required) {
    rules.push((val) => (val !== '' && val !== undefined && val !== null) || 'Required')
  }
  // Empty optional values are left out of the file, so limits skip them.
  if (isNumberField(field)) {
    if (field.minimum !== undefined) {
      const min = Number(field.minimum)
      rules.push((val) => val === '' || Number(val) >= min || `Minimum: ${min}`)
    }
    if (field.maximum !== undefined) {
      const max = Number(field.maximum)
      rules.push((val) => val === '' || Number(val) <= max || `Maximum: ${max}`)
    }
  } else if (field.maxLength) {
    const max = field.maxLength
    rules.push((val) => !val || val.length <= max || `Maximum ${max} characters`)
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
  if (!canSave.value) return
  submittedValues.clear()
  for (const [key, value] of editedValues) {
    submittedValues.set(key, value)
  }
  // Edits stay until the parent confirms the save with confirmSaved().
  emit('save', new Map(editedValues))
}

function resetEdits() {
  editedValues.clear()
  submittedValues.clear()
  advancedChanged.value = false
}

function discardChanges() {
  resetEdits()
  emit('discard')
}

/** Called by the parent once the server accepted the save and the file reloaded. */
function confirmSaved() {
  // Edits made while the save was in flight stay pending.
  for (const [key, value] of submittedValues) {
    if (editedValues.get(key) === value) editedValues.delete(key)
  }
  submittedValues.clear()
  advancedChanged.value = false
  clearTimeout(saveSuccessTimer)
  saveSuccess.value = true
  saveSuccessTimer = window.setTimeout(() => {
    saveSuccess.value = false
  }, 2000)
}

defineExpose({ hasChanges, confirmSaved })
</script>

<style scoped>
.config-editor {
  display: flex;
  flex-direction: column;
}

/* ---- Sticky header: stays under the server tabs while the page scrolls ---- */
.editor-header-sticky {
  position: sticky;
  top: 0;
  z-index: 6;
  background-color: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.editor-header {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  padding: var(--xy-space-md) var(--xy-space-md) var(--xy-space-sm);
  gap: var(--xy-space-sm) var(--xy-space-md);
}

/* A zero basis keeps the file name, search and actions on one row until the
   name has shrunk to its minimum. */
.editor-header-info {
  min-width: 6rem;
  flex: 1 1 0;
}

.editor-header-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  flex-shrink: 0;
}

.editor-file-name {
  font-size: var(--xy-font-size-base);
  font-weight: 700;
  color: var(--xy-text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  letter-spacing: 0.01em;
}

.editor-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-xs) var(--xy-space-sm);
  margin-top: var(--xy-space-xs);
}

.editor-meta-text {
  font-size: var(--xy-font-size-xs);
}

.editor-category-badge {
  font-size: var(--xy-font-size-xs);
}

.editor-modified-count {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  color: var(--xy-warning);
  background: color-mix(in srgb, var(--xy-warning) 10%, transparent);
  padding: 0.15rem 0.5rem;
  border-radius: var(--xy-radius-sm);
  white-space: nowrap;
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

/* ---- Search ---- */
.search-input {
  flex: 0 1 13rem;
  min-width: 10rem;
}

.search-input :deep(.q-field__control) {
  background-color: var(--xy-surface-0);
}

.search-input :deep(.q-field--focused .q-field__control) {
  border-color: var(--xy-primary);
}

/* ---- Scrollspy group tabs ---- */
.group-tabs {
  --fade-start: 0px;
  --fade-end: 0px;
  display: flex;
  overflow-x: auto;
  scrollbar-width: none;
  border-top: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  position: relative;
  /* Edge fades show more tabs sit past the visible strip. */
  mask-image: linear-gradient(
    to right,
    transparent 0,
    black var(--fade-start),
    black calc(100% - var(--fade-end)),
    transparent 100%
  );
}

.group-tabs.fade-start {
  --fade-start: 40px;
}

.group-tabs.fade-end {
  --fade-end: 40px;
}

.group-tabs::-webkit-scrollbar {
  display: none;
}

.group-tab {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.65rem 1.5rem;
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-sm);
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
  border-radius: var(--xy-radius-sm);
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
  font-size: var(--xy-font-size-2xs);
  font-weight: 700;
  background: var(--xy-surface-3);
  color: var(--xy-text-muted);
  padding: 0.15rem 0.5rem;
  border-radius: var(--xy-radius-lg);
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
  border-radius: var(--xy-radius-sm) var(--xy-radius-sm) 0 0;
}

/* ---- Validation banner ---- */
.validation-banner {
  margin: var(--xy-space-sm) var(--xy-space-md);
  max-height: 8rem;
  overflow-y: auto;
  background-color: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
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
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

/* ---- Settings list (darker bg for contrast against header) ---- */
.settings-body {
  background-color: var(--xy-base);
  padding-bottom: var(--xy-space-md);
}

.no-fields {
  text-align: center;
  padding: var(--xy-space-xl);
  font-size: var(--xy-font-size-sm);
}

/* Each group is its own sticky context, so the incoming group header pushes the
   previous one out instead of stacking under it. */
.settings-group {
  scroll-margin-top: var(--editor-header-height, 0px);
}

.settings-group + .settings-group {
  margin-top: var(--xy-space-lg);
}

.group-header {
  position: sticky;
  top: var(--editor-header-height, 0px);
  z-index: 5;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.65rem 1rem 0.5rem;
  border: 1px solid color-mix(in srgb, var(--group-accent, var(--xy-primary)) 28%, var(--xy-border));
  background-color: color-mix(
    in srgb,
    var(--group-accent, var(--xy-primary)) 5%,
    var(--xy-surface-1)
  );
}

.group-header-title {
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--xy-text-primary);
}

.group-header-count {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  color: var(--group-accent, var(--xy-text-muted));
  opacity: 0.7;
  padding: 0.1rem 0.45rem;
  border-radius: var(--xy-radius-sm);
}

.setting-row {
  display: grid;
  grid-template-columns: minmax(160px, 2fr) 3fr;
  align-items: center;
  column-gap: var(--xy-space-lg);
  padding: 0.5rem 1rem;
  border-bottom: 1px solid var(--xy-border);
  transition:
    background-color var(--xy-transition-fast),
    outline-color var(--xy-transition-fast);
  outline: 1px solid transparent;
  outline-offset: -1px;
}

/* Subtle alternating rows for scan-ability. */
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

/* When a description is present, top-align so the value stays paired with the label */
.setting-row:has(.setting-description) {
  align-items: start;
  padding-top: 0.6rem;
}

.setting-edited {
  background-color: color-mix(in srgb, var(--xy-warning) 4%, transparent);
  outline-color: var(--xy-warning-border);
}

.setting-edited:hover {
  outline-color: var(--xy-warning);
}

.setting-invalid,
.setting-invalid:hover {
  outline-color: var(--xy-danger-border);
}

.setting-managed {
  background-color: color-mix(in srgb, var(--xy-accent) 4%, transparent);
}

.setting-key {
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
  color: var(--xy-text-primary);
  min-width: 0;
  overflow-wrap: break-word;
  word-break: break-word;
}

.setting-description {
  font-size: var(--xy-font-size-2xs);
  color: var(--xy-text-muted);
  line-height: 1.4;
  margin-top: 3px;
}

.managed-badge {
  font-size: var(--xy-font-size-2xs);
  margin-left: 0.4rem;
  vertical-align: middle;
}

.field-required {
  color: var(--xy-danger);
  margin-left: 0.15rem;
}

.setting-value {
  min-width: 0;
}

.setting-error {
  margin-top: var(--xy-space-xs);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-danger);
}

.managed-field-display {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 0.55rem 0.7rem;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 35%, transparent);
  border-radius: var(--xy-radius-lg);
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-0));
}

.managed-state-label {
  display: inline-flex;
  align-items: center;
  font-size: var(--xy-font-size-2xs);
  font-weight: 700;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.managed-value {
  font-size: var(--xy-font-size-sm);
  color: var(--xy-text-primary);
  overflow-wrap: break-word;
  word-break: break-word;
}

.managed-source-hint {
  font-size: var(--xy-font-size-2xs);
}

.managed-source-label {
  color: var(--xy-accent);
  font-style: normal;
  font-weight: 500;
}

.inline-toggle {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.toggle-label {
  font-size: var(--xy-font-size-2xs);
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
  border-radius: var(--xy-radius-sm);
}

.inline-input :deep(.q-field__control:hover) {
  border-color: color-mix(in srgb, var(--xy-primary) 50%, var(--xy-border));
}

.inline-input :deep(.q-field--focused .q-field__control) {
  border-color: var(--xy-primary);
}

/* ---- Reduced motion ---- */
@media (prefers-reduced-motion: reduce) {
  .setting-row,
  .group-tab,
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
@media (max-width: 599px) {
  .editor-header {
    padding: var(--xy-space-sm);
  }

  .editor-file-name {
    font-size: var(--xy-font-size-sm);
  }

  .search-input {
    order: 3;
    flex-basis: 100%;
  }

  .validation-banner {
    margin: var(--xy-space-xs) var(--xy-space-sm);
  }

  .group-tab {
    padding: 0.55rem 1rem;
  }

  .setting-row {
    grid-template-columns: minmax(0, 1fr);
    row-gap: 0.15rem;
    padding: 0.5rem var(--xy-space-sm);
  }

  .inline-input {
    max-width: none;
  }
}
</style>
