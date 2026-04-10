<template>
  <q-dialog :model-value="modelValue" persistent @hide="emit('update:modelValue', false)">
    <q-card class="operation-dialog-card">
      <q-card-section class="operation-dialog-header">
        <div class="operation-dialog-title">{{ title }}</div>
        <div v-if="subtitle" class="operation-dialog-subtitle">{{ subtitle }}</div>
      </q-card-section>

      <q-card-section class="operation-dialog-body">
        <update-progress-panel
          :context-facts="contextFacts"
          :output-lines="outputLines"
          :show-navigate-away-message="showNavigateAwayMessage"
          :show-output-area="showOutputArea"
          :steps="steps" />
      </q-card-section>

      <q-card-actions align="right" class="operation-dialog-footer">
        <q-btn
          :label="complete ? 'Close' : 'Hide'"
          color="primary"
          flat
          no-caps
          @click="emit('update:modelValue', false)" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import UpdateProgressPanel from './UpdateProgressPanel.vue'
import type { OperationContextFact, StepState } from './UpdateProgressPanel.types'

withDefaults(
  defineProps<{
    modelValue: boolean
    title: string
    subtitle?: string
    steps: StepState[]
    contextFacts?: OperationContextFact[]
    outputLines?: string[]
    showOutputArea?: boolean
    complete?: boolean
    showNavigateAwayMessage?: boolean
  }>(),
  {
    subtitle: '',
    contextFacts: () => [],
    outputLines: () => [],
    showOutputArea: false,
    complete: false,
    showNavigateAwayMessage: true,
  },
)

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()
</script>

<style scoped>
.operation-dialog-card {
  width: min(840px, calc(100vw - 2rem));
  max-width: 100%;
  min-height: min(36rem, calc(100vh - 2rem));
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 1rem;
  background-clip: padding-box;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.operation-dialog-header {
  padding-bottom: 1rem;
  border-bottom: 1px solid var(--xy-border);
  border-top-left-radius: inherit;
  border-top-right-radius: inherit;
  background:
    linear-gradient(135deg, var(--xy-accent-glow-soft), transparent 60%), var(--xy-surface-0);
}

.operation-dialog-title {
  font-family: var(--xy-font-display);
  font-size: 1.1rem;
  color: var(--xy-text-primary);
}

.operation-dialog-subtitle {
  margin-top: 0.3rem;
  font-size: 0.82rem;
  color: var(--xy-text-muted);
  line-height: 1.45;
}

.operation-dialog-body {
  flex: 1 1 auto;
  padding-top: 1rem;
  min-height: 0;
}

.operation-dialog-footer {
  border-top: 1px solid var(--xy-border);
  background: transparent;
  border-bottom-left-radius: inherit;
  border-bottom-right-radius: inherit;
}
</style>
