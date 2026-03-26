<template>
  <section class="resolved-preview" data-testid="resolved-command-preview">
    <div class="resolved-preview__header">
      <div>
        <div class="resolved-preview__title font-display">Resolved preview</div>
        <div class="resolved-preview__copy text-xy-secondary">
          This is the exact argv Xylona will launch after placeholders resolve.
        </div>
      </div>
      <q-btn flat color="accent" label="Copy" @click="copyCommand" />
    </div>

    <div class="resolved-preview__command">
      <code class="resolved-preview__base">{{ baseCommand }}</code>
      <template v-for="block in resolvedBlocks" :key="block.id">
        <code
          v-for="(token, tokenIndex) in block.resolvedTokens"
          :key="`${block.id}-${tokenIndex}`"
          class="resolved-preview__token"
          :class="tokenClass(block.provenance)">
          {{ token }}
        </code>
      </template>
    </div>

    <div class="resolved-preview__legend">
      <span
        v-for="item in legendItems"
        :key="item.provenance"
        class="resolved-preview__legend-item">
        <span class="resolved-preview__legend-dot" :class="tokenClass(item.provenance)"></span>
        {{ item.label }}
      </span>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { copyToClipboard, useQuasar } from 'quasar'

import type { ResolvedStartArgBlock, StartArgProvenance } from './start-args'

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

const fullCommand = computed(() => {
  const tokens = props.resolvedBlocks.flatMap((block) => block.resolvedTokens)
  return [props.baseCommand, ...tokens].filter((value) => value !== '').join(' ')
})

function tokenClass(provenance: StartArgProvenance) {
  return `resolved-preview__token--${provenance}`
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
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
  border: 1px solid var(--xy-border);
  border-radius: 10px;
  background: var(--xy-surface-1);
}

.resolved-preview__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.resolved-preview__title {
  font-size: 0.95rem;
  color: var(--xy-text-primary);
}

.resolved-preview__copy {
  font-size: 0.82rem;
}

.resolved-preview__command {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  padding: var(--xy-space-md);
  border-radius: 10px;
  background: var(--xy-base);
  border: 1px solid var(--xy-border);
}

.resolved-preview__base,
.resolved-preview__token {
  font-family: var(--xy-font-mono);
  font-size: 0.82rem;
  padding: 4px 8px;
  border-radius: 999px;
  white-space: nowrap;
}

.resolved-preview__base {
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-4) 82%, transparent);
}

.resolved-preview__token--system {
  color: #dbd0ff;
  background: rgba(124, 92, 255, 0.18);
}

.resolved-preview__token--locked,
.resolved-preview__token--edited {
  color: #ffe0a3;
  background: rgba(245, 158, 11, 0.18);
}

.resolved-preview__token--default {
  color: #9ddfff;
  background: rgba(28, 183, 207, 0.18);
}

.resolved-preview__token--added {
  color: #b7f5c5;
  background: rgba(34, 197, 94, 0.18);
}

.resolved-preview__legend {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
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
</style>
