<template>
  <div class="schema-editor">
    <!-- Header -->
    <div class="editor-header">
      <div class="editor-header-info">
        <q-btn flat dense round icon="arrow_back" size="sm" @click="$emit('back')" />
        <div>
          <div class="editor-title font-display">Schema Editor</div>
          <div class="editor-subtitle text-xy-secondary font-mono">{{ filePath }}</div>
        </div>
      </div>
      <div class="editor-header-actions">
        <q-btn-toggle
          v-model="mode"
          flat
          dense
          toggle-color="primary"
          :options="[
            { label: 'Form Builder', value: 'form' },
            { label: 'Raw JSON', value: 'json' },
          ]"
          class="mode-toggle" />
        <q-btn color="primary" label="Save Schema" icon="save" size="sm" @click="handleSave" />
      </div>
    </div>

    <q-separator />

    <!-- Form Builder Mode -->
    <div v-if="mode === 'form'" class="form-builder">
      <div class="form-builder-toolbar">
        <div class="toolbar-left">
          <q-checkbox
            v-if="fields.length > 0"
            :model-value="visibleFields.length > 0 && selectedFields.length >= visibleFields.length"
            :indeterminate-value="true"
            :model-value-indeterminate="
              selectedFields.length > 0 && selectedFields.length < visibleFields.length
            "
            dense
            size="sm"
            class="select-all-checkbox"
            @update:model-value="toggleSelectAll">
            <q-tooltip>{{
              selectedFields.length >= visibleFields.length ? 'Deselect all' : 'Select all'
            }}</q-tooltip>
          </q-checkbox>
          <span class="text-xy-secondary field-count">
            {{ fields.length }} field{{ fields.length !== 1 ? 's' : '' }}
          </span>
          <q-btn
            v-if="fields.length > 0"
            flat
            dense
            round
            icon="unfold_more"
            size="xs"
            class="text-xy-muted"
            @click="forceExpanded = true">
            <q-tooltip>Expand all</q-tooltip>
          </q-btn>
          <q-btn
            v-if="fields.length > 0"
            flat
            dense
            round
            icon="unfold_less"
            size="xs"
            class="text-xy-muted"
            @click="forceExpanded = false">
            <q-tooltip>Collapse all</q-tooltip>
          </q-btn>
        </div>
        <div class="toolbar-actions">
          <q-btn
            outline
            color="accent"
            label="Import Fields"
            icon="upload_file"
            size="sm"
            @click="showImportDialog = true" />
          <q-btn outline color="primary" label="Add Field" icon="add" size="sm" @click="addField" />
        </div>
      </div>

      <!-- Search and filters -->
      <div v-if="fields.length > 0" class="schema-search">
        <q-input
          v-model="searchQuery"
          dense
          outlined
          placeholder="Search fields..."
          class="schema-search-input"
          clearable
          @clear="searchQuery = ''">
          <template #prepend>
            <q-icon name="search" size="xs" class="text-xy-muted" />
          </template>
        </q-input>
        <q-btn-toggle
          v-model="managedFilter"
          flat
          dense
          no-caps
          toggle-color="accent"
          size="xs"
          :options="[
            { label: 'All', value: 'all' },
            { label: 'Managed', value: 'managed' },
            { label: 'Unmanaged', value: 'unmanaged' },
          ]"
          class="schema-filter-toggle" />
        <q-btn-toggle
          v-model="requiredFilter"
          flat
          dense
          no-caps
          toggle-color="accent"
          size="xs"
          :options="[
            { label: 'All', value: 'all' },
            { label: 'Required', value: 'required' },
            { label: 'Optional', value: 'optional' },
          ]"
          class="schema-filter-toggle" />
        <span v-if="hasActiveFilters" class="text-xy-muted schema-search-count">
          {{ filteredFields.length }} / {{ fields.length }}
        </span>
      </div>

      <!-- Selection toolbar -->
      <div v-if="selectedFields.length > 0" class="selection-toolbar">
        <span class="text-xy-secondary selection-count">
          {{ selectedFields.length }} selected
        </span>
        <q-input
          v-model="bulkGroupName"
          dense
          outlined
          placeholder="Group name..."
          class="bulk-group-input"
          @keyup.enter="applyBulkGroup">
          <template #append>
            <q-btn
              flat
              dense
              round
              icon="check"
              size="xs"
              color="primary"
              :disable="!bulkGroupName.trim()"
              @click="applyBulkGroup">
              <q-tooltip>Apply group</q-tooltip>
            </q-btn>
          </template>
          <q-menu
            v-if="bulkGroupSuggestions.length > 0"
            fit
            no-parent-event
            :model-value="bulkGroupSuggestions.length > 0">
            <q-list dense>
              <q-item
                v-for="g in bulkGroupSuggestions"
                :key="g"
                clickable
                @click="selectBulkGroupSuggestion(g)">
                <q-item-section>{{ g }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-input>
        <q-btn
          flat
          dense
          size="xs"
          label="Clear group"
          class="text-xy-muted"
          @click="clearBulkGroup" />
        <q-btn
          flat
          dense
          size="xs"
          label="Deselect all"
          class="text-xy-muted"
          @click="selectedFields = []" />
      </div>

      <div v-if="fields.length === 0" class="form-builder-empty">
        <q-icon name="playlist_add" size="48px" class="text-xy-muted q-mb-md" />
        <div class="text-xy-secondary">No fields defined yet</div>
        <div class="text-caption text-xy-muted q-mt-xs">
          Add fields to define the configuration schema for this file.
        </div>
      </div>

      <div v-else class="form-builder-fields">
        <template v-for="group in displayGroups" :key="group.name">
          <div
            v-if="group.fields.length > 0"
            class="schema-field-group"
            :class="{ 'drag-over-group': dragOverGroup === group.name }"
            @dragover.prevent="onGroupDragOver($event, group.name)"
            @dragleave="onGroupDragLeave($event, group.name)"
            @drop.prevent="onGroupDrop(group.name)">
            <div
              class="schema-group-header"
              :draggable="group.name !== ''"
              :class="{ dragging: draggedGroup === group.name }"
              @click="toggleGroupExpand(group.name)"
              @dragstart="onGroupDragStart($event, group.name)"
              @dragend="onGroupDragEnd">
              <q-icon
                v-if="group.name"
                name="drag_indicator"
                size="xs"
                class="text-xy-muted drag-handle"
                @click.stop />
              <q-icon
                :name="isGroupExpanded(group.name) ? 'expand_more' : 'chevron_right'"
                size="xs"
                class="text-xy-muted" />
              <span class="schema-group-title">{{ group.displayName }}</span>
              <span class="schema-group-count text-xy-muted">{{ group.fields.length }}</span>
              <div class="schema-group-actions" @click.stop>
                <q-btn
                  v-if="group.name"
                  flat
                  dense
                  round
                  icon="arrow_upward"
                  size="xs"
                  class="text-xy-muted group-move-btn"
                  @click="moveGroupUp(group.name)">
                  <q-tooltip>Move group up</q-tooltip>
                </q-btn>
                <q-btn
                  v-if="group.name"
                  flat
                  dense
                  round
                  icon="arrow_downward"
                  size="xs"
                  class="text-xy-muted group-move-btn"
                  @click="moveGroupDown(group.name)">
                  <q-tooltip>Move group down</q-tooltip>
                </q-btn>
              </div>
            </div>
            <div v-show="isGroupExpanded(group.name)" class="schema-group-content">
              <div
                v-for="(field, index) in group.fields"
                :key="field.key || index"
                draggable="true"
                class="field-select-row"
                :class="{
                  dragging: draggedField === field,
                  'drag-over-above': dragOverField === field && dragOverPosition === 'above',
                  'drag-over-below': dragOverField === field && dragOverPosition === 'below',
                }"
                @dragstart="onFieldDragStart($event, field, group.name)"
                @dragend="onFieldDragEnd"
                @dragover.prevent="onFieldDragOver($event, field, group.name)"
                @dragleave="onFieldDragLeave"
                @drop.prevent="onFieldDrop(field, group.name)">
                <q-icon
                  name="drag_indicator"
                  size="xs"
                  class="text-xy-muted drag-handle field-drag-handle" />
                <q-checkbox
                  :model-value="isFieldSelected(field)"
                  dense
                  size="sm"
                  class="field-select-checkbox"
                  @update:model-value="toggleFieldSelection(field)" />
                <config-schema-field-card
                  :model-value="field"
                  :force-expanded="forceExpanded"
                  :available-groups="availableGroups"
                  class="field-select-card"
                  @update:model-value="updateFieldByRef(field, $event)"
                  @remove="removeFieldByRef(field)"
                  @move-up="moveFieldUp(field)"
                  @move-down="moveFieldDown(field)" />
              </div>
            </div>
          </div>
        </template>
      </div>

      <!-- Import Fields Dialog — teleports to body at runtime -->
      <q-dialog v-model="showImportDialog">
        <q-card class="import-dialog">
          <q-card-section class="dialog-header">
            <div class="text-h6 font-display">Import Fields from Sample</div>
          </q-card-section>
          <q-separator />
          <q-card-section>
            <config-import-input @detected="handleImportDetection" />
          </q-card-section>
          <q-separator />
          <q-card-actions align="right" class="q-pa-md">
            <q-btn flat label="Cancel" @click="showImportDialog = false" />
            <q-btn color="primary" label="Import" :disable="!pendingImport" @click="applyImport" />
          </q-card-actions>
        </q-card>
      </q-dialog>
    </div>

    <!-- Raw JSON Mode -->
    <div v-else class="json-editor">
      <div
        class="json-status-bar"
        :class="{
          'json-valid': jsonValid && jsonWarnings.length === 0,
          'json-warning': jsonValid && jsonWarnings.length > 0,
          'json-invalid': !jsonValid,
        }">
        <q-icon
          :name="!jsonValid ? 'error' : jsonWarnings.length > 0 ? 'warning' : 'check_circle'"
          size="xs" />
        <span v-if="!jsonValid">{{ jsonError }}</span>
        <span v-else-if="jsonWarnings.length > 0">{{ jsonWarnings.join('; ') }}</span>
        <span v-else>Valid JSON</span>
        <span v-if="jsonValid" class="text-xy-muted">
          &middot; {{ jsonFieldCount }} field{{ jsonFieldCount !== 1 ? 's' : '' }}
        </span>
      </div>
      <div ref="monacoContainer" class="monaco-container"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useQuasar } from 'quasar'
import ConfigSchemaFieldCard from './ConfigSchemaFieldCard.vue'
import type { SchemaFieldModel } from './ConfigSchemaFieldCard.vue'
import ConfigImportInput from './ConfigImportInput.vue'
import type { ImportDetectionResult, ImportedField } from 'src/utils/config-import'
import { groupFields } from '../game_servers/config-field-helpers'

interface SchemaProperty {
  type?: string
  title?: string
  description?: string
  default?: unknown
  enum?: string[]
  minimum?: number
  maximum?: number
  maxLength?: number
  'x-managed'?: { source: string }
  'x-allow-multiple'?: boolean
  'x-group'?: string
  'x-order'?: number
  [key: string]: unknown
}

interface JsonSchema {
  type: string
  properties: Record<string, SchemaProperty>
  required?: string[]
  'x-groups'?: string[]
}

const props = defineProps<{
  filePath: string
  schema: JsonSchema
}>()

const emit = defineEmits<{
  save: [schema: JsonSchema]
  back: []
}>()

const $q = useQuasar()
const showImportDialog = ref(false)
const pendingImport = ref<ImportDetectionResult | null>(null)

const mode = ref<'form' | 'json'>('form')
const forceExpanded = ref<boolean | undefined>(undefined)
const fields = ref<SchemaFieldModel[]>([])
const jsonValid = ref(true)
const jsonError = ref('')
const jsonWarnings = ref<string[]>([])
const jsonFieldCount = ref(0)
const monacoContainer = ref<HTMLElement | null>(null)
const groupOrder = ref<string[]>([])

let monacoEditor: unknown = null
let monacoModule: typeof import('monaco-editor') | null = null

// Convert schema to field models on init
onMounted(() => {
  fields.value = schemaToFields(props.schema)
})

watch(
  () => props.schema,
  (newSchema) => {
    fields.value = schemaToFields(newSchema)
  },
)

// Sync between modes
watch(mode, async (newMode) => {
  if (newMode === 'json') {
    const schema = fieldsToSchema()
    const jsonStr = JSON.stringify(schema, null, 2)
    await nextTick()
    await initMonaco(jsonStr)
  } else {
    // Sync from JSON back to form
    if (monacoEditor && monacoModule) {
      const editor = monacoEditor as import('monaco-editor').editor.IStandaloneCodeEditor
      const jsonStr = editor.getValue()
      try {
        const parsed = JSON.parse(jsonStr) as JsonSchema
        fields.value = schemaToFields(parsed)
        jsonValid.value = true
      } catch {
        // Keep existing fields if JSON is invalid
      }
      disposeMonaco()
    }
  }
})

onUnmounted(() => {
  disposeMonaco()
})

function schemaToFields(schema: JsonSchema): SchemaFieldModel[] {
  if (!schema?.properties) return []
  const requiredFields = new Set(schema.required || [])

  // Initialize group order from schema.
  groupOrder.value = schema['x-groups'] ? [...schema['x-groups']] : []

  const result = Object.entries(schema.properties).map(([key, prop], index) => ({
    key,
    title: prop.title || '',
    type: prop.type || 'string',
    description: prop.description || '',
    defaultValue: prop.default !== undefined ? String(prop.default) : '',
    required: requiredFields.has(key),
    minimum: prop.minimum ?? null,
    maximum: prop.maximum ?? null,
    maxLength: prop.maxLength ?? null,
    enumOptionsStr: prop.enum ? prop.enum.map((v: unknown) => String(v)).join(', ') : '',
    enumLabelsStr: Array.isArray(prop['x-enum-labels'])
      ? (prop['x-enum-labels'] as unknown[]).map(String).join(', ')
      : '',
    managed: !!prop['x-managed'],
    managedSource: prop['x-managed']?.source || '',
    allowMultiple: !!prop['x-allow-multiple'],
    group: prop['x-group'] || '',
    order: prop['x-order'] ?? index,
  }))

  // Sort by order so the form builder reflects schema ordering.
  result.sort((a, b) => a.order - b.order)

  // If no explicit x-groups was set, derive group order from the sorted fields.
  if (groupOrder.value.length === 0) {
    const seen = new Set<string>()
    for (const field of result) {
      if (field.group && !seen.has(field.group)) {
        groupOrder.value.push(field.group)
        seen.add(field.group)
      }
    }
  }

  return result
}

function fieldsToSchema(): JsonSchema {
  const properties: Record<string, SchemaProperty> = {}
  const required: string[] = []

  for (let i = 0; i < fields.value.length; i++) {
    const field = fields.value[i]
    if (!field.key) continue

    // Assign order from current position.
    field.order = i

    const prop: SchemaProperty = {
      type: field.type || 'string',
    }

    if (field.title) prop.title = field.title
    if (field.description) prop.description = field.description
    if (field.defaultValue) {
      if (field.type === 'integer' || field.type === 'number') {
        prop.default = Number(field.defaultValue)
      } else if (field.type === 'boolean') {
        prop.default = field.defaultValue === 'true'
      } else {
        prop.default = field.defaultValue
      }
    }
    if (field.minimum !== null && field.minimum !== undefined) prop.minimum = field.minimum
    if (field.maximum !== null && field.maximum !== undefined) prop.maximum = field.maximum
    if (field.maxLength !== null && field.maxLength !== undefined) prop.maxLength = field.maxLength
    if (field.enumOptionsStr) {
      prop.enum = field.enumOptionsStr
        .split(',')
        .map((s) => s.trim())
        .filter((s) => s)
    }
    if (field.enumLabelsStr) {
      prop['x-enum-labels'] = field.enumLabelsStr
        .split(',')
        .map((s) => s.trim())
        .filter((s) => s)
    }
    if (field.managed && field.managedSource) {
      prop['x-managed'] = { source: field.managedSource }
    }
    if (field.allowMultiple) {
      prop['x-allow-multiple'] = true
    }
    if (field.group) {
      prop['x-group'] = field.group.toLowerCase()
    }
    prop['x-order'] = field.order
    if (field.required) {
      required.push(field.key)
    }

    properties[field.key] = prop
  }

  const schema: JsonSchema = { type: 'object', properties }
  if (groupOrder.value.length > 0) {
    schema['x-groups'] = groupOrder.value
  }
  if (required.length > 0) {
    schema.required = required
  }
  return schema
}

function addField() {
  const maxOrder = fields.value.reduce((max, f) => Math.max(max, f.order), -1)
  fields.value.push({
    key: '',
    title: '',
    type: 'string',
    description: '',
    defaultValue: '',
    required: false,
    minimum: null,
    maximum: null,
    maxLength: null,
    enumOptionsStr: '',
    enumLabelsStr: '',
    managed: false,
    managedSource: '',
    allowMultiple: false,
    group: '',
    order: maxOrder + 1,
  })
}

const availableGroups = computed(() => [
  ...new Set(fields.value.map((f) => f.group).filter((g) => g)),
])

// Search / filter
const searchQuery = ref('')
const managedFilter = ref<'all' | 'managed' | 'unmanaged'>('all')
const requiredFilter = ref<'all' | 'required' | 'optional'>('all')

const hasActiveFilters = computed(
  () => searchQuery.value.trim() || managedFilter.value !== 'all' || requiredFilter.value !== 'all',
)

const filteredFields = computed(() => {
  let result = fields.value

  if (searchQuery.value.trim()) {
    const lower = searchQuery.value.toLowerCase()
    result = result.filter(
      (f) =>
        f.key.toLowerCase().includes(lower) ||
        f.title.toLowerCase().includes(lower) ||
        f.group.toLowerCase().includes(lower),
    )
  }

  if (managedFilter.value === 'managed') {
    result = result.filter((f) => f.managed)
  } else if (managedFilter.value === 'unmanaged') {
    result = result.filter((f) => !f.managed)
  }

  if (requiredFilter.value === 'required') {
    result = result.filter((f) => f.required)
  } else if (requiredFilter.value === 'optional') {
    result = result.filter((f) => !f.required)
  }

  return result
})

const visibleFields = computed(() => filteredFields.value)

const displayGroups = computed(() => groupFields(filteredFields.value, groupOrder.value))

// Group expand/collapse
const expandedGroups = reactive(new Map<string, boolean>())

function isGroupExpanded(name: string): boolean {
  return expandedGroups.get(name) ?? true
}

function toggleGroupExpand(name: string) {
  expandedGroups.set(name, !isGroupExpanded(name))
}

// Multi-select + bulk group assignment
const selectedFields = ref<SchemaFieldModel[]>([])
const bulkGroupName = ref('')

const bulkGroupSuggestions = computed(() => {
  if (!bulkGroupName.value) return availableGroups.value
  const lower = bulkGroupName.value.toLowerCase()
  return availableGroups.value.filter(
    (g) => g.toLowerCase().includes(lower) && g.toLowerCase() !== bulkGroupName.value.toLowerCase(),
  )
})

function toggleSelectAll() {
  const visible = visibleFields.value
  const allVisible = visible.every((f) => selectedFields.value.includes(f))
  if (allVisible) {
    // Deselect only visible fields
    selectedFields.value = selectedFields.value.filter((f) => !visible.includes(f))
  } else {
    // Select all visible fields (keep any already-selected non-visible ones)
    const existing = new Set(selectedFields.value)
    for (const f of visible) {
      existing.add(f)
    }
    selectedFields.value = [...existing]
  }
}

function isFieldSelected(field: SchemaFieldModel): boolean {
  return selectedFields.value.includes(field)
}

function toggleFieldSelection(field: SchemaFieldModel) {
  const idx = selectedFields.value.indexOf(field)
  if (idx === -1) {
    selectedFields.value.push(field)
  } else {
    selectedFields.value.splice(idx, 1)
  }
}

function selectBulkGroupSuggestion(g: string) {
  bulkGroupName.value = g
  applyBulkGroup()
}

function applyBulkGroup() {
  const groupName = bulkGroupName.value.trim().toLowerCase()
  if (!groupName) return

  for (const field of selectedFields.value) {
    field.group = groupName
  }

  // Add to groupOrder if not already present.
  if (!groupOrder.value.includes(groupName)) {
    groupOrder.value.push(groupName)
  }

  selectedFields.value = []
  bulkGroupName.value = ''
}

function clearBulkGroup() {
  for (const field of selectedFields.value) {
    field.group = ''
  }
  selectedFields.value = []
  bulkGroupName.value = ''
}

function updateFieldByRef(original: SchemaFieldModel, updated: SchemaFieldModel) {
  const index = fields.value.indexOf(original)
  if (index !== -1) {
    fields.value[index] = updated
  }
}

function removeFieldByRef(field: SchemaFieldModel) {
  const index = fields.value.indexOf(field)
  if (index !== -1) {
    fields.value.splice(index, 1)
  }
}

function moveFieldUp(field: SchemaFieldModel) {
  const sameGroup = fields.value.filter((f) => f.group === field.group)
  const groupIdx = sameGroup.indexOf(field)
  if (groupIdx <= 0) return

  const prev = sameGroup[groupIdx - 1]
  const flatIdx = fields.value.indexOf(field)
  const prevFlatIdx = fields.value.indexOf(prev)

  fields.value.splice(flatIdx, 1)
  fields.value.splice(prevFlatIdx, 0, field)
}

function moveFieldDown(field: SchemaFieldModel) {
  const sameGroup = fields.value.filter((f) => f.group === field.group)
  const groupIdx = sameGroup.indexOf(field)
  if (groupIdx < 0 || groupIdx >= sameGroup.length - 1) return

  const next = sameGroup[groupIdx + 1]
  const flatIdx = fields.value.indexOf(field)
  const nextFlatIdx = fields.value.indexOf(next)

  fields.value.splice(flatIdx, 1)
  fields.value.splice(nextFlatIdx, 0, field)
}

/**
 * Ensures groupOrder contains all current groups. If a group exists in the
 * fields but is missing from groupOrder (e.g. added via bulk assignment after
 * initial load), it gets appended. This prevents move operations from silently
 * failing when groupOrder is out of sync.
 */
function ensureGroupOrder() {
  const seen = new Set(groupOrder.value)
  for (const field of fields.value) {
    if (field.group && !seen.has(field.group)) {
      groupOrder.value.push(field.group)
      seen.add(field.group)
    }
  }
}

function moveGroupUp(groupName: string) {
  ensureGroupOrder()
  const idx = groupOrder.value.indexOf(groupName)
  if (idx <= 0) return
  groupOrder.value.splice(idx, 1)
  groupOrder.value.splice(idx - 1, 0, groupName)
}

function moveGroupDown(groupName: string) {
  ensureGroupOrder()
  const idx = groupOrder.value.indexOf(groupName)
  if (idx < 0 || idx >= groupOrder.value.length - 1) return
  groupOrder.value.splice(idx, 1)
  groupOrder.value.splice(idx + 1, 0, groupName)
}

// --- Drag-and-drop: fields within groups ---
const draggedField = ref<SchemaFieldModel | null>(null)
const draggedFieldGroup = ref('')
const dragOverField = ref<SchemaFieldModel | null>(null)
const dragOverPosition = ref<'above' | 'below'>('below')

function onFieldDragStart(event: DragEvent, field: SchemaFieldModel, groupName: string) {
  draggedField.value = field
  draggedFieldGroup.value = groupName
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', 'field')
  }
}

function onFieldDragEnd() {
  draggedField.value = null
  draggedFieldGroup.value = ''
  dragOverField.value = null
}

function onFieldDragOver(event: DragEvent, targetField: SchemaFieldModel, targetGroup: string) {
  if (!draggedField.value || draggedField.value === targetField) return
  // Only allow drag within the same group.
  if (draggedFieldGroup.value !== targetGroup) return

  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const midY = rect.top + rect.height / 2
  dragOverPosition.value = event.clientY < midY ? 'above' : 'below'
  dragOverField.value = targetField
}

function onFieldDragLeave() {
  dragOverField.value = null
}

function onFieldDrop(targetField: SchemaFieldModel, targetGroup: string) {
  if (!draggedField.value || draggedField.value === targetField) return
  if (draggedFieldGroup.value !== targetGroup) return

  const fromIdx = fields.value.indexOf(draggedField.value)
  if (fromIdx === -1) return

  // Remove from current position.
  fields.value.splice(fromIdx, 1)

  // Find the target index after removal.
  let toIdx = fields.value.indexOf(targetField)
  if (toIdx === -1) return

  if (dragOverPosition.value === 'below') {
    toIdx++
  }
  fields.value.splice(toIdx, 0, draggedField.value)

  draggedField.value = null
  draggedFieldGroup.value = ''
  dragOverField.value = null
}

// --- Drag-and-drop: groups ---
const draggedGroup = ref<string | null>(null)
const dragOverGroup = ref<string | null>(null)

function onGroupDragStart(event: DragEvent, groupName: string) {
  if (!groupName) return // Don't drag General group.
  draggedGroup.value = groupName
  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', 'group')
  }
}

function onGroupDragEnd() {
  draggedGroup.value = null
  dragOverGroup.value = null
}

function onGroupDragOver(_event: DragEvent, groupName: string) {
  if (!draggedGroup.value || draggedGroup.value === groupName) return
  if (!groupName) return // Can't drop before General.
  dragOverGroup.value = groupName
}

function onGroupDragLeave(event: DragEvent, groupName: string) {
  // Only clear if actually leaving this group (not entering a child).
  const related = event.relatedTarget as HTMLElement | null
  const current = event.currentTarget as HTMLElement | null
  if (current && related && current.contains(related)) return
  if (dragOverGroup.value === groupName) {
    dragOverGroup.value = null
  }
}

function onGroupDrop(targetGroupName: string) {
  if (!draggedGroup.value || draggedGroup.value === targetGroupName) return
  if (!targetGroupName) return

  ensureGroupOrder()
  const fromIdx = groupOrder.value.indexOf(draggedGroup.value)
  const toIdx = groupOrder.value.indexOf(targetGroupName)
  if (fromIdx === -1 || toIdx === -1) return

  groupOrder.value.splice(fromIdx, 1)
  groupOrder.value.splice(toIdx, 0, draggedGroup.value)

  draggedGroup.value = null
  dragOverGroup.value = null
}

async function initMonaco(content: string) {
  if (!monacoContainer.value) return

  try {
    monacoModule = await import('monaco-editor')
    monacoEditor = monacoModule.editor.create(monacoContainer.value, {
      value: content,
      language: 'json',
      theme: 'vs-dark',
      minimap: { enabled: false },
      fontSize: 13,
      fontFamily: 'JetBrains Mono, monospace',
      lineNumbers: 'on',
      scrollBeyondLastLine: false,
      automaticLayout: true,
      tabSize: 2,
    })

    const editor = monacoEditor as import('monaco-editor').editor.IStandaloneCodeEditor
    editor.onDidChangeModelContent(() => {
      validateJson(editor.getValue())
    })
    validateJson(content)
  } catch {
    // Monaco may not be available
  }
}

function disposeMonaco() {
  if (monacoEditor) {
    const editor = monacoEditor as import('monaco-editor').editor.IStandaloneCodeEditor
    editor.dispose()
    monacoEditor = null
  }
}

function validateJson(jsonStr: string) {
  try {
    const parsed: unknown = JSON.parse(jsonStr)
    const warnings: string[] = []

    // Detect if user pasted a ConfigSchemaEntry array instead of a schema object
    if (Array.isArray(parsed)) {
      jsonValid.value = false
      jsonError.value =
        'Expected a schema object { "type": "object", "properties": {...} }, not an array. ' +
        'Did you paste the full config schemas array? This editor handles a single schema.'
      jsonWarnings.value = []
      jsonFieldCount.value = 0
      return
    }

    if (typeof parsed !== 'object' || parsed === null) {
      jsonValid.value = false
      jsonError.value = 'Expected a JSON object'
      jsonWarnings.value = []
      jsonFieldCount.value = 0
      return
    }

    const obj = parsed as Record<string, unknown>

    // Detect if user pasted a ConfigSchemaEntry (has path/format but no properties)
    if ('path' in obj && 'format' in obj && !('properties' in obj)) {
      jsonValid.value = false
      jsonError.value =
        'This looks like a config schema entry (has "path" and "format"). ' +
        'This editor expects only the inner "schema" object with "type" and "properties".'
      jsonWarnings.value = []
      jsonFieldCount.value = 0
      return
    }

    // Warn if missing expected structure
    if (obj.type !== 'object') {
      warnings.push('Missing or unexpected "type" (expected "object")')
    }
    if (!obj.properties || typeof obj.properties !== 'object') {
      warnings.push('Missing "properties" object')
    }

    // Check for non-string enum values (backend requires string enums)
    if (obj.properties && typeof obj.properties === 'object') {
      const props = obj.properties as Record<string, Record<string, unknown>>
      let nonStringEnumCount = 0
      for (const [, prop] of Object.entries(props)) {
        if (Array.isArray(prop.enum)) {
          const hasNonString = prop.enum.some((v: unknown) => typeof v !== 'string')
          if (hasNonString) {
            nonStringEnumCount++
          }
        }
      }
      if (nonStringEnumCount > 0) {
        warnings.push(
          `${nonStringEnumCount} field${nonStringEnumCount !== 1 ? 's have' : ' has'} non-string enum values (will be auto-converted on save)`,
        )
      }
    }

    const schema = obj as JsonSchema
    jsonValid.value = true
    jsonError.value = ''
    jsonWarnings.value = warnings
    jsonFieldCount.value = schema.properties ? Object.keys(schema.properties).length : 0
  } catch (err) {
    jsonValid.value = false
    jsonError.value = err instanceof Error ? err.message : 'Invalid JSON'
    jsonWarnings.value = []
    jsonFieldCount.value = 0
  }
}

function handleImportDetection(result: ImportDetectionResult) {
  pendingImport.value = result
}

function importedFieldToFieldModel(field: ImportedField): SchemaFieldModel {
  return {
    key: field.key,
    title: field.title,
    type: field.type,
    description: '',
    defaultValue: field.value !== null && field.value !== undefined ? String(field.value) : '',
    required: false,
    minimum: null,
    maximum: null,
    maxLength: null,
    enumOptionsStr: '',
    enumLabelsStr: '',
    managed: false,
    managedSource: '',
    allowMultiple: field.allowMultiple,
    group: field.group || '',
    order: fields.value.length,
  }
}

function applyImport() {
  if (!pendingImport.value?.fields) return

  const existingKeys = new Set(fields.value.map((f) => f.key))
  let added = 0
  let skipped = 0

  for (const field of pendingImport.value.fields) {
    if (existingKeys.has(field.key)) {
      skipped++
    } else {
      fields.value.push(importedFieldToFieldModel(field))
      added++
    }
  }

  showImportDialog.value = false
  pendingImport.value = null

  $q.notify({
    type: 'positive',
    message: `Added ${added} new field${added !== 1 ? 's' : ''}${skipped > 0 ? `, ${skipped} already existed (skipped)` : ''}`,
    position: 'bottom',
  })
}

/**
 * Normalizes a parsed schema object to ensure backend compatibility.
 * - Converts non-string enum values to strings (backend expects []string).
 * - Ensures top-level structure has type and properties.
 */
function normalizeSchema(schema: JsonSchema): JsonSchema {
  const normalized: JsonSchema = {
    type: schema.type || 'object',
    properties: {},
  }
  if (schema.required && schema.required.length > 0) {
    normalized.required = schema.required
  }
  if (schema['x-groups'] && schema['x-groups'].length > 0) {
    normalized['x-groups'] = schema['x-groups']
  }

  if (!schema.properties) return normalized

  for (const [key, prop] of Object.entries(schema.properties)) {
    const normalizedProp = { ...prop }

    // Convert non-string enum values to strings
    if (Array.isArray(normalizedProp.enum)) {
      normalizedProp.enum = normalizedProp.enum.map((v: unknown) => String(v))
    }

    normalized.properties[key] = normalizedProp
  }

  return normalized
}

function handleSave() {
  if (mode.value === 'json' && monacoEditor) {
    const editor = monacoEditor as import('monaco-editor').editor.IStandaloneCodeEditor
    try {
      const raw: unknown = JSON.parse(editor.getValue())

      // Block saving arrays or non-objects
      if (Array.isArray(raw) || typeof raw !== 'object' || raw === null) {
        $q.notify({
          type: 'xylona-error',
          caption: 'Schema must be a JSON object with "type" and "properties", not an array.',
          position: 'top',
          timeout: 5000,
        })
        return
      }

      const parsed = normalizeSchema(raw as JsonSchema)
      emit('save', parsed)
    } catch {
      // Don't save invalid JSON
      return
    }
  } else {
    emit('save', fieldsToSchema())
  }
}
</script>

<style scoped>
.schema-editor {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--xy-space-sm) var(--xy-space-md);
  gap: var(--xy-space-md);
}

.editor-header-info {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.editor-title {
  font-size: 1rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.editor-subtitle {
  font-size: 0.75rem;
}

.editor-header-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.mode-toggle {
  border: 1px solid var(--xy-border);
  border-radius: 6px;
}

/* Form builder */
.form-builder {
  flex: 1;
  overflow-y: auto;
  padding: var(--xy-space-md);
}

.form-builder-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--xy-space-md);
}

.toolbar-left {
  display: flex;
  align-items: center;
  gap: 2px;
}

.field-count {
  font-size: 0.8rem;
}

.toolbar-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.form-builder-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: var(--xy-space-2xl);
  text-align: center;
}

.form-builder-fields {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.schema-field-group {
  margin-bottom: var(--xy-space-md);
}

.schema-search {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-sm);
}

.schema-search-input {
  flex: 1;
  max-width: 320px;
}

.schema-search-count {
  font-size: 0.75rem;
  white-space: nowrap;
}

.schema-filter-toggle {
  border: 1px solid var(--xy-border);
  border-radius: 6px;
  flex-shrink: 0;
}

.schema-group-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) 0;
  cursor: pointer;
  user-select: none;
}

.schema-group-title {
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.schema-group-count {
  font-size: 0.65rem;
}

.schema-group-content {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

/* Selection UI */
.selection-toolbar {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  margin-bottom: var(--xy-space-sm);
  border: 1px solid var(--xy-accent);
  border-radius: 8px;
  background-color: var(--xy-accent-muted);
}

.selection-count {
  font-size: 0.8rem;
  font-weight: 600;
  white-space: nowrap;
}

.bulk-group-input {
  flex: 1;
  max-width: 240px;
}

.field-select-row {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-xs);
}

.field-select-checkbox {
  margin-top: 10px;
  flex-shrink: 0;
}

.field-select-card {
  flex: 1;
  min-width: 0;
}

/* JSON editor */
.json-editor {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

.json-status-bar {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) var(--xy-space-md);
  font-size: 0.75rem;
  border-bottom: 1px solid var(--xy-border);
}

.json-valid {
  color: var(--xy-success);
}

.json-warning {
  color: var(--xy-warning);
}

.json-invalid {
  color: var(--xy-danger);
}

.monaco-container {
  flex: 1;
  min-height: 70vh;
}

/* Import dialog */
.import-dialog {
  min-width: 500px;
  max-width: 640px;
  background-color: var(--xy-surface-1);
}

.import-dialog .dialog-header {
  padding: var(--xy-space-md);
}

.schema-group-actions {
  display: flex;
  align-items: center;
  margin-left: auto;
}

.group-move-btn {
  opacity: 0;
  transition: opacity var(--xy-transition-fast);
}

.schema-group-header:hover .group-move-btn {
  opacity: 1;
}

/* Drag-and-drop */
.drag-handle {
  cursor: grab;
  flex-shrink: 0;
  opacity: 0.4;
  transition: opacity var(--xy-transition-fast);
}

.drag-handle:active {
  cursor: grabbing;
}

.schema-group-header:hover .drag-handle,
.field-select-row:hover > .drag-handle {
  opacity: 1;
}

.field-drag-handle {
  margin-top: 10px;
}

/* Field drag states */
.field-select-row.dragging {
  opacity: 0.3;
}

.field-select-row.drag-over-above {
  border-top: 2px solid var(--xy-accent);
}

.field-select-row.drag-over-below {
  border-bottom: 2px solid var(--xy-accent);
}

/* Group drag states */
.schema-group-header.dragging {
  opacity: 0.3;
}

.schema-field-group.drag-over-group {
  outline: 2px dashed var(--xy-accent);
  outline-offset: 2px;
  border-radius: 8px;
}
</style>
