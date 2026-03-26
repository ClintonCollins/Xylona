<template>
  <div class="template-editor">
    <div v-if="availablePlatforms.length === 0" class="template-editor__empty text-xy-muted">
      Enable Linux or Windows support above to configure structured start arguments.
    </div>

    <template v-else>
      <div v-if="availablePlatforms.length > 1" class="template-editor__tabs">
        <button
          v-for="platform in availablePlatforms"
          :key="platform"
          type="button"
          class="template-editor__tab"
          :class="{ 'template-editor__tab--active': selectedPlatform === platform }"
          @click="selectedPlatform = platform">
          {{ platformLabel(platform) }}
        </button>
      </div>

      <div class="template-editor__base">
        <q-input
          :model-value="currentBaseCommand"
          outlined
          label="Base Command"
          hint="Executed directly without shell parsing."
          @update:model-value="updateBaseCommand(String($event ?? ''))" />
      </div>

      <div class="template-editor__preview">
        <span class="template-editor__preview-label">Preview</span>
        <code class="template-editor__preview-command">{{ previewCommand }}</code>
      </div>

      <div class="template-editor__rows">
        <article
          v-for="(block, index) in currentTemplate"
          :key="block.id"
          class="template-editor__row">
          <div class="template-editor__row-grid">
            <label class="template-editor__field">
              <span class="template-editor__field-label">Mutability</span>
              <select
                class="template-editor__select"
                :value="block.ownership"
                @change="
                  updateBlock(index, {
                    ownership: ($event.target as HTMLSelectElement).value as StartArgOwnership,
                  })
                ">
                <option value="system">System</option>
                <option value="locked">Locked</option>
                <option value="editable">Editable</option>
              </select>
            </label>

            <q-input
              :model-value="block.label ?? ''"
              outlined
              label="Label"
              @update:model-value="updateBlock(index, { label: String($event ?? '') })" />

            <q-input
              :model-value="joinTokensInput(block.tokens)"
              type="textarea"
              autogrow
              outlined
              label="Tokens"
              hint="One token per line."
              @update:model-value="
                updateBlock(index, {
                  tokens: splitTokensInput(String($event ?? '')),
                })
              " />

            <q-input
              v-if="block.ownership === 'system'"
              :model-value="block.managedSource ?? ''"
              outlined
              label="Managed Source"
              hint="Canonical backend key, for example server_executable."
              @update:model-value="updateBlock(index, { managedSource: String($event ?? '') })" />
          </div>

          <div class="template-editor__row-actions">
            <q-btn
              flat
              dense
              icon="arrow_upward"
              :disable="index === 0"
              @click="moveBlock(index, -1)" />
            <q-btn
              flat
              dense
              icon="arrow_downward"
              :disable="index === currentTemplate.length - 1"
              @click="moveBlock(index, 1)" />
            <q-btn flat dense icon="delete" color="negative" @click="removeBlock(index)" />
          </div>
        </article>
      </div>

      <div class="template-editor__footer">
        <q-btn flat color="accent" icon="add" label="Add argument to template" @click="addBlock" />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'

import type { StartArgBlock, StartArgOwnership } from '@/components/game_servers/start-args'
import {
  formatTokensInline,
  joinTokensInput,
  splitTokensInput,
} from '@/components/game_servers/start-args'

type Platform = 'linux' | 'windows'

const props = withDefaults(
  defineProps<{
    linuxBaseCommand: string
    linuxEnabled?: boolean
    linuxTemplate: StartArgBlock[]
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

const previewCommand = computed(() => {
  const tokens = currentTemplate.value.flatMap((block) => block.tokens)
  return [currentBaseCommand.value, formatTokensInline(tokens)]
    .filter((value) => value !== '')
    .join(' ')
})

function platformLabel(platform: Platform) {
  return platform === 'windows' ? 'Windows' : 'Linux'
}

function updateBaseCommand(value: string) {
  if (selectedPlatform.value === 'windows') {
    emit('update:windowsBaseCommand', value)
    return
  }

  emit('update:linuxBaseCommand', value)
}

function updateBlock(index: number, patch: Partial<StartArgBlock>) {
  const nextTemplate = currentTemplate.value.map((block, currentIndex) =>
    currentIndex === index
      ? normalizeBlock({ ...block, ...patch }, currentIndex)
      : normalizeBlock(block, currentIndex),
  )
  emitTemplate(nextTemplate)
}

function moveBlock(index: number, direction: -1 | 1) {
  const targetIndex = index + direction
  if (targetIndex < 0 || targetIndex >= currentTemplate.value.length) {
    return
  }

  const nextTemplate = [...currentTemplate.value]
  const [moved] = nextTemplate.splice(index, 1)
  nextTemplate.splice(targetIndex, 0, moved)
  emitTemplate(normalizeTemplate(nextTemplate))
}

function removeBlock(index: number) {
  const nextTemplate = currentTemplate.value.filter((_, currentIndex) => currentIndex !== index)
  emitTemplate(normalizeTemplate(nextTemplate))
}

function addBlock() {
  const nextTemplate = normalizeTemplate([
    ...currentTemplate.value,
    {
      id: createBlockId(),
      order: currentTemplate.value.length,
      ownership: 'editable',
      tokens: [],
      label: '',
      managedSource: '',
    },
  ])
  emitTemplate(nextTemplate)
}

function emitTemplate(template: StartArgBlock[]) {
  if (selectedPlatform.value === 'windows') {
    emit('update:windowsTemplate', template)
    return
  }

  emit('update:linuxTemplate', template)
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
    tokens: [...block.tokens],
  }
}

function createBlockId() {
  return `template-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}
</script>

<style scoped>
.template-editor {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-md);
}

.template-editor__empty {
  padding: var(--xy-space-md);
  border-radius: 10px;
  border: 1px dashed var(--xy-border);
  background: var(--xy-surface-0);
}

.template-editor__tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.template-editor__tab {
  padding: 8px 12px;
  border-radius: 999px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
  color: var(--xy-text-secondary);
  cursor: pointer;
}

.template-editor__tab--active {
  border-color: var(--xy-accent);
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
}

.template-editor__base {
  max-width: 420px;
}

.template-editor__preview {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: var(--xy-space-md);
  border-radius: 10px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-0);
}

.template-editor__preview-label {
  font-size: 0.72rem;
  letter-spacing: 0.06em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
}

.template-editor__preview-command {
  font-family: var(--xy-font-mono);
  color: var(--xy-text-primary);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.template-editor__rows {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.template-editor__row {
  display: flex;
  gap: var(--xy-space-md);
  justify-content: space-between;
  padding: var(--xy-space-md);
  border-radius: 10px;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.template-editor__row-grid {
  display: grid;
  flex: 1;
  gap: 12px;
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

.template-editor__row-actions {
  display: inline-flex;
  flex-direction: column;
  gap: 4px;
}

.template-editor__footer {
  display: flex;
}

@media (max-width: 720px) {
  .template-editor__row {
    flex-direction: column;
  }

  .template-editor__row-actions {
    flex-direction: row;
    justify-content: flex-start;
  }
}
</style>
