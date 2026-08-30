<script lang="ts" setup>
import { ref, watch } from 'vue'

import type { GameOperationFieldOption } from '@/proto/xylona_pb'

const props = defineProps<{
  label: string
  modelValue: string
  options: GameOperationFieldOption[]
  placeholder: string
  testId?: string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string]
}>()

const filteredOptions = ref<GameOperationFieldOption[]>(props.options)

watch(
  () => props.options,
  (options) => {
    filteredOptions.value = options
  },
)

function filterOptions(value: string, update: (callback: () => void) => void) {
  const query = value.trim().toLocaleLowerCase()
  update(() => {
    filteredOptions.value =
      query === ''
        ? props.options
        : props.options.filter((option) =>
            [option.label, option.value, option.description, option.category].some((candidate) =>
              candidate.toLocaleLowerCase().includes(query),
            ),
          )
  })
}

function updateValue(value: unknown) {
  if (typeof value === 'string') emit('update:modelValue', value)
}

function createValue(value: string, done: (value?: string, mode?: 'toggle') => void) {
  const exactValue = value.trim()
  if (exactValue === '') return
  emit('update:modelValue', exactValue)
  done(exactValue, 'toggle')
}

function hideBrokenIcon(event: Event) {
  const image = event.currentTarget
  if (image instanceof HTMLImageElement) image.hidden = true
}

function accentStyle(value: string) {
  if (!/^#[\da-f]{6}$/i.test(value)) return undefined
  return { '--catalog-accent': value }
}
</script>

<template>
  <q-select
    :data-testid="testId"
    dense
    emit-value
    fill-input
    hide-selected
    input-debounce="0"
    :label="label"
    map-options
    :model-value="modelValue"
    new-value-mode="toggle"
    option-label="label"
    :options="filteredOptions"
    option-value="value"
    outlined
    :placeholder="placeholder"
    spellcheck="false"
    use-input
    virtual-scroll-slice-size="40"
    @filter="filterOptions"
    @input-value="updateValue"
    @new-value="createValue"
    @update:model-value="updateValue">
    <template #option="scope">
      <q-item v-bind="scope.itemProps" class="catalog-option">
        <q-item-section v-if="scope.opt.iconUrl || accentStyle(scope.opt.accentColor)" avatar>
          <img
            v-if="scope.opt.iconUrl"
            alt=""
            class="catalog-option__icon"
            height="36"
            loading="lazy"
            :src="scope.opt.iconUrl"
            width="36"
            @error="hideBrokenIcon" />
          <span
            v-else
            aria-hidden="true"
            class="catalog-option__swatch"
            :style="accentStyle(scope.opt.accentColor)"></span>
        </q-item-section>
        <q-item-section>
          <q-item-label class="catalog-option__label">{{ scope.opt.label }}</q-item-label>
          <q-item-label
            v-if="
              scope.opt.value !== scope.opt.label ||
              (scope.opt.description && scope.opt.description !== scope.opt.value) ||
              scope.opt.category
            "
            caption
            class="catalog-option__metadata">
            <code v-if="scope.opt.value !== scope.opt.label">{{ scope.opt.value }}</code>
            <span v-if="scope.opt.description && scope.opt.description !== scope.opt.value">
              {{ scope.opt.description }}
            </span>
            <span v-if="scope.opt.category" class="catalog-option__category">
              {{ scope.opt.category }}
            </span>
          </q-item-label>
        </q-item-section>
      </q-item>
    </template>
    <template #no-option>
      <q-item>
        <q-item-section class="catalog-empty">
          No server value matches. Press Enter to use the exact text.
        </q-item-section>
      </q-item>
    </template>
  </q-select>
</template>

<style scoped>
.catalog-option {
  min-height: 3.5rem;
}

.catalog-option__icon {
  width: 2.75rem;
  height: 2.75rem;
  object-fit: contain;
  background: var(--xy-surface-0);
  border-radius: var(--xy-radius-sm);
}

.catalog-option__swatch {
  display: grid;
  width: 2.25rem;
  height: 2.25rem;
  place-items: center;
  background: color-mix(in srgb, var(--catalog-accent) 20%, var(--xy-surface-0));
  border: 1px solid color-mix(in srgb, var(--catalog-accent) 72%, var(--xy-border));
  border-radius: var(--xy-radius-md);
}

.catalog-option__swatch::after {
  width: 0.75rem;
  height: 0.75rem;
  content: '';
  background: var(--catalog-accent);
  border-radius: var(--xy-radius-pill);
}

.catalog-option__label {
  font-weight: 700;
}

.catalog-option__metadata {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-xs) var(--xy-space-sm);
  align-items: center;
  margin-top: var(--xy-space-2xs);
}

.catalog-option__metadata code {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.catalog-option__category {
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-2xs);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-pill);
}

.catalog-empty {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
}
</style>
