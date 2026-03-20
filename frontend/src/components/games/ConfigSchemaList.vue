<template>
  <div class="config-schema-list">
    <div class="schema-list-header">
      <div class="schema-list-title font-display">Configuration Files</div>
      <q-btn
        outline
        color="primary"
        label="Add Config File"
        icon="add"
        size="sm"
        @click="showAddDialog = true" />
    </div>

    <div v-if="configSchemas.length === 0" class="schema-list-empty">
      <div class="text-xy-muted">No configuration files defined yet.</div>
      <div class="text-caption text-xy-muted q-mt-xs">
        Add config files to enable schema-driven configuration editing for game servers.
      </div>
    </div>

    <div v-else class="schema-list-content">
      <div v-for="(files, category) in groupedSchemas" :key="category" class="schema-category">
        <div class="schema-category-header">
          <span
            class="category-dot"
            :style="{ backgroundColor: getCategoryColor(String(category)) }"></span>
          <span class="category-label">{{ category }}</span>
        </div>

        <q-list dense separator class="schema-file-list">
          <q-item v-for="(schema, index) in files" :key="schema.path" class="schema-file-item">
            <q-item-section avatar>
              <q-icon name="description" size="sm" class="text-xy-secondary" />
            </q-item-section>
            <q-item-section>
              <q-item-label class="font-mono schema-file-path">{{ schema.path }}</q-item-label>
              <q-item-label caption>
                <span class="schema-file-meta schema-format-edit" @click.stop>
                  {{ schema.format }}
                  <q-popup-edit
                    v-slot="scope"
                    v-model="schema.format"
                    auto-save
                    @save="
                      (val: string) =>
                        updateSchemaFormat(getGlobalIndex(String(category), index), val)
                    ">
                    <q-select
                      v-model="scope.value"
                      :options="formatOptions"
                      emit-value
                      map-options
                      dense
                      outlined
                      autofocus
                      @update:model-value="scope.set" />
                  </q-popup-edit>
                  <q-icon name="edit" size="10px" class="format-edit-icon" />
                </span>
                <span v-if="getFieldCount(schema)" class="schema-file-meta">
                  &middot; {{ getFieldCount(schema) }} fields
                </span>
                <span v-if="getManagedCount(schema)" class="schema-file-meta">
                  &middot; {{ getManagedCount(schema) }} managed
                </span>
                <q-badge
                  v-if="schema.generate_before_start"
                  outline
                  color="info"
                  label="gen-on-start"
                  class="q-ml-xs schema-gen-badge" />
              </q-item-label>
            </q-item-section>
            <q-item-section side>
              <div class="schema-file-actions">
                <q-btn
                  flat
                  dense
                  round
                  icon="edit"
                  size="sm"
                  @click="$emit('editSchema', getGlobalIndex(String(category), index))">
                  <q-tooltip>Edit schema</q-tooltip>
                </q-btn>
                <q-btn
                  flat
                  dense
                  round
                  icon="delete"
                  size="sm"
                  color="negative"
                  @click="removeSchema(getGlobalIndex(String(category), index))">
                  <q-tooltip>Remove</q-tooltip>
                </q-btn>
              </div>
            </q-item-section>
          </q-item>
        </q-list>
      </div>
    </div>

    <add-config-file-dialog
      v-model="showAddDialog"
      :existing-categories="existingCategories"
      @add="handleAddSchema" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import AddConfigFileDialog from './AddConfigFileDialog.vue'

export interface ConfigSchemaEntry {
  path: string
  format: string
  category: string
  generate_before_start: boolean
  xml_key_mode?: {
    mode: string
    element: string
    key_attr: string
    value_attr: string
  }
  schema?: {
    type: string
    properties: Record<string, SchemaProperty>
  }
}

interface SchemaProperty {
  type?: string
  'x-managed'?: {
    source: string
  }
  [key: string]: unknown
}

const props = defineProps<{
  modelValue: ConfigSchemaEntry[]
}>()

const emit = defineEmits<{
  'update:modelValue': [schemas: ConfigSchemaEntry[]]
  editSchema: [index: number]
}>()

const configSchemas = computed(() => props.modelValue)

const showAddDialog = ref(false)

const CATEGORY_COLORS = [
  '#3B82F6',
  '#22C55E',
  '#F59E0B',
  '#8B5CF6',
  '#EF4444',
  '#06B6D4',
  '#EC4899',
  '#F97316',
]

const existingCategories = computed(() => [...new Set(configSchemas.value.map((s) => s.category))])

const categoryColorMap = computed(() => {
  const map = new Map<string, string>()
  const categories = existingCategories.value
  categories.forEach((cat, i) => {
    map.set(cat, CATEGORY_COLORS[i % CATEGORY_COLORS.length])
  })
  return map
})

const groupedSchemas = computed(() => {
  const groups: Record<string, ConfigSchemaEntry[]> = {}
  for (const schema of configSchemas.value) {
    const cat = schema.category || 'Uncategorized'
    if (!groups[cat]) {
      groups[cat] = []
    }
    groups[cat].push(schema)
  }
  return groups
})

function getCategoryColor(category: string): string {
  return categoryColorMap.value.get(category) || CATEGORY_COLORS[0]
}

function getFieldCount(schema: ConfigSchemaEntry): number {
  if (!schema.schema?.properties) return 0
  return Object.keys(schema.schema.properties).length
}

function getManagedCount(schema: ConfigSchemaEntry): number {
  if (!schema.schema?.properties) return 0
  return Object.values(schema.schema.properties).filter((p) => p['x-managed']).length
}

function getGlobalIndex(category: string, localIndex: number): number {
  // Find the global index in the flat configSchemas array
  let seen = 0
  for (let i = 0; i < configSchemas.value.length; i++) {
    const cat = configSchemas.value[i].category || 'Uncategorized'
    if (cat === category) {
      if (seen === localIndex) {
        return i
      }
      seen++
    }
  }
  return -1
}

function removeSchema(index: number) {
  const updated = [...configSchemas.value]
  updated.splice(index, 1)
  emit('update:modelValue', updated)
}

function handleAddSchema(entry: ConfigSchemaEntry) {
  emit('update:modelValue', [...configSchemas.value, entry])
}

const formatOptions = [
  { label: 'Properties', value: 'properties' },
  { label: 'INI', value: 'ini' },
  { label: 'JSON', value: 'json' },
  { label: 'YAML', value: 'yaml' },
  { label: 'TOML', value: 'toml' },
  { label: 'XML', value: 'xml' },
]

function updateSchemaFormat(globalIndex: number, newFormat: string) {
  if (globalIndex === -1) return
  const updated = [...configSchemas.value]
  updated[globalIndex] = { ...updated[globalIndex], format: newFormat }
  // Remove xml_key_mode if format changed away from XML
  if (newFormat !== 'xml') {
    delete updated[globalIndex].xml_key_mode
  }
  emit('update:modelValue', updated)
}
</script>

<style scoped>
.config-schema-list {
  margin-top: var(--xy-space-md);
}

.schema-list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--xy-space-sm);
}

.schema-list-title {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  letter-spacing: 0.02em;
}

.schema-list-empty {
  padding: var(--xy-space-lg);
  text-align: center;
  border: 1px dashed var(--xy-border);
  border-radius: 8px;
  background-color: var(--xy-surface-0);
}

.schema-list-content {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.schema-category-header {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs) 0;
  font-size: 0.7rem;
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

.schema-file-list {
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  background-color: var(--xy-surface-0);
  overflow: hidden;
}

.schema-file-item {
  min-height: 48px;
}

.schema-file-path {
  font-size: 0.8rem;
  color: var(--xy-text-primary);
}

.schema-file-meta {
  font-size: 0.7rem;
  color: var(--xy-text-muted);
}

.schema-gen-badge {
  font-size: 0.55rem;
}

.schema-file-actions {
  display: flex;
  gap: 2px;
}

.schema-format-edit {
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 1px 4px;
  border-radius: 4px;
  transition: background-color var(--xy-transition-fast);
}

.schema-format-edit:hover {
  background-color: var(--xy-surface-2);
}

.format-edit-icon {
  opacity: 0;
  transition: opacity var(--xy-transition-fast);
  color: var(--xy-text-muted);
}

.schema-format-edit:hover .format-edit-icon {
  opacity: 1;
}
</style>
