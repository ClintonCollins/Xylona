<template>
  <div class="blocklist-editor">
    <div v-if="showHeader" class="blocklist-editor__header">
      <div class="font-display blocklist-editor__title">Blocklist</div>
      <div class="text-xy-secondary blocklist-editor__copy">
        Prevent risky or reserved arguments from being saved at the server level.
      </div>
    </div>

    <div class="blocklist-editor__toolbar">
      <div class="blocklist-editor__toolbar-copy">
        <div class="blocklist-editor__count text-xy-secondary">
          {{ blocklist.length }} protected {{ blocklist.length === 1 ? 'rule' : 'rules' }}
        </div>
        <div class="blocklist-editor__toolbar-note text-xy-muted">
          Protect only game-managed flags.
        </div>
      </div>
      <button class="blocklist-editor__action" type="button" @click="openComposer">
        <q-icon aria-hidden="true" name="add" size="16px" />
        Add protected argument
      </button>
    </div>

    <div v-if="blocklist.length === 0 && !removed" class="blocklist-editor__empty text-xy-muted">
      No protected arguments yet. Add a rule only for flags that should never be overridden.
    </div>

    <div ref="rowsRef" class="blocklist-editor__rows">
      <template v-for="{ entry, index } in rows" :key="index">
        <removed-item-undo
          v-if="index < 0"
          :clear-on-save="removed?.clearOnSave"
          :label="entry.pattern.trim() || 'rule'"
          @dismiss="removed = null"
          @undo="undoRemove" />
        <div v-else :aria-label="`Rule ${index + 1}`" class="blocklist-editor__row" role="group">
          <div class="blocklist-editor__row-title">
            {{ String(index + 1).padStart(2, '0') }}
          </div>
          <div class="blocklist-editor__row-fields">
            <q-input
              :error="!isValidRegex(entry.pattern)"
              :model-value="entry.pattern"
              class="blocklist-editor__field blocklist-editor__field--pattern"
              dense
              error-message="Invalid regex"
              hide-bottom-space
              label="Pattern"
              outlined
              placeholder="-javaagent:"
              @update:model-value="updateEntry(index, 'pattern', String($event ?? ''))" />
            <q-input
              :model-value="entry.reason"
              class="blocklist-editor__field blocklist-editor__field--reason"
              dense
              hide-bottom-space
              label="Reason"
              outlined
              placeholder="Why this flag stays protected"
              @update:model-value="updateEntry(index, 'reason', String($event ?? ''))" />
          </div>
          <q-btn
            :aria-label="`Remove ${entry.pattern.trim() || `rule ${index + 1}`}`"
            class="blocklist-editor__delete"
            color="negative"
            dense
            flat
            icon="delete"
            round
            @click="removeEntry(index)" />
        </div>
      </template>
    </div>

    <div v-if="composerOpen" class="blocklist-editor__composer">
      <div class="blocklist-editor__composer-head">
        <div class="blocklist-editor__composer-title">Add protected argument</div>
        <div class="text-xy-secondary blocklist-editor__composer-copy">
          Regex plus a short reason.
        </div>
      </div>
      <div class="blocklist-editor__composer-fields">
        <q-input
          v-model="draftPattern"
          :error="draftPattern !== '' && !isValidRegex(draftPattern)"
          aria-required="true"
          class="blocklist-editor__field blocklist-editor__field--pattern"
          dense
          error-message="Invalid regex"
          hide-bottom-space
          label="Pattern *"
          outlined
          placeholder="-javaagent:" />
        <q-input
          v-model="draftReason"
          class="blocklist-editor__field blocklist-editor__field--reason"
          dense
          hide-bottom-space
          label="Reason"
          outlined
          placeholder="Why this flag stays protected" />
      </div>
      <div class="blocklist-editor__composer-actions">
        <q-btn flat label="Cancel" no-caps @click="closeComposer" />
        <q-btn
          :disable="draftPattern.trim() === '' || !isValidRegex(draftPattern)"
          color="accent"
          label="Add rule"
          no-caps
          outline
          @click="addEntry" />
      </div>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, inject, nextTick, ref, shallowRef } from 'vue'

import type { StartArgBlocklistEntry } from '@/components/game_servers/start-args'
import { gameFormContextKey } from './GameFormTypes'
import RemovedItemUndo from './RemovedItemUndo.vue'

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
const composerOpen = ref(false)
const rowsRef = ref<HTMLElement | null>(null)
const formIsDirty = inject(gameFormContextKey, null)?.isDirty

// The most recent removal. Its negative id keys the undo row, so each removal mounts a fresh one.
const removed = shallowRef<{
  entry: StartArgBlocklistEntry
  index: number
  after?: StartArgBlocklistEntry
  clearOnSave: boolean
  id: number
} | null>(null)
let removals = 0

// Undo puts the rule back after the neighbour it had, or at its old index if that one is gone.
const removedAt = computed(() => {
  const removal = removed.value
  if (!removal) {
    return -1
  }
  const after = removal.after ? props.blocklist.indexOf(removal.after) : -1
  return after >= 0 ? after + 1 : Math.min(removal.index, props.blocklist.length)
})

// The rules as shown: the removed one keeps its slot as an undo row.
const rows = computed(() => {
  const rows = props.blocklist.map((entry, index) => ({ entry, index }))
  if (removed.value) {
    rows.splice(removedAt.value, 0, { entry: removed.value.entry, index: -removed.value.id })
  }
  return rows
})

function updateEntry(index: number, key: 'pattern' | 'reason', value: string) {
  removed.value = null
  const nextBlocklist = props.blocklist.map((entry, currentIndex) =>
    currentIndex === index ? { ...entry, [key]: value } : entry,
  )
  emit('update:blocklist', nextBlocklist)
}

// One click removes the rule and the next Save makes it permanent, so an undo row takes its place.
function removeEntry(index: number) {
  const entry = props.blocklist[index]
  if (!entry) {
    return
  }

  const clearOnSave = formIsDirty?.value === false
  const after = props.blocklist[index - 1]
  emit(
    'update:blocklist',
    props.blocklist.filter((_, currentIndex) => currentIndex !== index),
  )
  removed.value = { entry, index, after, clearOnSave, id: ++removals }
}

function undoRemove() {
  const removal = removed.value
  if (!removal) {
    return
  }

  const at = removedAt.value
  removed.value = null
  emit('update:blocklist', [
    ...props.blocklist.slice(0, at),
    removal.entry,
    ...props.blocklist.slice(at),
  ])
  void nextTick(() =>
    rowsRef.value?.querySelectorAll('.blocklist-editor__row')[at]?.querySelector('input')?.focus(),
  )
}

function addEntry() {
  if (draftPattern.value.trim() === '' || !isValidRegex(draftPattern.value)) {
    return
  }

  removed.value = null
  emit('update:blocklist', [
    ...props.blocklist,
    {
      pattern: draftPattern.value.trim(),
      reason: draftReason.value.trim() || 'Blocked by game definition',
    },
  ])
  draftPattern.value = ''
  draftReason.value = ''
  composerOpen.value = false
}

function isValidRegex(value: string) {
  try {
    new RegExp(value)
    return true
  } catch {
    return false
  }
}

function openComposer() {
  composerOpen.value = true
}

function closeComposer() {
  composerOpen.value = false
  draftPattern.value = ''
  draftReason.value = ''
}
</script>

<style scoped>
.blocklist-editor {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
}

.blocklist-editor__header {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.blocklist-editor__title {
  font-size: var(--xy-font-size-base);
  color: var(--xy-text-primary);
}

.blocklist-editor__copy {
  font-size: var(--xy-font-size-sm);
}

.blocklist-editor__empty {
  padding: 0.95rem;
  border-radius: var(--xy-radius-xl);
  border: 1px dashed var(--xy-border);
  background: color-mix(in srgb, var(--xy-surface-0) 84%, transparent);
}

.blocklist-editor__toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding-bottom: 0.1rem;
}

.blocklist-editor__toolbar-copy {
  display: flex;
  flex-direction: column;
  gap: 0.14rem;
}

.blocklist-editor__count {
  font-size: var(--xy-font-size-sm);
  color: color-mix(in srgb, var(--xy-accent) 12%, var(--xy-text-secondary) 88%);
}

.blocklist-editor__toolbar-note {
  font-size: var(--xy-font-size-xs);
  line-height: 1.35;
}

.blocklist-editor__action {
  appearance: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 2rem;
  padding: 0.32rem 0.72rem;
  border-radius: var(--xy-radius-md);
  border: 1px solid color-mix(in srgb, var(--xy-accent) 30%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-0) 92%);
  color: color-mix(in srgb, var(--xy-accent) 34%, var(--xy-text-primary) 66%);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    color var(--xy-transition-fast),
    transform 120ms ease-out,
    box-shadow var(--xy-transition-fast);
}

.blocklist-editor__action:hover {
  border-color: color-mix(in srgb, var(--xy-accent) 42%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 11%, var(--xy-surface-0) 89%);
}

.blocklist-editor__action:active {
  transform: translateY(1px);
}

.blocklist-editor__action :deep(.q-icon) {
  transition: transform 180ms ease-out;
}

.blocklist-editor__action:hover :deep(.q-icon) {
  transform: rotate(90deg) scale(1.06);
}

.blocklist-editor__rows {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.blocklist-editor__row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: start;
  gap: 0.6rem;
  padding: 0.52rem 0.6rem;
  border-radius: var(--xy-radius-lg);
  border: 1px solid color-mix(in srgb, var(--xy-accent) 10%, var(--xy-border) 90%);
  background: color-mix(in srgb, var(--xy-accent) 2%, var(--xy-surface-0) 98%);
}

.blocklist-editor__row-title {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.7rem;
  min-height: 1.7rem;
  padding: 0 0.35rem;
  border-radius: var(--xy-radius-pill);
  border: 1px solid color-mix(in srgb, var(--xy-accent) 30%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 8%, var(--xy-surface-1) 92%);
  font-size: var(--xy-font-size-2xs);
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--xy-accent) 58%, var(--xy-text-primary) 42%);
}

.blocklist-editor__row-fields {
  display: grid;
  grid-template-columns: minmax(156px, 0.62fr) minmax(0, 1.38fr);
  gap: 0.5rem;
}

.blocklist-editor__field {
  min-width: 0;
}

.blocklist-editor__delete {
  margin-top: 0;
}

.blocklist-editor__field :deep(.q-field__control) {
  min-height: 36px;
  background: color-mix(in srgb, var(--xy-accent) 2%, var(--xy-surface-1) 98%);
}

.blocklist-editor__field :deep(.q-field__marginal),
.blocklist-editor__field :deep(.q-field__prepend),
.blocklist-editor__field :deep(.q-field__append) {
  background: transparent;
}

.blocklist-editor__field :deep(.q-field__bottom) {
  min-height: 0;
  padding-top: 0;
}

.blocklist-editor__field--pattern :deep(.q-field__native),
.blocklist-editor__field--pattern :deep(input) {
  font-family: var(--xy-font-mono);
}

.blocklist-editor__row :deep(.q-btn) {
  min-height: 40px;
  min-width: 40px;
}

@media (pointer: coarse), (any-pointer: coarse) {
  .blocklist-editor__action {
    min-height: 44px;
  }

  .blocklist-editor__row :deep(.q-btn) {
    min-height: 44px;
    min-width: 44px;
  }
}

.blocklist-editor__composer {
  display: grid;
  gap: 0.7rem;
  padding: 0.82rem;
  border-radius: var(--xy-radius-xl);
  border: 1px solid color-mix(in srgb, var(--xy-accent) 38%, var(--xy-border));
  background:
    linear-gradient(180deg, color-mix(in srgb, var(--xy-accent) 5%, transparent), transparent 48%),
    color-mix(in srgb, var(--xy-accent) 3%, var(--xy-surface-0) 97%);
}

.blocklist-editor-composer-enter-active,
.blocklist-editor-composer-leave-active {
  transition:
    opacity 180ms cubic-bezier(0.25, 1, 0.5, 1),
    transform 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

.blocklist-editor-composer-enter-from,
.blocklist-editor-composer-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

@media (prefers-reduced-motion: reduce) {
  .blocklist-editor-composer-enter-active,
  .blocklist-editor-composer-leave-active,
  .blocklist-editor__action,
  .blocklist-editor__action :deep(.q-icon) {
    transition: none;
  }
}

.blocklist-editor__composer-head {
  display: flex;
  flex-direction: column;
  gap: 0.28rem;
}

.blocklist-editor__composer-title {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

.blocklist-editor__composer-copy {
  font-size: var(--xy-font-size-sm);
}

.blocklist-editor__composer-fields {
  display: grid;
  grid-template-columns: minmax(190px, 0.76fr) minmax(0, 1.24fr);
  gap: 0.6rem;
}

.blocklist-editor__composer-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.65rem;
}

@media (max-width: 599px) {
  .blocklist-editor__toolbar {
    flex-direction: column;
    align-items: stretch;
  }

  .blocklist-editor__row {
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: center;
    gap: 0.45rem;
    padding: 0.48rem 0.52rem;
  }

  .blocklist-editor__row-title {
    display: none;
  }

  .blocklist-editor__row-fields,
  .blocklist-editor__composer-fields {
    grid-template-columns: minmax(0, 1fr);
  }

  .blocklist-editor__row-fields {
    grid-column: 1 / -1;
    gap: 0.4rem;
  }

  .blocklist-editor__delete {
    justify-self: end;
  }

  .blocklist-editor__composer-actions {
    flex-direction: column-reverse;
    align-items: stretch;
  }
}
</style>
