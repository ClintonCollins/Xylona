<template>
  <q-dialog v-model="dialogOpen" persistent>
    <q-card class="add-config-dialog">
      <q-card-section class="dialog-header">
        <div class="text-h6 font-display">Add Config File</div>
      </q-card-section>

      <q-separator />

      <q-card-section class="dialog-body">
        <q-form ref="formRef" class="column q-gutter-y-md">
          <q-input
            v-model="entry.path"
            outlined
            dense
            label="File Path"
            hint="Relative to server directory, e.g. server.properties"
            :rules="[(val: string) => !!val || 'Path is required']">
          </q-input>

          <q-select
            v-model="entry.format"
            outlined
            dense
            label="Format"
            :options="formatOptions"
            emit-value
            map-options
            :rules="[(val: string) => !!val || 'Format is required']">
          </q-select>

          <q-input
            v-model="entry.category"
            outlined
            dense
            label="Category"
            hint="Group related files together, e.g. Core, Plugins"
            :rules="[(val: string) => !!val || 'Category is required']">
            <template #append>
              <q-icon v-if="filteredCategories.length > 0" name="arrow_drop_down" />
            </template>
            <q-menu v-if="filteredCategories.length > 0" auto-close fit>
              <q-list dense>
                <q-item
                  v-for="cat in filteredCategories"
                  :key="cat"
                  clickable
                  @click="entry.category = cat">
                  <q-item-section>{{ cat }}</q-item-section>
                </q-item>
              </q-list>
            </q-menu>
          </q-input>

          <q-toggle
            v-model="entry.generate_before_start"
            label="Generate before start"
            dense
            color="primary">
            <q-tooltip>
              Create this file from schema defaults when it doesn't exist and the server starts
            </q-tooltip>
          </q-toggle>

          <!-- XML Key Mode (conditional) -->
          <div v-if="entry.format === 'xml'" class="xml-options">
            <div class="xy-section-overline">XML Key Mode</div>

            <q-option-group
              v-model="xmlKeyMode.mode"
              :options="xmlModeOptions"
              type="radio"
              dense
              inline
              color="primary" />

            <div v-if="xmlKeyMode.mode === 'attributes'" class="xml-attr-fields column q-gutter-y-sm q-mt-sm">
              <q-input
                v-model="xmlKeyMode.element"
                outlined
                dense
                label="Element Name"
                hint="e.g. property"
                :rules="[(val: string) => !!val || 'Element name is required for attributes mode']">
              </q-input>
              <q-input
                v-model="xmlKeyMode.key_attr"
                outlined
                dense
                label="Key Attribute"
                hint="e.g. name">
              </q-input>
              <q-input
                v-model="xmlKeyMode.value_attr"
                outlined
                dense
                label="Value Attribute"
                hint="e.g. value">
              </q-input>
            </div>
          </div>
        </q-form>
      </q-card-section>

      <q-separator />

      <q-card-actions align="right" class="q-pa-md">
        <q-btn flat label="Cancel" @click="handleClose" />
        <q-btn color="primary" label="Add" @click="handleSubmit" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import type { QForm } from 'quasar'
import type { ConfigSchemaEntry } from './ConfigSchemaList.vue'

const props = defineProps<{
  modelValue: boolean
  existingCategories: string[]
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  add: [entry: ConfigSchemaEntry]
}>()

const dialogOpen = computed({
  get: () => props.modelValue,
  set: (val) => emit('update:modelValue', val),
})

const formRef = ref<QForm | null>(null)

const formatOptions = [
  { label: 'Properties', value: 'properties' },
  { label: 'INI', value: 'ini' },
  { label: 'JSON', value: 'json' },
  { label: 'YAML', value: 'yaml' },
  { label: 'TOML', value: 'toml' },
  { label: 'XML', value: 'xml' },
]

const xmlModeOptions = [
  { label: 'Elements', value: 'elements' },
  { label: 'Attributes', value: 'attributes' },
]

const entry = reactive({
  path: '',
  format: '',
  category: '',
  generate_before_start: false,
})

const xmlKeyMode = reactive({
  mode: 'elements',
  element: 'property',
  key_attr: 'name',
  value_attr: 'value',
})

const filteredCategories = computed(() => {
  if (!entry.category) return props.existingCategories
  const lower = entry.category.toLowerCase()
  return props.existingCategories.filter((c) => c.toLowerCase().includes(lower) && c !== entry.category)
})

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      // Reset form
      entry.path = ''
      entry.format = ''
      entry.category = ''
      entry.generate_before_start = false
      xmlKeyMode.mode = 'elements'
      xmlKeyMode.element = 'property'
      xmlKeyMode.key_attr = 'name'
      xmlKeyMode.value_attr = 'value'
    }
  },
)

function handleClose() {
  dialogOpen.value = false
}

async function handleSubmit() {
  const valid = await formRef.value?.validate()
  if (!valid) return

  const newEntry: ConfigSchemaEntry = {
    path: entry.path,
    format: entry.format,
    category: entry.category,
    generate_before_start: entry.generate_before_start,
    schema: {
      type: 'object',
      properties: {},
    },
  }

  if (entry.format === 'xml') {
    newEntry.xml_key_mode = { ...xmlKeyMode }
  }

  emit('add', newEntry)
  handleClose()
}
</script>

<style scoped>
.add-config-dialog {
  min-width: 460px;
  max-width: 560px;
  background-color: var(--xy-surface-1);
}

.dialog-header {
  padding: var(--xy-space-md);
}

.dialog-body {
  padding: var(--xy-space-md);
}

.xml-options {
  padding: var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  background-color: var(--xy-surface-0);
}
</style>
