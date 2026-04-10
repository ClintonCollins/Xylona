<template>
  <div
    :class="`template-editor--${selectedPlatform}`"
    :data-testid="rootTestId"
    class="template-editor">
    <div v-if="availablePlatforms.length === 0" class="template-editor__empty text-xy-muted">
      Enable Linux or Windows support above to configure structured start arguments.
    </div>
    <template v-else>
      <div
        v-if="showPlatformTabs && availablePlatforms.length > 1"
        aria-label="Platform launch sequence"
        class="platform-tabs"
        role="tablist">
        <button
          v-for="platform in availablePlatforms"
          :key="platform"
          :aria-selected="selectedPlatform === platform"
          :class="{ 'platform-tab--active': selectedPlatform === platform }"
          class="platform-tab"
          role="tab"
          type="button"
          @click="selectedPlatform = platform">
          <q-icon
            :class="
              platform === 'windows'
                ? selectedPlatform === 'windows'
                  ? 'platform-icon-windows-active'
                  : 'platform-icon-inactive'
                : selectedPlatform === 'linux'
                  ? 'platform-icon-linux-active'
                  : 'platform-icon-inactive'
            "
            :name="platform === 'windows' ? 'desktop_windows' : 'terminal'"
            size="14px" />
          <span class="font-display">{{ platformLabel(platform) }}</span>
        </button>
      </div>

      <section
        v-if="showPreviewSection"
        class="template-editor__card template-editor__card--preview">
        <div class="template-editor__toolbar">
          <div class="template-editor__toolbar-copy">
            <p class="template-editor__eyebrow">Launch Sequence</p>
            <span class="template-editor__toolbar-meta">
              {{ platformLabel(selectedPlatform) }} ·
              {{ argumentCountLabel(currentTemplate.length) }}
            </span>
          </div>
          <div class="template-editor__toolbar-actions">
            <q-btn
              color="primary"
              data-testid="start-args-add-block"
              dense
              icon="add"
              label="Add argument"
              no-caps
              @click="openAddDialog" />
          </div>
        </div>

        <section class="template-editor__command-shell" data-testid="start-args-preview-shell">
          <div
            class="template-editor__command-line"
            @dragover.prevent="activatePreviewEnd"
            @drop.prevent="dropAtPreviewEnd">
            <span aria-hidden="true" class="template-editor__prompt">{{
              selectedPlatform === 'windows' ? '>' : '$'
            }}</span>
            <label class="template-editor__base-command">
              <span class="template-editor__sr-only">Base command</span>
              <input
                :value="currentBaseCommand"
                aria-label="Base command"
                class="template-editor__base-command-input font-mono"
                data-testid="start-args-base-command"
                placeholder="Base command"
                type="text"
                @input="updateBaseCommand(($event.target as HTMLInputElement).value)" />
            </label>

            <template v-if="currentTemplate.length > 0">
              <template v-for="(block, index) in currentTemplate" :key="block.id">
                <span
                  v-if="dragInsertionIndex === index"
                  aria-hidden="true"
                  class="template-editor__drop-marker"></span>

                <button
                  :aria-label="previewSegmentAriaLabel(block, index)"
                  :class="previewChipClass(block, index)"
                  :data-testid="`preview-chip-${block.id}`"
                  :style="viewTransitionStyleForChip(block.id)"
                  :title="previewChipTitle(block)"
                  class="template-editor__arg-chip"
                  draggable="true"
                  type="button"
                  @click="openEditDialog(block.id, 'preview')"
                  @dragend="onDragEnd"
                  @dragstart="onPreviewDragStart(index, $event)"
                  @dragover.prevent.stop="activatePreviewInsertion(index, $event)"
                  @drop.prevent.stop="dropOnPreview(index, $event)">
                  <q-icon
                    aria-hidden="true"
                    class="template-editor__arg-chip-handle"
                    name="drag_indicator"
                    size="16px" />
                  <span class="template-editor__arg-chip-text font-mono">{{
                    previewChipText(block)
                  }}</span>
                </button>
              </template>

              <span
                v-if="dragInsertionIndex === currentTemplate.length"
                aria-hidden="true"
                class="template-editor__drop-marker"></span>
            </template>

            <button
              v-else
              class="template-editor__empty-chip"
              data-testid="preview-empty-add"
              type="button"
              @click="openAddDialog">
              Add first runtime argument
            </button>
          </div>

          <div class="template-editor__command-footer">
            <span class="template-editor__command-hint">
              Click an argument to edit it. Drag to reorder.
            </span>
            <div class="template-editor__command-tools">
              <button
                :disabled="!canResetOrder"
                class="template-editor__button template-editor__button--quiet"
                data-testid="start-args-reset-order"
                type="button"
                @click="resetCurrentPlatformOrder">
                Reset order
              </button>
              <button
                :disabled="!canResetPlatform"
                class="template-editor__button template-editor__button--quiet"
                data-testid="start-args-reset-platform"
                type="button"
                @click="resetCurrentPlatform">
                Reset all
              </button>
            </div>
          </div>
        </section>
      </section>

      <section
        v-if="showAdvancedSection"
        class="template-editor__card template-editor__card--advanced">
        <button
          :aria-expanded="String(isAdvancedExpanded)"
          class="template-editor__advanced-toggle"
          data-testid="start-args-advanced-panel-toggle"
          type="button"
          @click="toggleAdvanced">
          <span class="template-editor__advanced-copy">
            <span class="template-editor__eyebrow">Advanced Sequence</span>
            <span class="template-editor__advanced-meta">
              {{ argumentCountLabel(currentTemplate.length) }} · fallback ordering
            </span>
          </span>
          <span class="template-editor__toggle-indicator font-display">
            {{ isAdvancedExpanded ? 'Hide details' : 'Order details' }}
            <q-icon :name="isAdvancedExpanded ? 'expand_less' : 'expand_more'" size="18px" />
          </span>
        </button>

        <div
          v-if="isAdvancedExpanded"
          aria-label="Advanced launch sequence inspector"
          class="template-editor__sequence-list"
          role="list">
          <article
            v-for="(block, index) in currentTemplate"
            :key="block.id"
            :class="{ 'template-editor__sequence-row--selected': selectedBlockID === block.id }"
            :data-testid="`advanced-row-${block.id}`"
            class="template-editor__sequence-row">
            <button
              :aria-label="inventorySelectAriaLabel(block, index)"
              :aria-pressed="String(selectedBlockID === block.id)"
              class="template-editor__sequence-item"
              type="button"
              @click="openEditDialog(block.id, 'advanced')">
              <span class="template-editor__order">{{ index + 1 }}</span>
              <code class="template-editor__sequence-preview font-mono">
                {{ inventoryPreview(block) }}
              </code>
            </button>
            <span :class="ownershipPillClass(block.ownership)" class="template-editor__state">
              {{ ownershipLabel(block.ownership) }}
            </span>
            <div class="template-editor__sequence-actions">
              <div
                :aria-label="`Reorder argument ${index + 1}`"
                class="template-editor__stepper"
                role="group">
                <button
                  :disabled="index <= 0"
                  aria-label="Move argument up"
                  class="template-editor__icon template-editor__icon--step"
                  type="button"
                  @click="moveBlock(index, -1)">
                  ↑
                </button>
                <button
                  :disabled="index >= currentTemplate.length - 1"
                  aria-label="Move argument down"
                  class="template-editor__icon template-editor__icon--step"
                  type="button"
                  @click="moveBlock(index, 1)">
                  ↓
                </button>
              </div>
            </div>
          </article>
        </div>
      </section>

      <q-dialog
        :model-value="dialogOpen"
        transition-hide="fade"
        transition-show="fade"
        @update:model-value="handleDialogModelChange">
        <q-card v-if="dialogOpen" class="template-editor__dialog" data-testid="start-args-dialog">
          <q-card-section class="template-editor__dialog-head">
            <div class="template-editor__dialog-head-main">
              <p class="template-editor__eyebrow">
                {{ dialogMode === 'add' ? 'Add Argument' : 'Edit Argument' }}
              </p>
              <h3 class="template-editor__dialog-title font-display">{{ dialogTitle }}</h3>
              <p class="template-editor__dialog-copy text-xy-secondary">{{ dialogSubtitle }}</p>
              <div v-if="dialogMode === 'edit'" class="template-editor__dialog-hero">
                <span
                  :class="dialogHeroChipClass"
                  :style="dialogHeroTransitionStyle"
                  class="template-editor__dialog-hero-chip">
                  <span class="template-editor__dialog-hero-text font-mono">
                    {{ dialogHeroText }}
                  </span>
                </span>
              </div>
            </div>
            <span :class="ownershipPillClass(dialogDraft.ownership)" class="template-editor__state">
              {{ ownershipLabel(dialogDraft.ownership) }}
            </span>
          </q-card-section>

          <q-card-section class="template-editor__dialog-body">
            <q-select
              :model-value="dialogDraft.ownership"
              :options="ownershipOptions"
              data-testid="start-args-dialog-ownership"
              emit-value
              label="Mutability"
              map-options
              outlined
              @update:model-value="
                dialogDraft.ownership = ($event as StartArgOwnership) ?? 'editable'
              " />

            <q-input
              :model-value="dialogDraft.label"
              data-testid="start-args-dialog-label"
              label="Label"
              outlined
              @update:model-value="dialogDraft.label = String($event ?? '')" />

            <q-input
              :model-value="dialogDraft.tokensText"
              autogrow
              data-testid="tokens-input"
              hint="One argument per line. These become argv tokens exactly as written."
              label="Arguments"
              outlined
              type="textarea"
              @update:model-value="dialogDraft.tokensText = String($event ?? '')" />

            <q-select
              v-if="dialogDraft.ownership === 'system'"
              :model-value="dialogDraft.managedSource"
              :options="startArgManagedSourceOptions"
              data-testid="start-args-dialog-managed-source"
              emit-value
              hint="Choose from the valid runtime placeholders."
              label="Managed Source"
              map-options
              outlined
              @update:model-value="dialogDraft.managedSource = String($event ?? '')" />
          </q-card-section>

          <q-card-actions align="between" class="template-editor__dialog-actions">
            <q-btn
              v-if="dialogMode === 'edit'"
              color="negative"
              data-testid="start-args-remove-block"
              flat
              label="Remove"
              @click="removeEditingBlock" />
            <div class="template-editor__dialog-actions-main">
              <q-btn flat label="Cancel" @click="closeDialog" />
              <q-btn
                :label="dialogMode === 'add' ? 'Add argument' : 'Save changes'"
                color="primary"
                data-testid="save-arg-button"
                @click="saveDialog" />
            </div>
          </q-card-actions>
        </q-card>
      </q-dialog>
    </template>
  </div>
</template>

<script lang="ts" setup>
import { computed, getCurrentInstance, nextTick, reactive, ref, watch } from 'vue'

import type { StartArgBlock, StartArgOwnership } from '@/components/game_servers/start-args'
import { formatTokensInline, joinTokensInput, splitTokensInput, } from '@/components/game_servers/start-args'
import { getManagedSourceLabel, startArgManagedSourceOptions, } from '@/components/shared/placeholder-definitions'

type Platform = 'linux' | 'windows'

type ViewTransitionCapableDocument = Document & {
  startViewTransition?: (updateCallback: () => void | Promise<void>) => { finished: Promise<void> }
}

const props = withDefaults(
  defineProps<{
    activePlatform?: Platform
    advancedExpanded?: boolean
    baselineLinuxBaseCommand?: string
    baselineLinuxTemplate?: StartArgBlock[]
    baselineWindowsBaseCommand?: string
    baselineWindowsTemplate?: StartArgBlock[]
    linuxBaseCommand: string
    linuxEnabled?: boolean
    linuxTemplate: StartArgBlock[]
    mode?: 'full' | 'preview' | 'advanced'
    windowsBaseCommand: string
    windowsEnabled?: boolean
    windowsTemplate: StartArgBlock[]
  }>(),
  { linuxEnabled: true, mode: 'full', windowsEnabled: true },
)

const emit = defineEmits<{
  'update:activePlatform': [value: Platform]
  'update:advancedExpanded': [value: boolean]
  'update:linuxBaseCommand': [value: string]
  'update:linuxTemplate': [value: StartArgBlock[]]
  'update:windowsBaseCommand': [value: string]
  'update:windowsTemplate': [value: StartArgBlock[]]
}>()

const ownershipOptions = [
  { label: 'System', value: 'system' },
  { label: 'Locked', value: 'locked' },
  { label: 'Editable', value: 'editable' },
] as const satisfies ReadonlyArray<{ label: string; value: StartArgOwnership }>

const availablePlatforms = computed<Platform[]>(() => {
  const platforms: Platform[] = []
  if (props.windowsEnabled) platforms.push('windows')
  if (props.linuxEnabled) platforms.push('linux')
  return platforms
})

const internalPlatform = ref<Platform>('windows')
const internalAdvancedExpanded = ref(false)
const selectedBlockIDs = ref<Record<Platform, string | null>>({ linux: null, windows: null })
const draggedIndex = ref<number | null>(null)
const dragInsertionIndex = ref<number | null>(null)
const dialogOpen = ref(false)
const dialogMode = ref<'add' | 'edit'>('add')
const dialogEditingBlockID = ref<string | null>(null)
const activeViewTransitionName = ref<string | null>(null)
const activeViewTransitionBlockID = ref<string | null>(null)

const dialogDraft = reactive<{
  label: string
  managedSource: string
  ownership: StartArgOwnership
  tokensText: string
}>({
  label: '',
  managedSource: '',
  ownership: 'editable',
  tokensText: '',
})

const instance = getCurrentInstance()
const hasControlledAdvancedExpanded = computed(() => {
  const vnodeProps = instance?.vnode.props ?? {}
  return 'advancedExpanded' in vnodeProps || 'advanced-expanded' in vnodeProps
})

const selectedPlatform = computed<Platform>({
  get: () => {
    const candidate = props.activePlatform ?? internalPlatform.value
    if (availablePlatforms.value.includes(candidate)) return candidate
    return availablePlatforms.value[0] ?? 'windows'
  },
  set: (value) => {
    internalPlatform.value = value
    emit('update:activePlatform', value)
  },
})

const isAdvancedExpanded = computed<boolean>({
  get: () =>
    hasControlledAdvancedExpanded.value
      ? (props.advancedExpanded ?? false)
      : internalAdvancedExpanded.value,
  set: (value) => {
    internalAdvancedExpanded.value = value
    emit('update:advancedExpanded', value)
  },
})

const currentBaseCommand = computed(() =>
  selectedPlatform.value === 'windows' ? props.windowsBaseCommand : props.linuxBaseCommand,
)
const currentTemplate = computed(() =>
  selectedPlatform.value === 'windows' ? props.windowsTemplate : props.linuxTemplate,
)
const otherPlatform = computed<Platform>(() =>
  selectedPlatform.value === 'windows' ? 'linux' : 'windows',
)
const otherTemplate = computed(() =>
  otherPlatform.value === 'windows' ? props.windowsTemplate : props.linuxTemplate,
)
const baselineBaseCommand = computed(() =>
  selectedPlatform.value === 'windows'
    ? (props.baselineWindowsBaseCommand ?? props.windowsBaseCommand)
    : (props.baselineLinuxBaseCommand ?? props.linuxBaseCommand),
)
const baselineTemplate = computed(() =>
  selectedPlatform.value === 'windows'
    ? (props.baselineWindowsTemplate ?? props.windowsTemplate)
    : (props.baselineLinuxTemplate ?? props.linuxTemplate),
)
const selectedBlockID = computed({
  get: () => selectedBlockIDs.value[selectedPlatform.value],
  set: (value: string | null) => {
    selectedBlockIDs.value = { ...selectedBlockIDs.value, [selectedPlatform.value]: value }
  },
})
const showPlatformTabs = computed(() => props.mode !== 'advanced')
const showPreviewSection = computed(() => props.mode !== 'advanced')
const showAdvancedSection = computed(() => props.mode !== 'preview')
const rootTestId = computed(() => `start-args-template-editor-${props.mode}`)
const canResetPlatform = computed(
  () =>
    currentBaseCommand.value !== baselineBaseCommand.value ||
    templateSignature(currentTemplate.value) !== templateSignature(baselineTemplate.value),
)
const canResetOrder = computed(
  () =>
    templatesShareSameIDs(currentTemplate.value, baselineTemplate.value) &&
    templateIDSequence(currentTemplate.value) !== templateIDSequence(baselineTemplate.value),
)
const dialogTitle = computed(() => {
  if (dialogMode.value === 'add') return 'Add runtime argument'
  return dialogDraft.label.trim() !== '' ? dialogDraft.label.trim() : 'Edit runtime argument'
})
const dialogSubtitle = computed(() => {
  const preview = formatTokensInline(splitTokensInput(dialogDraft.tokensText)).trim()
  if (preview !== '') return preview
  return dialogMode.value === 'add'
    ? 'Create a new argument at the end of the preview, then drag it into place.'
    : 'Update the selected argument and keep the preview readable.'
})
const dialogHeroText = computed(() => {
  const preview = formatTokensInline(splitTokensInput(dialogDraft.tokensText)).trim()
  if (preview !== '') return preview

  const label = dialogDraft.label.trim()
  if (label !== '') return label

  return 'Runtime argument'
})
const dialogHeroChipClass = computed(() => [`template-editor__arg-chip--${dialogDraft.ownership}`])
const dialogHeroTransitionStyle = computed(() =>
  activeViewTransitionName.value === null
    ? undefined
    : ({ viewTransitionName: activeViewTransitionName.value } as const),
)

watch(
  availablePlatforms,
  (platforms) => {
    if (!platforms.includes(selectedPlatform.value)) {
      selectedPlatform.value = platforms[0] ?? 'windows'
    }
  },
  { immediate: true },
)

watch(
  currentTemplate,
  (template) => {
    if (template.length === 0) {
      selectedBlockID.value = null
      return
    }

    const currentSelection = selectedBlockID.value
    const hasSelection =
      currentSelection !== null && template.some((block) => block.id === currentSelection)
    if (!hasSelection) selectedBlockID.value = template[0].id

    if (
      dialogMode.value === 'edit' &&
      dialogEditingBlockID.value !== null &&
      !template.some((block) => block.id === dialogEditingBlockID.value)
    ) {
      closeDialog()
    }
  },
  { immediate: true, deep: true },
)

watch(selectedPlatform, () => {
  onDragEnd()
  closeDialog()
})

function platformLabel(platform: Platform) {
  return platform === 'windows' ? 'Windows' : 'Linux'
}

function argumentCountLabel(count: number) {
  return `${count} argument${count === 1 ? '' : 's'}`
}

function previewChipClass(block: StartArgBlock, index: number) {
  return [
    `template-editor__arg-chip--${block.ownership}`,
    selectedBlockID.value === block.id ? 'template-editor__arg-chip--selected' : '',
    draggedIndex.value === index ? 'template-editor__arg-chip--dragging' : '',
  ]
}

function previewSegmentAriaLabel(block: StartArgBlock, index: number) {
  return `Edit argument ${index + 1}: ${previewChipTitle(block)}`
}

function viewTransitionStyleForChip(blockID: string) {
  if (activeViewTransitionName.value === null || activeViewTransitionBlockID.value !== blockID) {
    return undefined
  }

  return { viewTransitionName: activeViewTransitionName.value } as const
}

function previewChipText(block: StartArgBlock) {
  const preview = formatTokensInline(block.tokens).trim()
  if (preview !== '') return preview
  return blockTitle(block)
}

function previewChipTitle(block: StartArgBlock) {
  const preview = formatTokensInline(block.tokens).trim()
  if (preview !== '') return preview
  return blockTitle(block)
}

function ownershipLabel(ownership: StartArgOwnership) {
  if (ownership === 'system') return 'System'
  if (ownership === 'locked') return 'Locked'
  return 'Editable'
}

function ownershipPillClass(ownership: StartArgOwnership) {
  return `template-editor__state--${ownership}`
}

function blockTitle(block: StartArgBlock) {
  const label = block.label?.trim() ?? ''
  if (label !== '') return label

  const managedSource = block.managedSource?.trim() ?? ''
  if (managedSource !== '') return getManagedSourceLabel(managedSource)

  return block.tokens[0] ?? 'Untitled argument'
}

function inventoryPreview(block: StartArgBlock) {
  const preview = previewChipText(block)
  return preview.length <= 80 ? preview : `${preview.slice(0, 80)}…`
}

function inventorySelectAriaLabel(block: StartArgBlock, index: number) {
  return `Edit argument ${index + 1}: ${blockTitle(block)} (${ownershipLabel(block.ownership)})`
}

function toggleAdvanced() {
  isAdvancedExpanded.value = !isAdvancedExpanded.value
}

function updateBaseCommand(value: string) {
  if (selectedPlatform.value === 'windows') {
    emit('update:windowsBaseCommand', value)
    return
  }

  emit('update:linuxBaseCommand', value)
}

function selectBlock(blockID: string | null) {
  selectedBlockID.value = blockID
}

function openEditDialog(blockID: string, origin: 'preview' | 'advanced' = 'preview') {
  const block = currentTemplate.value.find((candidate) => candidate.id === blockID)
  if (!block) return

  const applyOpen = () => {
    selectBlock(blockID)
    dialogMode.value = 'edit'
    dialogEditingBlockID.value = blockID
    dialogDraft.label = block.label ?? ''
    dialogDraft.managedSource = block.managedSource ?? ''
    dialogDraft.ownership = block.ownership
    dialogDraft.tokensText = joinTokensInput(block.tokens ?? [])
    dialogOpen.value = true
  }

  if (origin === 'preview') {
    void runArgumentViewTransition(blockID, applyOpen)
    return
  }

  applyOpen()
}

function openAddDialog() {
  dialogMode.value = 'add'
  dialogEditingBlockID.value = null
  dialogDraft.label = ''
  dialogDraft.managedSource = ''
  dialogDraft.ownership = 'editable'
  dialogDraft.tokensText = ''
  dialogOpen.value = true
}

function closeDialog() {
  const editingBlockID = dialogMode.value === 'edit' ? dialogEditingBlockID.value : null
  const canMorphBack =
    editingBlockID !== null && currentTemplate.value.some((block) => block.id === editingBlockID)

  if (canMorphBack) {
    void runArgumentViewTransition(editingBlockID, () => {
      dialogOpen.value = false
    })
    return
  }

  dialogOpen.value = false
}

function handleDialogModelChange(value: boolean) {
  if (!value) closeDialog()
}

function saveDialog() {
  const patch = buildDialogBlockPatch()

  if (dialogMode.value === 'add') {
    const blockID = createBlockId()
    const nextTemplate = normalizeTemplate([
      ...currentTemplate.value,
      {
        id: blockID,
        order: currentTemplate.value.length,
        ownership: patch.ownership ?? 'editable',
        label: patch.label ?? '',
        managedSource: patch.managedSource ?? '',
        tokens: patch.tokens ?? [],
      },
    ])
    selectBlock(blockID)
    emitTemplate(nextTemplate)
    closeDialog()
    return
  }

  const editingIndex = currentTemplate.value.findIndex(
    (block) => block.id === dialogEditingBlockID.value,
  )
  if (editingIndex < 0) return

  const editingBlockID = dialogEditingBlockID.value
  if (editingBlockID !== null) {
    void runArgumentViewTransition(editingBlockID, () => {
      updateBlock(editingIndex, patch)
      dialogOpen.value = false
    })
    return
  }

  updateBlock(editingIndex, patch)
  dialogOpen.value = false
}

function removeEditingBlock() {
  const editingIndex = currentTemplate.value.findIndex(
    (block) => block.id === dialogEditingBlockID.value,
  )
  if (editingIndex < 0) return
  dialogOpen.value = false
  removeBlock(editingIndex)
}

async function runArgumentViewTransition(blockID: string, update: () => void | Promise<void>) {
  const reduceMotion =
    typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const transitionDocument =
    typeof document !== 'undefined' ? (document as ViewTransitionCapableDocument) : null

  if (reduceMotion || transitionDocument?.startViewTransition === undefined) {
    await update()
    return
  }

  activeViewTransitionName.value = 'runtime-arg-focus'
  activeViewTransitionBlockID.value = blockID

  const transition = transitionDocument.startViewTransition(async () => {
    await update()
    await nextTick()
  })

  try {
    await transition.finished
  } finally {
    activeViewTransitionName.value = null
    activeViewTransitionBlockID.value = null
  }
}

function buildDialogBlockPatch(): Partial<StartArgBlock> {
  const ownership = dialogDraft.ownership
  return {
    ownership,
    label: dialogDraft.label,
    managedSource: ownership === 'system' ? dialogDraft.managedSource : '',
    tokens: splitTokensInput(dialogDraft.tokensText),
  }
}

function updateBlock(index: number, patch: Partial<StartArgBlock>) {
  const targetBlock = currentTemplate.value[index]
  if (!targetBlock) return

  emitTemplate(applyPatchToTemplateByIndex(currentTemplate.value, index, patch))
  syncSharedBlockMetadata(targetBlock.id, patch)
}

function moveBlock(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (targetIndex < 0 || targetIndex >= currentTemplate.value.length) return
  reorderBlock(index, targetIndex)
}

function reorderBlock(fromIndex: number, toIndex: number) {
  if (
    fromIndex === toIndex ||
    fromIndex < 0 ||
    toIndex < 0 ||
    fromIndex >= currentTemplate.value.length ||
    toIndex >= currentTemplate.value.length
  ) {
    return
  }

  const nextTemplate = [...currentTemplate.value]
  const [moved] = nextTemplate.splice(fromIndex, 1)
  nextTemplate.splice(toIndex, 0, moved)
  selectBlock(moved.id)
  emitTemplate(normalizeTemplate(nextTemplate))
}

function removeBlock(index: number) {
  if (index < 0 || index >= currentTemplate.value.length) return

  const nextSelection =
    currentTemplate.value[index + 1]?.id ?? currentTemplate.value[index - 1]?.id ?? null
  selectBlock(nextSelection)
  emitTemplate(
    normalizeTemplate(currentTemplate.value.filter((_, currentIndex) => currentIndex !== index)),
  )
}

function resetCurrentPlatformOrder() {
  if (!canResetOrder.value) return

  const currentBlocksByID = new Map(
    currentTemplate.value.map((block) => [block.id, cloneStartArgBlock(block)]),
  )
  emitTemplate(
    normalizeTemplate(
      baselineTemplate.value
        .map((block) => currentBlocksByID.get(block.id))
        .filter((block): block is StartArgBlock => block !== undefined),
    ),
  )
}

function resetCurrentPlatform() {
  if (!canResetPlatform.value) return

  updateBaseCommand(baselineBaseCommand.value)
  emitTemplate(normalizeTemplate(cloneStartArgTemplate(baselineTemplate.value)))
}

function onPreviewDragStart(index: number, event: DragEvent) {
  draggedIndex.value = index
  dragInsertionIndex.value = index

  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', currentTemplate.value[index]?.id ?? '')
  }
}

function activatePreviewInsertion(targetIndex: number, event: DragEvent) {
  if (draggedIndex.value === null) return
  dragInsertionIndex.value = resolvePreviewInsertionIndex(targetIndex, event)
}

function activatePreviewEnd(event: DragEvent) {
  if (draggedIndex.value === null) return

  if (event.currentTarget === event.target) {
    dragInsertionIndex.value = currentTemplate.value.length
  }
}

function dropOnPreview(targetIndex: number, event: DragEvent) {
  completePreviewDrop(resolvePreviewInsertionIndex(targetIndex, event))
}

function dropAtPreviewEnd() {
  completePreviewDrop(currentTemplate.value.length)
}

function completePreviewDrop(insertionIndex: number) {
  if (draggedIndex.value === null) return

  const fromIndex = draggedIndex.value
  onDragEnd()

  const nextIndex = insertionIndex > fromIndex ? insertionIndex - 1 : insertionIndex
  const boundedIndex = Math.max(0, Math.min(nextIndex, currentTemplate.value.length - 1))
  reorderBlock(fromIndex, boundedIndex)
}

function resolvePreviewInsertionIndex(targetIndex: number, event: DragEvent) {
  const currentTarget = event.currentTarget
  if (!(currentTarget instanceof HTMLElement)) return targetIndex
  if (typeof event.clientX !== 'number' || typeof event.clientY !== 'number') return targetIndex

  const bounds = currentTarget.getBoundingClientRect()
  const useVerticalAxis = bounds.height > bounds.width
  const midpoint = useVerticalAxis ? bounds.top + bounds.height / 2 : bounds.left + bounds.width / 2
  const placeAfter = useVerticalAxis ? event.clientY > midpoint : event.clientX > midpoint
  return placeAfter ? targetIndex + 1 : targetIndex
}

function onDragEnd() {
  draggedIndex.value = null
  dragInsertionIndex.value = null
}

function emitTemplate(template: StartArgBlock[]) {
  emitTemplateForPlatform(selectedPlatform.value, template)
}

function emitTemplateForPlatform(platform: Platform, template: StartArgBlock[]) {
  if (platform === 'windows') {
    emit('update:windowsTemplate', template)
    return
  }

  emit('update:linuxTemplate', template)
}

const sharedBlockPatchKeys = ['ownership', 'label', 'managedSource'] as const

function syncSharedBlockMetadata(blockID: string, patch: Partial<StartArgBlock>) {
  const sharedPatch: Partial<StartArgBlock> = {}
  for (const key of sharedBlockPatchKeys) {
    if (Object.prototype.hasOwnProperty.call(patch, key)) sharedPatch[key] = patch[key]
  }
  if (Object.keys(sharedPatch).length === 0) return

  const nextOtherTemplate = applyPatchToTemplateByID(otherTemplate.value, blockID, sharedPatch)
  if (nextOtherTemplate) emitTemplateForPlatform(otherPlatform.value, nextOtherTemplate)
}

function applyPatchToTemplateByIndex(
  template: StartArgBlock[],
  targetIndex: number,
  patch: Partial<StartArgBlock>,
) {
  return template.map((block, currentIndex) =>
    currentIndex === targetIndex
      ? normalizeBlock({ ...block, ...patch }, currentIndex)
      : normalizeBlock(block, currentIndex),
  )
}

function applyPatchToTemplateByID(
  template: StartArgBlock[],
  blockID: string,
  patch: Partial<StartArgBlock>,
) {
  let found = false
  const nextTemplate = template.map((block, currentIndex) => {
    if (block.id !== blockID) return normalizeBlock(block, currentIndex)
    found = true
    return normalizeBlock({ ...block, ...patch }, currentIndex)
  })
  return found ? nextTemplate : null
}

function normalizeTemplate(template: StartArgBlock[]) {
  return template.map((block, index) => normalizeBlock(block, index))
}

function normalizeBlock(block: StartArgBlock, order: number): StartArgBlock {
  return {
    ...block,
    order,
    label: block.label ?? '',
    managedSource: block.managedSource ?? '',
    tokens: [...(block.tokens ?? [])],
  }
}

function createBlockId() {
  return `template-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

function cloneStartArgTemplate(template: StartArgBlock[]) {
  return template.map((block) => cloneStartArgBlock(block))
}

function cloneStartArgBlock(block: StartArgBlock): StartArgBlock {
  return { ...block, tokens: [...block.tokens] }
}

function templateSignature(template: StartArgBlock[]) {
  return JSON.stringify(
    template.map((block) => ({
      id: block.id,
      label: block.label ?? '',
      managedSource: block.managedSource ?? '',
      order: block.order,
      ownership: block.ownership,
      tokens: [...block.tokens],
    })),
  )
}

function templatesShareSameIDs(current: StartArgBlock[], baseline: StartArgBlock[]) {
  if (current.length !== baseline.length) return false

  const currentIDs = [...current.map((block) => block.id)].sort()
  const baselineIDs = [...baseline.map((block) => block.id)].sort()
  return currentIDs.every((id, index) => id === baselineIDs[index])
}

function templateIDSequence(template: StartArgBlock[]) {
  return template.map((block) => block.id).join('|')
}
</script>

<style scoped>
.template-editor {
  --template-editor-platform: var(--xy-platform-windows);
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

@keyframes template-editor-prompt-pulse {
  0%,
  100% {
    text-shadow: 0 0 12px color-mix(in srgb, var(--template-editor-platform) 14%, transparent);
    opacity: 0.92;
  }
  50% {
    text-shadow: 0 0 24px color-mix(in srgb, var(--template-editor-platform) 28%, transparent);
    opacity: 1;
  }
}

@keyframes template-editor-dialog-in {
  from {
    opacity: 0;
    transform: translateY(10px) scale(0.985);
  }
  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

@keyframes template-editor-shell-glow {
  0%,
  100% {
    box-shadow: var(--xy-shadow-2xl);
  }
  50% {
    box-shadow: 0 18px 38px rgba(0, 0, 0, 0.5);
  }
}

.template-editor--linux {
  --template-editor-platform: var(--xy-platform-linux);
}

.template-editor__sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border: 0;
}

.template-editor__empty,
.template-editor__collapsed {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-lg);
  border: 1px dashed var(--xy-border);
  border-radius: 14px;
  background: var(--xy-surface-0);
}

.platform-tabs {
  display: inline-flex;
  background: var(--xy-surface-0);
  border-radius: 8px;
  padding: 3px;
  gap: 2px;
}

.platform-tab {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  min-height: 44px;
  padding: 6px 16px;
  border-radius: 6px;
  border: none;
  background: transparent;
  cursor: pointer;
  font-size: 0.75rem;
  color: var(--xy-text-muted);
  transition:
    background var(--xy-transition-fast),
    color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
  font-family: inherit;
}

.platform-tab:hover {
  color: var(--xy-text-secondary);
}

.platform-tab--active {
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
}

.platform-icon-inactive {
  color: var(--xy-text-muted);
}

.platform-icon-windows-active {
  color: var(--xy-platform-windows);
}

.platform-icon-linux-active {
  color: var(--xy-platform-linux);
}

.template-editor__card {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: clamp(16px, 1.7vw, 22px);
  border: 1px solid var(--xy-border);
  border-radius: 18px;
  background: var(--xy-surface-gradient-subtle), var(--xy-surface-1);
  box-shadow: var(--xy-shadow-md);
}

.template-editor__card--advanced {
  gap: 0.7rem;
  padding: 14px 16px;
  border-color: color-mix(in srgb, var(--xy-border) 60%, transparent);
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--template-editor-platform) 3%, transparent),
      transparent 42%
    ),
    var(--xy-surface-0);
  box-shadow: none;
}

.template-editor__toolbar,
.template-editor__shell-head,
.template-editor__advanced-toggle,
.template-editor__dialog-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.template-editor__toolbar-copy,
.template-editor__advanced-copy {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.template-editor__dialog-head-main {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
  min-width: 0;
}

.template-editor__eyebrow {
  margin: 0;
  font-family: var(--xy-font-display);
  font-size: 0.74rem;
  letter-spacing: 0.18em;
  text-transform: uppercase;
  color: var(--xy-accent);
}

.template-editor__toolbar-meta,
.template-editor__shell-copy,
.template-editor__advanced-meta,
.template-editor__dialog-copy {
  color: var(--xy-text-secondary);
  font-size: 0.8rem;
  line-height: 1.45;
}

.template-editor__toolbar-meta,
.template-editor__advanced-meta {
  color: color-mix(in srgb, var(--template-editor-platform) 22%, var(--xy-text-secondary) 78%);
}

.template-editor__toolbar-actions,
.template-editor__dialog-actions-main,
.template-editor__sequence-actions,
.template-editor__command-tools {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.template-editor__button,
.template-editor__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 38px;
  padding: 0 14px;
  border-radius: 999px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  color: var(--xy-text-secondary);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    transform 120ms ease-out,
    box-shadow var(--xy-transition-fast);
}

.template-editor__button:hover,
.template-editor__icon:hover {
  border-color: var(--xy-border-hover);
  color: var(--xy-text-primary);
}

.template-editor__button:disabled,
.template-editor__icon:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.template-editor__button:active:not(:disabled),
.template-editor__icon:active:not(:disabled) {
  transform: translateY(1px);
}

.template-editor__toggle-indicator {
  min-height: 2.15rem;
  padding: 0.38rem 0.42rem;
  border-radius: 999px;
  border: none;
  background: transparent;
  color: var(--xy-accent);
  font-size: 0.82rem;
}

.template-editor__toggle-indicator:hover {
  color: var(--xy-accent-hover);
}

.template-editor__toggle-indicator :deep(.q-icon) {
  transition: transform 180ms ease-out;
}

.template-editor__toggle-indicator:hover :deep(.q-icon) {
  transform: scale(1.12);
}

.template-editor__button--quiet {
  min-height: 34px;
  padding: 0 12px;
  border-color: transparent;
  background: transparent;
  color: var(--xy-text-muted);
}

.template-editor__button--quiet:hover {
  border-color: var(--xy-border);
  background: color-mix(in srgb, var(--xy-surface-0) 70%, transparent);
}

.template-editor__command-shell {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 12px;
  border: 1px solid color-mix(in srgb, var(--template-editor-platform) 18%, var(--xy-border) 82%);
  border-radius: 16px;
  background:
    linear-gradient(
      180deg,
      color-mix(in srgb, var(--template-editor-platform) 5%, rgba(255, 255, 255, 0.02)),
      transparent 35%
    ),
    color-mix(in srgb, var(--template-editor-platform) 2%, var(--xy-surface-0) 98%);
  animation: template-editor-shell-glow 4.6s ease-in-out infinite;
}

.template-editor__terminal-label {
  font-family: var(--xy-font-display);
  font-size: 0.76rem;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--template-editor-platform) 72%, var(--xy-text-secondary) 28%);
}

.template-editor__command-line {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  overflow: hidden;
  min-height: 44px;
}

.template-editor__command-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding-top: 8px;
  border-top: 1px solid
    color-mix(in srgb, var(--template-editor-platform) 12%, var(--xy-border) 88%);
}

.template-editor__command-hint {
  color: var(--xy-text-muted);
  font-size: 0.74rem;
  line-height: 1.4;
}

.template-editor__prompt {
  flex: 0 0 auto;
  color: var(--template-editor-platform);
  font-family: var(--xy-font-display);
  font-size: 1rem;
  animation: template-editor-prompt-pulse 2.8s ease-in-out infinite;
}

.template-editor__base-command {
  flex: 0 0 auto;
  min-width: 144px;
}

.template-editor__base-command-input {
  width: min(192px, 38vw);
  min-height: 36px;
  padding: 0 10px;
  border: 1px solid color-mix(in srgb, var(--template-editor-platform) 14%, var(--xy-border) 86%);
  border-radius: 8px;
  background: color-mix(in srgb, var(--template-editor-platform) 4%, var(--xy-surface-1) 96%);
  color: var(--xy-text-primary);
  font-size: 0.79rem;
  outline: none;
  transition:
    border-color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
}

.template-editor__base-command-input::placeholder {
  color: var(--xy-text-muted);
}

.template-editor__base-command-input:focus {
  border-color: color-mix(in srgb, var(--template-editor-platform) 56%, transparent);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--template-editor-platform) 16%, transparent);
}

.template-editor__arg-chip,
.template-editor__empty-chip {
  --template-editor-chip-accent: var(--xy-success);
  --template-editor-chip-text: var(--xy-text-primary);
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  gap: 5px;
  max-width: min(220px, 34vw);
  min-height: 32px;
  padding: 0 7px;
  border: 1px solid color-mix(in srgb, var(--template-editor-chip-accent) 12%, var(--xy-border) 88%);
  border-radius: 4px;
  background: color-mix(
    in srgb,
    var(--template-editor-chip-accent) 0.8%,
    var(--xy-surface-0) 99.2%
  );
  color: var(--template-editor-chip-text);
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast),
    filter var(--xy-transition-fast);
}

.template-editor__arg-chip:hover,
.template-editor__empty-chip:hover {
  border-color: color-mix(in srgb, var(--template-editor-chip-accent) 18%, var(--xy-border) 82%);
  background: color-mix(
    in srgb,
    var(--template-editor-chip-accent) 1.5%,
    var(--xy-surface-0) 98.5%
  );
  filter: brightness(1.04);
}

.template-editor__arg-chip {
  cursor: grab;
}

.template-editor__arg-chip:active {
  cursor: grabbing;
}

.template-editor__arg-chip:focus-visible,
.template-editor__empty-chip:focus-visible {
  outline: none;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--template-editor-chip-accent) 18%, transparent);
}

.template-editor__arg-chip--system {
  --template-editor-chip-accent: var(--xy-warning);
  --template-editor-chip-text: color-mix(
    in srgb,
    var(--xy-warning) 56%,
    var(--xy-text-primary) 44%
  );
}

.template-editor__arg-chip--locked {
  --template-editor-chip-accent: var(--xy-danger);
  --template-editor-chip-text: color-mix(in srgb, var(--xy-danger) 52%, var(--xy-text-primary) 48%);
}

.template-editor__arg-chip--editable {
  --template-editor-chip-accent: var(--xy-success);
  --template-editor-chip-text: color-mix(
    in srgb,
    var(--xy-success) 48%,
    var(--xy-text-primary) 52%
  );
}

.template-editor__arg-chip--selected {
  border-color: color-mix(in srgb, var(--xy-text-primary) 12%, var(--xy-border) 88%);
}

.template-editor__arg-chip--dragging {
  opacity: 0.55;
}

.template-editor__arg-chip-handle {
  color: color-mix(in srgb, var(--template-editor-chip-accent) 66%, var(--xy-text-muted) 34%);
  opacity: 0.24;
  transition:
    opacity var(--xy-transition-fast),
    color var(--xy-transition-fast),
    transform 180ms ease-out;
}

.template-editor__arg-chip:hover .template-editor__arg-chip-handle,
.template-editor__arg-chip--selected .template-editor__arg-chip-handle {
  opacity: 0.44;
  transform: translateX(1px);
}

.template-editor__arg-chip-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.77rem;
}

.template-editor__empty-chip {
  --template-editor-chip-accent: var(--template-editor-platform);
}

.template-editor__drop-marker {
  flex: 0 0 auto;
  width: 12px;
  height: 28px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--template-editor-platform) 76%, transparent);
  box-shadow: 0 0 0 4px color-mix(in srgb, var(--template-editor-platform) 14%, transparent);
}

.template-editor__advanced-toggle {
  width: 100%;
  padding: 0;
  border: none;
  background: transparent;
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.template-editor__dialog-title {
  margin: 0;
  color: var(--xy-text-primary);
  font-size: 1rem;
}

.template-editor__dialog-hero {
  display: flex;
  align-items: center;
  min-width: 0;
}

.template-editor__dialog-hero-chip {
  --template-editor-chip-accent: var(--xy-success);
  --template-editor-chip-text: var(--xy-text-primary);
  display: inline-flex;
  align-items: center;
  max-width: min(100%, 28rem);
  min-height: 34px;
  padding: 0 0.75rem;
  border: 1px solid color-mix(in srgb, var(--template-editor-chip-accent) 16%, var(--xy-border) 84%);
  border-radius: 10px;
  background: color-mix(in srgb, var(--template-editor-chip-accent) 3%, var(--xy-surface-0) 97%);
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.05),
    0 18px 32px -28px color-mix(in srgb, var(--template-editor-chip-accent) 36%, transparent);
  color: var(--template-editor-chip-text);
}

.template-editor__dialog-hero-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 0.79rem;
}

.template-editor__toggle-indicator {
  display: inline-flex;
  align-items: center;
  gap: 0.42rem;
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.template-editor__sequence-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.template-editor__sequence-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: 6px;
  align-items: center;
}

.template-editor__sequence-row--selected .template-editor__sequence-item {
  border-color: color-mix(in srgb, var(--template-editor-platform) 42%, var(--xy-border) 58%);
  background: color-mix(in srgb, var(--template-editor-platform) 3%, var(--xy-surface-0) 97%);
}

.template-editor__sequence-item {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 8px;
  align-items: center;
  min-height: 36px;
  padding: 0 10px;
  border: 1px solid var(--xy-border);
  border-radius: 9px;
  background: color-mix(in srgb, var(--xy-surface-0) 76%, transparent);
  color: inherit;
  cursor: pointer;
  text-align: left;
}

.template-editor__order {
  color: color-mix(in srgb, var(--template-editor-platform) 44%, var(--xy-text-muted) 56%);
  font-size: 0.7rem;
}

.template-editor__sequence-preview {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--xy-text-primary);
  font-size: 0.76rem;
}

.template-editor__state {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  color: var(--xy-text-secondary);
  font-size: 0.64rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.template-editor__stepper {
  display: inline-grid;
  grid-template-rows: 1fr 1fr;
  border: 1px solid color-mix(in srgb, var(--template-editor-platform) 16%, var(--xy-border) 84%);
  border-radius: 9px;
  overflow: hidden;
  background: color-mix(in srgb, var(--template-editor-platform) 3%, var(--xy-surface-0) 97%);
}

.template-editor__icon--step {
  min-width: 30px;
  min-height: 18px;
  padding: 0;
  border: none;
  border-radius: 0;
  background: transparent;
}

.template-editor__icon--step + .template-editor__icon--step {
  border-top: 1px solid color-mix(in srgb, var(--xy-border) 82%, transparent);
}

.template-editor__icon--step:hover:not(:disabled) {
  background: color-mix(in srgb, var(--template-editor-platform) 8%, var(--xy-surface-0) 92%);
  color: color-mix(in srgb, var(--template-editor-platform) 44%, var(--xy-text-primary) 56%);
}

@media (prefers-reduced-motion: reduce) {
  .template-editor-panel-enter-active,
  .template-editor-panel-leave-active,
  .template-editor__command-shell,
  .template-editor__prompt {
    animation: none;
  }

  .template-editor__button,
  .template-editor__icon,
  .template-editor__toggle-indicator :deep(.q-icon),
  .template-editor__arg-chip,
  .template-editor__empty-chip,
  .template-editor__arg-chip-handle {
    transition: none;
  }
}

.template-editor__state--system {
  border-color: var(--xy-warning-border-soft);
  background: color-mix(in srgb, var(--xy-warning) 10%, var(--xy-surface-0) 90%);
  color: color-mix(in srgb, var(--xy-warning) 74%, var(--xy-text-primary) 26%);
}

.template-editor__state--locked {
  border-color: color-mix(in srgb, var(--xy-danger) 34%, var(--xy-border) 66%);
  background: color-mix(in srgb, var(--xy-danger) 10%, var(--xy-surface-0) 90%);
  color: color-mix(in srgb, var(--xy-danger) 72%, var(--xy-text-primary) 28%);
}

.template-editor__state--editable {
  border-color: var(--xy-success-border-soft);
  background: color-mix(in srgb, var(--xy-success) 10%, var(--xy-surface-0) 90%);
  color: var(--xy-success-text-soft);
}

.template-editor__dialog {
  width: min(560px, calc(100vw - 32px));
  border-radius: 18px;
  background: var(--xy-surface-1);
  color: var(--xy-text-primary);
  animation: template-editor-dialog-in 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

::view-transition-group(runtime-arg-focus) {
  animation-duration: 260ms;
  animation-timing-function: cubic-bezier(0.22, 1, 0.36, 1);
}

::view-transition-old(runtime-arg-focus),
::view-transition-new(runtime-arg-focus) {
  height: 100%;
  border-radius: 10px;
}

.template-editor-panel-enter-active,
.template-editor-panel-leave-active {
  transition:
    opacity 180ms cubic-bezier(0.25, 1, 0.5, 1),
    transform 220ms cubic-bezier(0.22, 1, 0.36, 1);
}

.template-editor-panel-enter-from,
.template-editor-panel-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

.template-editor__dialog-body {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.template-editor__dialog-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

@media (max-width: 900px) {
  .template-editor__toolbar,
  .template-editor__advanced-toggle,
  .template-editor__dialog-head,
  .template-editor__dialog-actions,
  .template-editor__command-footer {
    flex-direction: column;
    align-items: flex-start;
  }

  .template-editor__toolbar-actions,
  .template-editor__dialog-actions-main,
  .template-editor__command-tools {
    width: 100%;
  }

  .template-editor__command-line {
    align-items: flex-start;
    overflow: visible;
  }

  .template-editor__prompt {
    padding-top: 9px;
  }

  .template-editor__base-command {
    flex: 1 0 100%;
    min-width: 0;
  }

  .template-editor__button {
    min-height: 44px;
  }

  .template-editor__button--quiet {
    min-height: 40px;
  }

  .template-editor__icon {
    min-width: 44px;
    min-height: 44px;
    padding: 0;
  }

  .template-editor__base-command-input {
    width: 100%;
  }

  .template-editor__arg-chip,
  .template-editor__empty-chip {
    max-width: none;
    width: 100%;
    justify-content: flex-start;
    min-height: 44px;
    padding-block: 8px;
  }

  .template-editor__arg-chip-text {
    white-space: normal;
    overflow: visible;
    text-overflow: clip;
    word-break: break-word;
  }

  .template-editor__command-hint {
    display: none;
  }

  .template-editor__sequence-row {
    grid-template-columns: minmax(0, 1fr) auto;
  }

  .template-editor__state {
    display: none;
  }

  .template-editor__sequence-actions {
    align-self: stretch;
  }

  .template-editor__stepper {
    height: 100%;
  }

  .template-editor__icon--step {
    min-width: 40px;
    min-height: 44px;
  }
}
</style>
