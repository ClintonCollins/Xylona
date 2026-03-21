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
            :model-value-indeterminate="selectedFields.length > 0 && selectedFields.length < visibleFields.length"
            dense
            size="sm"
            class="select-all-checkbox"
            @update:model-value="toggleSelectAll">
            <q-tooltip>{{ selectedFields.length >= visibleFields.length ? 'Deselect all' : 'Select all' }}</q-tooltip>
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
          <q-menu v-if="bulkGroupSuggestions.length > 0" fit no-parent-event :model-value="bulkGroupSuggestions.length > 0">
            <q-list dense>
              <q-item
                v-for="g in bulkGroupSuggestions"
                :key="g"
                clickable
                @click="bulkGroupName = g; applyBulkGroup()">
                <q-item-section>{{ g }}</q-item-section>
              </q-item>
            </q-list>
          </q-menu>
        </q-input>
        <q-btn flat dense size="xs" label="Clear group" class="text-xy-muted" @click="clearBulkGroup" />
        <q-btn flat dense size="xs" label="Deselect all" class="text-xy-muted" @click="selectedFields = []" />
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
          <div v-if="group.fields.length > 0" class="schema-field-group">
            <div
              class="schema-group-header"
              @click="toggleGroupExpand(group.name)">
              <q-icon
                :name="isGroupExpanded(group.name) ? 'expand_more' : 'chevron_right'"
                size="xs"
                class="text-xy-muted" />
              <span class="schema-group-title">{{ group.displayName }}</span>
              <span class="schema-group-count text-xy-muted">{{ group.fields.length }}</span>
            </div>
            <div v-show="isGroupExpanded(group.name)" class="schema-group-content">
              <div v-for="(field, index) in group.fields" :key="field.key || index" class="field-select-row">
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
                  @remove="removeFieldByRef(field)" />
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
      <div class="json-status-bar" :class="{ 'json-valid': jsonValid, 'json-invalid': !jsonValid }">
        <q-icon :name="jsonValid ? 'check_circle' : 'error'" size="xs" />
        <span>{{ jsonValid ? 'Valid JSON' : jsonError }}</span>
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
  [key: string]: unknown
}

interface JsonSchema {
  type: string
  properties: Record<string, SchemaProperty>
  required?: string[]
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
const jsonFieldCount = ref(0)
const monacoContainer = ref<HTMLElement | null>(null)

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
  return Object.entries(schema.properties).map(([key, prop]) => ({
    key,
    title: prop.title || '',
    type: prop.type || 'string',
    description: prop.description || '',
    defaultValue: prop.default !== undefined ? String(prop.default) : '',
    required: requiredFields.has(key),
    minimum: prop.minimum ?? null,
    maximum: prop.maximum ?? null,
    maxLength: prop.maxLength ?? null,
    enumOptionsStr: prop.enum ? prop.enum.join(', ') : '',
    managed: !!prop['x-managed'],
    managedSource: prop['x-managed']?.source || '',
    allowMultiple: !!prop['x-allow-multiple'],
    group: prop['x-group'] || '',
  }))
}

function fieldsToSchema(): JsonSchema {
  const properties: Record<string, SchemaProperty> = {}
  const required: string[] = []

  for (const field of fields.value) {
    if (!field.key) continue

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
    if (field.managed && field.managedSource) {
      prop['x-managed'] = { source: field.managedSource }
    }
    if (field.allowMultiple) {
      prop['x-allow-multiple'] = true
    }
    if (field.group) {
      prop['x-group'] = field.group.toLowerCase()
    }
    if (field.required) {
      required.push(field.key)
    }

    properties[field.key] = prop
  }

  const schema: JsonSchema = { type: 'object', properties }
  if (required.length > 0) {
    schema.required = required
  }
  return schema
}

function addField() {
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
    managed: false,
    managedSource: '',
    allowMultiple: false,
    group: '',
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

const displayGroups = computed(() => groupFields(filteredFields.value))

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

function applyBulkGroup() {
  const groupName = bulkGroupName.value.trim().toLowerCase()
  if (!groupName) return

  for (const field of selectedFields.value) {
    field.group = groupName
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
    const parsed = JSON.parse(jsonStr) as JsonSchema
    jsonValid.value = true
    jsonError.value = ''
    jsonFieldCount.value = parsed.properties ? Object.keys(parsed.properties).length : 0
  } catch (err) {
    jsonValid.value = false
    jsonError.value = err instanceof Error ? err.message : 'Invalid JSON'
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
    managed: false,
    managedSource: '',
    allowMultiple: field.allowMultiple,
    group: field.group || '',
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

function handleSave() {
  if (mode.value === 'json' && monacoEditor) {
    const editor = monacoEditor as import('monaco-editor').editor.IStandaloneCodeEditor
    try {
      const parsed = JSON.parse(editor.getValue()) as JsonSchema
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
  background-color: rgba(28, 183, 207, 0.05);
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
</style>
