<template>
  <div class="placeholder-input" :class="{ 'placeholder-input--focused': isFocused }">
    <label v-if="label" class="placeholder-input__label">{{ label }}</label>

    <div class="placeholder-input__container">
      <div
        ref="editorRef"
        class="placeholder-input__editor"
        contenteditable="true"
        :data-placeholder="placeholder"
        @input="onInput"
        @focus="onFocus"
        @blur="onBlur"
        @keydown="onKeydown"
        @paste="onPaste"></div>

      <!-- Variables hint button -->
      <button
        v-show="isEmpty || isFocused"
        ref="hintBtnRef"
        class="placeholder-input__hint-btn"
        type="button"
        tabindex="-1"
        @mousedown.prevent
        @click="showReferencePanel = !showReferencePanel">
        <q-icon name="mdi-code-braces" size="14px" />
        <span>Variables</span>
      </button>

      <!-- Reference panel popup -->
      <q-popup-proxy
        v-model="showReferencePanel"
        :target="hintBtnRef"
        anchor="bottom left"
        self="top left"
        :offset="[0, 4]"
        no-parent-event>
        <q-card class="placeholder-input__reference-panel">
          <q-card-section class="q-pa-sm">
            <div class="text-overline q-mb-xs" style="font-size: 0.65rem; letter-spacing: 0.1em">
              Available Variables
            </div>
            <q-list dense>
              <q-item
                v-for="ph in availablePlaceholders"
                :key="ph.key"
                clickable
                class="placeholder-input__reference-item"
                @click="insertPlaceholderFromPanel(ph.key)">
                <q-item-section side>
                  <q-icon name="mdi-plus-circle-outline" size="16px" color="accent" />
                </q-item-section>
                <q-item-section>
                  <q-item-label class="font-mono" style="font-size: 0.8rem">{{
                    formatPlaceholder(ph.key)
                  }}</q-item-label>
                  <q-item-label caption style="font-size: 0.7rem">
                    {{ ph.description }}
                  </q-item-label>
                </q-item-section>
              </q-item>
            </q-list>
          </q-card-section>
        </q-card>
      </q-popup-proxy>
    </div>

    <!-- Autocomplete dropdown -->
    <div
      v-if="showAutocomplete && filteredPlaceholders.length > 0"
      ref="autocompleteRef"
      class="placeholder-input__autocomplete"
      :style="autocompletePosition">
      <div
        v-for="(ph, idx) in filteredPlaceholders"
        :key="ph.key"
        class="placeholder-input__autocomplete-item"
        :class="{ 'placeholder-input__autocomplete-item--active': idx === activeAutocompleteIndex }"
        @mousedown.prevent="selectAutocomplete(ph.key)">
        <span class="font-mono" style="font-size: 0.8rem; color: var(--xy-accent)">
          {{ ph.key }}
        </span>
        <span class="placeholder-input__autocomplete-desc">{{ ph.description }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import { placeholders } from '@/components/shared/placeholder-definitions'
import type { PlaceholderDefinition } from '@/components/shared/placeholder-definitions'

const props = defineProps<{
  modelValue: string
  label?: string
  placeholder?: string
  commandOnly?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const editorRef = ref<HTMLDivElement | null>(null)
const hintBtnRef = ref<HTMLElement | null>(null)
const autocompleteRef = ref<HTMLDivElement | null>(null)

const isFocused = ref(false)
const isEmpty = ref(true)
const showReferencePanel = ref(false)
const showAutocomplete = ref(false)
const autocompleteFilter = ref('')
const activeAutocompleteIndex = ref(0)
const autocompletePosition = ref<Record<string, string>>({})

// Track whether we are programmatically updating innerHTML to avoid feedback loops
let isUpdatingFromModel = false

const availablePlaceholders = computed<PlaceholderDefinition[]>(() => {
  if (props.commandOnly) {
    return placeholders
  }
  return placeholders.filter((p) => !p.commandOnly)
})

const filteredPlaceholders = computed<PlaceholderDefinition[]>(() => {
  const filter = autocompleteFilter.value.toUpperCase()
  return availablePlaceholders.value.filter((p) => p.key.includes(filter))
})

// -- Chip HTML generation --

function formatPlaceholder(key: string): string {
  return `\u007B\u007B${key}\u007D\u007D`
}

function textToHtml(text: string): string {
  if (!text) return ''

  // Escape HTML entities in the raw text first
  const escaped = text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')

  // Replace placeholder patterns with chip spans
  return escaped.replace(/\{\{([A-Z_]+)\}\}/g, (_match, key: string) => {
    const ph = placeholders.find((p) => p.key === key)
    if (ph) {
      return `<span class="ph-chip" contenteditable="false" data-ph-key="${key}">{{${key}}}</span>`
    }
    return `{{${key}}}`
  })
}

function htmlToText(container: HTMLElement): string {
  let result = ''
  for (const node of container.childNodes) {
    if (node.nodeType === Node.TEXT_NODE) {
      result += node.textContent ?? ''
    } else if (node.nodeType === Node.ELEMENT_NODE) {
      const el = node as HTMLElement
      if (el.classList.contains('ph-chip')) {
        const key = el.getAttribute('data-ph-key')
        result += `{{${key}}}`
      } else if (el.tagName === 'BR') {
        result += '\n'
      } else {
        // Recurse into other elements (e.g., divs created by contenteditable newlines)
        const inner = htmlToText(el)
        // Contenteditable wraps new lines in divs
        if (el.tagName === 'DIV' && result.length > 0 && !result.endsWith('\n')) {
          result += '\n'
        }
        result += inner
      }
    }
  }
  return result
}

// -- Cursor utilities --

interface CursorState {
  node: Node | null
  offset: number
}

function saveCursor(): CursorState | null {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0) return null
  const range = sel.getRangeAt(0)
  if (!editorRef.value?.contains(range.startContainer)) return null
  return { node: range.startContainer, offset: range.startOffset }
}

function restoreCursor(state: CursorState | null) {
  if (!state || !state.node || !editorRef.value?.contains(state.node)) return
  try {
    const sel = window.getSelection()
    if (!sel) return
    const range = document.createRange()
    range.setStart(state.node, state.offset)
    range.collapse(true)
    sel.removeAllRanges()
    sel.addRange(range)
  } catch {
    // If the node no longer exists or offset is out of range, ignore
  }
}

function placeCursorAtEnd() {
  if (!editorRef.value) return
  const sel = window.getSelection()
  if (!sel) return
  const range = document.createRange()
  range.selectNodeContents(editorRef.value)
  range.collapse(false)
  sel.removeAllRanges()
  sel.addRange(range)
}

// -- Autocomplete positioning --

function updateAutocompletePosition() {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0 || !editorRef.value) return

  const range = sel.getRangeAt(0).cloneRange()
  range.collapse(true)
  const rect = range.getBoundingClientRect()
  const editorRect = editorRef.value.getBoundingClientRect()

  autocompletePosition.value = {
    top: `${rect.bottom - editorRect.top + 4}px`,
    left: `${rect.left - editorRect.left}px`,
  }
}

// -- Autocomplete detection --

function detectAutocomplete() {
  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0) {
    showAutocomplete.value = false
    return
  }

  const range = sel.getRangeAt(0)
  if (range.startContainer.nodeType !== Node.TEXT_NODE) {
    showAutocomplete.value = false
    return
  }

  const textNode = range.startContainer as Text
  const textBefore = textNode.textContent?.slice(0, range.startOffset) ?? ''

  // Find the last `{{` that hasn't been closed with `}}`
  const lastOpen = textBefore.lastIndexOf('{{')
  if (lastOpen === -1) {
    showAutocomplete.value = false
    return
  }

  const afterOpen = textBefore.slice(lastOpen + 2)
  // If there's a `}}` after the `{{`, it's already closed
  if (afterOpen.includes('}}')) {
    showAutocomplete.value = false
    return
  }

  // Filter text is everything after `{{`
  autocompleteFilter.value = afterOpen
  activeAutocompleteIndex.value = 0
  showAutocomplete.value = true
  updateAutocompletePosition()
}

// -- Event handlers --

function onInput() {
  if (isUpdatingFromModel) return

  const editor = editorRef.value
  if (!editor) return

  const text = htmlToText(editor)
  isEmpty.value = text.length === 0

  emit('update:modelValue', text)
  detectAutocomplete()
}

function onFocus() {
  isFocused.value = true
}

function onBlur() {
  isFocused.value = false
  // Delay hiding autocomplete so mousedown on items can fire first
  setTimeout(() => {
    showAutocomplete.value = false
  }, 150)
}

function onPaste(e: ClipboardEvent) {
  e.preventDefault()
  const text = e.clipboardData?.getData('text/plain') ?? ''
  document.execCommand('insertText', false, text)
}

function onKeydown(e: KeyboardEvent) {
  if (!showAutocomplete.value) return

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    activeAutocompleteIndex.value = Math.min(
      activeAutocompleteIndex.value + 1,
      filteredPlaceholders.value.length - 1,
    )
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    activeAutocompleteIndex.value = Math.max(activeAutocompleteIndex.value - 1, 0)
  } else if (e.key === 'Enter' || e.key === 'Tab') {
    if (filteredPlaceholders.value.length > 0) {
      e.preventDefault()
      selectAutocomplete(filteredPlaceholders.value[activeAutocompleteIndex.value].key)
    }
  } else if (e.key === 'Escape') {
    e.preventDefault()
    showAutocomplete.value = false
  }
}

function selectAutocomplete(key: string) {
  const editor = editorRef.value
  if (!editor) return

  const sel = window.getSelection()
  if (!sel || sel.rangeCount === 0) return

  const range = sel.getRangeAt(0)
  if (range.startContainer.nodeType !== Node.TEXT_NODE) return

  const textNode = range.startContainer as Text
  const textBefore = textNode.textContent?.slice(0, range.startOffset) ?? ''
  const lastOpen = textBefore.lastIndexOf('{{')
  if (lastOpen === -1) return

  const textAfter = textNode.textContent?.slice(range.startOffset) ?? ''

  // Create chip element
  const chip = document.createElement('span')
  chip.className = 'ph-chip'
  chip.contentEditable = 'false'
  chip.setAttribute('data-ph-key', key)
  chip.textContent = `{{${key}}}`

  // Split the text node: before the `{{`, the chip, then after cursor
  const beforeText = textBefore.slice(0, lastOpen)
  const afterTextNode = document.createTextNode(textAfter)
  const beforeTextNode = document.createTextNode(beforeText)

  const parent = textNode.parentNode!
  parent.insertBefore(beforeTextNode, textNode)
  parent.insertBefore(chip, textNode)
  // Add a zero-width space after the chip so cursor has somewhere to go
  const spacer = document.createTextNode('\u200B')
  parent.insertBefore(spacer, textNode)
  parent.insertBefore(afterTextNode, textNode)
  parent.removeChild(textNode)

  // Place cursor after the spacer
  const newRange = document.createRange()
  newRange.setStartAfter(spacer)
  newRange.collapse(true)
  sel.removeAllRanges()
  sel.addRange(newRange)

  showAutocomplete.value = false

  // Emit updated value
  const text = htmlToText(editor)
  isEmpty.value = text.length === 0
  emit('update:modelValue', text)
}

function insertPlaceholderFromPanel(key: string) {
  showReferencePanel.value = false

  const editor = editorRef.value
  if (!editor) return

  editor.focus()

  // Insert at end
  const chip = document.createElement('span')
  chip.className = 'ph-chip'
  chip.contentEditable = 'false'
  chip.setAttribute('data-ph-key', key)
  chip.textContent = `{{${key}}}`

  const spacer = document.createTextNode('\u200B')
  editor.appendChild(chip)
  editor.appendChild(spacer)

  placeCursorAtEnd()

  const text = htmlToText(editor)
  isEmpty.value = text.length === 0
  emit('update:modelValue', text)
}

// -- Sync model value to editor --

function syncModelToEditor() {
  const editor = editorRef.value
  if (!editor) return

  const currentText = htmlToText(editor)
  if (currentText === props.modelValue) return

  isUpdatingFromModel = true
  const cursor = isFocused.value ? saveCursor() : null
  editor.innerHTML = textToHtml(props.modelValue)
  isEmpty.value = !props.modelValue

  if (isFocused.value && cursor) {
    void nextTick(() => {
      restoreCursor(cursor)
      isUpdatingFromModel = false
    })
  } else {
    isUpdatingFromModel = false
  }
}

watch(
  () => props.modelValue,
  () => {
    syncModelToEditor()
  },
)

onMounted(() => {
  syncModelToEditor()

  // Handle chip deletion via keyboard by observing mutations
  if (editorRef.value) {
    const observer = new MutationObserver(() => {
      if (isUpdatingFromModel) return
      const text = htmlToText(editorRef.value!)
      isEmpty.value = text.length === 0
      emit('update:modelValue', text)
    })
    observer.observe(editorRef.value, {
      childList: true,
      subtree: true,
      characterData: true,
    })

    onUnmounted(() => {
      observer.disconnect()
    })
  }
})

// Close autocomplete on outside click
function onDocumentClick(e: MouseEvent) {
  if (
    autocompleteRef.value &&
    !autocompleteRef.value.contains(e.target as Node) &&
    editorRef.value &&
    !editorRef.value.contains(e.target as Node)
  ) {
    showAutocomplete.value = false
  }
}

onMounted(() => {
  document.addEventListener('click', onDocumentClick)
})

onUnmounted(() => {
  document.removeEventListener('click', onDocumentClick)
})
</script>

<style scoped lang="scss">
.placeholder-input {
  position: relative;
}

.placeholder-input__label {
  display: block;
  font-size: 0.75rem;
  color: var(--xy-text-secondary);
  margin-bottom: 4px;
  font-family: var(--xy-font-body);
  letter-spacing: 0.02em;
}

.placeholder-input--focused .placeholder-input__label {
  color: var(--xy-primary);
}

.placeholder-input__container {
  position: relative;
}

.placeholder-input__editor {
  width: 100%;
  min-height: 40px;
  padding: 8px 12px;
  background-color: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: 0.85rem;
  line-height: 1.6;
  outline: none;
  transition:
    border-color var(--xy-transition-base),
    box-shadow var(--xy-transition-base);
  overflow-y: auto;
  white-space: pre-wrap;
  word-break: break-word;

  &:hover {
    border-color: var(--xy-text-muted);
  }

  &:empty::before {
    content: attr(data-placeholder);
    color: var(--xy-text-muted);
    pointer-events: none;
  }
}

.placeholder-input--focused .placeholder-input__editor {
  border-color: var(--xy-primary);
  box-shadow: 0 0 0 1px var(--xy-border-active);
}

/* Chip styles — global because they live inside contenteditable */
.placeholder-input__editor :deep(.ph-chip) {
  display: inline;
  background: color-mix(in srgb, var(--xy-accent) 20%, transparent);
  color: var(--xy-accent);
  border-radius: 4px;
  padding: 1px 6px;
  font-family: var(--xy-font-mono);
  font-size: 0.85em;
  white-space: nowrap;
  user-select: all;
  cursor: default;
  vertical-align: baseline;
}

/* Hint button */
.placeholder-input__hint-btn {
  position: absolute;
  top: 6px;
  right: 6px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 2px 8px;
  font-size: 0.7rem;
  font-family: var(--xy-font-body);
  color: var(--xy-text-muted);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  cursor: pointer;
  transition:
    color var(--xy-transition-fast),
    border-color var(--xy-transition-fast);

  &:hover {
    color: var(--xy-accent);
    border-color: var(--xy-accent);
  }
}

/* Reference panel (popup) */
.placeholder-input__reference-panel {
  background: var(--xy-surface-2) !important;
  border: 1px solid var(--xy-border);
  min-width: 260px;
  max-width: 340px;
}

.placeholder-input__reference-item {
  border-radius: 4px;
  min-height: 36px;
  padding: 4px 8px;

  &:hover {
    background: var(--xy-surface-3);
  }
}

/* Autocomplete dropdown */
.placeholder-input__autocomplete {
  position: absolute;
  z-index: 100;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  box-shadow: var(--xy-shadow-md);
  min-width: 220px;
  max-width: 340px;
  max-height: 200px;
  overflow-y: auto;
  padding: 4px 0;
}

.placeholder-input__autocomplete-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 12px;
  cursor: pointer;
  transition: background var(--xy-transition-fast);

  &:hover,
  &--active {
    background: var(--xy-surface-3);
  }
}

.placeholder-input__autocomplete-desc {
  font-size: 0.72rem;
  color: var(--xy-text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
