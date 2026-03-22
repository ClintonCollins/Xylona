<template>
  <div class="update-progress-panel">
    <div class="steps-list">
      <div
        v-for="stepState in steps"
        :key="stepState.step"
        class="update-step"
        :class="stepStatusClass(stepState.status)">
        <div class="step-icon-wrapper">
          <div
            v-if="stepState.status === StepStatus.IN_PROGRESS"
            class="step-icon step-icon--in-progress">
            <q-spinner color="accent" size="1.1em" />
          </div>
          <div
            v-else-if="stepState.status === StepStatus.COMPLETED"
            class="step-icon step-icon--completed">
            <q-icon name="check_circle" size="1.1em" />
          </div>
          <div
            v-else-if="stepState.status === StepStatus.FAILED"
            class="step-icon step-icon--failed">
            <q-icon name="cancel" size="1.1em" />
          </div>
          <div v-else class="step-icon step-icon--pending">
            <q-icon name="radio_button_unchecked" size="1.1em" />
          </div>
        </div>
        <span class="step-label">{{ stepLabel(stepState.step) }}</span>
        <span v-if="stepState.message" class="step-message">{{ stepState.message }}</span>
      </div>
    </div>
    <p class="navigate-away-msg">
      You can safely navigate away — the update will continue in the background.
    </p>
  </div>
</template>

<script setup lang="ts">
import { UpdateStep, StepStatus } from '@/proto/xylona_pb'

interface StepState {
  step: UpdateStep
  status: StepStatus
  message?: string
}

defineProps<{
  steps: StepState[]
}>()

function stepLabel(step: UpdateStep): string {
  switch (step) {
    case UpdateStep.STOPPING:
      return 'Stopping'
    case UpdateStep.BACKING_UP:
      return 'Backing Up'
    case UpdateStep.DOWNLOADING:
      return 'Downloading'
    case UpdateStep.INSTALLING:
      return 'Installing'
    case UpdateStep.RESTARTING:
      return 'Restarting'
    case UpdateStep.ROLLING_BACK:
      return 'Rolling Back'
    default:
      return 'Unknown'
  }
}

function stepStatusClass(status: StepStatus): string {
  switch (status) {
    case StepStatus.IN_PROGRESS:
      return 'update-step--in-progress'
    case StepStatus.COMPLETED:
      return 'update-step--completed'
    case StepStatus.FAILED:
      return 'update-step--failed'
    default:
      return 'update-step--pending'
  }
}
</script>

<style scoped>
.update-progress-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1.25rem 1.5rem;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
}

.steps-list {
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
}

.update-step {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  font-family: var(--xy-font-body);
  font-size: 0.9rem;
  color: var(--xy-text-secondary);
  transition: color 0.2s ease;
}

.update-step--pending {
  color: var(--xy-text-muted);
  opacity: 0.6;
}

.update-step--in-progress {
  color: var(--xy-text-primary);
}

.update-step--completed {
  color: var(--xy-success);
}

.update-step--failed {
  color: var(--xy-danger);
}

.step-icon-wrapper {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.4em;
  flex-shrink: 0;
}

.step-icon--in-progress {
  color: var(--xy-accent);
}

.step-icon--completed {
  color: var(--xy-success);
}

.step-icon--failed {
  color: var(--xy-danger);
}

.step-icon--pending {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

.step-label {
  font-weight: 500;
}

.step-message {
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  margin-left: 0.25rem;
}

.navigate-away-msg {
  margin: 0;
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
  border-top: 1px solid var(--xy-border);
  padding-top: 0.75rem;
}
</style>
