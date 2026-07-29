<template>
  <div
    ref="rootElement"
    :class="{ 'console-input-disabled': disabled }"
    class="console-command-input"
    @focusout="onFocusOut">
    <div
      v-if="suggestionsOpen"
      :id="listboxID"
      aria-label="Known console commands"
      class="console-command-menu"
      role="listbox">
      <div class="console-command-menu__header">
        <div>
          <div class="console-command-menu__title">Known commands</div>
          <div class="console-command-menu__context">
            {{ gameName }} · {{ commands.length }} documented
          </div>
        </div>
        <span class="console-command-menu__key">Tab to complete</span>
      </div>

      <div v-if="visibleMatches.length > 0" class="console-command-menu__results">
        <button
          v-for="(match, index) in visibleMatches"
          :id="optionID(index)"
          :key="match.entry.command"
          :aria-disabled="disabled"
          :aria-selected="activeIndex === index"
          :class="{
            'console-command-option--active': activeIndex === index,
            'console-command-option--readonly': disabled,
          }"
          class="console-command-option"
          role="option"
          tabindex="-1"
          type="button"
          @mouseenter="activeIndex = index"
          @mousedown.prevent="selectMatch(index)">
          <span class="console-command-option__topline">
            <code class="console-command-option__syntax">
              {{ match.entry.syntax || match.entry.command }}
            </code>
            <span v-if="match.entry.category" class="console-command-option__category">
              {{ match.entry.category }}
            </span>
            <span
              v-if="riskLabel(match.entry.risk)"
              :class="`console-command-option__risk--${riskTone(match.entry.risk)}`"
              class="console-command-option__risk">
              {{ riskLabel(match.entry.risk) }}
            </span>
          </span>
          <span v-if="match.entry.summary" class="console-command-option__summary">
            {{ match.entry.summary }}
          </span>
          <span
            v-if="activeIndex === index && match.entry.availability"
            class="console-command-option__availability">
            {{ match.entry.availability }}
          </span>
          <span
            v-if="match.field !== 'command' && match.field !== 'syntax'"
            class="console-command-option__matched">
            Matched {{ matchFieldLabel(match.field) }}
          </span>
        </button>
      </div>

      <div v-else class="console-command-menu__empty" role="status">
        <q-icon aria-hidden="true" name="keyboard" size="20px" />
        <div>
          <div class="console-command-menu__empty-title">No known command matches</div>
          <div class="console-command-menu__empty-copy">
            Press Enter to send your input exactly as typed.
          </div>
        </div>
      </div>

      <div class="console-command-menu__footer">
        <span v-if="disabled">Read-only — commands can be sent when the server is online</span>
        <span v-else-if="matches.length > visibleMatches.length">
          Showing {{ visibleMatches.length }} of {{ matches.length }} matches
        </span>
        <span v-else>{{ matches.length }} {{ matches.length === 1 ? 'match' : 'matches' }}</span>
        <span v-if="!disabled" aria-hidden="true">↑↓ Browse · Esc close</span>
      </div>
    </div>

    <div class="console-command-input__control">
      <span aria-hidden="true" class="console-command-input__prompt">&gt;</span>
      <q-btn
        v-if="commands.length > 0"
        :aria-label="`Browse ${commands.length} known ${gameName} commands`"
        :class="{ 'console-command-input__browse--active': suggestionsOpen }"
        class="console-command-input__browse"
        dense
        flat
        icon="manage_search"
        square
        @mousedown="onBrowseMousedown"
        @click="toggleCommandBrowser">
        <q-tooltip>Browse {{ commands.length }} known commands</q-tooltip>
      </q-btn>
      <input
        id="consoleInput"
        ref="inputElement"
        :aria-activedescendant="activeDescendant"
        aria-autocomplete="list"
        aria-describedby="console-command-input-help"
        :aria-controls="commands.length > 0 ? listboxID : undefined"
        :aria-expanded="suggestionsOpen"
        aria-label="Game server console command"
        :disabled="disabled"
        :placeholder="placeholderText"
        :value="modelValue"
        autocomplete="off"
        autofocus
        class="console-command-input__field"
        name="consoleInput"
        role="combobox"
        spellcheck="false"
        type="text"
        @focus="onInputFocus"
        @input="onInput"
        @keydown="onInputKeydown" />
      <span id="console-command-input-help" class="console-command-input__assistive">
        <template v-if="disabled && disabledReason !== ''">{{ disabledReason }}.</template>
        <template v-else>
          Enter sends exactly what you type. Tab completes a known command when suggestions are
          open.
        </template>
      </span>
      <q-btn
        :disable="disabled"
        :loading="loading"
        aria-label="Send command"
        class="console-command-input__send"
        color="primary"
        dense
        flat
        icon="send"
        name="send"
        type="button"
        @click="submitCurrentInput">
        <q-tooltip v-if="permissionDenied">Requires console permission</q-tooltip>
      </q-btn>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { computed, nextTick, ref, watch } from 'vue'

import { GameConsoleCommandRisk, type GameConsoleCommand } from '@/proto/shared_pb'
import {
  completeConsoleCommandInput,
  matchConsoleCommands,
  type ConsoleCommandMatchField,
} from './console-command-matcher'

const props = withDefaults(
  defineProps<{
    commands: GameConsoleCommand[]
    disabled?: boolean
    disabledReason?: string
    gameName?: string
    loading?: boolean
    modelValue: string
    permissionDenied?: boolean
  }>(),
  {
    disabled: false,
    disabledReason: '',
    gameName: 'Game server',
    loading: false,
    permissionDenied: false,
  },
)

const emit = defineEmits<{
  history: [direction: 'up' | 'down']
  submit: []
  'update:modelValue': [value: string]
}>()

const maximumVisibleMatches = 10
const listboxID = 'console-command-suggestions'
const rootElement = ref<HTMLElement | null>(null)
const inputElement = ref<HTMLInputElement | null>(null)
const inputFocused = ref(false)
const browserRequested = ref(false)
const dismissed = ref(false)
const activeIndex = ref(0)

const matches = computed(() => matchConsoleCommands(props.commands, props.modelValue))
const visibleMatches = computed(() => matches.value.slice(0, maximumVisibleMatches))
const placeholderText = computed(() => {
  if (props.disabled && props.disabledReason !== '') {
    return props.disabledReason
  }
  return 'Enter command...'
})
// The command browser stays open as read-only documentation while the input is
// disabled; only typing-triggered suggestions require an enabled input.
const suggestionsOpen = computed(
  () =>
    props.commands.length > 0 &&
    !dismissed.value &&
    ((inputFocused.value && !props.disabled) || browserRequested.value),
)
const activeDescendant = computed(() => {
  if (!suggestionsOpen.value || visibleMatches.value.length === 0) {
    return undefined
  }
  return optionID(activeIndex.value)
})

watch(
  () => visibleMatches.value.length,
  (length) => {
    if (length === 0) {
      activeIndex.value = 0
      return
    }
    activeIndex.value = Math.min(activeIndex.value, length - 1)
  },
)

watch(
  () => props.disabled,
  (disabled) => {
    if (!disabled) {
      return
    }
    dismissed.value = true
    browserRequested.value = false
  },
)

function optionID(index: number): string {
  return `console-command-option-${index}`
}

function riskLabel(risk: GameConsoleCommandRisk): string {
  if (risk === GameConsoleCommandRisk.CAUTION) {
    return 'Caution'
  }
  if (risk === GameConsoleCommandRisk.DESTRUCTIVE) {
    return 'Destructive'
  }
  return ''
}

function riskTone(risk: GameConsoleCommandRisk): string {
  return risk === GameConsoleCommandRisk.DESTRUCTIVE ? 'danger' : 'warning'
}

function matchFieldLabel(field: ConsoleCommandMatchField): string {
  if (field === 'alias') {
    return 'an alias'
  }
  return field
}

function onInputFocus(): void {
  inputFocused.value = true
  dismissed.value = false
}

function onFocusOut(event: FocusEvent): void {
  const nextTarget = event.relatedTarget
  if (nextTarget instanceof Node && rootElement.value?.contains(nextTarget)) {
    return
  }

  inputFocused.value = false
  browserRequested.value = false
  dismissed.value = true
}

function onInput(event: Event): void {
  const target = event.target
  if (!(target instanceof HTMLInputElement)) {
    return
  }

  activeIndex.value = 0
  dismissed.value = false
  emit('update:modelValue', target.value)
}

function moveActiveMatch(step: number): void {
  const count = visibleMatches.value.length
  if (count === 0) {
    return
  }
  activeIndex.value = (activeIndex.value + step + count) % count
}

function selectMatch(index: number): void {
  if (props.disabled) {
    return
  }
  const match = visibleMatches.value[index]
  if (!match) {
    return
  }

  emit('update:modelValue', completeConsoleCommandInput(props.modelValue, match.entry))
  activeIndex.value = index
  dismissed.value = true
  browserRequested.value = false
  void nextTick(() => inputElement.value?.focus())
}

function submitCurrentInput(): void {
  dismissed.value = true
  browserRequested.value = false
  emit('submit')
}

function onBrowseMousedown(event: MouseEvent): void {
  // Keep focus on the input while it is usable; when the input is disabled the
  // button takes focus itself so focusout can close the read-only browser.
  if (!props.disabled) {
    event.preventDefault()
  }
}

function toggleCommandBrowser(): void {
  if (props.commands.length === 0) {
    return
  }

  const shouldOpen = !suggestionsOpen.value
  browserRequested.value = shouldOpen
  dismissed.value = !shouldOpen
  if (shouldOpen && !props.disabled) {
    inputElement.value?.focus()
  }
}

function onInputKeydown(event: KeyboardEvent): void {
  if (event.isComposing) {
    return
  }

  if (event.key === 'Enter') {
    event.preventDefault()
    submitCurrentInput()
    return
  }

  if (event.key === 'Escape' && suggestionsOpen.value) {
    event.preventDefault()
    dismissed.value = true
    browserRequested.value = false
    return
  }

  if (event.key === 'Tab' && suggestionsOpen.value && visibleMatches.value.length > 0) {
    event.preventDefault()
    selectMatch(activeIndex.value)
    return
  }

  if (event.key === 'ArrowUp' || event.key === 'ArrowDown') {
    if (suggestionsOpen.value && visibleMatches.value.length > 0) {
      event.preventDefault()
      moveActiveMatch(event.key === 'ArrowDown' ? 1 : -1)
      return
    }
    event.preventDefault()
    emit('history', event.key === 'ArrowUp' ? 'up' : 'down')
  }
}
</script>

<style scoped>
.console-command-input {
  position: relative;
  flex-shrink: 0;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border-top: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
  transition:
    background-color var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    box-shadow var(--xy-transition-fast);
}

.console-command-input:focus-within {
  border-top-color: var(--xy-accent-border);
  background: color-mix(in srgb, var(--xy-surface-1) 98%, var(--xy-accent) 2%);
  box-shadow: inset 0 0 0 2px var(--xy-focus-ring);
}

.console-input-disabled {
  background: var(--xy-surface-0);
}

.console-command-input__control {
  display: flex;
  min-width: 0;
  min-height: 2.5rem;
  align-items: center;
  gap: var(--xy-space-xs);
  padding: 0 var(--xy-space-xs) 0 var(--xy-space-sm);
}

.console-command-input__prompt {
  flex: 0 0 auto;
  color: var(--xy-accent);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
  opacity: 0.9;
  user-select: none;
}

.console-command-input:focus-within .console-command-input__prompt {
  opacity: 1;
}

.console-input-disabled .console-command-input__prompt {
  color: var(--xy-text-muted);
}

.console-command-input__browse,
.console-command-input__send {
  flex: 0 0 auto;
  min-width: 2rem;
  min-height: 2rem;
  padding: 0;
  border-radius: var(--xy-radius-sm);
}

.console-command-input__browse {
  color: var(--xy-text-secondary);
}

.console-command-input__browse--active {
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
}

.console-command-input__send {
  margin-inline-start: var(--xy-space-xs);
  background: var(--xy-primary-muted);
}

.console-command-input__browse :deep(.q-icon),
.console-command-input__send :deep(.q-icon) {
  font-size: var(--xy-font-size-lg);
}

.console-command-input__field {
  min-width: 0;
  flex: 1 1 auto;
  align-self: stretch;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--xy-text-primary);
  caret-color: var(--xy-accent);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  line-height: 1.4;
}

.console-command-input__field::placeholder {
  color: var(--xy-text-secondary);
  opacity: 1;
}

.console-command-input__field:disabled {
  color: var(--xy-text-muted);
  cursor: not-allowed;
}

.console-command-input__assistive {
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

.console-command-menu {
  position: absolute;
  right: 0;
  bottom: 100%;
  left: 0;
  z-index: calc(var(--xy-z-overlay) - 1);
  display: flex;
  max-height: min(26rem, 62dvh);
  flex-direction: column;
  overflow: hidden;
  border: 1px solid var(--xy-border-active);
  border-radius: var(--xy-radius-lg) var(--xy-radius-lg) 0 0;
  background: var(--xy-surface-1);
  box-shadow: var(--xy-shadow-xl);
}

.console-command-menu__header,
.console-command-menu__footer {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.console-command-menu__header {
  padding: var(--xy-space-sm) var(--xy-space-base);
  border-bottom: 1px solid var(--xy-border);
  background: var(--xy-surface-2);
}

.console-command-menu__title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
}

.console-command-menu__context,
.console-command-menu__key,
.console-command-menu__footer {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
}

.console-command-menu__key {
  flex: 0 0 auto;
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  background: var(--xy-surface-1);
  font-family: var(--xy-font-mono);
}

.console-command-menu__results {
  min-height: 0;
  overflow-y: auto;
  overscroll-behavior: contain;
}

.console-command-option {
  display: flex;
  width: 100%;
  flex-direction: column;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm) var(--xy-space-base);
  border: 0;
  border-bottom: 1px solid var(--xy-border);
  background: transparent;
  color: var(--xy-text-primary);
  cursor: pointer;
  text-align: left;
}

.console-command-option:hover,
.console-command-option--active {
  background: var(--xy-accent-muted);
}

.console-command-option--readonly {
  cursor: default;
}

.console-command-option--active {
  box-shadow: inset 2px 0 0 var(--xy-accent);
}

.console-command-option__topline {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: var(--xy-space-sm);
}

.console-command-option__syntax {
  min-width: 0;
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.console-command-option__category,
.console-command-option__risk {
  flex: 0 0 auto;
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  border-radius: var(--xy-radius-pill);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
}

.console-command-option__category {
  margin-inline-start: auto;
  background: var(--xy-surface-3);
  color: var(--xy-text-secondary);
}

.console-command-option__risk--warning {
  border: 1px solid var(--xy-warning-border);
  background: var(--xy-warning-bg);
  color: var(--xy-warning-hover);
}

.console-command-option__risk--danger {
  border: 1px solid var(--xy-danger-border);
  background: var(--xy-danger-bg);
  color: var(--xy-danger-hover);
}

.console-command-option__summary,
.console-command-option__availability {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  line-height: 1.4;
}

.console-command-option__availability {
  color: var(--xy-text-emphasis-soft);
}

.console-command-option__matched {
  color: var(--xy-accent-hover);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-2xs);
}

.console-command-menu__empty {
  display: flex;
  min-height: 6rem;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-base);
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
}

.console-command-menu__empty-title {
  color: var(--xy-text-primary);
  font-weight: 600;
}

.console-command-menu__empty-copy {
  margin-top: var(--xy-space-xs);
  font-size: var(--xy-font-size-xs);
}

.console-command-menu__footer {
  min-height: 2rem;
  padding: var(--xy-space-xs) var(--xy-space-base);
  border-top: 1px solid var(--xy-border);
  background: var(--xy-surface-2);
}

@media (max-width: 767px) {
  .console-command-input {
    padding: var(--xy-space-sm);
  }

  .console-command-input__control {
    min-height: 2.75rem;
    padding-inline: var(--xy-space-xs);
  }

  .console-command-input__browse,
  .console-command-input__send {
    min-width: 44px;
    min-height: 44px;
  }

  .console-command-menu {
    max-height: min(30rem, 68dvh);
  }

  .console-command-menu__key {
    display: none;
  }

  .console-command-option {
    min-height: 3.25rem;
  }

  .console-command-option__category {
    display: none;
  }

  .console-command-menu__footer {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
