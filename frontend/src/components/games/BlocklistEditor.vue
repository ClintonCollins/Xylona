<template>
  <div class="blocklist-editor">
    <div v-if="showHeader" class="blocklist-editor__header">
      <div class="font-display blocklist-editor__title">Blocklist</div>
      <div class="text-xy-secondary blocklist-editor__copy">
        Prevent risky or reserved arguments from being saved at the server level.
      </div>
    </div>

    <div v-if="blocklist.length === 0" class="blocklist-editor__empty text-xy-muted">
      No patterns configured.
    </div>

    <div class="blocklist-editor__rows">
      <article
        v-for="(entry, index) in blocklist"
        :key="`${entry.pattern}-${index}`"
        class="blocklist-editor__row">
        <q-input
          :model-value="entry.pattern"
          outlined
          label="Pattern"
          :error="!isValidRegex(entry.pattern)"
          error-message="Invalid regex"
          @update:model-value="updateEntry(index, 'pattern', String($event ?? ''))" />
        <q-input
          :model-value="entry.reason"
          outlined
          label="Reason"
          @update:model-value="updateEntry(index, 'reason', String($event ?? ''))" />
        <q-btn
          flat
          dense
          icon="delete"
          color="negative"
          aria-label="Remove blocklist pattern"
          @click="removeEntry(index)" />
      </article>
    </div>

    <div class="blocklist-editor__add">
      <q-input
        v-model="draftPattern"
        outlined
        label="Pattern"
        :error="draftPattern !== '' && !isValidRegex(draftPattern)"
        error-message="Invalid regex" />
      <q-input v-model="draftReason" outlined label="Reason" />
      <q-btn
        color="accent"
        label="Add pattern"
        :disable="draftPattern.trim() === '' || !isValidRegex(draftPattern)"
        @click="addEntry" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'

import type { StartArgBlocklistEntry } from '@/components/game_servers/start-args'

const props = withDefaults(
  defineProps<{
    blocklist: StartArgBlocklistEntry[]
    showHeader?: boolean
  }>(),
  {
    showHeader: true,
  },
)

const emit = defineEmits<{
  'update:blocklist': [value: StartArgBlocklistEntry[]]
}>()

const draftPattern = ref('')
const draftReason = ref('')

function updateEntry(index: number, key: 'pattern' | 'reason', value: string) {
  const nextBlocklist = props.blocklist.map((entry, currentIndex) =>
    currentIndex === index ? { ...entry, [key]: value } : entry,
  )
  emit('update:blocklist', nextBlocklist)
}

function removeEntry(index: number) {
  emit(
    'update:blocklist',
    props.blocklist.filter((_, currentIndex) => currentIndex !== index),
  )
}

function addEntry() {
  if (draftPattern.value.trim() === '' || !isValidRegex(draftPattern.value)) {
    return
  }

  emit('update:blocklist', [
    ...props.blocklist,
    {
      pattern: draftPattern.value.trim(),
      reason: draftReason.value.trim() || 'Blocked by game definition',
    },
  ])
  draftPattern.value = ''
  draftReason.value = ''
}

function isValidRegex(value: string) {
  try {
    new RegExp(value)
    return true
  } catch {
    return false
  }
}
</script>

<style scoped>
.blocklist-editor {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.blocklist-editor__header {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.blocklist-editor__title {
  font-size: 0.95rem;
  color: var(--xy-text-primary);
}

.blocklist-editor__copy {
  font-size: 0.82rem;
}

.blocklist-editor__empty {
  padding: var(--xy-space-md);
  border-radius: 10px;
  border: 1px dashed var(--xy-border);
  background: var(--xy-surface-0);
}

.blocklist-editor__rows,
.blocklist-editor__add {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.blocklist-editor__row {
  display: grid;
  gap: 12px;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr) auto;
  align-items: start;
}

.blocklist-editor__row :deep(.q-btn) {
  min-height: 40px;
  min-width: 40px;
}

@media (max-width: 720px) {
  .blocklist-editor__row {
    grid-template-columns: 1fr;
  }
}
</style>
