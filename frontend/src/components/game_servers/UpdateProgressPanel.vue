<template>
  <div class="operation-timeline-panel">
    <div
      v-if="statusStep"
      class="operation-active-band"
      :class="`operation-active-band--${statusTone}`">
      <div class="operation-active-pulse"></div>
      <div class="operation-active-copy">
        <div class="operation-active-label">{{ statusLabel }}</div>
        <div class="operation-active-title">{{ stepLabel(statusStep) }}</div>
        <div v-if="statusSecondaryText" class="operation-active-next">
          {{ statusSecondaryText }}
        </div>
      </div>
      <div class="operation-active-state">{{ statusStateText }}</div>
    </div>

    <div
      class="operation-timeline-layout"
      :class="{ 'operation-timeline-layout--with-rail': hasContextFacts }">
      <section class="operation-timeline-shell">
        <div class="operation-timeline-head">
          <div>
            <div class="operation-timeline-title">Timeline</div>
            <div class="operation-timeline-copy">
              Follow the operation as it moves through each event.
            </div>
          </div>
        </div>

        <div class="operation-timeline-entries">
          <article
            v-for="(stepState, index) in steps"
            :key="stepState.step"
            class="operation-timeline-entry"
            :class="entryStatusClass(stepState.status)">
            <div class="operation-timeline-stamp">{{ stepStamp(stepState, index) }}</div>
            <div class="operation-timeline-marker">
              <div class="operation-timeline-pin">
                <q-spinner
                  v-if="stepState.status === StepStatus.IN_PROGRESS"
                  class="operation-timeline-spinner"
                  color="info"
                  size="0.85rem" />
                <q-icon
                  v-else-if="stepState.status === StepStatus.COMPLETED"
                  class="operation-timeline-icon operation-timeline-icon--complete"
                  color="positive"
                  name="check"
                  size="0.85rem" />
                <q-icon
                  v-else-if="stepState.status === StepStatus.FAILED"
                  class="operation-timeline-icon operation-timeline-icon--failed"
                  color="negative"
                  name="close"
                  size="0.8rem" />
              </div>
            </div>
            <div class="operation-timeline-detail">
              <div class="operation-timeline-kind">{{ stepKind(stepState) }}</div>
              <div class="operation-timeline-name">{{ stepLabel(stepState) }}</div>
              <div
                class="operation-timeline-desc"
                :class="{ 'operation-timeline-desc--placeholder': !stepDescription(stepState) }">
                {{ stepDescription(stepState) || ' ' }}
              </div>
            </div>
          </article>
        </div>
      </section>

      <aside v-if="hasContextFacts" class="operation-context-rail">
        <div class="operation-context-title">Context</div>
        <div class="operation-context-copy">
          Additional facts shown only when this operation has useful extra context.
        </div>
        <div class="operation-context-items">
          <div v-for="fact in contextFacts" :key="fact.label" class="operation-context-item">
            <strong>{{ fact.label }}</strong>
            <span>{{ fact.value }}</span>
          </div>
        </div>
      </aside>
    </div>

    <button
      v-if="showOutputSection"
      type="button"
      class="operation-output-toggle"
      :disabled="!hasOutputLines"
      @click="outputExpanded = !outputExpanded">
      <div>
        <div class="operation-output-title">Operation Output</div>
        <div class="operation-output-copy">{{ outputSummary }}</div>
      </div>
      <div
        class="operation-output-caret"
        :class="{ 'operation-output-caret--open': outputExpanded }">
        ⌄
      </div>
    </button>

    <pre v-if="outputExpanded && hasOutputLines" class="operation-output-lines">{{
      outputLines.join('\n')
    }}</pre>

    <p v-if="props.showNavigateAwayMessage" class="navigate-away-msg">
      {{ footerStatusMessage }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { UpdateStep, StepStatus } from '@/proto/xylona_pb'
import type { OperationContextFact, StepState } from './UpdateProgressPanel.types'

const props = withDefaults(
  defineProps<{
    steps: StepState[]
    contextFacts?: OperationContextFact[]
    outputLines?: string[]
    showOutputArea?: boolean
    showNavigateAwayMessage?: boolean
  }>(),
  {
    contextFacts: () => [],
    outputLines: () => [],
    showOutputArea: false,
    showNavigateAwayMessage: true,
  },
)

const outputExpanded = ref(false)

const hasContextFacts = computed(() => props.contextFacts.length > 0)
const hasOutputLines = computed(() => props.outputLines.length > 0)
const showOutputSection = computed(() => props.showOutputArea || hasOutputLines.value)
const activeStep = computed(() =>
  props.steps.find((stepState) => stepState.status === StepStatus.IN_PROGRESS),
)
const failedStep = computed(() =>
  props.steps.find((stepState) => stepState.status === StepStatus.FAILED),
)
const latestCompletedStep = computed(() =>
  [...props.steps].reverse().find((stepState) => stepState.status === StepStatus.COMPLETED),
)
const statusStep = computed(
  () =>
    activeStep.value ??
    failedStep.value ??
    latestCompletedStep.value ??
    props.steps.find((stepState) => stepState.status === StepStatus.PENDING) ??
    props.steps[0],
)
const nextStep = computed(() => {
  if (!activeStep.value) {
    return props.steps.find((stepState) => stepState.status === StepStatus.PENDING)
  }

  const activeIndex = props.steps.findIndex(
    (stepState) => stepState.step === activeStep.value?.step,
  )
  return props.steps
    .slice(activeIndex + 1)
    .find((stepState) => stepState.status === StepStatus.PENDING)
})
const statusTone = computed(() => {
  if (activeStep.value) return 'live'
  if (failedStep.value) return 'failed'
  if (latestCompletedStep.value) return 'complete'
  return 'pending'
})
const statusLabel = computed(() => {
  if (activeStep.value) return 'Active event'
  if (failedStep.value) return 'Operation failed'
  if (latestCompletedStep.value) return 'Operation complete'
  return 'Queued event'
})
const statusSecondaryText = computed(() => {
  if (activeStep.value && nextStep.value) {
    return `Next: ${stepLabel(nextStep.value)}`
  }

  if (failedStep.value) {
    return failedStep.value.message || 'Review the timeline for the failing step.'
  }

  if (latestCompletedStep.value) {
    return latestCompletedStep.value.message || 'All timeline events finished successfully.'
  }

  return 'Waiting for the operation to begin.'
})
const statusStateText = computed(() => {
  switch (statusTone.value) {
    case 'live':
      return 'live'
    case 'failed':
      return 'failed'
    case 'complete':
      return 'complete'
    default:
      return 'queued'
  }
})
const footerStatusMessage = computed(() => {
  switch (statusTone.value) {
    case 'live':
      return 'You can safely navigate away — the update will continue in the background.'
    case 'failed':
      return 'This operation stopped early. Review the timeline or output details before closing.'
    case 'complete':
      return 'Operation finished. You can close this dialog whenever you are ready.'
    default:
      return 'Xylona is preparing this operation.'
  }
})
const outputSummary = computed(() => {
  const count = props.outputLines.length
  const lineLabel = count === 1 ? 'line' : 'lines'
  const latest = props.outputLines[props.outputLines.length - 1]
  if (!latest) {
    return 'No operation output yet'
  }
  return `${count} ${lineLabel} available · latest: “${latest}”`
})

function stepLabel(stepState: StepState): string {
  if (stepState.label) {
    return stepState.label
  }

  const step = stepState.step
  if (typeof step !== 'number') {
    return step
  }

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

function stepKind(stepState: StepState): string {
  const step = stepState.step
  if (step === 'software-selection') return 'Validate'
  if (step === 'software-download') return 'Download'
  if (step === 'software-apply') return 'Apply'

  if (typeof step !== 'number') {
    return 'Operation'
  }

  switch (step) {
    case UpdateStep.STOPPING:
      return 'Stop'
    case UpdateStep.BACKING_UP:
      return 'Backup'
    case UpdateStep.DOWNLOADING:
      return 'Download'
    case UpdateStep.INSTALLING:
      return 'Apply'
    case UpdateStep.RESTARTING:
      return 'Restart'
    case UpdateStep.ROLLING_BACK:
      return 'Rollback'
    default:
      return 'Operation'
  }
}

function stepDescription(stepState: StepState): string {
  return stepState.message ?? ''
}

function stepStamp(stepState: StepState, index: number): string {
  switch (stepState.status) {
    case StepStatus.COMPLETED:
      return 'Done'
    case StepStatus.IN_PROGRESS:
      return 'Live'
    case StepStatus.FAILED:
      return 'Fail'
    case StepStatus.PENDING:
      return index === props.steps.findIndex((step) => step.status === StepStatus.PENDING)
        ? 'Next'
        : 'Queued'
    default:
      return ''
  }
}

function entryStatusClass(status: StepStatus): string {
  switch (status) {
    case StepStatus.IN_PROGRESS:
      return 'operation-timeline-entry--live'
    case StepStatus.COMPLETED:
      return 'operation-timeline-entry--complete'
    case StepStatus.FAILED:
      return 'operation-timeline-entry--failed'
    default:
      return 'operation-timeline-entry--pending'
  }
}
</script>

<style scoped>
.operation-timeline-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.operation-active-band {
  display: grid;
  grid-template-columns: auto 1fr auto;
  align-items: center;
  gap: 0.75rem;
  padding: 0.8rem 0.9rem;
  border: 1px solid var(--xy-accent-muted);
  border-radius: 0.95rem;
  background: linear-gradient(90deg, rgba(28, 183, 207, 0.05), var(--xy-surface-overlay-soft));
}

.operation-active-band--complete {
  border-color: var(--xy-success-bg-soft);
  background: linear-gradient(90deg, var(--xy-success-bg-faint), var(--xy-surface-overlay-soft));
}

.operation-active-band--failed {
  border-color: var(--xy-danger-bg-faint);
  background: linear-gradient(90deg, rgba(239, 68, 68, 0.05), var(--xy-surface-overlay-soft));
}

.operation-active-band--pending {
  border-color: var(--xy-border);
  background: linear-gradient(
    90deg,
    var(--xy-surface-overlay-soft),
    var(--xy-surface-overlay-faint)
  );
}

.operation-active-pulse {
  width: 0.55rem;
  height: 0.55rem;
  border-radius: 999px;
  background: var(--xy-accent);
  box-shadow: 0 0 0 0.35rem var(--xy-accent-glow-soft);
}

.operation-active-band--complete .operation-active-pulse {
  background: var(--xy-success);
  box-shadow: 0 0 0 0.35rem var(--xy-success-bg-faint);
}

.operation-active-band--failed .operation-active-pulse {
  background: var(--xy-danger);
  box-shadow: 0 0 0 0.35rem var(--xy-danger-bg-faint);
}

.operation-active-band--pending .operation-active-pulse {
  background: rgba(255, 255, 255, 0.42);
  box-shadow: 0 0 0 0.35rem var(--xy-surface-sheen-soft);
}

.operation-active-label {
  font-size: 0.62rem;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.operation-active-title {
  margin-top: 0.15rem;
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.operation-active-next {
  margin-top: 0.2rem;
  font-size: 0.76rem;
  color: var(--xy-text-secondary);
}

.operation-active-state {
  padding: 0.35rem 0.6rem;
  border-radius: 999px;
  background: var(--xy-accent-glow-soft);
  color: var(--xy-text-secondary);
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.1em;
}

.operation-active-band--complete .operation-active-state {
  background: var(--xy-success-bg-faint);
}

.operation-active-band--failed .operation-active-state {
  background: var(--xy-danger-bg-faint);
}

.operation-active-band--pending .operation-active-state {
  background: var(--xy-surface-sheen-soft);
}

.operation-timeline-layout {
  display: grid;
  grid-template-columns: 1fr;
  gap: 1rem;
}

.operation-timeline-layout--with-rail {
  grid-template-columns: minmax(0, 1fr) 15rem;
}

.operation-timeline-shell,
.operation-context-rail,
.operation-output-lines,
.operation-output-toggle {
  border: 1px solid var(--xy-border);
  border-radius: 1.15rem;
  background: linear-gradient(
    180deg,
    var(--xy-surface-overlay-soft),
    var(--xy-surface-overlay-faint)
  );
}

.operation-timeline-shell {
  padding: 1.05rem;
}

.operation-timeline-head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 1rem;
  margin-bottom: 0.95rem;
}

.operation-timeline-title {
  font-family: var(--xy-font-display);
  font-size: 1.15rem;
  color: var(--xy-text-primary);
}

.operation-timeline-copy {
  margin-top: 0.35rem;
  max-width: 34rem;
  color: var(--xy-text-secondary);
  font-size: 0.78rem;
  line-height: 1.45;
}

.operation-timeline-entries {
  display: grid;
  gap: 1rem;
}

.operation-timeline-entry {
  display: grid;
  grid-template-columns: 4rem 1rem minmax(0, 1fr);
  gap: 0.9rem;
  align-items: flex-start;
}

.operation-timeline-stamp {
  padding-top: 0.25rem;
  text-align: right;
  color: var(--xy-accent);
  font-family: var(--xy-font-mono);
  font-size: 0.73rem;
}

.operation-timeline-marker {
  position: relative;
  height: 100%;
  display: flex;
  justify-content: center;
}

.operation-timeline-marker::before {
  content: '';
  position: absolute;
  top: 0;
  bottom: -0.7rem;
  width: 1px;
  background: linear-gradient(180deg, rgba(28, 183, 207, 0.2), var(--xy-border));
}

.operation-timeline-entry:last-child .operation-timeline-marker::before {
  bottom: 0.8rem;
}

.operation-timeline-pin {
  position: relative;
  z-index: 1;
  width: 0.85rem;
  height: 0.85rem;
  margin-top: 0.28rem;
  border-radius: 999px;
  background: var(--xy-surface-1);
  border: 2px solid var(--xy-border-hover);
  display: flex;
  align-items: center;
  justify-content: center;
}

.operation-timeline-entry--complete .operation-timeline-pin {
  border-color: var(--xy-success);
  background: var(--xy-syntax-green-bg);
}

.operation-timeline-entry--failed .operation-timeline-pin {
  border-color: var(--xy-danger);
  background: var(--xy-danger-bg);
}

.operation-timeline-entry--live .operation-timeline-pin {
  border-color: var(--xy-accent);
  background: var(--xy-success-bg-faint);
}

.operation-timeline-spinner,
.operation-timeline-icon {
  position: relative;
  z-index: 1;
}

.operation-timeline-detail {
  padding: 1rem 1.05rem 1.05rem;
  border: 1px solid var(--xy-border);
  border-radius: 1rem;
  background: rgba(0, 0, 0, 0.16);
}

.operation-timeline-entry--live .operation-timeline-detail {
  border-color: var(--xy-accent-muted);
  background: linear-gradient(180deg, rgba(28, 183, 207, 0.035), rgba(0, 0, 0, 0.16));
}

.operation-timeline-kind {
  font-size: 0.62rem;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: var(--xy-text-muted);
}

.operation-timeline-name {
  margin-top: 0.48rem;
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--xy-text-primary);
  line-height: 1.4;
}

.operation-timeline-desc {
  margin-top: 0.55rem;
  min-height: calc(1.6em * 2);
  color: var(--xy-text-secondary);
  font-size: 0.8rem;
  line-height: 1.6;
}

.operation-timeline-desc--placeholder {
  visibility: hidden;
}

.operation-context-rail {
  padding: 0.9rem;
}

.operation-context-title {
  font-size: 0.68rem;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--xy-text-muted);
}

.operation-context-copy {
  margin-top: 0.45rem;
  color: var(--xy-text-secondary);
  font-size: 0.76rem;
  line-height: 1.45;
}

.operation-context-items {
  margin-top: 0.8rem;
  display: grid;
  gap: 0.55rem;
}

.operation-context-item {
  padding: 0.7rem 0.75rem;
  border-radius: 0.9rem;
  border: 1px solid var(--xy-surface-sheen-soft);
  background: rgba(0, 0, 0, 0.16);
}

.operation-context-item strong {
  display: block;
  font-size: 0.7rem;
  color: var(--xy-text-muted);
  text-transform: uppercase;
  letter-spacing: 0.1em;
}

.operation-context-item span {
  display: block;
  margin-top: 0.35rem;
  color: var(--xy-text-primary);
  font-size: 0.82rem;
  line-height: 1.4;
}

.operation-output-toggle {
  appearance: none;
  display: grid;
  grid-template-columns: 1fr auto;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  padding: 0.85rem 0.95rem;
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition:
    border-color 0.2s ease,
    background-color 0.2s ease;
}

.operation-output-toggle:disabled {
  cursor: default;
}

.operation-output-toggle:not(:disabled):hover {
  border-color: var(--xy-accent-border-soft);
}

.operation-output-title {
  font-size: 0.92rem;
  font-weight: 600;
  color: var(--xy-text-primary);
}

.operation-output-copy {
  margin-top: 0.22rem;
  color: var(--xy-text-secondary);
  font-size: 0.78rem;
  line-height: 1.45;
}

.operation-output-caret {
  color: var(--xy-accent);
  font-size: 1.1rem;
  transition: transform 0.2s ease;
}

.operation-output-caret--open {
  transform: rotate(180deg);
}

.operation-output-lines {
  margin: 0;
  padding: 0.95rem 1rem;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: 0.76rem;
  line-height: 1.55;
  white-space: pre-wrap;
  max-height: 12rem;
  overflow: auto;
}

.navigate-away-msg {
  margin: 0;
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
  border-top: 1px solid var(--xy-border);
  padding-top: 0.75rem;
}

@media (max-width: 860px) {
  .operation-timeline-layout--with-rail {
    grid-template-columns: 1fr;
  }

  .operation-timeline-head {
    align-items: flex-start;
    flex-direction: column;
  }

  .operation-timeline-entry {
    grid-template-columns: 1fr;
    gap: 0.45rem;
  }

  .operation-timeline-stamp {
    text-align: left;
    padding-top: 0;
  }

  .operation-timeline-marker {
    display: none;
  }

  .operation-active-band {
    grid-template-columns: auto 1fr;
  }

  .operation-active-state {
    justify-self: start;
  }
}
</style>
