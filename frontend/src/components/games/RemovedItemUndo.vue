<template>
  <div ref="rootRef" class="removed-item-undo" role="status" @keydown.esc.stop="dismiss">
    <span class="removed-item-undo__text">Removed {{ label }}.</span>
    <q-btn
      ref="undoRef"
      :aria-label="`Undo removing ${label}`"
      color="primary"
      dense
      flat
      label="Undo"
      no-caps
      @click="emit('undo')" />
    <q-btn aria-label="Dismiss" dense flat icon="close" round size="sm" @click="dismiss">
      <q-tooltip>Dismiss</q-tooltip>
    </q-btn>
  </div>
</template>

<script lang="ts" setup>
import { inject, onMounted, ref, watch } from 'vue'

import { gameFormContextKey } from './GameFormTypes'

// Stands in for a just-removed row until Undo, Dismiss (or Escape), the list's next edit, or
// Save. It takes focus on mount, so keyboard and screen-reader users land on Undo.
const props = defineProps<{
  label: string
  // Pass true only when the game form was clean before the removal. Otherwise the form
  // turning clean can mean the removal undid unsaved work, not a Save, and the undo must stay.
  clearOnSave?: boolean
}>()

const emit = defineEmits<{ undo: []; dismiss: [] }>()

const rootRef = ref<HTMLElement | null>(null)
const undoRef = ref<{ $el: HTMLElement } | null>(null)
const formIsDirty = inject(gameFormContextKey, null)?.isDirty

onMounted(() => undoRef.value?.$el.focus())

watch(
  () => formIsDirty?.value,
  (dirty) => {
    if (dirty === false && props.clearOnSave) {
      emit('dismiss')
    }
  },
)

const focusable =
  'button:not([disabled]), input:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'

// The row is about to go, so hand focus to the nearest visible control before it, not <body>.
function dismiss() {
  controlBefore(rootRef.value)?.focus()
  emit('dismiss')
}

function controlBefore(start: Element | null): HTMLElement | undefined {
  for (let element = start; element; element = element.parentElement) {
    for (
      let sibling = element.previousElementSibling;
      sibling;
      sibling = sibling.previousElementSibling
    ) {
      const target = [sibling, ...sibling.querySelectorAll(focusable)]
        .filter(
          (candidate) => candidate.matches(focusable) && candidate.checkVisibility?.() !== false,
        )
        .pop()
      if (target instanceof HTMLElement) {
        return target
      }
    }
  }
  return undefined
}
</script>

<style scoped>
.removed-item-undo {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-2xs) var(--xy-space-xs) var(--xy-space-2xs) var(--xy-space-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  background: var(--xy-surface-2);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: 1.4;
}

.removed-item-undo__text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}
</style>
