<template>
  <section class="form-section form-section--last console-commands-section">
    <div class="section-header console-commands-heading">
      <span class="section-bar console-commands-section-bar"></span>
      <div>
        <h2 class="section-title font-display">Console Commands</h2>
        <p class="console-commands-intro text-xy-muted">
          Build the command reference shown to server administrators while they work in the console.
        </p>
      </div>
      <span class="section-line"></span>
      <q-btn
        color="primary"
        data-testid="add-console-command"
        icon="add"
        label="Add Command"
        no-caps
        @click="addCommand" />
    </div>

    <div
      v-if="validationErrors.length > 0"
      class="console-commands-validation"
      data-testid="console-command-validation"
      role="alert">
      <q-icon aria-hidden="true" name="error_outline" size="20px" />
      <div>
        <strong>Command catalog needs attention.</strong>
        <span>{{ validationErrors[0]?.message }}</span>
      </div>
    </div>

    <div class="console-commands-layout">
      <aside class="console-command-index" aria-label="Console command catalog">
        <div class="console-command-index__header">
          <div>
            <div class="console-command-index__title font-display">Catalog</div>
            <div class="console-command-index__count text-xy-muted">
              {{ game.consoleCommands.length }}
              {{ game.consoleCommands.length === 1 ? 'command' : 'commands' }}
            </div>
          </div>
          <q-input
            v-model="commandFilter"
            aria-label="Filter console commands"
            class="console-command-filter"
            clearable
            dense
            outlined
            placeholder="Filter commands">
            <template #prepend>
              <q-icon aria-hidden="true" name="search" />
            </template>
          </q-input>
        </div>

        <div v-if="game.consoleCommands.length === 0" class="console-command-index__empty">
          <q-icon aria-hidden="true" name="terminal" size="24px" />
          <span>No commands yet</span>
          <small>Add the first documented server command.</small>
        </div>

        <div v-else-if="filteredCommandEntries.length === 0" class="console-command-index__empty">
          <q-icon aria-hidden="true" name="search_off" size="24px" />
          <span>No matches</span>
          <small>Try a command, category, or summary.</small>
        </div>

        <div v-else class="console-command-list" role="list">
          <button
            v-for="{ command, index } in filteredCommandEntries"
            :key="index"
            :aria-current="selectedCommand === command ? 'true' : undefined"
            :class="{ 'console-command-list__item--active': selectedCommand === command }"
            :data-testid="`console-command-list-item-${index}`"
            class="console-command-list__item"
            role="listitem"
            type="button"
            @click="selectCommand(command)">
            <span class="console-command-list__main">
              <code>{{ command.command.trim() || 'Untitled command' }}</code>
              <span>{{ command.summary.trim() || 'No summary yet' }}</span>
            </span>
            <span class="console-command-list__meta">
              <span v-if="command.category">{{ command.category }}</span>
              <span :class="riskClass(command.risk)">{{ riskLabel(command.risk) }}</span>
              <q-icon
                v-if="commandHasValidationError(index)"
                aria-label="Validation error"
                class="console-command-list__error"
                name="error_outline"
                size="17px" />
            </span>
          </button>
        </div>
      </aside>

      <div
        v-if="selectedCommand"
        ref="editorRootRef"
        class="console-command-workspace"
        tabindex="-1">
        <div class="console-command-workspace__toolbar">
          <div>
            <div class="console-command-workspace__title font-display">
              {{ selectedCommand.command.trim() || 'Untitled command' }}
            </div>
            <div class="console-command-workspace__position text-xy-muted">
              Command {{ selectedCommandIndex + 1 }} of {{ game.consoleCommands.length }}
            </div>
          </div>
          <div class="console-command-workspace__actions">
            <q-btn
              :aria-label="`Move ${selectedCommand.command || 'command'} up`"
              :disable="selectedCommandIndex <= 0"
              data-testid="move-console-command-up"
              flat
              icon="arrow_upward"
              round
              @click="moveCommand(-1)">
              <q-tooltip>Move command up</q-tooltip>
            </q-btn>
            <q-btn
              :aria-label="`Move ${selectedCommand.command || 'command'} down`"
              :disable="selectedCommandIndex >= game.consoleCommands.length - 1"
              data-testid="move-console-command-down"
              flat
              icon="arrow_downward"
              round
              @click="moveCommand(1)">
              <q-tooltip>Move command down</q-tooltip>
            </q-btn>
            <q-btn
              :aria-label="`Remove ${selectedCommand.command || 'command'}`"
              color="negative"
              data-testid="remove-console-command"
              flat
              icon="delete_outline"
              round
              @click="removeSelectedCommand">
              <q-tooltip>Remove command</q-tooltip>
            </q-btn>
          </div>
        </div>

        <div class="console-command-preview" data-testid="console-command-preview">
          <div class="console-command-preview__label font-display">Operator preview</div>
          <code class="console-command-preview__syntax">{{ previewSyntax }}</code>
          <div class="console-command-preview__summary">
            {{ selectedCommand.summary.trim() || 'Add a concise summary for this command.' }}
          </div>
          <p v-if="selectedCommand.description" class="console-command-preview__description">
            {{ selectedCommand.description }}
          </p>
          <div class="console-command-preview__meta">
            <span v-if="selectedCommand.category">{{ selectedCommand.category }}</span>
            <span :class="riskClass(selectedCommand.risk)">
              {{ riskLabel(selectedCommand.risk) }}
            </span>
            <span v-if="selectedCommand.availability">{{ selectedCommand.availability }}</span>
          </div>
          <div v-if="selectedCommand.aliases.length > 0" class="console-command-preview__aliases">
            <span>Aliases</span>
            <code v-for="alias in selectedCommand.aliases" :key="alias">{{ alias }}</code>
          </div>
          <div v-if="selectedCommand.keywords.length > 0" class="console-command-preview__aliases">
            <span>Keywords</span>
            <span v-for="keyword in selectedCommand.keywords" :key="keyword">{{ keyword }}</span>
          </div>
          <div
            v-if="selectedCommand.arguments.length > 0"
            class="console-command-preview__detail-list">
            <strong>Arguments</strong>
            <div v-for="(argument, index) in selectedCommand.arguments" :key="index">
              <code>{{
                argumentToken(argument.name, argument.required, argument.repeatable)
              }}</code>
              <span>{{ argument.description || 'No description provided.' }}</span>
              <small v-if="argumentMetadata(argument)">{{ argumentMetadata(argument) }}</small>
            </div>
          </div>
          <div
            v-if="selectedCommand.examples.length > 0"
            class="console-command-preview__detail-list">
            <strong>Examples</strong>
            <div v-for="(example, index) in selectedCommand.examples" :key="index">
              <code>{{ example.command || 'Example command pending' }}</code>
              <span v-if="example.description">{{ example.description }}</span>
            </div>
          </div>
          <ul v-if="selectedCommand.notes.length > 0" class="console-command-preview__notes">
            <li v-for="(note, index) in selectedCommand.notes" :key="index">
              {{ note || 'Note pending' }}
            </li>
          </ul>
          <a
            v-if="isValidDocumentationURL(selectedCommand.documentationUrl)"
            class="console-command-preview__link"
            :href="selectedCommand.documentationUrl"
            rel="noopener noreferrer"
            target="_blank">
            Open documentation
            <q-icon aria-hidden="true" name="open_in_new" size="15px" />
          </a>
        </div>

        <div class="command-editor-section">
          <div class="command-editor-section__heading">
            <div>
              <h3 class="font-display">Command identity</h3>
              <p>Define what operators type and how the entry is grouped.</p>
            </div>
          </div>
          <div class="command-field-grid">
            <q-input
              ref="canonicalInputRef"
              v-model="selectedCommand.command"
              :error="Boolean(errorFor('command'))"
              :error-message="errorFor('command')"
              class="command-code-input"
              data-testid="console-command-command"
              hint="Exact completion text, including any required leading slash"
              label="Canonical command *"
              no-error-icon
              outlined
              persistent-hint />
            <q-input
              v-model="selectedCommand.syntax"
              class="command-code-input"
              data-testid="console-command-syntax"
              hint="Human-readable usage, such as ban &lt;player&gt; [reason]"
              label="Display syntax"
              outlined
              persistent-hint />
            <q-input
              v-model="selectedCommand.category"
              data-testid="console-command-category"
              label="Category"
              outlined />
            <q-select
              v-model="selectedCommand.risk"
              :options="riskOptions"
              data-testid="console-command-risk"
              emit-value
              label="Risk"
              map-options
              outlined />
          </div>
          <q-input
            v-model="selectedCommand.summary"
            class="command-wide-field"
            data-testid="console-command-summary"
            hint="A short result-focused line shown in suggestions"
            label="Summary"
            outlined
            persistent-hint />
          <q-input
            v-model="selectedCommand.description"
            autogrow
            class="command-wide-field"
            data-testid="console-command-description"
            label="Details"
            outlined
            type="textarea" />
          <div class="command-field-grid">
            <q-select
              v-model="selectedCommand.aliases"
              :options="selectedCommand.aliases"
              data-testid="console-command-aliases"
              hide-dropdown-icon
              label="Aliases"
              multiple
              new-value-mode="add-unique"
              outlined
              use-chips
              use-input />
            <q-select
              v-model="selectedCommand.keywords"
              :options="selectedCommand.keywords"
              data-testid="console-command-keywords"
              hide-dropdown-icon
              label="Search keywords"
              multiple
              new-value-mode="add-unique"
              outlined
              use-chips
              use-input />
          </div>
        </div>

        <div class="command-editor-section">
          <div class="command-editor-section__heading">
            <div>
              <h3 class="font-display">Availability and guidance</h3>
              <p>Explain where the command works and link to its source documentation.</p>
            </div>
          </div>
          <q-input
            v-model="selectedCommand.availability"
            autogrow
            data-testid="console-command-availability"
            label="Availability"
            outlined
            type="textarea" />
          <q-input
            ref="documentationInputRef"
            v-model="selectedCommand.documentationUrl"
            :error="Boolean(errorFor('documentationUrl'))"
            :error-message="errorFor('documentationUrl')"
            class="command-wide-field"
            data-testid="console-command-documentation-url"
            label="Documentation URL"
            no-error-icon
            outlined
            type="url" />
        </div>

        <div class="command-editor-section">
          <div class="command-editor-section__heading">
            <div>
              <h3 class="font-display">Arguments</h3>
              <p>Describe each value in the same order operators type it.</p>
            </div>
            <q-btn
              data-testid="add-console-command-argument"
              flat
              icon="add"
              label="Add argument"
              no-caps
              @click="addArgument" />
          </div>
          <div
            v-if="selectedCommand.arguments.length === 0"
            class="command-collection-empty text-xy-muted">
            This command has no arguments.
          </div>
          <div
            v-for="(argument, index) in selectedCommand.arguments"
            :key="index"
            class="command-collection-row">
            <div class="command-collection-row__header">
              <strong>Argument {{ index + 1 }}</strong>
              <div>
                <q-btn
                  :aria-label="`Move argument ${index + 1} up`"
                  :disable="index === 0"
                  dense
                  flat
                  icon="arrow_upward"
                  round
                  @click="moveItem(selectedCommand.arguments, index, -1)" />
                <q-btn
                  :aria-label="`Move argument ${index + 1} down`"
                  :disable="index === selectedCommand.arguments.length - 1"
                  dense
                  flat
                  icon="arrow_downward"
                  round
                  @click="moveItem(selectedCommand.arguments, index, 1)" />
                <q-btn
                  :aria-label="`Remove argument ${index + 1}`"
                  color="negative"
                  dense
                  flat
                  icon="close"
                  round
                  @click="removeItem(selectedCommand.arguments, index)" />
              </div>
            </div>
            <div class="command-field-grid">
              <q-input
                :ref="(element) => setArgumentNameRef(index, element)"
                v-model="argument.name"
                :error="Boolean(errorFor('argumentName', index))"
                :error-message="errorFor('argumentName', index)"
                :data-testid="`console-command-argument-name-${index}`"
                label="Name *"
                no-error-icon
                outlined />
              <q-input v-model="argument.valueType" label="Value type" outlined />
            </div>
            <q-input
              v-model="argument.description"
              autogrow
              class="command-wide-field"
              label="Description"
              outlined
              type="textarea" />
            <div class="command-field-grid">
              <q-select
                v-model="argument.suggestedValues"
                :options="argument.suggestedValues"
                hide-dropdown-icon
                label="Suggested values"
                multiple
                new-value-mode="add-unique"
                outlined
                use-chips
                use-input />
              <q-input v-model="argument.defaultValue" label="Default value" outlined />
            </div>
            <div class="command-toggle-row">
              <q-toggle v-model="argument.required" label="Required" />
              <q-toggle v-model="argument.repeatable" label="Repeatable" />
            </div>
          </div>
        </div>

        <div class="command-editor-section">
          <div class="command-editor-section__heading">
            <div>
              <h3 class="font-display">Examples</h3>
              <p>Show realistic complete commands and what each one does.</p>
            </div>
            <q-btn
              data-testid="add-console-command-example"
              flat
              icon="add"
              label="Add example"
              no-caps
              @click="addExample" />
          </div>
          <div
            v-if="selectedCommand.examples.length === 0"
            class="command-collection-empty text-xy-muted">
            No examples have been added.
          </div>
          <div
            v-for="(example, index) in selectedCommand.examples"
            :key="index"
            class="command-collection-row">
            <div class="command-collection-row__header">
              <strong>Example {{ index + 1 }}</strong>
              <div>
                <q-btn
                  :aria-label="`Move example ${index + 1} up`"
                  :disable="index === 0"
                  dense
                  flat
                  icon="arrow_upward"
                  round
                  @click="moveItem(selectedCommand.examples, index, -1)" />
                <q-btn
                  :aria-label="`Move example ${index + 1} down`"
                  :disable="index === selectedCommand.examples.length - 1"
                  dense
                  flat
                  icon="arrow_downward"
                  round
                  @click="moveItem(selectedCommand.examples, index, 1)" />
                <q-btn
                  :aria-label="`Remove example ${index + 1}`"
                  color="negative"
                  dense
                  flat
                  icon="close"
                  round
                  @click="removeItem(selectedCommand.examples, index)" />
              </div>
            </div>
            <q-input
              :ref="(element) => setExampleCommandRef(index, element)"
              v-model="example.command"
              :error="Boolean(errorFor('exampleCommand', index))"
              :error-message="errorFor('exampleCommand', index)"
              :data-testid="`console-command-example-command-${index}`"
              class="command-code-input"
              label="Command *"
              no-error-icon
              outlined />
            <q-input
              v-model="example.description"
              autogrow
              class="command-wide-field"
              label="Description"
              outlined
              type="textarea" />
          </div>
        </div>

        <div class="command-editor-section command-editor-section--last">
          <div class="command-editor-section__heading">
            <div>
              <h3 class="font-display">Notes and warnings</h3>
              <p>Add operator guidance that does not fit the summary or risk level.</p>
            </div>
            <q-btn
              data-testid="add-console-command-note"
              flat
              icon="add"
              label="Add note"
              no-caps
              @click="addNote" />
          </div>
          <div
            v-if="selectedCommand.notes.length === 0"
            class="command-collection-empty text-xy-muted">
            No additional notes.
          </div>
          <div v-for="(_, index) in selectedCommand.notes" :key="index" class="command-note-row">
            <q-input
              v-model="selectedCommand.notes[index]"
              autogrow
              :label="`Note ${index + 1}`"
              outlined
              type="textarea" />
            <div class="command-note-row__actions">
              <q-btn
                :aria-label="`Move note ${index + 1} up`"
                :disable="index === 0"
                dense
                flat
                icon="arrow_upward"
                round
                @click="moveItem(selectedCommand.notes, index, -1)" />
              <q-btn
                :aria-label="`Move note ${index + 1} down`"
                :disable="index === selectedCommand.notes.length - 1"
                dense
                flat
                icon="arrow_downward"
                round
                @click="moveItem(selectedCommand.notes, index, 1)" />
              <q-btn
                :aria-label="`Remove note ${index + 1}`"
                color="negative"
                dense
                flat
                icon="close"
                round
                @click="removeItem(selectedCommand.notes, index)" />
            </div>
          </div>
        </div>
      </div>

      <div v-else class="console-command-workspace-empty">
        <q-icon aria-hidden="true" name="terminal" size="36px" />
        <h3 class="font-display">Create a command reference</h3>
        <p>
          Add documented administrative commands so operators can discover syntax and risk without
          leaving the server console.
        </p>
        <q-btn color="primary" icon="add" label="Add first command" no-caps @click="addCommand" />
      </div>
    </div>
  </section>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useFormChild } from 'quasar'
import { computed, inject, nextTick, ref, watch, type ComponentPublicInstance } from 'vue'

import {
  GameConsoleCommandArgumentSchema,
  GameConsoleCommandExampleSchema,
  GameConsoleCommandRisk,
  GameConsoleCommandSchema,
  type GameConsoleCommand,
  type GameConsoleCommandArgument,
} from '@/proto/shared_pb'
import { gameFormContextKey } from './GameFormTypes'

type ValidationField = 'command' | 'documentationUrl' | 'argumentName' | 'exampleCommand'

interface CatalogValidationError {
  commandIndex: number
  field: ValidationField
  itemIndex?: number
  message: string
}

interface Focusable {
  focus: () => void
}

const ctx = inject(gameFormContextKey)
if (!ctx) throw new Error('GameFormConsoleCommandsTab must be used inside GameForm')

const { activeFormTab, game } = ctx
const commandFilter = ref<string | null>('')
const selectedCommand = ref<GameConsoleCommand | null>(null)
const validationAttempted = ref(false)
const validationErrors = ref<CatalogValidationError[]>([])
const editorRootRef = ref<HTMLElement | null>(null)
const canonicalInputRef = ref<Focusable | null>(null)
const documentationInputRef = ref<Focusable | null>(null)
const argumentNameRefs = new Map<number, Focusable>()
const exampleCommandRefs = new Map<number, Focusable>()

const riskOptions = [
  { label: 'Not specified', value: GameConsoleCommandRisk.UNSPECIFIED },
  { label: 'None', value: GameConsoleCommandRisk.NONE },
  { label: 'Caution', value: GameConsoleCommandRisk.CAUTION },
  { label: 'Destructive', value: GameConsoleCommandRisk.DESTRUCTIVE },
]

const selectedCommandIndex = computed(() => {
  if (!selectedCommand.value) {
    return -1
  }
  return game.value.consoleCommands.indexOf(selectedCommand.value)
})

const filteredCommandEntries = computed(() => {
  const filter = (commandFilter.value ?? '').trim().toLocaleLowerCase()
  return game.value.consoleCommands
    .map((command, index) => ({ command, index }))
    .filter(({ command }) => {
      if (!filter) {
        return true
      }
      return [
        command.command,
        command.summary,
        command.category,
        ...command.aliases,
        ...command.keywords,
      ]
        .join(' ')
        .toLocaleLowerCase()
        .includes(filter)
    })
})

const previewSyntax = computed(() => {
  if (!selectedCommand.value) {
    return ''
  }
  return (
    selectedCommand.value.syntax.trim() ||
    selectedCommand.value.command.trim() ||
    'Command syntax preview'
  )
})

watch(
  () => game.value.consoleCommands,
  (commands) => {
    if (commands.length === 0) {
      selectedCommand.value = null
    } else if (!selectedCommand.value || !commands.includes(selectedCommand.value)) {
      selectedCommand.value = commands[0] ?? null
    }

    if (validationAttempted.value) {
      validationErrors.value = collectValidationErrors()
    }
  },
  { deep: true, immediate: true },
)

function addCommand(): void {
  const command = create(GameConsoleCommandSchema, {
    risk: GameConsoleCommandRisk.NONE,
  })
  game.value.consoleCommands.push(command)
  selectedCommand.value = command
  commandFilter.value = ''
  void nextTick(() => canonicalInputRef.value?.focus())
}

function selectCommand(command: GameConsoleCommand): void {
  selectedCommand.value = command
}

function removeSelectedCommand(): void {
  const index = selectedCommandIndex.value
  if (index < 0) {
    return
  }

  game.value.consoleCommands.splice(index, 1)
  selectedCommand.value =
    game.value.consoleCommands[index] ?? game.value.consoleCommands[index - 1] ?? null
}

function moveCommand(step: number): void {
  const index = selectedCommandIndex.value
  moveItem(game.value.consoleCommands, index, step)
}

function moveItem<T>(items: T[], index: number, step: number): void {
  const nextIndex = index + step
  if (index < 0 || nextIndex < 0 || nextIndex >= items.length) {
    return
  }

  const [item] = items.splice(index, 1)
  if (item === undefined) {
    return
  }
  items.splice(nextIndex, 0, item)
}

function removeItem<T>(items: T[], index: number): void {
  items.splice(index, 1)
}

function addArgument(): void {
  selectedCommand.value?.arguments.push(create(GameConsoleCommandArgumentSchema))
}

function addExample(): void {
  selectedCommand.value?.examples.push(create(GameConsoleCommandExampleSchema))
}

function addNote(): void {
  selectedCommand.value?.notes.push('')
}

function riskLabel(risk: GameConsoleCommandRisk): string {
  return riskOptions.find((option) => option.value === risk)?.label ?? 'Not specified'
}

function riskClass(risk: GameConsoleCommandRisk): string {
  if (risk === GameConsoleCommandRisk.DESTRUCTIVE) {
    return 'command-risk command-risk--destructive'
  }
  if (risk === GameConsoleCommandRisk.CAUTION) {
    return 'command-risk command-risk--caution'
  }
  return 'command-risk command-risk--none'
}

function argumentToken(name: string, required: boolean, repeatable: boolean): string {
  const value = `${name.trim() || 'argument'}${repeatable ? '…' : ''}`
  return required ? `<${value}>` : `[${value}]`
}

function argumentMetadata(argument: GameConsoleCommandArgument): string {
  const details = []
  if (argument.valueType) {
    details.push(`Type: ${argument.valueType}`)
  }
  if (argument.defaultValue) {
    details.push(`Default: ${argument.defaultValue}`)
  }
  if (argument.suggestedValues.length > 0) {
    details.push(`Suggested: ${argument.suggestedValues.join(', ')}`)
  }
  return details.join(' · ')
}

function isValidDocumentationURL(value: string): boolean {
  if (!value.trim()) {
    return true
  }

  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function collectValidationErrors(): CatalogValidationError[] {
  const errors: CatalogValidationError[] = []
  const commandCounts = new Map<string, number>()

  for (const command of game.value.consoleCommands) {
    const normalized = command.command.trim().toLocaleLowerCase()
    if (normalized) {
      commandCounts.set(normalized, (commandCounts.get(normalized) ?? 0) + 1)
    }
  }

  game.value.consoleCommands.forEach((command, commandIndex) => {
    const normalized = command.command.trim().toLocaleLowerCase()
    if (!normalized) {
      errors.push({
        commandIndex,
        field: 'command',
        message: `Command ${commandIndex + 1} needs a canonical command.`,
      })
    } else if ((commandCounts.get(normalized) ?? 0) > 1) {
      errors.push({
        commandIndex,
        field: 'command',
        message: `"${command.command.trim()}" is used by more than one command.`,
      })
    }

    if (!isValidDocumentationURL(command.documentationUrl)) {
      errors.push({
        commandIndex,
        field: 'documentationUrl',
        message: `Documentation URL for "${command.command.trim() || `command ${commandIndex + 1}`}" must be a valid HTTP or HTTPS URL.`,
      })
    }

    command.arguments.forEach((argument, itemIndex) => {
      if (!argument.name.trim()) {
        errors.push({
          commandIndex,
          field: 'argumentName',
          itemIndex,
          message: `Argument ${itemIndex + 1} for "${command.command.trim() || `command ${commandIndex + 1}`}" needs a name.`,
        })
      }
    })

    command.examples.forEach((example, itemIndex) => {
      if (!example.command.trim()) {
        errors.push({
          commandIndex,
          field: 'exampleCommand',
          itemIndex,
          message: `Example ${itemIndex + 1} for "${command.command.trim() || `command ${commandIndex + 1}`}" needs a command.`,
        })
      }
    })
  })

  return errors
}

function errorFor(field: ValidationField, itemIndex?: number): string {
  if (!validationAttempted.value) {
    return ''
  }

  const error = validationErrors.value.find(
    (candidate) =>
      candidate.commandIndex === selectedCommandIndex.value &&
      candidate.field === field &&
      candidate.itemIndex === itemIndex,
  )
  return error?.message ?? ''
}

function commandHasValidationError(commandIndex: number): boolean {
  return validationErrors.value.some((error) => error.commandIndex === commandIndex)
}

function validate(): boolean {
  validationAttempted.value = true
  validationErrors.value = collectValidationErrors()
  const firstError = validationErrors.value[0]
  if (!firstError) {
    return true
  }

  activeFormTab.value = 'console-commands'
  selectedCommand.value = game.value.consoleCommands[firstError.commandIndex] ?? null
  void nextTick(() => focusValidationError(firstError))
  return false
}

function resetValidation(): void {
  validationAttempted.value = false
  validationErrors.value = []
}

function focusValidationError(error: CatalogValidationError): void {
  if (error.field === 'command') {
    canonicalInputRef.value?.focus()
    return
  }
  if (error.field === 'documentationUrl') {
    documentationInputRef.value?.focus()
    return
  }
  if (error.field === 'argumentName' && error.itemIndex !== undefined) {
    argumentNameRefs.get(error.itemIndex)?.focus()
    return
  }
  if (error.field === 'exampleCommand' && error.itemIndex !== undefined) {
    exampleCommandRefs.get(error.itemIndex)?.focus()
    return
  }
  editorRootRef.value?.focus()
}

function setArgumentNameRef(
  index: number,
  element: Element | ComponentPublicInstance | null,
): void {
  setFocusableRef(argumentNameRefs, index, element)
}

function setExampleCommandRef(
  index: number,
  element: Element | ComponentPublicInstance | null,
): void {
  setFocusableRef(exampleCommandRefs, index, element)
}

function setFocusableRef(
  refs: Map<number, Focusable>,
  index: number,
  element: Element | ComponentPublicInstance | null,
): void {
  if (element && 'focus' in element && typeof element.focus === 'function') {
    refs.set(index, element as Focusable)
  } else {
    refs.delete(index)
  }
}

useFormChild({
  validate,
  resetValidation,
})

defineExpose({
  validate,
  resetValidation,
})
</script>

<style scoped>
.console-commands-section {
  min-width: 0;
}

.console-commands-heading {
  align-items: flex-start;
}

.console-commands-section-bar {
  background-color: var(--xy-accent);
}

.console-commands-intro {
  margin: 4px 0 0;
  max-width: 68ch;
  font-size: var(--xy-font-size-sm);
}

.console-commands-validation {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin-bottom: 16px;
  padding: 12px 14px;
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg-faint);
  border: 1px solid var(--xy-danger-border);
  border-radius: var(--xy-radius-md);
}

.console-commands-validation div {
  display: grid;
  gap: 2px;
}

.console-commands-validation span {
  color: var(--xy-text-primary);
}

.console-commands-layout {
  display: grid;
  grid-template-columns: minmax(240px, 30%) minmax(0, 1fr);
  min-height: 620px;
  overflow: hidden;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.console-command-index {
  min-width: 0;
  background: var(--xy-surface-1);
  border-right: 1px solid var(--xy-border);
}

.console-command-index__header {
  display: grid;
  gap: 12px;
  padding: 16px;
  border-bottom: 1px solid var(--xy-border);
}

.console-command-index__title {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-base);
}

.console-command-index__count {
  margin-top: 2px;
  font-size: var(--xy-font-size-xs);
}

.console-command-filter {
  width: 100%;
}

.console-command-list {
  max-height: 740px;
  overflow-y: auto;
}

.console-command-list__item {
  display: grid;
  gap: 8px;
  width: 100%;
  padding: 13px 16px;
  color: var(--xy-text-primary);
  text-align: left;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--xy-border);
  cursor: pointer;
}

.console-command-list__item:hover {
  background: var(--xy-surface-2);
}

.console-command-list__item:focus-visible {
  position: relative;
  z-index: 1;
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: -2px;
}

.console-command-list__item--active {
  background: var(--xy-primary-muted);
  box-shadow: inset 3px 0 0 var(--xy-primary);
}

.console-command-list__main {
  display: grid;
  gap: 3px;
  min-width: 0;
}

.console-command-list__main code {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.console-command-list__main span {
  overflow: hidden;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.console-command-list__meta {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  min-width: 0;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
}

.console-command-list__error {
  margin-left: auto;
  color: var(--xy-danger-hover);
}

.console-command-index__empty,
.console-command-workspace-empty {
  display: flex;
  flex-direction: column;
  gap: 6px;
  align-items: center;
  justify-content: center;
  min-height: 220px;
  padding: 28px;
  color: var(--xy-text-secondary);
  text-align: center;
}

.console-command-index__empty small {
  color: var(--xy-text-muted);
}

.console-command-workspace {
  min-width: 0;
  padding: 20px 24px 28px;
  outline: none;
}

.console-command-workspace__toolbar {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
  margin-bottom: 16px;
}

.console-command-workspace__title {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-lg);
}

.console-command-workspace__position {
  margin-top: 2px;
  font-size: var(--xy-font-size-xs);
}

.console-command-workspace__actions {
  display: flex;
  flex: 0 0 auto;
  gap: 2px;
}

.console-command-preview {
  display: grid;
  gap: 7px;
  margin-bottom: 28px;
  padding: 16px 18px;
  background: var(--xy-base);
  border: 1px solid var(--xy-border-active);
  border-radius: var(--xy-radius-md);
}

.console-command-preview__label {
  color: var(--xy-accent);
  font-size: var(--xy-font-size-xs);
}

.console-command-preview__syntax {
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-base);
}

.console-command-preview__summary {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.console-command-preview__description {
  max-width: 70ch;
  margin: 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  white-space: pre-wrap;
}

.console-command-preview__meta,
.console-command-preview__aliases {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.console-command-preview__aliases code {
  padding: 2px 6px;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-sm);
}

.console-command-preview__detail-list {
  display: grid;
  gap: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--xy-border);
}

.console-command-preview__detail-list > strong {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.console-command-preview__detail-list > div {
  display: grid;
  grid-template-columns: minmax(120px, auto) minmax(0, 1fr);
  gap: 10px;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.console-command-preview__detail-list code {
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.console-command-preview__detail-list small {
  grid-column: 2;
  color: var(--xy-text-muted);
}

.console-command-preview__notes {
  display: grid;
  gap: 3px;
  margin: 2px 0 0;
  padding-left: 20px;
  color: var(--xy-warning-hover);
  font-size: var(--xy-font-size-xs);
}

.console-command-preview__link {
  display: inline-flex;
  gap: 5px;
  align-items: center;
  width: fit-content;
  color: var(--xy-primary-hover);
  font-size: var(--xy-font-size-xs);
  text-decoration: none;
}

.console-command-preview__link:hover {
  text-decoration: underline;
}

.console-command-preview__link:focus-visible {
  border-radius: var(--xy-radius-sm);
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 2px;
}

.command-risk {
  display: inline-flex;
  align-items: center;
  width: fit-content;
  padding: 2px 7px;
  border-radius: var(--xy-radius-pill);
}

.command-risk--none {
  color: var(--xy-text-secondary);
  background: var(--xy-surface-3);
}

.command-risk--caution {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg-faint);
}

.command-risk--destructive {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg-faint);
}

.command-editor-section {
  display: grid;
  gap: 16px;
  padding: 24px 0;
  border-top: 1px solid var(--xy-border);
}

.command-editor-section--last {
  padding-bottom: 0;
}

.command-editor-section__heading,
.command-collection-row__header {
  display: flex;
  gap: 16px;
  align-items: flex-start;
  justify-content: space-between;
}

.command-editor-section__heading h3 {
  margin: 0;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-base);
}

.command-editor-section__heading p {
  margin: 3px 0 0;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.command-field-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}

.command-wide-field {
  width: 100%;
}

.command-code-input :deep(input),
.command-code-input :deep(textarea) {
  font-family: var(--xy-font-mono);
}

.command-collection-empty {
  padding: 14px 0;
  font-size: var(--xy-font-size-sm);
}

.command-collection-row {
  display: grid;
  gap: 14px;
  padding: 18px 0 4px;
  border-top: 1px solid var(--xy-border);
}

.command-collection-row:first-of-type {
  border-top: 0;
}

.command-collection-row__header strong {
  color: var(--xy-text-secondary);
}

.command-collection-row__header > div {
  display: flex;
  gap: 2px;
}

.command-toggle-row {
  display: flex;
  flex-wrap: wrap;
  gap: 20px;
}

.command-note-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px;
  align-items: flex-start;
}

.command-note-row__actions {
  display: flex;
  gap: 2px;
}

.console-command-workspace-empty {
  min-height: 520px;
}

.console-command-workspace-empty h3 {
  margin: 6px 0 0;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-lg);
}

.console-command-workspace-empty p {
  max-width: 48ch;
  margin: 0 0 12px;
  color: var(--xy-text-secondary);
}

@media (max-width: 900px) {
  .console-commands-layout {
    grid-template-columns: 1fr;
  }

  .console-command-index {
    border-right: 0;
    border-bottom: 1px solid var(--xy-border);
  }

  .console-command-index__header {
    grid-template-columns: minmax(120px, auto) minmax(180px, 1fr);
    align-items: center;
  }

  .console-command-list {
    display: flex;
    max-height: none;
    overflow-x: auto;
    overflow-y: hidden;
  }

  .console-command-list__item {
    flex: 0 0 230px;
    border-right: 1px solid var(--xy-border);
    border-bottom: 0;
  }

  .console-command-list__item--active {
    box-shadow: inset 0 -3px 0 var(--xy-primary);
  }
}

@media (max-width: 640px) {
  .console-commands-heading {
    flex-wrap: wrap;
  }

  .console-commands-heading .section-line {
    display: none;
  }

  .console-commands-heading > .q-btn {
    width: 100%;
  }

  .console-command-index__header,
  .command-field-grid {
    grid-template-columns: 1fr;
  }

  .console-command-workspace {
    padding: 16px;
  }

  .console-command-workspace__toolbar,
  .command-editor-section__heading {
    align-items: flex-start;
  }

  .command-editor-section__heading {
    flex-direction: column;
  }

  .command-editor-section__heading > .q-btn {
    width: 100%;
  }

  .command-note-row {
    grid-template-columns: 1fr;
  }

  .console-command-preview__detail-list > div {
    grid-template-columns: 1fr;
    gap: 2px;
  }

  .console-command-preview__detail-list small {
    grid-column: 1;
  }

  .command-note-row__actions {
    justify-content: flex-end;
  }
}
</style>
