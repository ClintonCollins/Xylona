<template>
  <q-dialog :model-value="show" @update:model-value="onDialogChange">
    <q-card class="similar-arg-dialog">
      <q-card-section class="similar-arg-dialog__header">
        <div class="font-display similar-arg-dialog__title">Possible duplicate argument</div>
        <div class="text-xy-secondary similar-arg-dialog__copy">
          Xylona found another argument with the same prefix. Choose whether to replace it or keep
          both.
        </div>
      </q-card-section>

      <q-card-section class="similar-arg-dialog__compare">
        <div class="similar-arg-dialog__column">
          <div class="similar-arg-dialog__label">Existing</div>
          <code class="similar-arg-dialog__value">{{ existingValue }}</code>
        </div>
        <div class="similar-arg-dialog__column">
          <div class="similar-arg-dialog__label">New</div>
          <code class="similar-arg-dialog__value">{{ newValue }}</code>
        </div>
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="Cancel" @click="$emit('cancel')" />
        <q-btn color="warning" flat label="Add both" @click="$emit('add-both')" />
        <q-btn color="primary" label="Replace existing" @click="$emit('replace')" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { computed } from 'vue'

import type { ResolvedStartArgBlock } from './start-args'
import { formatTokensInline } from './start-args'

const props = defineProps<{
  existingBlock: ResolvedStartArgBlock | null
  newTokens: string[]
  show: boolean
}>()

const emit = defineEmits<{
  'add-both': []
  cancel: []
  replace: []
}>()

const existingValue = computed(() =>
  props.existingBlock ? formatTokensInline(props.existingBlock.tokens) : '',
)
const newValue = computed(() => formatTokensInline(props.newTokens))

function onDialogChange(value: boolean) {
  if (!value) {
    emit('cancel')
  }
}
</script>

<style scoped>
.similar-arg-dialog {
  width: min(560px, calc(100vw - 32px));
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
}

.similar-arg-dialog__header {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.similar-arg-dialog__title {
  font-size: 1rem;
  color: var(--xy-text-primary);
}

.similar-arg-dialog__copy {
  font-size: 0.85rem;
  line-height: 1.45;
}

.similar-arg-dialog__compare {
  display: grid;
  gap: var(--xy-space-md);
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
}

.similar-arg-dialog__column {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm);
  border-radius: 8px;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
}

.similar-arg-dialog__label {
  font-size: 0.72rem;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.similar-arg-dialog__value {
  font-family: var(--xy-font-mono);
  color: var(--xy-text-primary);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
</style>
