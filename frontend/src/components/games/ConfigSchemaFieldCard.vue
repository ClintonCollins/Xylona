<template>
  <q-card bordered class="field-card" flat>
    <q-card-section
      :aria-expanded="expanded"
      class="field-card-header"
      role="button"
      tabindex="0"
      @click="expanded = !expanded"
      @keydown.enter="expanded = !expanded"
      @keydown.space.prevent="expanded = !expanded">
      <div class="field-card-summary">
        <q-icon
          :name="expanded ? 'expand_less' : 'expand_more'"
          class="text-xy-muted expand-icon"
          size="sm" />
        <span class="field-card-key font-mono">{{ field.key || '(unnamed)' }}</span>
        <q-badge
          v-if="field.type"
          :label="field.type"
          class="field-type-badge"
          color="primary"
          outline />
        <q-badge
          v-if="field.managed"
          class="field-managed-badge"
          color="accent"
          label="Managed"
          outline />
        <span
          v-if="field.title && field.title !== field.key"
          class="field-card-label text-xy-secondary">
          {{ field.title }}
        </span>
      </div>
      <div class="field-card-actions" @click.stop>
        <q-btn
          class="text-xy-muted"
          dense
          flat
          icon="arrow_upward"
          round
          size="xs"
          @click="$emit('move-up')">
          <q-tooltip>Move up</q-tooltip>
        </q-btn>
        <q-btn
          class="text-xy-muted"
          dense
          flat
          icon="arrow_downward"
          round
          size="xs"
          @click="$emit('move-down')">
          <q-tooltip>Move down</q-tooltip>
        </q-btn>
        <q-btn color="negative" dense flat icon="delete" round size="sm" @click="$emit('remove')">
          <q-tooltip>Remove field</q-tooltip>
        </q-btn>
      </div>
    </q-card-section>

    <q-slide-transition>
      <div v-show="expanded">
        <q-separator />
        <q-card-section class="field-card-body">
          <div class="field-form column q-gutter-y-sm">
            <div class="row q-col-gutter-sm">
              <q-input
                v-model="field.key"
                class="col-6"
                dense
                input-class="font-mono"
                label="Key"
                outlined
                @update:model-value="emitUpdate" />
              <q-input
                v-model="field.title"
                class="col-6"
                dense
                label="Display Label"
                outlined
                @update:model-value="emitUpdate" />
            </div>

            <div class="row q-col-gutter-sm">
              <q-select
                v-model="field.type"
                :options="typeOptions"
                class="col-6"
                dense
                emit-value
                label="Type"
                map-options
                outlined
                @update:model-value="handleTypeChange" />
              <q-input
                v-model="field.description"
                class="col-6"
                dense
                label="Description"
                outlined
                @update:model-value="emitUpdate" />
            </div>

            <q-input
              v-model="field.defaultValue"
              dense
              input-class="font-mono"
              label="Default Value"
              outlined
              @update:model-value="emitUpdate" />

            <!-- Validation rules -->
            <div class="validation-section">
              <div class="xy-section-overline">Validation</div>
              <div class="row q-col-gutter-sm">
                <q-toggle
                  v-model="field.required"
                  class="col-auto"
                  dense
                  label="Required"
                  @update:model-value="emitUpdate" />

                <q-input
                  v-if="field.type === 'integer' || field.type === 'number'"
                  v-model.number="field.minimum"
                  class="col"
                  dense
                  label="Minimum"
                  outlined
                  type="number"
                  @update:model-value="emitUpdate" />
                <q-input
                  v-if="field.type === 'integer' || field.type === 'number'"
                  v-model.number="field.maximum"
                  class="col"
                  dense
                  label="Maximum"
                  outlined
                  type="number"
                  @update:model-value="emitUpdate" />

                <q-input
                  v-if="field.type === 'string'"
                  v-model.number="field.maxLength"
                  class="col"
                  dense
                  label="Max Length"
                  outlined
                  type="number"
                  @update:model-value="emitUpdate" />
              </div>
            </div>

            <!-- Enum options -->
            <div
              v-if="field.type === 'string' || field.type === 'integer' || field.type === 'number'"
              class="enum-section">
              <q-input
                v-model="field.enumOptionsStr"
                dense
                hint="Comma-separated list of allowed values (leave empty for free-text)"
                label="Enum Options"
                outlined
                @update:model-value="emitUpdate" />
              <q-input
                v-if="field.enumOptionsStr"
                v-model="field.enumLabelsStr"
                dense
                hint="Comma-separated display labels matching each enum value (optional)"
                label="Enum Labels"
                outlined
                @update:model-value="emitUpdate" />
            </div>

            <!-- Managed field -->
            <div class="managed-section">
              <div class="row items-center q-gutter-sm">
                <q-toggle
                  v-model="field.managed"
                  color="accent"
                  dense
                  label="Managed"
                  @update:model-value="emitUpdate" />
                <q-select
                  v-if="field.managed"
                  v-model="field.managedSource"
                  :options="managedSourceOptions"
                  class="managed-source-select"
                  dense
                  emit-value
                  label="Source"
                  map-options
                  outlined
                  @update:model-value="emitUpdate" />
              </div>
            </div>

            <!-- Allow Multiple -->
            <q-toggle
              v-model="field.allowMultiple"
              dense
              label="Allow Multiple Values"
              @update:model-value="emitUpdate" />
          </div>
        </q-card-section>
      </div>
    </q-slide-transition>
  </q-card>
</template>

<script lang="ts" setup>
import { reactive, ref, watch } from 'vue'
import { managedSourceOptions } from '@/components/shared/placeholder-definitions'

export interface SchemaFieldModel {
  key: string
  title: string
  type: string
  description: string
  defaultValue: string
  required: boolean
  minimum: number | null
  maximum: number | null
  maxLength: number | null
  enumOptionsStr: string
  enumLabelsStr: string
  managed: boolean
  managedSource: string
  allowMultiple: boolean
  group: string
  order: number
}

const props = defineProps<{
  modelValue: SchemaFieldModel
  forceExpanded?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [field: SchemaFieldModel]
  remove: []
  'move-up': []
  'move-down': []
}>()

const expanded = ref(false)

const field = reactive<SchemaFieldModel>({ ...props.modelValue })

watch(
  () => props.forceExpanded,
  (val) => {
    if (val !== undefined) {
      expanded.value = val
    }
  },
)

watch(
  () => props.modelValue,
  (newVal) => {
    Object.assign(field, newVal)
  },
)

const typeOptions = [
  { label: 'String', value: 'string' },
  { label: 'Integer', value: 'integer' },
  { label: 'Number', value: 'number' },
  { label: 'Boolean', value: 'boolean' },
]

function handleTypeChange() {
  // Reset type-specific fields
  if (field.type !== 'integer' && field.type !== 'number') {
    field.minimum = null
    field.maximum = null
  }
  if (field.type !== 'string') {
    field.maxLength = null
  }
  if (field.type !== 'string' && field.type !== 'integer' && field.type !== 'number') {
    field.enumOptionsStr = ''
    field.enumLabelsStr = ''
  }
  emitUpdate()
}

function emitUpdate() {
  emit('update:modelValue', { ...field })
}
</script>

<style scoped>
.field-card {
  background-color: var(--xy-surface-0);
  border-color: var(--xy-border);
  transition: border-color var(--xy-transition-fast);
}

.field-card:hover {
  border-color: var(--xy-surface-4);
}

.field-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  cursor: pointer;
  padding: var(--xy-space-sm) var(--xy-space-md);
  user-select: none;
}

.field-card-summary {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  flex: 1;
  min-width: 0;
}

.expand-icon {
  flex-shrink: 0;
}

.field-card-key {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.field-type-badge {
  font-size: 0.55rem;
}

.field-managed-badge {
  font-size: 0.55rem;
}

.field-card-label {
  font-size: 0.75rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.field-card-body {
  padding: var(--xy-space-sm) var(--xy-space-md) var(--xy-space-md);
}

.validation-section,
.enum-section,
.managed-section {
  padding-top: var(--xy-space-xs);
}

.managed-source-select {
  min-width: 180px;
}
</style>
