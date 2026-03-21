<template>
  <div class="config-editor">
    <!-- Header -->
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
          <span class="text-xy-muted editor-meta-text">{{ fields.length }} fields</span>
          <span v-if="managedCount > 0" class="text-xy-muted editor-meta-text">
            {{ managedCount }} managed
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

    <!-- Validation errors -->
    <q-banner v-if="validationErrors.length > 0" dense class="validation-banner q-mb-sm">
      <template #avatar>
        <q-icon name="error_outline" color="negative" size="sm" />
      </template>
      <div class="validation-errors">
        <div v-for="(error, i) in validationErrors" :key="i" class="validation-error-item">
          <strong>{{ error.field }}:</strong> {{ error.message }}
        </div>
      </div>
    </q-banner>

    <!-- Fields -->
    <div class="editor-fields">
      <div
        v-if="fields.length === 0 && advancedFields.length === 0"
        class="no-fields text-xy-muted">
        No fields defined in the schema for this file.
      </div>

      <div v-for="field in fields" :key="field.key" class="field-row">
        <!-- Managed field (read-only) -->
        <div v-if="field.isManaged" class="field-managed">
          <div class="field-label-row">
            <label class="field-label">{{ field.title || field.key }}</label>
            <q-badge color="accent" class="field-managed-badge">
              <q-icon name="lock" size="10px" class="q-mr-xs" />
              Managed
            </q-badge>
          </div>
          <q-input
            :model-value="field.value || field.defaultValue"
            dense
            outlined
            readonly
            class="field-input managed-input"
            input-class="font-mono">
            <template #append>
              <q-icon name="lock" size="xs" color="accent">
                <q-tooltip>This field is managed by server settings</q-tooltip>
              </q-icon>
            </template>
          </q-input>
          <div class="field-hint text-xy-muted">
            <span class="managed-source-label">
              Source: {{ getManagedSourceLabel(field.managedSource) }}
            </span>
            — edit in Settings tab.
          </div>
        </div>

        <!-- Editable field -->
        <div v-else class="field-editable">
          <div class="field-label-row">
            <label class="field-label">
              {{ field.title || field.key }}
              <span v-if="field.required" class="field-required">*</span>
            </label>
            <q-badge
              v-if="field.isMissingFromFile"
              outline
              color="info"
              label="Not yet saved"
              class="field-missing-badge" />
          </div>
          <div v-if="field.description" class="field-description text-xy-secondary">
            {{ field.description }}
          </div>

          <!-- Boolean toggle -->
          <q-toggle
            v-if="field.fieldType === 'boolean'"
            :model-value="getFieldValue(field) === 'true'"
            :label="getFieldValue(field) === 'true' ? 'Enabled' : 'Disabled'"
            dense
            color="primary"
            @update:model-value="(val: boolean) => setFieldValue(field.key, String(val))" />

          <!-- Enum dropdown -->
          <q-select
            v-else-if="field.enumOptions.length > 0"
            :model-value="getFieldValue(field)"
            :options="field.enumOptions"
            dense
            outlined
            emit-value
            map-options
            class="field-input"
            @update:model-value="(val: string) => setFieldValue(field.key, val)" />

          <!-- Integer input -->
          <q-input
            v-else-if="field.fieldType === 'integer' || field.fieldType === 'number'"
            :model-value="getFieldValue(field)"
            dense
            outlined
            type="number"
            :rules="getNumberRules(field)"
            class="field-input"
            input-class="font-mono"
            @update:model-value="
              (val: string | number | null) => setFieldValue(field.key, String(val ?? ''))
            " />

          <!-- String input (default) -->
          <q-input
            v-else
            :model-value="getFieldValue(field)"
            dense
            outlined
            :maxlength="field.maxLength ?? undefined"
            :rules="getStringRules(field)"
            class="field-input"
            input-class="font-mono"
            @update:model-value="
              (val: string | number | null) => setFieldValue(field.key, String(val ?? ''))
            " />

          <div
            v-if="field.defaultValue && field.isMissingFromFile"
            class="field-default text-xy-muted">
            Default: <code class="font-mono">{{ field.defaultValue }}</code>
          </div>
        </div>
      </div>

      <!-- Advanced fields -->
      <config-advanced-fields :fields="advancedFields" @update="handleAdvancedUpdate" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { ConfigFieldData, ConfigValidationError, AdvancedField } from '@/proto/xylona_pb'
import { getManagedSourceLabel } from '@/components/shared/placeholder-definitions'
import ConfigAdvancedFields from './ConfigAdvancedFields.vue'

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

const managedCount = computed(() => props.fields.filter((f) => f.isManaged).length)

const hasChanges = computed(() => editedValues.size > 0 || advancedChanged.value)

// Reset edits when file changes
watch(
  () => props.filePath,
  () => {
    editedValues.clear()
    advancedChanged.value = false
  },
)

function getFieldValue(field: ConfigFieldData): string {
  if (editedValues.has(field.key)) {
    return editedValues.get(field.key)!
  }
  return field.value || field.defaultValue
}

function setFieldValue(key: string, value: string) {
  // Find the original field to see if value changed
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

function handleAdvancedUpdate(fields: AdvancedField[]) {
  advancedChanged.value = true
  emit('updateAdvanced', fields)
}

function handleSave() {
  // Build complete field values map (original + edits)
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
  margin-top: 4px;
}

.editor-meta-text {
  font-size: 0.7rem;
}

.editor-category-badge {
  font-size: 0.6rem;
}

.editor-divider {
  background-color: var(--xy-border);
}

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

.editor-fields {
  flex: 1;
  overflow-y: auto;
  padding: var(--xy-space-md);
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.no-fields {
  text-align: center;
  padding: var(--xy-space-xl);
  font-size: 0.85rem;
}

.field-row {
  padding: var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  background-color: var(--xy-surface-0);
  transition: border-color var(--xy-transition-fast);
}

.field-row:hover {
  border-color: var(--xy-surface-4);
}

.field-managed {
  opacity: 0.85;
}

.field-label-row {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-xs);
}

.field-label {
  font-size: 0.8rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.field-required {
  color: var(--xy-danger);
}

.field-managed-badge {
  font-size: 0.55rem;
}

.field-missing-badge {
  font-size: 0.55rem;
}

.field-description {
  font-size: 0.7rem;
  margin-bottom: var(--xy-space-xs);
}

.field-input {
  max-width: 480px;
}

.managed-input :deep(.q-field__control) {
  background-color: var(--xy-surface-1);
  border-color: var(--xy-accent-muted);
}

.field-hint {
  font-size: 0.65rem;
  margin-top: 4px;
  font-style: italic;
}

.managed-source-label {
  color: var(--xy-accent);
  font-style: normal;
  font-weight: 500;
}

.field-default {
  font-size: 0.65rem;
  margin-top: 4px;
}

.field-default code {
  font-size: 0.7rem;
  background-color: var(--xy-surface-2);
  padding: 1px 4px;
  border-radius: 3px;
}
</style>
