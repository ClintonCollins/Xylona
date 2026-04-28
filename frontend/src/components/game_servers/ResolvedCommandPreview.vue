<template>
  <section class="resolved-preview" data-testid="resolved-command-preview">
    <div class="resolved-preview__header">
      <div class="resolved-preview__header-main">
        <div class="resolved-preview__title">
          <q-icon name="terminal" size="18px" />
          <span>Resolved launch</span>
          <strong>{{ tokenCountLabel }}</strong>
        </div>
        <div class="resolved-preview__copy text-xy-secondary">
          Exact command after placeholders resolve. Updates as you edit.
        </div>
      </div>
      <q-btn
        :disable="commandPartCount === 0"
        color="accent"
        dense
        flat
        icon="content_copy"
        label="Copy"
        @click="copyCommand" />
    </div>

    <div class="resolved-preview__shell">
      <div class="resolved-preview__shell-bar">
        <span class="resolved-preview__shell-dots" aria-hidden="true">
          <span></span>
          <span></span>
          <span></span>
        </span>
        <span>launch command</span>
        <span class="resolved-preview__shell-state">live preview</span>
      </div>

      <code
        v-if="commandPartCount > 0"
        aria-label="Resolved launch command"
        class="resolved-preview__command"
        ><span class="resolved-preview__prompt">$</span
        ><span
          v-for="segment in commandSegments"
          :key="segment.key"
          :class="segmentClass(segment.provenance)"
          class="resolved-preview__segment"
          >{{ segment.value }}</span
        ></code
      >
      <div v-else class="resolved-preview__empty">No resolved launch command yet.</div>
    </div>

    <div class="resolved-preview__legend">
      <span
        v-for="item in legendItems"
        :key="item.provenance"
        class="resolved-preview__legend-item">
        <span :class="legendDotClass(item.provenance)" class="resolved-preview__legend-dot"></span>
        {{ item.label }}
      </span>
    </div>
  </section>
</template>

<script lang="ts" setup>
import { computed } from 'vue'
import { copyToClipboard, useQuasar } from 'quasar'

import type { ResolvedStartArgBlock, StartArgProvenance } from './start-args'

type CommandSegmentProvenance = StartArgProvenance | 'base'

const props = defineProps<{
  baseCommand: string
  resolvedBlocks: ResolvedStartArgBlock[]
}>()

const $q = useQuasar()

const legendItems: Array<{ provenance: StartArgProvenance; label: string }> = [
  { provenance: 'system', label: 'System' },
  { provenance: 'locked', label: 'Locked' },
  { provenance: 'default', label: 'Default' },
  { provenance: 'edited', label: 'Edited' },
  { provenance: 'added', label: 'Added' },
]

const commandSegments = computed(() => {
  const segments: Array<{
    key: string
    provenance: CommandSegmentProvenance
    value: string
  }> = []

  if (props.baseCommand !== '') {
    segments.push({
      key: 'base-command',
      provenance: 'base',
      value: props.baseCommand,
    })
  }

  for (const block of props.resolvedBlocks) {
    for (const [tokenIndex, token] of block.resolvedTokens.entries()) {
      segments.push({
        key: `${block.id}-${tokenIndex}`,
        provenance: block.provenance,
        value: token,
      })
    }
  }

  return segments
})

const fullCommand = computed(() => commandSegments.value.map((segment) => segment.value).join(' '))

const commandPartCount = computed(() => commandSegments.value.length)

const tokenCountLabel = computed(() =>
  commandPartCount.value === 1 ? '1 token' : `${commandPartCount.value} tokens`,
)

function segmentClass(provenance: CommandSegmentProvenance) {
  return `resolved-preview__segment--${provenance}`
}

function legendDotClass(provenance: StartArgProvenance) {
  return `resolved-preview__legend-dot--${provenance}`
}

async function copyCommand() {
  await copyToClipboard(fullCommand.value)
  $q.notify({
    type: 'positive',
    position: 'top',
    caption: 'Resolved command copied to clipboard.',
    icon: 'task_alt',
  })
}
</script>

<style scoped>
.resolved-preview {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 18%, var(--xy-border) 82%);
  border-radius: 8px;
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-accent) 3.5%, transparent), transparent),
    var(--xy-surface-1);
  box-shadow: 0 14px 34px color-mix(in srgb, var(--xy-base) 36%, transparent);
}

.resolved-preview__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.resolved-preview__header-main {
  min-width: 0;
}

.resolved-preview__title {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: 0.92rem;
  line-height: 1.25;
}

.resolved-preview__title :deep(.q-icon) {
  color: var(--xy-accent);
}

.resolved-preview__title strong {
  padding: 2px 8px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 18%, var(--xy-border) 82%);
  border-radius: 999px;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: 0.68rem;
  font-weight: 600;
  text-transform: uppercase;
}

.resolved-preview__copy {
  margin-top: 2px;
  max-width: 56ch;
  font-size: 0.78rem;
  line-height: 1.35;
}

.resolved-preview__shell {
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 16%, var(--xy-border) 84%);
  border-radius: 8px;
  background: color-mix(in srgb, var(--xy-accent) 2%, var(--xy-base) 98%);
}

.resolved-preview__shell-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  min-height: 30px;
  padding: 0 12px;
  border-bottom: 1px solid color-mix(in srgb, var(--xy-accent) 12%, var(--xy-border) 88%);
  background: color-mix(in srgb, var(--xy-surface-0) 84%, transparent);
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: 0.66rem;
  text-transform: uppercase;
}

.resolved-preview__shell-state {
  color: color-mix(in srgb, var(--xy-accent) 72%, var(--xy-text-muted) 28%);
}

.resolved-preview__shell-dots {
  display: inline-flex;
  gap: 4px;
}

.resolved-preview__shell-dots span {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: var(--xy-border-hover);
}

.resolved-preview__shell-dots span:first-child {
  background: color-mix(in srgb, var(--xy-danger) 58%, var(--xy-border-hover) 42%);
}

.resolved-preview__shell-dots span:nth-child(2) {
  background: color-mix(in srgb, var(--xy-warning) 58%, var(--xy-border-hover) 42%);
}

.resolved-preview__shell-dots span:nth-child(3) {
  background: color-mix(in srgb, var(--xy-success) 58%, var(--xy-border-hover) 42%);
}

.resolved-preview__command {
  display: block;
  min-height: 2.85rem;
  padding: 10px 12px 12px;
  margin: 0;
  overflow-x: auto;
  overflow-y: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: 0.82rem;
  line-height: 1.6;
  white-space: pre;
  overflow-wrap: normal;
}

.resolved-preview__empty {
  display: flex;
  min-height: 56px;
  align-items: center;
  padding: 10px 12px;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: 0.82rem;
}

.resolved-preview__prompt {
  color: var(--xy-accent);
  font-family: var(--xy-font-display);
}

.resolved-preview__segment::before {
  content: ' ';
  color: var(--xy-text-muted);
}

.resolved-preview__segment--base {
  color: var(--xy-text-primary);
  font-weight: 700;
}

.resolved-preview__segment--system {
  color: var(--xy-syntax-purple);
}

.resolved-preview__segment--locked,
.resolved-preview__segment--edited {
  color: var(--xy-syntax-amber);
}

.resolved-preview__segment--default {
  color: var(--xy-syntax-cyan);
}

.resolved-preview__segment--added {
  color: var(--xy-syntax-green);
}

.resolved-preview__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 12px;
}

.resolved-preview__legend-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 0.75rem;
  color: var(--xy-text-secondary);
}

.resolved-preview__legend-dot {
  width: 10px;
  height: 10px;
  border-radius: 999px;
}

.resolved-preview__legend-dot--system {
  background: var(--xy-syntax-purple);
}

.resolved-preview__legend-dot--locked,
.resolved-preview__legend-dot--edited {
  background: var(--xy-syntax-amber);
}

.resolved-preview__legend-dot--default {
  background: var(--xy-syntax-cyan);
}

.resolved-preview__legend-dot--added {
  background: var(--xy-syntax-green);
}

@media (max-width: 720px) {
  .resolved-preview__header {
    align-items: stretch;
    gap: var(--xy-space-sm);
  }

  .resolved-preview__title {
    flex-wrap: wrap;
  }

  .resolved-preview__copy,
  .resolved-preview__legend {
    display: none;
  }

  .resolved-preview__shell-bar {
    min-height: 28px;
  }

  .resolved-preview__command {
    min-height: 2.7rem;
    font-size: 0.76rem;
  }
}
</style>
