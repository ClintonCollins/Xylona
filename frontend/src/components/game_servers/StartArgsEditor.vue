<template>
  <section class="start-args-editor" data-testid="start-args-editor">
    <div class="start-args-editor__header">
      <div>
        <div class="start-args-editor__title font-display">Start arguments</div>
        <div class="start-args-editor__copy text-xy-secondary">
          Edit the argument blocks Xylona passes to the server. One line equals one argv token.
        </div>
      </div>
      <div class="start-args-editor__base">
        <q-icon name="lock" size="14px" />
        <span>Base command</span>
        <code>{{ baseCommand }}</code>
      </div>
    </div>

    <q-banner v-if="!allowEditing" class="start-args-editor__banner" dense inline-actions rounded>
      Start command editing is disabled for this game definition.
    </q-banner>

    <div class="start-args-editor__list" role="list">
      <div v-if="displayBlocks.length === 0" class="start-args-editor__empty">
        <q-icon name="terminal" size="20px" />
        <span>No argument blocks are configured for this template.</span>
      </div>

      <article
        v-for="(block, index) in displayBlocks"
        :key="block.id"
        :data-testid="`arg-row-${block.id}`"
        class="start-args-editor__row"
        role="listitem">
        <div class="start-args-editor__sequence">{{ formatSequence(index) }}</div>
        <div class="start-args-editor__row-main">
          <div class="start-args-editor__row-meta">
            <q-badge :class="badgeClass(block.provenance)" :label="badgeLabel(block.provenance)" />
            <span v-if="block.label" class="start-args-editor__label">{{ block.label }}</span>
          </div>
          <code class="start-args-editor__tokens">{{ formatTokensInline(block.tokens) }}</code>
          <div
            v-if="block.originalTokens && block.originalTokens.length > 0"
            class="start-args-editor__previous">
            was {{ formatTokensInline(block.originalTokens) }}
          </div>
        </div>

        <div class="start-args-editor__actions">
          <q-btn
            v-if="canEdit(block)"
            :data-testid="`edit-${block.id}`"
            :title="`Edit ${block.label || 'argument'}`"
            aria-label="Edit argument"
            dense
            flat
            icon="edit"
            size="sm"
            @click="openEditDialog(block)" />
          <q-btn
            v-if="canRemove(block)"
            :data-testid="`remove-${block.id}`"
            :title="`Remove ${block.label || 'argument'}`"
            aria-label="Remove argument"
            color="negative"
            dense
            flat
            icon="delete"
            size="sm"
            @click="removeBlock(block)" />
          <q-btn
            v-if="canReset(block)"
            :data-testid="`reset-${block.id}`"
            :title="`Reset ${block.label || 'argument'}`"
            aria-label="Reset argument"
            dense
            flat
            icon="restart_alt"
            size="sm"
            @click="resetBlock(block)" />
          <q-btn
            v-if="canMoveUp(index, block)"
            :data-testid="`move-up-${block.id}`"
            :title="`Move ${block.label || 'argument'} up`"
            aria-label="Move argument up"
            dense
            flat
            icon="arrow_upward"
            size="sm"
            @click="moveAddedBlock(index, -1)" />
          <q-btn
            v-if="canMoveDown(index, block)"
            :data-testid="`move-down-${block.id}`"
            :title="`Move ${block.label || 'argument'} down`"
            aria-label="Move argument down"
            dense
            flat
            icon="arrow_downward"
            size="sm"
            @click="moveAddedBlock(index, 1)" />
        </div>
      </article>
    </div>

    <div v-if="allowEditing" class="start-args-editor__footer">
      <q-btn
        color="accent"
        data-testid="add-arg-button"
        flat
        icon="add"
        label="Add argument"
        @click="openAddDialog" />
    </div>

    <q-dialog :model-value="dialogOpen" @update:model-value="onDialogModelChange">
      <q-card class="start-args-editor__dialog">
        <q-card-section>
          <div class="font-display start-args-editor__dialog-title">{{ dialogTitle }}</div>
          <div class="text-xy-secondary start-args-editor__dialog-copy">
            Use one line per token. Tokens are passed exactly as written without shell splitting.
          </div>
        </q-card-section>

        <q-card-section class="start-args-editor__dialog-body">
          <q-input v-model="formState.label" label="Label" outlined />
          <q-input
            v-model="formState.tokensText"
            autogrow
            data-testid="tokens-input"
            label="Tokens"
            outlined
            type="textarea" />
          <q-banner v-if="formError" class="bg-negative text-white rounded-borders" dense>
            {{ formError }}
          </q-banner>
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat label="Cancel" @click="closeDialog" />
          <q-btn
            :label="dialogMode === 'add' ? 'Add argument' : 'Save changes'"
            color="primary"
            data-testid="save-arg-button"
            @click="saveDialog" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <similar-arg-dialog
      :existing-block="similarBlock"
      :new-tokens="pendingSimilarTokens"
      :show="similarDialogOpen"
      @cancel="clearSimilarDialog"
      @replace="replaceExistingArg"
      @add-both="confirmAddBoth" />
  </section>
</template>

<script lang="ts" setup>
import { computed, reactive, ref } from 'vue'

import SimilarArgDialog from './SimilarArgDialog.vue'
import type {
  ResolvedStartArgBlock,
  StartArgBlock,
  StartArgBlocklistEntry,
  StartArgPatch,
} from './start-args'
import {
  applyAddAction,
  clonePatches,
  createPendingSimilarAction,
  findSimilarArg,
  formatTokensInline,
  joinTokensInput,
  resolveStartArgs,
  splitTokensInput,
  validateTokensAgainstBlocklist,
} from './start-args'

const props = defineProps<{
  allowEditing: boolean
  baseCommand: string
  blocklist: StartArgBlocklistEntry[]
  patches: StartArgPatch[]
  template: StartArgBlock[]
}>()

const emit = defineEmits<{
  'update:patches': [patches: StartArgPatch[]]
}>()

const dialogOpen = ref(false)
const dialogMode = ref<'add' | 'edit'>('add')
const editingBlockId = ref('')
const formError = ref('')
const similarDialogOpen = ref(false)
const similarBlock = ref<ResolvedStartArgBlock | null>(null)
const pendingSimilar = ref<ReturnType<typeof createPendingSimilarAction> | null>(null)
const formState = reactive({
  label: '',
  tokensText: '',
})

const displayBlocks = computed(
  () => resolveStartArgs(props.template, props.patches, {}).resolvedBlocks,
)
const pendingSimilarTokens = computed(() => pendingSimilar.value?.tokens ?? [])
const dialogTitle = computed(() =>
  dialogMode.value === 'add' ? 'Add argument block' : 'Edit argument block',
)

function badgeClass(provenance: ResolvedStartArgBlock['provenance']) {
  return `start-args-editor__badge start-args-editor__badge--${provenance}`
}

function badgeLabel(provenance: ResolvedStartArgBlock['provenance']) {
  switch (provenance) {
    case 'system':
      return 'System'
    case 'locked':
      return 'Locked'
    case 'edited':
      return 'Edited'
    case 'added':
      return 'Added'
    default:
      return 'Default'
  }
}

function formatSequence(index: number) {
  return String(index + 1).padStart(2, '0')
}

function canEdit(block: ResolvedStartArgBlock) {
  if (!props.allowEditing) {
    return false
  }

  if (block.provenance === 'added') {
    return true
  }

  return block.ownership === 'editable'
}

function canRemove(block: ResolvedStartArgBlock) {
  if (!props.allowEditing) {
    return false
  }

  return block.provenance === 'added' || block.provenance === 'default'
}

function canReset(block: ResolvedStartArgBlock) {
  return props.allowEditing && block.provenance === 'edited'
}

function canMoveUp(index: number, block: ResolvedStartArgBlock) {
  return props.allowEditing && block.provenance === 'added' && index > 0
}

function canMoveDown(index: number, block: ResolvedStartArgBlock) {
  return (
    props.allowEditing && block.provenance === 'added' && index < displayBlocks.value.length - 1
  )
}

function openAddDialog() {
  dialogMode.value = 'add'
  editingBlockId.value = ''
  formState.label = ''
  formState.tokensText = ''
  formError.value = ''
  dialogOpen.value = true
}

function openEditDialog(block: ResolvedStartArgBlock) {
  dialogMode.value = 'edit'
  editingBlockId.value = block.id
  formState.label = block.label ?? ''
  formState.tokensText = joinTokensInput(block.tokens)
  formError.value = ''
  dialogOpen.value = true
}

function closeDialog() {
  dialogOpen.value = false
  formError.value = ''
}

function onDialogModelChange(value: boolean) {
  if (!value) {
    closeDialog()
  }
}

function removeBlock(block: ResolvedStartArgBlock) {
  const nextPatches = clonePatches(props.patches).filter((patch) => patch.id !== block.id)
  if (block.provenance === 'added') {
    emit('update:patches', nextPatches)
    return
  }

  nextPatches.push({
    id: block.id,
    op: 'remove',
    afterId: null,
  })
  emit('update:patches', nextPatches)
}

function resetBlock(block: ResolvedStartArgBlock) {
  emit(
    'update:patches',
    clonePatches(props.patches).filter((patch) => patch.id !== block.id),
  )
}

function moveAddedBlock(index: number, direction: -1 | 1) {
  const block = displayBlocks.value[index]
  if (!block || block.provenance !== 'added') {
    return
  }

  const reordered = displayBlocks.value.filter((entry) => entry.id !== block.id)
  const targetIndex = Math.max(0, Math.min(reordered.length, index + direction))
  const newAfterId = targetIndex === 0 ? null : reordered[targetIndex - 1].id

  const nextPatches = clonePatches(props.patches).map((patch) =>
    patch.id === block.id ? { ...patch, afterId: newAfterId } : patch,
  )
  emit('update:patches', nextPatches)
}

function saveDialog() {
  const tokens = splitTokensInput(formState.tokensText)
  if (tokens.length === 0) {
    formError.value = 'Add at least one token before saving.'
    return
  }

  const blockedEntry = validateTokensAgainstBlocklist(tokens, props.blocklist)
  if (blockedEntry) {
    formError.value = blockedEntry.reason
    return
  }

  if (dialogMode.value === 'add') {
    const similar = findSimilarArg(tokens, displayBlocks.value)

    if (similar) {
      if (similar.ownership !== 'editable' && similar.provenance !== 'added') {
        formError.value =
          similar.provenance === 'system'
            ? 'This argument is managed by Xylona. Change it in Server Settings instead.'
            : 'This argument is locked by the game definition and cannot be replaced here.'
        return
      }

      pendingSimilar.value = createPendingSimilarAction(
        createPatchId(),
        displayBlocks.value.at(-1)?.id ?? null,
        formState.label.trim(),
        tokens,
      )
      similarBlock.value = similar
      similarDialogOpen.value = true
      dialogOpen.value = false
      return
    }

    const nextPatches = applyAddAction(clonePatches(props.patches), {
      id: createPatchId(),
      afterId: displayBlocks.value.at(-1)?.id ?? null,
      label: formState.label.trim(),
      tokens,
      mode: 'add',
    })
    emit('update:patches', nextPatches)
    closeDialog()
    return
  }

  const existingTemplateBlock = props.template.find((block) => block.id === editingBlockId.value)
  const nextPatches = clonePatches(props.patches).filter(
    (patch) => patch.id !== editingBlockId.value,
  )

  if (!existingTemplateBlock) {
    const existingAddPatch = props.patches.find((patch) => patch.id === editingBlockId.value)
    if (!existingAddPatch) {
      closeDialog()
      return
    }

    nextPatches.push({
      ...existingAddPatch,
      op: 'add',
      label: formState.label.trim(),
      tokens,
    })
    emit('update:patches', nextPatches)
    closeDialog()
    return
  }

  if (
    formatTokensInline(existingTemplateBlock.tokens) === formatTokensInline(tokens) &&
    (existingTemplateBlock.label ?? '') === formState.label.trim()
  ) {
    emit('update:patches', nextPatches)
    closeDialog()
    return
  }

  nextPatches.push({
    id: editingBlockId.value,
    op: 'edit',
    label: formState.label.trim(),
    tokens,
    afterId: null,
  })
  emit('update:patches', nextPatches)
  closeDialog()
}

function clearSimilarDialog() {
  similarDialogOpen.value = false
  similarBlock.value = null
  pendingSimilar.value = null
}

function confirmAddBoth() {
  if (!pendingSimilar.value) {
    clearSimilarDialog()
    return
  }

  emit('update:patches', applyAddAction(clonePatches(props.patches), pendingSimilar.value))
  clearSimilarDialog()
}

function replaceExistingArg() {
  if (!pendingSimilar.value || !similarBlock.value) {
    clearSimilarDialog()
    return
  }

  const nextPatches = clonePatches(props.patches).filter(
    (patch) => patch.id !== similarBlock.value?.id,
  )
  if (similarBlock.value.provenance === 'added') {
    nextPatches.push({
      id: similarBlock.value.id,
      op: 'add',
      label: pendingSimilar.value.label,
      tokens: [...pendingSimilar.value.tokens],
      afterId:
        props.patches.find((patch) => patch.id === similarBlock.value?.id)?.afterId ??
        pendingSimilar.value.afterId,
    })
  } else {
    nextPatches.push({
      id: similarBlock.value.id,
      op: 'edit',
      label: pendingSimilar.value.label || similarBlock.value.label,
      tokens: [...pendingSimilar.value.tokens],
      afterId: null,
    })
  }
  emit('update:patches', nextPatches)
  clearSimilarDialog()
}

function createPatchId() {
  return `patch-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}
</script>

<style scoped>
.start-args-editor {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-lg);
  padding: var(--xy-space-lg);
  border: 1px solid var(--xy-border);
  border-radius: 8px;
  background: var(--xy-surface-gradient-subtle), var(--xy-surface-1);
}

.start-args-editor__header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.start-args-editor__title {
  font-size: 1.02rem;
  line-height: 1.25;
  color: var(--xy-text-primary);
}

.start-args-editor__copy {
  max-width: 56ch;
  margin-top: 4px;
  font-size: 0.82rem;
  line-height: 1.45;
}

.start-args-editor__base {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  max-width: min(100%, 28rem);
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 14%, var(--xy-border) 86%);
  background: color-mix(in srgb, var(--xy-accent) 4%, var(--xy-surface-0) 96%);
  color: var(--xy-text-secondary);
}

.start-args-editor__base code {
  min-width: 0;
  overflow: hidden;
  font-family: var(--xy-font-mono);
  color: var(--xy-text-primary);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.start-args-editor__banner {
  background: var(--xy-warning-bg-soft);
  border: 1px solid var(--xy-warning-border);
  color: var(--xy-text-primary);
}

.start-args-editor__list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.start-args-editor__empty {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 88px;
  padding: var(--xy-space-lg);
  border: 1px dashed var(--xy-border);
  border-radius: 8px;
  background: color-mix(in srgb, var(--xy-surface-0) 82%, transparent);
  color: var(--xy-text-secondary);
  font-size: 0.85rem;
}

.start-args-editor__row {
  display: grid;
  grid-template-columns: 2.5rem minmax(0, 1fr) auto;
  align-items: flex-start;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  border-radius: 8px;
  border: 1px solid var(--xy-border);
  background: color-mix(in srgb, var(--xy-surface-0) 74%, var(--xy-surface-1) 26%);
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast);
}

.start-args-editor__row:hover {
  border-color: var(--xy-border-hover);
  background: color-mix(in srgb, var(--xy-surface-0) 62%, var(--xy-surface-2) 38%);
}

.start-args-editor__sequence {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2rem;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 18%, var(--xy-border) 82%);
  border-radius: 6px;
  background: color-mix(in srgb, var(--xy-accent) 4%, var(--xy-surface-0) 96%);
  color: color-mix(in srgb, var(--xy-accent) 68%, var(--xy-text-secondary) 32%);
  font-family: var(--xy-font-mono);
  font-size: 0.76rem;
}

.start-args-editor__row-main {
  display: flex;
  flex: 1;
  min-width: 0;
  flex-direction: column;
  gap: 6px;
}

.start-args-editor__row-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
}

.start-args-editor__label {
  font-size: 0.82rem;
  color: var(--xy-text-secondary);
}

.start-args-editor__tokens,
.start-args-editor__previous {
  font-family: var(--xy-font-mono);
  font-size: 0.83rem;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.start-args-editor__tokens {
  display: block;
  padding: 10px 12px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 74%, transparent);
  border-radius: 6px;
  background: color-mix(in srgb, var(--xy-base) 72%, var(--xy-surface-0) 28%);
  color: var(--xy-text-primary);
  line-height: 1.45;
}

.start-args-editor__previous {
  color: var(--xy-text-muted);
}

.start-args-editor__actions {
  display: inline-flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 4px;
  min-width: 7.25rem;
}

.start-args-editor__footer {
  display: flex;
  justify-content: flex-start;
}

.start-args-editor__dialog {
  width: min(640px, calc(100vw - 32px));
  border-radius: 8px;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
}

.start-args-editor__dialog-title {
  font-size: 1rem;
  color: var(--xy-text-primary);
}

.start-args-editor__dialog-copy {
  margin-top: 4px;
  font-size: 0.82rem;
}

.start-args-editor__dialog-body {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.start-args-editor__badge {
  border: 1px solid transparent;
}

.start-args-editor__badge--system {
  color: var(--xy-syntax-purple);
  background: var(--xy-syntax-purple-bg);
  border-color: var(--xy-syntax-purple-border);
}

.start-args-editor__badge--locked,
.start-args-editor__badge--edited {
  color: var(--xy-syntax-amber);
  background: var(--xy-syntax-amber-bg);
  border-color: var(--xy-syntax-amber-border);
}

.start-args-editor__badge--default {
  color: var(--xy-syntax-cyan);
  background: var(--xy-syntax-cyan-bg);
  border-color: var(--xy-syntax-cyan-border);
}

.start-args-editor__badge--added {
  color: var(--xy-syntax-green);
  background: var(--xy-syntax-green-bg);
  border-color: var(--xy-syntax-green-border);
}

@media (max-width: 720px) {
  .start-args-editor__row {
    grid-template-columns: 1fr;
  }

  .start-args-editor__actions {
    justify-content: flex-start;
    min-width: 0;
  }
}
</style>
