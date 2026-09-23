<template>
  <div v-if="visibleRows.length > 0" class="advanced-fields">
    <q-expansion-item
      v-model="expanded"
      dense
      expand-icon-class="text-xy-muted"
      header-class="advanced-header">
      <template #header>
        <q-item-section avatar>
          <q-icon aria-hidden="true" class="text-xy-muted" name="code" size="sm" />
        </q-item-section>
        <q-item-section>
          <q-item-label class="advanced-title">Advanced Fields</q-item-label>
          <q-item-label caption class="text-xy-muted">
            <template v-if="searching">
              {{ visibleRows.length }} of {{ fields.length }} match
            </template>
            <template v-else>
              {{ fields.length }} field{{ fields.length !== 1 ? 's' : '' }} not in schema
            </template>
          </q-item-label>
        </q-item-section>
      </template>

      <div class="advanced-content">
        <q-banner class="advanced-banner q-mb-sm" dense>
          <template #avatar>
            <q-icon aria-hidden="true" color="warning" name="info" size="xs" />
          </template>
          These fields exist in the config file but aren't defined in the schema. They will be
          preserved when saving.
        </q-banner>

        <div class="advanced-list">
          <div
            v-for="{ field, index } in visibleRows"
            :key="index"
            :data-test="`advanced-row-${field.key}`"
            class="advanced-row">
            <div v-if="field.section" class="advanced-section font-mono text-xy-muted">
              [{{ field.section }}]
            </div>
            <div class="advanced-field-row">
              <q-input
                v-model="field.key"
                :aria-label="`Field key: ${field.key}`"
                class="advanced-key"
                dense
                input-class="font-mono advanced-input-text"
                outlined
                readonly>
              </q-input>
              <span class="advanced-equals text-xy-muted">=</span>
              <q-input
                v-model="field.value"
                :aria-label="`Value for ${field.key}`"
                :type="isSecretConfigKey(field.key) && !revealed.has(index) ? 'password' : 'text'"
                class="advanced-value"
                dense
                input-class="font-mono advanced-input-text"
                outlined
                @update:model-value="emitUpdate">
                <template v-if="isSecretConfigKey(field.key)" #append>
                  <q-btn
                    :aria-label="revealed.has(index) ? 'Hide value' : 'Show value'"
                    :aria-pressed="revealed.has(index)"
                    :icon="revealed.has(index) ? 'visibility_off' : 'visibility'"
                    dense
                    flat
                    round
                    type="button"
                    @click="toggleReveal(index)">
                    <q-tooltip>{{ revealed.has(index) ? 'Hide value' : 'Show value' }}</q-tooltip>
                  </q-btn>
                </template>
              </q-input>
            </div>
          </div>
        </div>
      </div>
    </q-expansion-item>
  </div>
</template>

<script lang="ts" setup>
import { computed, reactive, ref, watch } from 'vue'
import type { AdvancedField } from '@/proto/xylona_pb'
import { filterFields, isSecretConfigKey } from './config-field-helpers'

const props = withDefaults(
  defineProps<{
    fields: AdvancedField[]
    search?: string
  }>(),
  { search: '' },
)

const emit = defineEmits<{
  update: [fields: AdvancedField[]]
}>()

const expanded = ref(false)
const revealed = reactive(new Set<number>())

interface LocalAdvancedField {
  key: string
  value: string
  section: string
}

const localFields = ref<LocalAdvancedField[]>([])

watch(
  () => props.fields,
  (newFields) => {
    revealed.clear()
    localFields.value = newFields.map((f) => ({
      key: f.key,
      value: f.value,
      section: f.section,
    }))
  },
  { immediate: true },
)

const searching = computed(() => props.search.trim() !== '')

const visibleRows = computed(() => {
  const rows = localFields.value.map((field, index) => ({ field, index }))
  if (!searching.value) return rows
  const matches = new Set(filterFields(localFields.value, props.search))
  return rows.filter((row) => matches.has(row.field))
})

// A search that matches advanced fields opens the panel so the matches show.
watch(
  () => props.search,
  () => {
    if (searching.value && visibleRows.value.length > 0) expanded.value = true
  },
)

function toggleReveal(index: number) {
  if (!revealed.delete(index)) revealed.add(index)
}

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
  border-radius: var(--xy-radius-lg);
  overflow: hidden;
  background-color: var(--xy-surface-1);
}

.advanced-header {
  background-color: var(--xy-surface-1);
}

.advanced-title {
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
  color: var(--xy-text-primary);
}

.advanced-content {
  padding: var(--xy-space-sm) var(--xy-space-md) var(--xy-space-md);
}

.advanced-banner {
  background-color: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-secondary);
}

.advanced-list {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.advanced-section {
  font-size: var(--xy-font-size-xs);
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
  font-size: var(--xy-font-size-sm);
}

.advanced-value {
  flex: 1;
}

.advanced-input-text {
  font-size: var(--xy-font-size-sm);
}
</style>
