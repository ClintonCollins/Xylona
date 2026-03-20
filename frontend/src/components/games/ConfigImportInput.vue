<template>
  <div class="config-import-input">
    <!-- Text area with drop zone -->
    <div
      class="import-drop-zone"
      :class="{ 'drop-active': isDragging }"
      @dragenter.prevent="isDragging = true"
      @dragover.prevent
      @dragleave.prevent="isDragging = false"
      @drop.prevent="handleDrop">
      <q-input
        v-model="content"
        type="textarea"
        outlined
        dense
        placeholder="Paste configuration file contents here..."
        input-class="font-mono import-textarea"
        :maxlength="MAX_CONTENT_LENGTH"
        @update:model-value="handleContentChange" />
      <div v-if="isDragging" class="drop-overlay">
        <q-icon name="upload_file" size="32px" />
        <span>Drop file here</span>
      </div>
    </div>

    <!-- Browse file button -->
    <div class="import-actions">
      <q-btn
        flat
        dense
        size="sm"
        icon="upload_file"
        label="Browse file"
        class="text-xy-secondary"
        @click="fileInput?.click()" />
      <input ref="fileInput" type="file" hidden @change="handleFileSelect" />
    </div>

    <!-- Detection status -->
    <div v-if="content.trim()" class="import-status">
      <template v-if="detecting">
        <q-spinner-dots size="16px" color="primary" />
        <span class="text-xy-secondary">Detecting format...</span>
      </template>

      <template v-else-if="sizeError">
        <q-icon name="error" size="16px" color="negative" />
        <span class="text-negative">{{ sizeError }}</span>
      </template>

      <template v-else-if="detectionResult?.format">
        <q-icon name="check_circle" size="16px" color="positive" />
        <span class="text-positive">
          Detected <strong>{{ detectionResult.format.toUpperCase() }}</strong> —
          {{ detectionResult.fields.length }} field{{
            detectionResult.fields.length !== 1 ? 's' : ''
          }}
          found
        </span>
      </template>

      <template v-else-if="detectionResult && !detectionResult.format">
        <q-icon name="info" size="16px" class="text-xy-muted" />
        <span class="text-xy-muted">Format not detected automatically</span>
      </template>

      <!-- Ambiguous alternatives -->
      <div v-if="detectionResult?.alternativeFormats?.length" class="import-alternatives">
        <span class="text-xy-muted text-caption">Also detected:</span>
        <q-btn
          v-for="alt in detectionResult.alternativeFormats"
          :key="alt.format"
          flat
          dense
          size="xs"
          :label="`${alt.format.toUpperCase()} (${alt.fieldCount} fields)`"
          class="alt-format-btn"
          @click="selectAlternative(alt.format)" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { detectAndParse, parseWithFormat } from 'src/utils/config-import'
import type { ImportDetectionResult } from 'src/utils/config-import'

const MAX_CONTENT_LENGTH = 1_048_576 // 1 MB
const MAX_FILE_SIZE = 1_048_576 // 1 MB

const emit = defineEmits<{
  detected: [result: ImportDetectionResult]
}>()

const content = ref('')
const isDragging = ref(false)
const detecting = ref(false)
const sizeError = ref<string | null>(null)
const detectionResult = ref<ImportDetectionResult | null>(null)
const fileInput = ref<HTMLInputElement | null>(null)

let debounceTimer: ReturnType<typeof setTimeout> | null = null

function handleContentChange() {
  sizeError.value = null
  if (debounceTimer) clearTimeout(debounceTimer)
  debounceTimer = setTimeout(() => runDetection(), 300)
}

function runDetection() {
  const text = content.value.trim()
  if (!text) {
    detectionResult.value = null
    return
  }

  detecting.value = true
  // Use setTimeout to let the UI update before potentially heavy parsing
  setTimeout(() => {
    const result = detectAndParse(text)
    detectionResult.value = result
    detecting.value = false
    emit('detected', result)
  }, 0)
}

function handleDrop(event: DragEvent) {
  isDragging.value = false
  const file = event.dataTransfer?.files[0]
  if (file) readFile(file)
}

function handleFileSelect(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) readFile(file)
  // Reset input so same file can be selected again
  input.value = ''
}

function readFile(file: File) {
  if (file.size > MAX_FILE_SIZE) {
    sizeError.value = 'File too large — maximum 1 MB.'
    return
  }

  sizeError.value = null
  const reader = new FileReader()
  reader.onload = () => {
    content.value = reader.result as string

    // Build result with filename
    const result = detectAndParse(content.value.trim())
    const resultWithFilename: ImportDetectionResult = {
      ...result,
      filename: file.name,
    }
    detectionResult.value = resultWithFilename
    detecting.value = false
    emit('detected', resultWithFilename)
  }
  reader.readAsText(file)
}

function selectAlternative(format: string) {
  const text = content.value.trim()
  if (!text) return

  const result = parseWithFormat(format, text)
  const newResult: ImportDetectionResult = {
    ...result,
    filename: detectionResult.value?.filename ?? null,
  }
  detectionResult.value = newResult
  emit('detected', newResult)
}

// Expose content for parent to read if needed
defineExpose({ content })
</script>

<style scoped>
.config-import-input {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.import-drop-zone {
  position: relative;
}

.import-drop-zone :deep(.import-textarea) {
  min-height: 160px;
  font-size: 0.8rem;
  line-height: 1.4;
}

.drop-active {
  outline: 2px dashed var(--xy-accent);
  outline-offset: -2px;
  border-radius: 8px;
}

.drop-overlay {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-xs);
  background-color: rgba(28, 183, 207, 0.08);
  border-radius: 8px;
  color: var(--xy-accent);
  font-size: 0.85rem;
  font-weight: 500;
  pointer-events: none;
}

.import-actions {
  display: flex;
  justify-content: flex-end;
}

.import-status {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
  font-size: 0.8rem;
  padding: var(--xy-space-xs) 0;
}

.import-alternatives {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  width: 100%;
  padding-top: var(--xy-space-xs);
}

.alt-format-btn {
  font-size: 0.7rem;
  text-transform: none;
}
</style>
