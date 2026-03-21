<template>
  <div v-if="fields.length > 0" class="advanced-fields">
    <q-expansion-item
      v-model="expanded"
      dense
      header-class="advanced-header"
      expand-icon-class="text-xy-muted">
      <template #header>
        <q-item-section avatar>
          <q-icon name="code" color="warning" size="sm" aria-hidden="true" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="advanced-title font-display"> Advanced Fields </q-item-label>
          <q-item-label caption class="text-xy-muted">
            {{ fields.length }} field{{ fields.length !== 1 ? 's' : '' }} not in schema
          </q-item-label>
        </q-item-section>
      </template>

      <div class="advanced-content">
        <q-banner dense class="advanced-banner q-mb-sm">
          <template #avatar>
            <q-icon name="info" color="warning" size="xs" aria-hidden="true" />
          </template>
          These fields exist in the config file but aren't defined in the schema. They will be
          preserved when saving.
        </q-banner>

        <div class="advanced-list">
          <div v-for="(field, index) in localFields" :key="index" class="advanced-row">
            <div v-if="field.section" class="advanced-section font-mono text-xy-muted">
              [{{ field.section }}]
            </div>
            <div class="advanced-field-row">
              <q-input
                v-model="field.key"
                :aria-label="`Field key: ${field.key}`"
                dense
                outlined
                readonly
                class="advanced-key"
                input-class="font-mono advanced-input-text">
              </q-input>
              <span class="advanced-equals text-xy-muted">=</span>
              <q-input
                v-model="field.value"
                :aria-label="`Value for ${field.key}`"
                dense
                outlined
                class="advanced-value"
                input-class="font-mono advanced-input-text"
                @update:model-value="emitUpdate">
              </q-input>
            </div>
          </div>
        </div>
      </div>
    </q-expansion-item>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import type { AdvancedField } from '@/proto/xylona_pb'

const props = defineProps<{
  fields: AdvancedField[]
}>()

const emit = defineEmits<{
  update: [fields: AdvancedField[]]
}>()

const expanded = ref(false)

interface LocalAdvancedField {
  key: string
  value: string
  section: string
}

const localFields = ref<LocalAdvancedField[]>([])

watch(
  () => props.fields,
  (newFields) => {
    localFields.value = newFields.map((f) => ({
      key: f.key,
      value: f.value,
      section: f.section,
    }))
  },
  { immediate: true },
)

function emitUpdate() {
  emit(
    'update',
    localFields.value.map(
      (f) => ({ key: f.key, value: f.value, section: f.section }) as AdvancedField,
    ),
  )
}
</script>

<style scoped>
.advanced-fields {
  margin-top: var(--xy-space-md);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  overflow: hidden;
  background-color: var(--xy-surface-1);
}

.advanced-header {
  background-color: var(--xy-surface-1);
}

.advanced-title {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.advanced-content {
  padding: var(--xy-space-sm) var(--xy-space-md) var(--xy-space-md);
}

.advanced-banner {
  background-color: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: 6px;
  font-size: 0.75rem;
  color: var(--xy-text-secondary);
}

.advanced-list {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.advanced-section {
  font-size: 0.75rem;
  padding: var(--xy-space-xs) 0;
}

.advanced-field-row {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
}

.advanced-key {
  flex: 0 0 40%;
  max-width: 40%;
}

.advanced-equals {
  flex-shrink: 0;
  font-family: var(--xy-font-mono);
  font-size: 0.8rem;
}

.advanced-value {
  flex: 1;
}

.advanced-input-text {
  font-size: 0.8rem;
}
</style>
