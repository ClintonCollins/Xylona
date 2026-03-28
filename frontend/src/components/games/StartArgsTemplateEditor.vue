<template>
  <div class="template-editor" :class="`template-editor--${selectedPlatform}`">
    <div v-if="availablePlatforms.length === 0" class="template-editor__empty text-xy-muted">
      Enable Linux or Windows support above to configure structured start arguments.
    </div>

    <template v-else>
      <div v-if="availablePlatforms.length > 1" class="platform-tabs">
        <button
          v-for="platform in availablePlatforms"
          :key="platform"
          type="button"
          class="platform-tab"
          :class="{ 'platform-tab--active': selectedPlatform === platform }"
          @click="selectedPlatform = platform">
          <q-icon
            :name="platform === 'windows' ? 'desktop_windows' : 'terminal'"
            size="14px"
            :class="
              selectedPlatform === platform
                ? platform === 'windows'
                  ? 'platform-icon-windows-active'
                  : 'platform-icon-linux-active'
                : 'platform-icon-inactive'
            " />
          <span class="font-display">{{ platformLabel(platform) }}</span>
        </button>
      </div>

      <section class="template-editor__command-shell">
        <div class="template-editor__command-head">
          <div>
            <p class="template-editor__eyebrow">Runtime Command</p>
            <h3 class="template-editor__title font-display">Structured launch sequence</h3>
          </div>
          <div class="template-editor__summary-pills">
            <span class="template-editor__summary-pill">
              {{ currentTemplate.length }} arg{{ currentTemplate.length === 1 ? '' : 's' }}
            </span>
            <span class="template-editor__summary-pill"> {{ editableBlockCount }} editable </span>
          </div>
        </div>

        <div class="template-editor__command-toolbar">
          <q-input
            :model-value="currentBaseCommand"
            class="template-editor__base-command"
            outlined
            stack-label
            label="Base Command"
            hint="Executed directly without shell parsing."
            input-class="template-editor__base-command-input"
            @update:model-value="updateBaseCommand(String($event ?? ''))">
            <template #prepend>
              <span class="template-editor__base-command-prompt" aria-hidden="true">
                {{ selectedPlatform === 'windows' ? '>' : '$' }}
              </span>
            </template>
          </q-input>

          <button
            type="button"
            class="template-editor__preview-toggle"
            :aria-expanded="String(isPreviewExpanded)"
            @click="togglePreview">
            <span class="template-editor__preview-toggle-copy">
              {{ isPreviewExpanded ? 'Hide launch preview' : 'Show launch preview' }}
            </span>
            <span class="template-editor__preview-toggle-state">
              {{ isPreviewExpanded ? 'Expanded' : 'Collapsed' }}
            </span>
          </button>
        </div>

        <div v-if="isPreviewExpanded" class="template-editor__terminal">
          <span class="template-editor__terminal-label">
            {{ platformLabel(selectedPlatform) }} launch preview
          </span>
          <div class="template-editor__terminal-shell">
            <span class="template-editor__terminal-prompt">
              {{ selectedPlatform === 'windows' ? '>' : '$' }}
            </span>
            <code class="template-editor__terminal-command">
              <template v-if="previewSegments.length > 0">
                <component
                  :is="segment.blockID ? 'button' : 'span'"
                  v-for="segment in previewSegments"
                  :key="segment.id"
                  :type="segment.blockID ? 'button' : undefined"
                  class="template-editor__preview-token"
                  :class="previewTokenClass(segment)"
                  :aria-label="segment.blockID ? previewSegmentAriaLabel(segment) : undefined"
                  :tabindex="segment.blockID ? 0 : -1"
                  :disabled="!segment.blockID"
                  @click="
                    segment.blockID
                      ? selectBlock(segment.blockID, { scrollIntoView: true })
                      : undefined
                  ">
                  {{ segment.text }}
                </component>
              </template>
              <span
                v-else
                class="template-editor__preview-token template-editor__preview-token--empty">
                No command has been configured yet.
              </span>
            </code>
          </div>
        </div>
      </section>

      <section class="template-editor__workspace">
        <div class="template-editor__inventory">
          <div class="template-editor__section-head">
            <div>
              <p class="template-editor__eyebrow">Argument Inventory</p>
              <h3 class="template-editor__section-title font-display">Order defines execution</h3>
            </div>
            <div class="template-editor__section-actions">
              <button
                v-if="isCompactViewport"
                type="button"
                class="template-editor__mobile-inventory-toggle"
                :aria-expanded="String(showInventoryList)"
                @click="toggleMobileInventory">
                {{ showInventoryList ? 'Hide sequence' : 'Show sequence' }}
              </button>
              <span v-else class="template-editor__drag-cue">Drag rows to reorder</span>

              <button
                type="button"
                class="template-editor__utility-button"
                data-testid="start-args-reset-order"
                :disabled="!canResetOrder"
                :aria-label="`Reset ${platformLabel(selectedPlatform)} argument order to the loaded sequence`"
                @click="resetCurrentPlatformOrder">
                Reset order
              </button>

              <button
                type="button"
                class="template-editor__utility-button"
                data-testid="start-args-reset-platform"
                :disabled="!canResetPlatform"
                :aria-label="`Reset ${platformLabel(selectedPlatform)} launch setup to the loaded baseline`"
                @click="resetCurrentPlatform">
                Reset launch setup
              </button>
            </div>
          </div>

          <div v-if="currentTemplate.length === 0" class="template-editor__inventory-empty">
            <p>No runtime arguments exist for this platform yet.</p>
            <q-btn flat color="accent" icon="add" label="Add first argument" @click="addBlock" />
          </div>

          <template v-else-if="showInventoryList">
            <div class="template-editor__inventory-header" aria-hidden="true">
              <span>Order</span>
              <span>Argument</span>
              <span>State</span>
              <span>Preview</span>
            </div>

            <div
              class="template-editor__inventory-list"
              role="list"
              aria-label="Runtime argument inventory">
              <template v-for="(block, index) in currentTemplate" :key="block.id">
                <div
                  class="template-editor__drop-slot"
                  :class="{ 'template-editor__drop-slot--active': dragSlotIndex === index }"
                  aria-hidden="true"></div>

                <article
                  :ref="(element) => setInventoryRowRef(block.id, element)"
                  class="template-editor__inventory-row"
                  :class="{
                    'template-editor__inventory-row--selected': selectedBlockID === block.id,
                    'template-editor__inventory-row--dragging': draggedIndex === index,
                  }"
                  @dragenter.prevent="onDragOver(index, $event)"
                  @dragover.prevent="onDragOver(index, $event)"
                  @drop.prevent="onDrop(index, $event)">
                  <span
                    class="template-editor__inventory-cell template-editor__inventory-cell--order">
                    <button
                      type="button"
                      class="template-editor__drag-handle"
                      aria-label="Drag to reorder argument"
                      draggable="true"
                      @click.stop
                      @dragstart="onDragStart(index, $event)"
                      @dragend="onDragEnd">
                      <q-icon name="drag_handle" size="1.1rem" aria-hidden="true" />
                    </button>
                    <span class="template-editor__order-index">{{ index + 1 }}</span>
                  </span>

                  <button
                    type="button"
                    class="template-editor__inventory-select"
                    :aria-label="inventorySelectAriaLabel(block, index)"
                    :aria-pressed="String(selectedBlockID === block.id)"
                    @click="selectBlock(block.id)">
                    <span
                      class="template-editor__inventory-cell template-editor__inventory-cell--argument">
                      <strong>{{ blockTitle(block) }}</strong>
                      <small>{{ inventoryNote(block) }}</small>
                    </span>

                    <span class="template-editor__inventory-cell">
                      <span
                        class="template-editor__ownership-pill"
                        :class="ownershipPillClass(block.ownership)">
                        {{ ownershipLabel(block.ownership) }}
                      </span>
                    </span>

                    <span
                      class="template-editor__inventory-cell template-editor__inventory-cell--preview">
                      {{ inventoryPreview(block) }}
                    </span>
                  </button>
                </article>
              </template>

              <div
                class="template-editor__drop-slot"
                :class="{
                  'template-editor__drop-slot--active': dragSlotIndex === currentTemplate.length,
                }"
                aria-hidden="true"></div>
            </div>
          </template>

          <div v-else class="template-editor__inventory-collapsed">
            <span class="template-editor__inventory-collapsed-label">Editing now</span>
            <strong>{{ selectedBlockTitle }}</strong>
            <span>
              Slot {{ selectedBlockIndex + 1 }} of {{ currentTemplate.length }}. Open the sequence
              to switch or reorder arguments.
            </span>
          </div>
        </div>

        <aside class="template-editor__drawer">
          <template v-if="selectedBlock">
            <div ref="drawerHeadElement" class="template-editor__drawer-head">
              <div>
                <p class="template-editor__eyebrow">Selected Argument</p>
                <div class="template-editor__selected-badge">Editing {{ selectedBlockTitle }}</div>
              </div>
              <span class="template-editor__drawer-order"> Slot {{ selectedBlockIndex + 1 }} </span>
            </div>

            <p class="template-editor__drawer-description">
              {{ blockSubtitle(selectedBlock) }}
            </p>

            <div class="template-editor__drawer-actions">
              <q-btn
                flat
                dense
                icon="arrow_upward"
                aria-label="Move argument up"
                :disable="selectedBlockIndex <= 0"
                @click="moveBlock(selectedBlockIndex, -1)" />
              <q-btn
                flat
                dense
                icon="arrow_downward"
                aria-label="Move argument down"
                :disable="selectedBlockIndex === currentTemplate.length - 1"
                @click="moveBlock(selectedBlockIndex, 1)" />
              <q-btn
                flat
                dense
                icon="delete"
                color="negative"
                aria-label="Remove argument"
                @click="removeBlock(selectedBlockIndex)" />
              <q-btn flat dense icon="add" aria-label="Add argument" @click="addBlock" />
            </div>

            <div class="template-editor__drawer-fields">
              <label class="template-editor__field">
                <span class="template-editor__field-label">Mutability</span>
                <select
                  class="template-editor__select"
                  :value="selectedBlock.ownership"
                  @change="
                    updateSelectedBlock({
                      ownership: ($event.target as HTMLSelectElement).value as StartArgOwnership,
                    })
                  ">
                  <option value="system">System</option>
                  <option value="locked">Locked</option>
                  <option value="editable">Editable</option>
                </select>
              </label>

              <q-input
                :model-value="selectedBlock.label ?? ''"
                outlined
                label="Label"
                @update:model-value="updateSelectedBlock({ label: String($event ?? '') })" />

              <q-input
                :model-value="joinTokensInput(selectedBlock.tokens)"
                type="textarea"
                autogrow
                outlined
                label="Arguments"
                hint="One argument per line."
                @update:model-value="
                  updateSelectedBlock({
                    tokens: splitTokensInput(String($event ?? '')),
                  })
                " />

              <q-select
                v-if="selectedBlock.ownership === 'system'"
                :model-value="selectedBlock.managedSource ?? ''"
                outlined
                emit-value
                map-options
                :options="startArgManagedSourceOptions"
                label="Managed Source"
                hint="Choose from the valid runtime placeholders."
                @update:model-value="
                  updateSelectedBlock({ managedSource: String($event ?? '') })
                " />
            </div>
          </template>

          <div v-else class="template-editor__drawer-empty">
            <p>Select an argument to edit its details.</p>
            <q-btn flat color="accent" icon="add" label="Add argument" @click="addBlock" />
          </div>
        </aside>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import type { StartArgBlock, StartArgOwnership } from '@/components/game_servers/start-args'
import {
  formatTokensInline,
  joinTokensInput,
  splitTokensInput,
} from '@/components/game_servers/start-args'
import {
  getManagedSourceLabel,
  startArgManagedSourceOptions,
} from '@/components/shared/placeholder-definitions'

type Platform = 'linux' | 'windows'

interface PreviewSegment {
  id: string
  blockID?: string
  ownership: StartArgOwnership
  text: string
}

const props = withDefaults(
  defineProps<{
    baselineLinuxBaseCommand?: string
    baselineLinuxTemplate?: StartArgBlock[]
    linuxBaseCommand: string
    linuxEnabled?: boolean
    linuxTemplate: StartArgBlock[]
    baselineWindowsBaseCommand?: string
    baselineWindowsTemplate?: StartArgBlock[]
    windowsBaseCommand: string
    windowsEnabled?: boolean
    windowsTemplate: StartArgBlock[]
  }>(),
  {
    linuxEnabled: true,
    windowsEnabled: true,
  },
)

const emit = defineEmits<{
  'update:linuxBaseCommand': [value: string]
  'update:linuxTemplate': [value: StartArgBlock[]]
  'update:windowsBaseCommand': [value: string]
  'update:windowsTemplate': [value: StartArgBlock[]]
}>()

const availablePlatforms = computed<Platform[]>(() => {
  const platforms: Platform[] = []
  if (props.windowsEnabled) {
    platforms.push('windows')
  }
  if (props.linuxEnabled) {
    platforms.push('linux')
  }
  return platforms
})

const selectedPlatform = ref<Platform>('windows')

watch(
  availablePlatforms,
  (platforms) => {
    if (!platforms.includes(selectedPlatform.value)) {
      selectedPlatform.value = platforms[0] ?? 'windows'
    }
  },
  { immediate: true },
)

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

const baselineBaseCommand = computed(() => {
  if (selectedPlatform.value === 'windows') {
    return props.baselineWindowsBaseCommand ?? props.windowsBaseCommand
  }

  return props.baselineLinuxBaseCommand ?? props.linuxBaseCommand
})

const baselineTemplate = computed(() => {
  if (selectedPlatform.value === 'windows') {
    return props.baselineWindowsTemplate ?? props.windowsTemplate
  }

  return props.baselineLinuxTemplate ?? props.linuxTemplate
})

const isCompactViewport = ref(readCompactViewport())
const selectedBlockIDs = ref<Record<Platform, string | null>>({
  linux: null,
  windows: null,
})
const previewExpandedByPlatform = ref<Record<Platform, boolean>>({
  linux: !isCompactViewport.value,
  windows: !isCompactViewport.value,
})
const mobileInventoryExpanded = ref(!isCompactViewport.value)
const draggedIndex = ref<number | null>(null)
const dragSlotIndex = ref<number | null>(null)
const drawerHeadElement = ref<HTMLElement | null>(null)
const inventoryRowElements = new Map<string, HTMLElement>()
let viewportMediaQuery: MediaQueryList | null = null
let viewportChangeHandler: ((event: MediaQueryListEvent) => void) | null = null

const selectedBlockID = computed({
  get() {
    return selectedBlockIDs.value[selectedPlatform.value]
  },
  set(value: string | null) {
    selectedBlockIDs.value = {
      ...selectedBlockIDs.value,
      [selectedPlatform.value]: value,
    }
  },
})

const selectedBlockIndex = computed(() =>
  currentTemplate.value.findIndex((block) => block.id === selectedBlockID.value),
)

const selectedBlock = computed(() => {
  if (selectedBlockIndex.value < 0) {
    return null
  }

  return currentTemplate.value[selectedBlockIndex.value] ?? null
})

const selectedBlockTitle = computed(() => {
  if (!selectedBlock.value) {
    return 'No argument selected'
  }

  return blockTitle(selectedBlock.value)
})

const editableBlockCount = computed(
  () => currentTemplate.value.filter((block) => block.ownership === 'editable').length,
)

const isPreviewExpanded = computed(() => previewExpandedByPlatform.value[selectedPlatform.value])

const showInventoryList = computed(
  () =>
    currentTemplate.value.length === 0 || !isCompactViewport.value || mobileInventoryExpanded.value,
)

const canResetPlatform = computed(
  () =>
    currentBaseCommand.value !== baselineBaseCommand.value ||
    templateSignature(currentTemplate.value) !== templateSignature(baselineTemplate.value),
)

const canResetOrder = computed(() => {
  if (!templatesShareSameIDs(currentTemplate.value, baselineTemplate.value)) {
    return false
  }

  return templateIDSequence(currentTemplate.value) !== templateIDSequence(baselineTemplate.value)
})

const previewSegments = computed<PreviewSegment[]>(() => {
  const segments: PreviewSegment[] = []
  const baseCommand = currentBaseCommand.value.trim()

  if (baseCommand !== '') {
    segments.push({
      id: 'base-command',
      ownership: 'system',
      text: baseCommand,
    })
  }

  currentTemplate.value.forEach((block) => {
    const previewText = formatTokensInline(block.tokens).trim()
    if (previewText === '') {
      return
    }

    segments.push({
      id: block.id,
      blockID: block.id,
      ownership: block.ownership,
      text: previewText,
    })
  })

  return segments
})

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

    if (!hasSelection) {
      selectedBlockID.value = template[0].id
    }
  },
  { immediate: true, deep: true },
)

watch(selectedPlatform, () => {
  onDragEnd()

  if (isCompactViewport.value) {
    mobileInventoryExpanded.value = false
  }
})

function platformLabel(platform: Platform) {
  return platform === 'windows' ? 'Windows' : 'Linux'
}

function readCompactViewport() {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false
  }

  return window.matchMedia('(max-width: 780px)').matches
}

function syncCompactViewport(isCompact: boolean) {
  isCompactViewport.value = isCompact

  if (isCompact) {
    mobileInventoryExpanded.value = currentTemplate.value.length === 0
    return
  }

  mobileInventoryExpanded.value = true
}

function ownershipLabel(ownership: StartArgOwnership) {
  if (ownership === 'system') {
    return 'System'
  }

  if (ownership === 'locked') {
    return 'Locked'
  }

  return 'Editable'
}

function previewTokenClass(segment: PreviewSegment) {
  return [
    `template-editor__preview-token--${segment.ownership}`,
    segment.blockID
      ? 'template-editor__preview-segment-button'
      : 'template-editor__preview-token--static',
    segment.blockID && selectedBlockID.value === segment.blockID
      ? 'template-editor__preview-segment-button--selected'
      : '',
  ]
}

function previewSegmentAriaLabel(segment: PreviewSegment) {
  if (!segment.blockID) {
    return ''
  }

  const blockIndex = currentTemplate.value.findIndex((block) => block.id === segment.blockID)
  if (blockIndex < 0) {
    return 'Edit launch argument'
  }

  const block = currentTemplate.value[blockIndex]
  return `Edit argument ${blockIndex + 1}: ${blockTitle(block)}`
}

function ownershipPillClass(ownership: StartArgOwnership) {
  return `template-editor__ownership-pill--${ownership}`
}

function blockTitle(block: StartArgBlock) {
  const trimmedLabel = block.label?.trim() ?? ''
  if (trimmedLabel !== '') {
    return trimmedLabel
  }

  const managedSource = block.managedSource?.trim() ?? ''
  if (managedSource !== '') {
    return getManagedSourceLabel(managedSource)
  }

  return block.tokens[0] ?? 'Untitled argument'
}

function blockSubtitle(block: StartArgBlock) {
  const tokenPreview = formatTokensInline(block.tokens)
  if (tokenPreview !== '') {
    return tokenPreview
  }

  const managedSource = block.managedSource?.trim() ?? ''
  if (managedSource !== '') {
    return `Managed source: ${getManagedSourceLabel(managedSource)}`
  }

  return 'No arguments configured yet.'
}

function inventoryNote(block: StartArgBlock) {
  const managedSource = block.managedSource?.trim() ?? ''

  if (block.ownership === 'system') {
    return managedSource !== ''
      ? `System managed • ${getManagedSourceLabel(managedSource)}`
      : 'System managed'
  }

  if (block.ownership === 'locked') {
    return 'Required launch flag'
  }

  if (block.tokens.length <= 1) {
    return 'Editable launch flag'
  }

  return `${block.tokens.length} launch arguments`
}

function inventoryPreview(block: StartArgBlock) {
  const preview = formatTokensInline(block.tokens)

  if (preview === '') {
    return 'No arguments set'
  }

  if (preview.length <= 42) {
    return preview
  }

  return `${preview.slice(0, 42)}…`
}

function inventorySelectAriaLabel(block: StartArgBlock, index: number) {
  return `Select argument ${index + 1}: ${blockTitle(block)} (${ownershipLabel(block.ownership)})`
}

function setInventoryRowRef(blockID: string, element: Element | null) {
  if (element instanceof HTMLElement) {
    inventoryRowElements.set(blockID, element)
    return
  }

  inventoryRowElements.delete(blockID)
}

async function scrollSelectedBlockIntoView(blockID: string) {
  await nextTick()

  inventoryRowElements.get(blockID)?.scrollIntoView?.({
    block: 'nearest',
    inline: 'nearest',
    behavior: 'smooth',
  })

  drawerHeadElement.value?.scrollIntoView?.({
    block: 'nearest',
    inline: 'nearest',
    behavior: 'smooth',
  })
}

function selectBlock(blockID: string, options?: { scrollIntoView?: boolean }) {
  selectedBlockID.value = blockID

  if (isCompactViewport.value) {
    mobileInventoryExpanded.value = false
  }

  if (options?.scrollIntoView) {
    void scrollSelectedBlockIntoView(blockID)
  }
}

function togglePreview() {
  previewExpandedByPlatform.value = {
    ...previewExpandedByPlatform.value,
    [selectedPlatform.value]: !previewExpandedByPlatform.value[selectedPlatform.value],
  }
}

function toggleMobileInventory() {
  mobileInventoryExpanded.value = !mobileInventoryExpanded.value
}

function updateBaseCommand(value: string) {
  if (selectedPlatform.value === 'windows') {
    emit('update:windowsBaseCommand', value)
    return
  }

  emit('update:linuxBaseCommand', value)
}

function updateSelectedBlock(patch: Partial<StartArgBlock>) {
  if (selectedBlockIndex.value < 0) {
    return
  }

  updateBlock(selectedBlockIndex.value, patch)
}

function updateBlock(index: number, patch: Partial<StartArgBlock>) {
  const targetBlock = currentTemplate.value[index]
  if (!targetBlock) {
    return
  }

  const nextTemplate = applyPatchToTemplateByIndex(currentTemplate.value, index, patch)
  emitTemplate(nextTemplate)
  syncSharedBlockMetadata(targetBlock.id, patch)
}

function moveBlock(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (targetIndex < 0 || targetIndex >= currentTemplate.value.length) {
    return
  }

  reorderBlock(index, targetIndex)
}

function reorderBlock(fromIndex: number, toIndex: number) {
  if (fromIndex === toIndex) {
    return
  }

  if (fromIndex < 0 || toIndex < 0) {
    return
  }

  if (fromIndex >= currentTemplate.value.length || toIndex >= currentTemplate.value.length) {
    return
  }

  const nextTemplate = [...currentTemplate.value]
  const [moved] = nextTemplate.splice(fromIndex, 1)
  nextTemplate.splice(toIndex, 0, moved)

  selectBlock(moved.id)
  emitTemplate(normalizeTemplate(nextTemplate))
}

function removeBlock(index: number) {
  if (index < 0 || index >= currentTemplate.value.length) {
    return
  }

  const nextSelection =
    currentTemplate.value[index + 1]?.id ?? currentTemplate.value[index - 1]?.id ?? null
  const nextTemplate = currentTemplate.value.filter((_, currentIndex) => currentIndex !== index)
  selectedBlockID.value = nextSelection
  emitTemplate(normalizeTemplate(nextTemplate))
}

function addBlock() {
  const blockID = createBlockId()
  const nextTemplate = normalizeTemplate([
    ...currentTemplate.value,
    {
      id: blockID,
      order: currentTemplate.value.length,
      ownership: 'editable',
      tokens: [],
      label: '',
      managedSource: '',
    },
  ])
  selectedBlockID.value = blockID
  emitTemplate(nextTemplate)
}

function resetCurrentPlatformOrder() {
  if (!canResetOrder.value) {
    return
  }

  const currentBlocksByID = new Map(
    currentTemplate.value.map((block) => [block.id, cloneStartArgBlock(block)]),
  )
  const reorderedBlocks = baselineTemplate.value
    .map((block) => currentBlocksByID.get(block.id))
    .filter((block): block is StartArgBlock => block !== undefined)

  emitTemplate(normalizeTemplate(reorderedBlocks))
}

function resetCurrentPlatform() {
  if (!canResetPlatform.value) {
    return
  }

  updateBaseCommand(baselineBaseCommand.value)
  emitTemplate(normalizeTemplate(cloneStartArgTemplate(baselineTemplate.value)))
}

function onDragStart(index: number, event: DragEvent) {
  draggedIndex.value = index
  dragSlotIndex.value = index

  if (event.dataTransfer) {
    event.dataTransfer.effectAllowed = 'move'
    event.dataTransfer.setData('text/plain', currentTemplate.value[index]?.id ?? '')
  }
}

function onDragOver(index: number, event: DragEvent) {
  if (draggedIndex.value === null) {
    return
  }

  dragSlotIndex.value = resolveDragSlot(index, event)
}

function onDrop(index: number, event: DragEvent) {
  if (draggedIndex.value === null) {
    return
  }

  const fromIndex = draggedIndex.value
  const slotIndex = resolveDragSlot(index, event)
  const targetIndex = slotIndex > fromIndex ? slotIndex - 1 : slotIndex
  onDragEnd()
  reorderBlock(fromIndex, targetIndex)
}

function onDragEnd() {
  draggedIndex.value = null
  dragSlotIndex.value = null
}

function resolveDragSlot(index: number, event: DragEvent) {
  const currentTarget = event.currentTarget
  if (!(currentTarget instanceof HTMLElement)) {
    return index
  }

  const bounds = currentTarget.getBoundingClientRect()
  const pointerY = event.clientY
  const midpoint = bounds.top + bounds.height / 2
  return pointerY < midpoint ? index : index + 1
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
  const sharedPatch = extractSharedBlockPatch(patch)
  if (Object.keys(sharedPatch).length === 0) {
    return
  }

  const nextOtherTemplate = applyPatchToTemplateByID(otherTemplate.value, blockID, sharedPatch)
  if (!nextOtherTemplate) {
    return
  }

  emitTemplateForPlatform(otherPlatform.value, nextOtherTemplate)
}

function extractSharedBlockPatch(patch: Partial<StartArgBlock>) {
  const sharedPatch: Partial<StartArgBlock> = {}

  for (const key of sharedBlockPatchKeys) {
    if (!Object.prototype.hasOwnProperty.call(patch, key)) {
      continue
    }

    sharedPatch[key] = patch[key]
  }

  return sharedPatch
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
    if (block.id !== blockID) {
      return normalizeBlock(block, currentIndex)
    }

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
  return {
    ...block,
    tokens: [...block.tokens],
  }
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
  if (current.length !== baseline.length) {
    return false
  }

  const currentIDs = [...current.map((block) => block.id)].sort()
  const baselineIDs = [...baseline.map((block) => block.id)].sort()
  return currentIDs.every((id, index) => id === baselineIDs[index])
}

function templateIDSequence(template: StartArgBlock[]) {
  return template.map((block) => block.id).join('|')
}

onMounted(() => {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return
  }

  viewportMediaQuery = window.matchMedia('(max-width: 780px)')
  viewportChangeHandler = (event: MediaQueryListEvent) => {
    syncCompactViewport(event.matches)
  }

  syncCompactViewport(viewportMediaQuery.matches)
  viewportMediaQuery.addEventListener('change', viewportChangeHandler)
})

onBeforeUnmount(() => {
  if (viewportMediaQuery && viewportChangeHandler) {
    viewportMediaQuery.removeEventListener('change', viewportChangeHandler)
  }

  viewportMediaQuery = null
  viewportChangeHandler = null
})
</script>

<style scoped>
.template-editor {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
  --template-editor-platform-accent: var(--xy-primary);
  --template-editor-platform-accent-emphasis: var(--xy-primary-hover);
}

.template-editor--windows {
  --template-editor-platform-accent: var(--xy-platform-windows);
  --template-editor-platform-accent-emphasis: var(--xy-platform-windows-hover);
}

.template-editor--linux {
  --template-editor-platform-accent: var(--xy-platform-linux);
  --template-editor-platform-accent-emphasis: var(--xy-platform-linux-hover);
}

.template-editor__empty {
  padding: var(--xy-space-md);
  border-radius: 10px;
  border: 1px dashed var(--xy-border);
  background: var(--xy-surface-0);
}

.platform-tabs {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-radius: 8px;
  background: var(--xy-surface-0);
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
  color: var(--xy-text-muted);
  font-size: 0.75rem;
  font-family: inherit;
  cursor: pointer;
  transition:
    background var(--xy-transition-fast),
    color var(--xy-transition-fast);
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

.template-editor__field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.template-editor__field-label {
  font-size: 0.78rem;
  color: var(--xy-text-secondary);
}

.template-editor__select {
  min-height: 42px;
  border-radius: 8px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  color: var(--xy-text-primary);
  padding: 0 12px;
}

.template-editor__command-shell,
.template-editor__inventory,
.template-editor__drawer {
  position: relative;
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: clamp(0.95rem, 0.8rem + 0.5vw, 1.2rem);
  border: 1px solid color-mix(in srgb, var(--xy-border) 82%, transparent);
  border-radius: 16px;
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--xy-surface-2) 68%, transparent),
    color-mix(in srgb, var(--xy-surface-1) 94%, transparent)
  );
  box-shadow: inset 0 1px 0 color-mix(in srgb, var(--xy-text-primary) 5%, transparent);
}

.template-editor__command-shell {
  gap: 0.9rem;
}

.template-editor__command-head,
.template-editor__section-head,
.template-editor__drawer-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.template-editor__eyebrow {
  margin: 0;
  font-size: 0.72rem;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--template-editor-platform-accent) 84%, var(--xy-text-secondary));
}

.template-editor__title,
.template-editor__section-title {
  margin: 0.2rem 0 0;
  color: var(--xy-text-primary);
  font-size: clamp(1rem, 0.88rem + 0.5vw, 1.2rem);
  letter-spacing: 0.03em;
}

.template-editor__summary-pills {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.45rem;
}

.template-editor__section-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.45rem;
}

.template-editor__summary-pill,
.template-editor__drag-cue,
.template-editor__drawer-order,
.template-editor__mobile-inventory-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2rem;
  padding: 0.25rem 0.7rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 78%, transparent);
  background: color-mix(in srgb, var(--xy-surface-0) 86%, transparent);
  color: var(--xy-text-secondary);
  font-size: 0.75rem;
  letter-spacing: 0.05em;
  text-transform: uppercase;
}

.template-editor__mobile-inventory-toggle,
.template-editor__preview-toggle {
  font-family: inherit;
  cursor: pointer;
}

.template-editor__mobile-inventory-toggle {
  min-height: 44px;
  padding: 0.5rem 0.9rem;
}

.template-editor__utility-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 2.4rem;
  padding: 0.4rem 0.8rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 78%, transparent);
  background: color-mix(in srgb, var(--xy-surface-0) 82%, transparent);
  color: color-mix(in srgb, var(--template-editor-platform-accent) 82%, var(--xy-text-primary));
  font-family: inherit;
  font-size: 0.77rem;
  letter-spacing: 0.05em;
  text-transform: uppercase;
  cursor: pointer;
  transition:
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    color var(--xy-transition-fast),
    transform var(--xy-transition-fast);
}

.template-editor__utility-button:hover:not(:disabled) {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--template-editor-platform-accent) 42%, var(--xy-border));
  background: color-mix(in srgb, var(--template-editor-platform-accent) 10%, var(--xy-surface-0));
  color: color-mix(in srgb, var(--template-editor-platform-accent) 94%, var(--xy-text-primary));
}

.template-editor__utility-button:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--template-editor-platform-accent) 58%, transparent);
  outline-offset: 2px;
}

.template-editor__utility-button:disabled {
  opacity: 0.48;
  cursor: not-allowed;
}

.template-editor__command-toolbar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.75rem;
  align-items: start;
}

.template-editor__base-command {
  min-width: 0;
}

.template-editor__base-command :deep(.q-field__control) {
  min-height: 62px;
  border-radius: 14px;
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--xy-surface-2) 42%, transparent),
    color-mix(in srgb, var(--xy-surface-0) 94%, transparent)
  );
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--xy-border) 90%, transparent),
    inset 0 1px 0 color-mix(in srgb, var(--xy-text-primary) 5%, transparent);
  cursor: text;
  overflow: hidden;
  transition:
    box-shadow var(--xy-transition-fast),
    background var(--xy-transition-fast);
}

.template-editor__base-command :deep(.q-field__inner),
.template-editor__base-command :deep(.q-field__control-container) {
  background: transparent;
  border-radius: inherit;
}

.template-editor__base-command :deep(.q-field__control::before) {
  border: 0 !important;
}

.template-editor__base-command :deep(.q-field__control::after) {
  border: 0 !important;
}

.template-editor__base-command:hover :deep(.q-field__control) {
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--xy-surface-3) 50%, transparent),
    color-mix(in srgb, var(--xy-surface-0) 94%, transparent)
  );
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--template-editor-platform-accent) 18%, var(--xy-border)),
    inset 0 1px 0 color-mix(in srgb, var(--xy-text-primary) 6%, transparent);
}

.template-editor__base-command.q-field--focused :deep(.q-field__control) {
  box-shadow:
    inset 0 0 0 1px color-mix(in srgb, var(--template-editor-platform-accent) 52%, var(--xy-border)),
    inset 0 1px 0 color-mix(in srgb, var(--xy-text-primary) 7%, transparent),
    0 0 0 4px color-mix(in srgb, var(--template-editor-platform-accent) 14%, transparent);
}

.template-editor__base-command :deep(.q-field__label) {
  color: color-mix(in srgb, var(--template-editor-platform-accent) 64%, var(--xy-text-secondary));
  font-size: 0.73rem;
  font-weight: 600;
  letter-spacing: 0.03em;
  text-transform: uppercase;
}

.template-editor__base-command :deep(.q-field__prepend) {
  position: relative;
  padding-right: 0.5rem;
  align-self: stretch;
  display: inline-flex;
  align-items: flex-end;
  padding-bottom: 0.5rem;
}

.template-editor__base-command :deep(.q-field__prepend)::after {
  content: '';
  display: block;
  width: 1px;
  height: 1.35rem;
  margin-left: 0.5rem;
  border-radius: 999px;
  background: color-mix(in srgb, var(--xy-border) 82%, transparent);
}

.template-editor__base-command :deep(.q-field__native),
.template-editor__base-command :deep(.q-field__input),
.template-editor__base-command-input {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: 0.98rem;
  letter-spacing: 0.02em;
  padding-top: 1.35rem;
  padding-bottom: 0.45rem;
  caret-color: color-mix(
    in srgb,
    var(--template-editor-platform-accent) 82%,
    var(--xy-text-primary)
  );
}

.template-editor__base-command :deep(.q-field__bottom) {
  padding-top: 0.4rem;
}

.template-editor__base-command :deep(.q-field__messages) {
  color: color-mix(
    in srgb,
    var(--xy-text-secondary) 92%,
    var(--template-editor-platform-accent) 12%
  );
  font-size: 0.74rem;
  letter-spacing: 0.01em;
}

.template-editor__base-command-prompt {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.6rem;
  height: 1.6rem;
  padding: 0 0.4rem;
  border-radius: 10px;
  border: 1px solid color-mix(in srgb, var(--template-editor-platform-accent) 12%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-surface-2) 74%, transparent);
  color: color-mix(in srgb, var(--template-editor-platform-accent) 78%, var(--xy-text-primary));
  font-family: var(--xy-font-mono);
  font-size: 0.86rem;
  font-weight: 600;
  line-height: 1;
}

.template-editor__preview-toggle {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  gap: 0.15rem;
  min-height: 56px;
  min-width: 10.5rem;
  padding: 0.65rem 0.95rem;
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 72%, transparent);
  background: color-mix(in srgb, var(--xy-surface-0) 82%, transparent);
  color: var(--xy-text-primary);
  text-align: left;
}

.template-editor__preview-toggle-copy {
  font-size: 0.84rem;
  font-weight: 600;
  line-height: 1.2;
}

.template-editor__preview-toggle-state {
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.template-editor__terminal {
  display: flex;
  flex-direction: column;
  gap: 0.55rem;
}

.template-editor__terminal-label {
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.template-editor__terminal-shell {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 0.8rem;
  align-items: flex-start;
  padding: 0.95rem 1rem;
  border-radius: 14px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 70%, transparent);
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--xy-surface-0) 98%, transparent),
    color-mix(in srgb, var(--xy-base) 92%, transparent)
  );
}

.template-editor__terminal-prompt {
  padding-top: 0.05rem;
  color: var(--template-editor-platform-accent);
  font-family: var(--xy-font-mono);
  font-size: 0.95rem;
}

.template-editor__terminal-command {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
  font-family: var(--xy-font-mono);
  font-size: 0.92rem;
  line-height: 1.55;
}

.template-editor__preview-token {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--xy-text-primary);
  overflow-wrap: anywhere;
}

.template-editor__preview-token--static {
  cursor: text;
}

.template-editor__preview-token--system {
  color: var(--xy-warning);
}

.template-editor__preview-token--editable {
  color: var(--xy-success);
}

.template-editor__preview-token--locked {
  color: var(--xy-danger);
}

.template-editor__preview-token--empty {
  color: var(--xy-text-muted);
}

.template-editor__preview-segment-button {
  border-radius: 999px;
  padding: 0.12rem 0.38rem;
  cursor: pointer;
  transition:
    background-color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.template-editor__preview-segment-button:hover {
  background: color-mix(in srgb, var(--template-editor-platform-accent) 16%, transparent);
}

.template-editor__preview-segment-button:focus-visible {
  outline: none;
  box-shadow: 0 0 0 2px color-mix(in srgb, var(--template-editor-platform-accent) 72%, transparent);
}

.template-editor__preview-segment-button--selected {
  background: color-mix(in srgb, var(--template-editor-platform-accent) 18%, transparent);
  box-shadow: inset 0 0 0 1px
    color-mix(in srgb, var(--template-editor-platform-accent) 34%, transparent);
}

.template-editor__drawer-description,
.template-editor__inventory-cell--argument small {
  color: var(--xy-text-secondary);
  line-height: 1.45;
}

.template-editor__workspace {
  display: grid;
  grid-template-columns: minmax(0, 1.45fr) minmax(19rem, 0.95fr);
  gap: var(--xy-space-md);
  align-items: start;
}

.template-editor__drawer {
  position: sticky;
  top: calc(var(--game-form-sticky-stack-offset, calc(50px + 4rem)) + var(--xy-space-sm));
}

.template-editor__inventory-header,
.template-editor__inventory-row {
  display: grid;
  grid-template-columns: 7.5rem minmax(13rem, 1.2fr) 8rem minmax(0, 1fr);
  gap: 0.75rem;
  align-items: center;
}

.template-editor__inventory-header {
  padding: 0 0.85rem 0.55rem;
  color: var(--xy-text-muted);
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.template-editor__inventory-list {
  display: flex;
  flex-direction: column;
  gap: 0;
}

.template-editor__inventory-row {
  position: relative;
  display: grid;
  grid-template-columns: 7.5rem minmax(0, 1fr);
  gap: 0.75rem;
  align-items: center;
  padding: 0.85rem;
  margin-block: 0.28rem;
  border: 1px solid color-mix(in srgb, var(--xy-border) 80%, transparent);
  border-inline-start-width: 4px;
  border-inline-start-color: transparent;
  border-radius: 14px;
  background: color-mix(in srgb, var(--xy-surface-0) 78%, transparent);
  transition:
    transform var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast);
}

.template-editor__inventory-row:hover {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--template-editor-platform-accent) 24%, var(--xy-border));
}

.template-editor__inventory-row:focus-within {
  outline: none;
}

.template-editor__inventory-row--selected {
  border-color: color-mix(in srgb, var(--xy-border) 80%, transparent);
  border-inline-start-color: var(--template-editor-platform-accent);
  background: color-mix(in srgb, var(--xy-surface-1) 82%, transparent);
}

.template-editor__inventory-row--dragging {
  opacity: 0.6;
  transform: scale(0.988);
  border-color: color-mix(in srgb, var(--template-editor-platform-accent) 28%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-surface-1) 68%, transparent);
}

.template-editor__drop-slot {
  position: relative;
  height: 0;
  opacity: 0;
  transition:
    height var(--xy-transition-fast),
    opacity var(--xy-transition-fast);
  pointer-events: none;
}

.template-editor__drop-slot::before {
  content: '';
  position: absolute;
  inset-block-start: 50%;
  inset-inline: 1rem;
  height: 3px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--template-editor-platform-accent) 86%, white);
  box-shadow:
    0 0 0 1px color-mix(in srgb, var(--template-editor-platform-accent) 28%, transparent),
    0 0 18px color-mix(in srgb, var(--template-editor-platform-accent) 20%, transparent);
  transform: translateY(-50%) scaleX(0.92);
}

.template-editor__drop-slot--active {
  height: 1rem;
  opacity: 1;
}

.template-editor__inventory-cell {
  min-width: 0;
}

.template-editor__inventory-select {
  display: grid;
  grid-template-columns: minmax(13rem, 1.2fr) 8rem minmax(0, 1fr);
  gap: 0.75rem;
  align-items: center;
  width: 100%;
  min-width: 0;
  padding: 0;
  border: none;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}

.template-editor__inventory-select:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--template-editor-platform-accent) 62%, transparent);
  outline-offset: 0.45rem;
  border-radius: 10px;
}

.template-editor__inventory-cell--order {
  display: inline-flex;
  align-items: center;
  gap: 0.65rem;
}

.template-editor__drag-handle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2.55rem;
  min-width: 2.55rem;
  height: 2.55rem;
  border: 1px solid color-mix(in srgb, var(--template-editor-platform-accent) 22%, var(--xy-border));
  border-radius: 12px;
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--xy-surface-3) 88%, transparent),
    color-mix(in srgb, var(--xy-surface-2) 96%, transparent)
  );
  color: color-mix(in srgb, var(--template-editor-platform-accent) 82%, var(--xy-text-primary));
  cursor: grab;
  box-shadow:
    inset 0 1px 0 color-mix(in srgb, var(--xy-text-primary) 6%, transparent),
    0 0 0 1px color-mix(in srgb, var(--xy-base) 28%, transparent);
  transition:
    transform var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    background var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.template-editor__drag-handle:hover {
  transform: translateY(-1px);
  border-color: color-mix(in srgb, var(--template-editor-platform-accent) 46%, var(--xy-border));
  background: linear-gradient(
    180deg,
    color-mix(in srgb, var(--template-editor-platform-accent) 16%, var(--xy-surface-3)),
    color-mix(in srgb, var(--template-editor-platform-accent) 12%, var(--xy-surface-2))
  );
  color: color-mix(in srgb, var(--template-editor-platform-accent) 96%, var(--xy-text-primary));
}

.template-editor__drag-handle:focus-visible {
  outline: 2px solid color-mix(in srgb, var(--template-editor-platform-accent) 62%, transparent);
  outline-offset: 2px;
}

.template-editor__drag-handle:active {
  cursor: grabbing;
  transform: translateY(0);
}

.template-editor__order-index {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.template-editor__inventory-cell--argument {
  display: flex;
  flex-direction: column;
  gap: 0.18rem;
}

.template-editor__inventory-cell--argument strong,
.template-editor__inventory-cell--preview {
  color: var(--xy-text-primary);
}

.template-editor__inventory-cell--preview {
  min-width: 0;
  font-family: var(--xy-font-mono);
  font-size: 0.85rem;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.template-editor__ownership-pill {
  display: inline-flex;
  align-items: center;
  min-height: 1.8rem;
  padding: 0.2rem 0.55rem;
  border-radius: 999px;
  border: 1px solid color-mix(in srgb, var(--xy-border) 84%, transparent);
  background: color-mix(in srgb, var(--xy-surface-2) 64%, transparent);
  font-size: 0.72rem;
  letter-spacing: 0.06em;
  text-transform: uppercase;
}

.template-editor__ownership-pill--system {
  color: var(--xy-warning);
}

.template-editor__ownership-pill--locked {
  color: var(--xy-danger);
}

.template-editor__ownership-pill--editable {
  color: var(--xy-success);
}

.template-editor__inventory-empty,
.template-editor__drawer-empty,
.template-editor__inventory-collapsed {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
  align-items: flex-start;
  padding: 1rem;
  border-radius: 14px;
  border: 1px dashed color-mix(in srgb, var(--xy-border) 78%, transparent);
  background: color-mix(in srgb, var(--xy-surface-0) 74%, transparent);
  color: var(--xy-text-secondary);
}

.template-editor__inventory-collapsed {
  gap: 0.3rem;
}

.template-editor__inventory-collapsed-label {
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
  color: color-mix(in srgb, var(--template-editor-platform-accent) 84%, var(--xy-text-secondary));
}

.template-editor__selected-badge {
  display: inline-flex;
  align-items: center;
  min-height: 2.1rem;
  padding: 0.35rem 0.75rem;
  border-radius: 999px;
  background: color-mix(in srgb, var(--template-editor-platform-accent) 16%, transparent);
  color: var(--xy-text-primary);
  font-size: 0.88rem;
  letter-spacing: 0.03em;
}

.template-editor__drawer-description {
  margin: 0;
}

.template-editor__drawer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 0.45rem;
}

.template-editor__drawer-actions :deep(.q-btn) {
  min-height: 2.5rem;
  min-width: 2.5rem;
  border-radius: 10px;
}

.template-editor__drawer-fields {
  display: grid;
  gap: 0.85rem;
}

.template-editor__drawer-fields :deep(.q-field) {
  background: color-mix(in srgb, var(--xy-surface-0) 70%, transparent);
  border-radius: 12px;
}

@media (max-width: 1080px) {
  .template-editor__workspace {
    grid-template-columns: minmax(0, 1fr);
  }

  .template-editor__drawer {
    position: relative;
    top: auto;
  }
}

@media (max-width: 780px) {
  .template-editor {
    gap: var(--xy-space-lg);
  }

  .platform-tabs {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .template-editor__command-head,
  .template-editor__section-head,
  .template-editor__drawer-head {
    flex-direction: column;
    align-items: stretch;
  }

  .template-editor__summary-pills,
  .template-editor__section-actions {
    justify-content: flex-start;
  }

  .template-editor__command-toolbar {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
  }

  .template-editor__inventory-header {
    display: none;
  }

  .template-editor__inventory-row {
    grid-template-columns: minmax(0, 1fr);
    gap: 0.55rem;
  }

  .template-editor__inventory-select {
    grid-template-columns: minmax(0, 1fr);
    gap: 0.55rem;
  }

  .template-editor__mobile-inventory-toggle,
  .template-editor__preview-toggle {
    width: 100%;
  }

  .template-editor__inventory-cell--order {
    justify-content: flex-start;
  }

  .template-editor__inventory-cell--preview {
    white-space: normal;
    overflow: visible;
    text-overflow: unset;
  }

  .template-editor__drawer-actions {
    justify-content: flex-start;
  }

  .template-editor__drawer-head {
    position: sticky;
    top: var(--xy-space-sm);
    z-index: 1;
    padding-bottom: 0.25rem;
    background: inherit;
  }
}
</style>
