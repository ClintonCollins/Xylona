<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { computed, onMounted, ref } from 'vue'
import type { Component } from 'vue'
import { useQuasar } from 'quasar'
import { useRoute } from 'vue-router'
import PageHeader from '@/components/shared/PageHeader.vue'
import {
  ExecuteGameServerOperationRequestSchema,
  GameOperationFieldType,
  GameOperationResultClassification,
  GameOperationResultSchema,
  GameOperationRisk,
  GameOperationValueSchema,
  ListGameServerOperationsRequestSchema,
} from '@/proto/xylona_pb'
import type {
  GameOperationDescriptor,
  GameOperationField,
  GameOperationFieldOption,
  GameOperationResult,
  GameOperationValue,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

type OperationValue = string | number | boolean
type PlayerIdentityMode = 'known' | 'manual'

const route = useRoute()
const $q = useQuasar()
const operations = ref<GameOperationDescriptor[]>([])
const gameServerName = ref('')
const loading = ref(true)
const loadError = ref('')
const search = ref('')
const selectedCategory = ref('All operations')
const expandedOperationID = ref('')
const operationValues = ref<Record<string, OperationValue>>({})
const playerModes = ref<Record<string, PlayerIdentityMode>>({})
const playerSearches = ref<Record<string, string>>({})
const operationResults = ref<Record<string, GameOperationResult>>({})
const executingOperationID = ref('')
const mobileLayout = computed(() => $q.screen.lt.md)

const bundledOperationRenderers: Readonly<Record<string, Component>> = Object.freeze({})

const categories = computed(() => [
  'All operations',
  ...Array.from(new Set(operations.value.map((operation) => operation.category))).sort(),
])

const filteredOperations = computed(() => {
  const needle = search.value.trim().toLocaleLowerCase()
  return operations.value.filter((operation) => {
    if (
      selectedCategory.value !== 'All operations' &&
      operation.category !== selectedCategory.value
    ) {
      return false
    }
    if (needle === '') {
      return true
    }
    return [
      operation.name,
      operation.summary,
      operation.category,
      operation.availabilityReasonText,
    ].some((value) => value.toLocaleLowerCase().includes(needle))
  })
})

onMounted(loadOperations)

async function loadOperations() {
  loading.value = true
  loadError.value = ''
  try {
    const response = await GetXylonaClient().listGameServerOperations(
      create(ListGameServerOperationsRequestSchema, {
        gameServerId: String(route.params.id ?? ''),
      }),
    )
    gameServerName.value = response.gameServerName
    operations.value = response.operations
    if (!categories.value.includes(selectedCategory.value)) {
      selectedCategory.value = 'All operations'
    }
  } catch {
    loadError.value =
      'The operation catalog could not be loaded. Check the server connection and retry.'
  } finally {
    loading.value = false
  }
}

function selectCategory(category: string) {
  selectedCategory.value = category
  expandedOperationID.value = ''
}

function toggleOperation(operation: GameOperationDescriptor) {
  if (expandedOperationID.value === operation.id) {
    expandedOperationID.value = ''
    return
  }
  expandedOperationID.value = operation.id
  initializeValues(operation)
}

function initializeValues(operation: GameOperationDescriptor) {
  const values: Record<string, OperationValue> = {}
  const modes: Record<string, PlayerIdentityMode> = {}
  const searches: Record<string, string> = {}
  for (const field of operation.fields) {
    if (field.type === GameOperationFieldType.BOOLEAN) {
      values[field.id] = field.defaultValue === 'true'
    } else if (field.type === GameOperationFieldType.PLAYER_IDENTITY) {
      modes[field.id] = field.options.length > 0 ? 'known' : 'manual'
      values[field.id] = field.options[0]?.value ?? ''
      searches[field.id] = ''
    } else {
      values[field.id] = field.defaultValue || field.options[0]?.value || ''
    }
  }
  operationValues.value = values
  playerModes.value = modes
  playerSearches.value = searches
}

function setPlayerMode(field: GameOperationField, mode: PlayerIdentityMode) {
  playerModes.value[field.id] = mode
  operationValues.value[field.id] = mode === 'known' ? (field.options[0]?.value ?? '') : ''
}

function selectPlayer(field: GameOperationField, option: GameOperationFieldOption) {
  operationValues.value[field.id] = option.value
}

function filteredPlayerOptions(field: GameOperationField) {
  const needle = (playerSearches.value[field.id] ?? '').trim().toLocaleLowerCase()
  if (needle === '') {
    return field.options
  }
  return field.options.filter((option) =>
    [option.label, option.value, option.description].some((value) =>
      value.toLocaleLowerCase().includes(needle),
    ),
  )
}

function fieldError(field: GameOperationField): string {
  const value = operationValues.value[field.id]
  if (field.required && (value === '' || value === undefined)) {
    return `${field.label} is required.`
  }
  if (
    field.type === GameOperationFieldType.PLAYER_IDENTITY &&
    playerModes.value[field.id] === 'manual' &&
    typeof value === 'string' &&
    value !== ''
  ) {
    try {
      if (!new RegExp(field.validationPattern).test(value)) {
        return 'Use a platform-prefixed identity such as Steam_PLAYER_1.'
      }
    } catch {
      return 'The Player identity rule is unavailable.'
    }
  }
  if (field.type === GameOperationFieldType.INTEGER && value !== '') {
    const numericValue = Number(value)
    if (!Number.isInteger(numericValue)) {
      return 'Enter a whole native value.'
    }
    if (field.minValue !== undefined && numericValue < field.minValue) {
      return `Enter ${field.minValue} or greater.`
    }
    if (field.maxValue !== undefined && numericValue > field.maxValue) {
      return `Enter ${field.maxValue} or less.`
    }
  }
  return ''
}

function reviewReady(operation: GameOperationDescriptor) {
  return operation.fields.every((field) => fieldError(field) === '')
}

function reviewValue(field: GameOperationField) {
  const value = String(operationValues.value[field.id] ?? '')
  const option = field.options.find((candidate) => candidate.value === value)
  return option ? `${option.label} — ${option.value}` : value
}

function fieldDescribedBy(field: GameOperationField) {
  const IDs: string[] = []
  if (field.description) {
    IDs.push(`${field.id}-description`)
  }
  if (fieldError(field)) {
    IDs.push(`${field.id}-error`)
  }
  return IDs.join(' ') || undefined
}

function riskLabel(risk: GameOperationRisk) {
  switch (risk) {
    case GameOperationRisk.ROUTINE:
      return 'Routine'
    case GameOperationRisk.CAUTION:
      return 'Caution'
    case GameOperationRisk.IRREVERSIBLE:
      return 'Irreversible'
    default:
      return 'Review'
  }
}

function riskIcon(risk: GameOperationRisk) {
  return risk === GameOperationRisk.ROUTINE ? 'check_circle' : 'warning'
}

function rendererFor(operation: GameOperationDescriptor) {
  return operation.rendererKey && Object.hasOwn(bundledOperationRenderers, operation.rendererKey)
    ? bundledOperationRenderers[operation.rendererKey]
    : undefined
}

function typedOperationValues(operation: GameOperationDescriptor): GameOperationValue[] {
  return operation.fields.map((field) => {
    const rawValue = operationValues.value[field.id]
    if (
      field.type === GameOperationFieldType.INTEGER ||
      field.type === GameOperationFieldType.DURATION
    ) {
      return create(GameOperationValueSchema, {
        fieldId: field.id,
        value: { case: 'integerValue', value: BigInt(String(rawValue)) },
      })
    }
    if (field.type === GameOperationFieldType.BOOLEAN) {
      return create(GameOperationValueSchema, {
        fieldId: field.id,
        value: { case: 'booleanValue', value: Boolean(rawValue) },
      })
    }
    return create(GameOperationValueSchema, {
      fieldId: field.id,
      value: { case: 'stringValue', value: String(rawValue ?? '') },
    })
  })
}

async function executeOperation(operation: GameOperationDescriptor) {
  if (executingOperationID.value !== '' || !reviewReady(operation)) {
    return
  }
  executingOperationID.value = operation.id
  try {
    const response = await GetXylonaClient().executeGameServerOperation(
      create(ExecuteGameServerOperationRequestSchema, {
        gameServerId: String(route.params.id ?? ''),
        operationId: operation.id,
        values: typedOperationValues(operation),
      }),
    )
    operationResults.value[operation.id] =
      response.result ?? failedOperationResult('The server returned no operation result.')
  } catch (unknownError) {
    const error = ConnectError.from(unknownError)
    operationResults.value[operation.id] = failedOperationResult(
      `The operation could not be completed: ${ConnectErrorToString(error)}`,
    )
  } finally {
    executingOperationID.value = ''
  }
}

function failedOperationResult(message: string) {
  return create(GameOperationResultSchema, {
    classification: GameOperationResultClassification.FAILED,
    message,
  })
}

function resultLabel(classification: GameOperationResultClassification) {
  switch (classification) {
    case GameOperationResultClassification.CONFIRMED:
      return 'Confirmed'
    case GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED:
      return 'Accepted, not verified'
    default:
      return 'Failed'
  }
}

function resultIcon(classification: GameOperationResultClassification) {
  switch (classification) {
    case GameOperationResultClassification.CONFIRMED:
      return 'check_circle'
    case GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED:
      return 'help_outline'
    default:
      return 'error_outline'
  }
}
</script>

<template>
  <div class="operations-page xy-page-content">
    <page-header
      icon="assignment"
      :subtitle="`Search the operations available to ${gameServerName || 'this server'}, configure semantic inputs, and review the intended effect.`"
      title="Operation Ledger" />

    <div v-if="loading" aria-live="polite" class="operations-state">
      <q-spinner color="primary" size="2rem" />
      <span>Loading operations…</span>
    </div>

    <div v-else-if="loadError" class="operations-state operations-state--error" role="alert">
      <q-icon aria-hidden="true" name="error_outline" />
      <span>{{ loadError }}</span>
      <button
        class="operations-button operations-button--secondary"
        type="button"
        @click="loadOperations">
        Retry
      </button>
    </div>

    <template v-else>
      <label class="operations-search">
        <span class="sr-only">Search operations</span>
        <q-icon aria-hidden="true" name="search" />
        <input
          v-model="search"
          data-testid="operation-search"
          placeholder="Search operations"
          type="search" />
      </label>

      <label v-if="mobileLayout" class="operations-mobile-category">
        <span>Category</span>
        <select v-model="selectedCategory" data-testid="mobile-category-picker">
          <option v-for="category in categories" :key="category" :value="category">
            {{ category }}
          </option>
        </select>
      </label>

      <div class="operations-ledger">
        <aside v-if="!mobileLayout" aria-label="Operation categories" class="operations-categories">
          <div class="operations-categories__title">Categories</div>
          <button
            v-for="category in categories"
            :key="category"
            :aria-current="selectedCategory === category ? 'true' : undefined"
            class="operations-category"
            :class="{ 'operations-category--active': selectedCategory === category }"
            :data-testid="`category-${category}`"
            type="button"
            @click="selectCategory(category)">
            <span>{{ category }}</span>
            <span class="operations-category__count">
              {{
                category === 'All operations'
                  ? operations.length
                  : operations.filter((operation) => operation.category === category).length
              }}
            </span>
          </button>
        </aside>

        <main class="operations-list">
          <div class="operations-list__heading">
            <div>
              <h2>{{ selectedCategory }}</h2>
              <p>
                {{ filteredOperations.length }} operation{{
                  filteredOperations.length === 1 ? '' : 's'
                }}
              </p>
            </div>
          </div>

          <div v-if="filteredOperations.length === 0" class="operations-state">
            <q-icon aria-hidden="true" name="search_off" />
            <span>No operations match this search and category.</span>
          </div>

          <article
            v-for="operation in filteredOperations"
            :key="operation.id"
            class="operation-entry"
            data-testid="operation-row">
            <button
              :aria-controls="`operation-${operation.id}`"
              :aria-expanded="expandedOperationID === operation.id"
              class="operation-row"
              data-testid="operation-toggle"
              type="button"
              @click="toggleOperation(operation)">
              <span class="operation-row__main">
                <span class="operation-row__title-line">
                  <span class="operation-row__name">{{ operation.name }}</span>
                  <span
                    class="operation-risk"
                    :class="`operation-risk--${riskLabel(operation.risk).toLowerCase()}`">
                    <q-icon aria-hidden="true" :name="riskIcon(operation.risk)" />
                    {{ riskLabel(operation.risk) }}
                  </span>
                </span>
                <span class="operation-row__summary">{{ operation.summary }}</span>
              </span>
              <span
                class="operation-row__availability"
                :class="{ 'operation-row__availability--disabled': !operation.available }">
                <q-icon aria-hidden="true" :name="operation.available ? 'check_circle' : 'block'" />
                {{ operation.available ? 'Available' : 'Unavailable' }}
              </span>
              <q-icon
                aria-hidden="true"
                class="operation-row__chevron"
                :name="expandedOperationID === operation.id ? 'expand_less' : 'expand_more'" />
            </button>

            <Transition name="ledger-expand">
              <section
                v-if="expandedOperationID === operation.id"
                :id="`operation-${operation.id}`"
                class="operation-expansion"
                data-testid="operation-expansion">
                <div v-if="!operation.available" class="operation-unavailable" role="status">
                  <q-icon aria-hidden="true" name="info" />
                  <div>
                    <strong>Operation unavailable</strong>
                    <p>{{ operation.availabilityReasonText }}</p>
                  </div>
                </div>

                <component
                  :is="rendererFor(operation)"
                  v-else-if="rendererFor(operation)"
                  :operation="operation" />

                <div v-else class="operation-form" data-testid="generic-operation-form">
                  <div
                    v-for="field in operation.fields"
                    :key="field.id"
                    :aria-describedby="fieldDescribedBy(field)"
                    :aria-labelledby="`${field.id}-label`"
                    class="operation-field"
                    role="group">
                    <template v-if="field.type === GameOperationFieldType.PLAYER_IDENTITY">
                      <div class="operation-field__heading">
                        <label :id="`${field.id}-label`" :for="`${field.id}-search`">
                          {{ field.label }}
                        </label>
                        <span v-if="field.required">Required</span>
                      </div>
                      <p :id="`${field.id}-description`">{{ field.description }}</p>
                      <div
                        class="operation-mode"
                        role="group"
                        :aria-label="`${field.label} source`">
                        <button
                          :aria-pressed="playerModes[field.id] === 'known'"
                          class="operation-mode__button"
                          data-testid="player-mode-known"
                          :disabled="field.options.length === 0"
                          type="button"
                          @click="setPlayerMode(field, 'known')">
                          Known Player
                        </button>
                        <button
                          v-if="field.allowManual"
                          :aria-pressed="playerModes[field.id] === 'manual'"
                          class="operation-mode__button"
                          data-testid="player-mode-manual"
                          type="button"
                          @click="setPlayerMode(field, 'manual')">
                          Manual identity
                        </button>
                      </div>

                      <template v-if="playerModes[field.id] === 'known'">
                        <label class="operation-control">
                          <span>Search known Players</span>
                          <input
                            :id="`${field.id}-search`"
                            v-model="playerSearches[field.id]"
                            autocomplete="off"
                            placeholder="Name or stable ID"
                            :required="field.required"
                            type="search" />
                        </label>
                        <div
                          class="player-options"
                          role="group"
                          :aria-label="`${field.label} results`">
                          <button
                            v-for="option in filteredPlayerOptions(field)"
                            :key="option.value"
                            :aria-pressed="operationValues[field.id] === option.value"
                            class="player-option"
                            type="button"
                            @click="selectPlayer(field, option)">
                            <span class="player-option__name">{{ option.label }}</span>
                            <span class="player-option__identity">{{
                              option.description || option.value
                            }}</span>
                          </button>
                          <p
                            v-if="filteredPlayerOptions(field).length === 0"
                            class="operation-field__empty">
                            No known Players match. Use manual identity to continue.
                          </p>
                        </div>
                      </template>
                      <label v-else class="operation-control">
                        <span>Stable platform identity</span>
                        <input
                          v-model="operationValues[field.id]"
                          autocomplete="off"
                          :aria-invalid="fieldError(field) !== ''"
                          data-testid="manual-player-identity"
                          placeholder="Steam_PLAYER_1"
                          :required="field.required"
                          spellcheck="false"
                          type="text" />
                      </label>
                    </template>

                    <template v-else-if="field.type === GameOperationFieldType.INTEGER">
                      <div class="operation-field__heading">
                        <label :id="`${field.id}-label`" :for="field.id">{{ field.label }}</label>
                        <span v-if="field.required">Required</span>
                      </div>
                      <p :id="`${field.id}-description`">{{ field.description }}</p>
                      <div class="operation-value-grid">
                        <label v-if="field.options.length > 0" class="operation-control">
                          <span>Preset</span>
                          <select
                            v-model="operationValues[field.id]"
                            :aria-invalid="fieldError(field) !== ''"
                            :required="field.required">
                            <option
                              v-for="option in field.options"
                              :key="option.value"
                              :value="option.value">
                              {{ option.label }} ({{ option.value }})
                            </option>
                          </select>
                        </label>
                        <label v-if="field.allowExactValue" class="operation-control">
                          <span>Exact native value</span>
                          <input
                            :id="field.id"
                            v-model="operationValues[field.id]"
                            data-testid="permission-exact-value"
                            :max="field.maxValue"
                            :min="field.minValue"
                            :aria-invalid="fieldError(field) !== ''"
                            :required="field.required"
                            step="1"
                            type="number" />
                        </label>
                      </div>
                    </template>

                    <label
                      v-else-if="field.type === GameOperationFieldType.BOOLEAN"
                      class="operation-check">
                      <input
                        v-model="operationValues[field.id]"
                        :required="field.required"
                        type="checkbox" />
                      <span :id="`${field.id}-label`">{{ field.label }}</span>
                    </label>

                    <label
                      v-else-if="field.type === GameOperationFieldType.ENUM"
                      class="operation-control">
                      <span :id="`${field.id}-label`">{{ field.label }}</span>
                      <select
                        v-model="operationValues[field.id]"
                        :aria-invalid="fieldError(field) !== ''"
                        :required="field.required">
                        <option
                          v-for="option in field.options"
                          :key="option.value"
                          :value="option.value">
                          {{ option.label }}
                        </option>
                      </select>
                    </label>

                    <label v-else class="operation-control">
                      <span :id="`${field.id}-label`">{{ field.label }}</span>
                      <input
                        v-model="operationValues[field.id]"
                        :aria-invalid="fieldError(field) !== ''"
                        :required="field.required"
                        :type="
                          field.type === GameOperationFieldType.DURATION ? 'number' : 'text'
                        " />
                    </label>

                    <p
                      v-if="fieldError(field)"
                      :id="`${field.id}-error`"
                      class="operation-field__error"
                      role="alert">
                      {{ fieldError(field) }}
                    </p>
                  </div>

                  <section
                    v-if="reviewReady(operation)"
                    class="operation-review"
                    data-testid="operation-review">
                    <div class="operation-review__heading">
                      <q-icon aria-hidden="true" name="fact_check" />
                      <h3>{{ operation.review?.title || 'Review intended effect' }}</h3>
                    </div>
                    <dl>
                      <template v-for="field in operation.fields" :key="field.id">
                        <dt>{{ field.label }}</dt>
                        <dd>{{ reviewValue(field) }}</dd>
                      </template>
                      <dt>Scope</dt>
                      <dd>{{ gameServerName || 'This game server' }}</dd>
                      <dt>Risk</dt>
                      <dd>{{ riskLabel(operation.risk) }}</dd>
                      <dt>Expected effect</dt>
                      <dd>{{ operation.review?.effect }}</dd>
                    </dl>
                    <p v-if="operation.review?.caution" class="operation-review__caution">
                      <q-icon aria-hidden="true" name="warning" />
                      {{ operation.review.caution }}
                    </p>
                    <div class="operation-review__actions">
                      <button
                        :aria-busy="executingOperationID === operation.id"
                        class="operations-button operations-button--primary"
                        data-testid="execute-operation"
                        :disabled="executingOperationID !== ''"
                        type="button"
                        @click="executeOperation(operation)">
                        {{
                          executingOperationID === operation.id
                            ? 'Executing…'
                            : `Execute ${operation.name}`
                        }}
                      </button>
                    </div>
                  </section>

                  <section
                    v-if="operationResults[operation.id]"
                    class="operation-result"
                    :class="`operation-result--${resultLabel(
                      operationResults[operation.id]!.classification,
                    )
                      .toLowerCase()
                      .replaceAll(/[^a-z]+/g, '-')}`"
                    data-testid="operation-result"
                    :role="
                      operationResults[operation.id]!.classification ===
                      GameOperationResultClassification.FAILED
                        ? 'alert'
                        : 'status'
                    ">
                    <div class="operation-result__heading">
                      <q-icon
                        aria-hidden="true"
                        :name="resultIcon(operationResults[operation.id]!.classification)" />
                      <strong>{{
                        resultLabel(operationResults[operation.id]!.classification)
                      }}</strong>
                    </div>
                    <p>{{ operationResults[operation.id]!.message }}</p>
                    <details v-if="operationResults[operation.id]!.transportDetails">
                      <summary>Transport details</summary>
                      <dl>
                        <dt>Method</dt>
                        <dd>{{ operationResults[operation.id]!.transportDetails?.method }}</dd>
                        <dt>Verification</dt>
                        <dd>
                          {{ operationResults[operation.id]!.transportDetails?.verification }}
                        </dd>
                      </dl>
                    </details>
                  </section>
                </div>
              </section>
            </Transition>
          </article>
        </main>
      </div>
    </template>
  </div>
</template>

<style scoped>
.operations-page {
  --operations-control-height: 2.75rem;
  --xy-text: var(--xy-text-primary);
  --xy-warning-text: var(--xy-warning-hover);
  --xy-success-text: var(--xy-success-text-soft);
  --xy-danger-text: var(--xy-danger-hover);
  --xy-border-strong: var(--xy-border-hover);
  color: var(--xy-text);
}

.operations-page ::selection {
  color: var(--xy-text);
  background: var(--xy-primary-muted);
}

.operations-search {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: var(--operations-control-height);
  padding: 0 var(--xy-space-base);
  margin-bottom: var(--xy-space-lg);
  color: var(--xy-text-muted);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.operations-search:focus-within {
  border-color: var(--xy-primary);
  box-shadow: 0 2px 12px color-mix(in srgb, var(--xy-primary) 24%, transparent);
}

.operations-search input {
  width: 100%;
  min-height: var(--operations-control-height);
  color: var(--xy-text);
  background: transparent;
  border: 0;
  outline: 0;
}

.operations-search input::placeholder,
.operation-control input::placeholder {
  color: var(--xy-text-muted);
  opacity: 1;
}

.operations-mobile-category {
  display: none;
}

.operations-ledger {
  display: grid;
  grid-template-columns: minmax(12rem, 14rem) minmax(0, 1fr);
  gap: var(--xy-space-xl);
  align-items: start;
}

.operations-categories {
  position: sticky;
  top: var(--xy-space-base);
}

.operations-categories__title {
  margin-bottom: var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.operations-category {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  min-height: 2.75rem;
  padding: var(--xy-space-sm) var(--xy-space-base);
  color: var(--xy-text-muted);
  text-align: left;
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-sm);
  cursor: pointer;
}

.operations-category:hover,
.operations-category--active {
  color: var(--xy-text);
  background: var(--xy-primary-muted);
}

.operations-category--active {
  font-weight: 700;
}

.operations-category__count {
  min-width: 1.75rem;
  padding: 0.1rem var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  text-align: center;
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-pill);
}

.operations-list {
  min-width: 0;
}

.operations-list__heading {
  display: flex;
  align-items: end;
  justify-content: space-between;
  min-height: 3rem;
  margin-bottom: var(--xy-space-sm);
}

.operations-list__heading h2,
.operation-review h3 {
  margin: 0;
  font-family: var(--xy-font-heading);
}

.operations-list__heading h2 {
  font-size: var(--xy-font-size-xl);
}

.operations-list__heading p,
.operation-field > p,
.operation-unavailable p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-muted);
}

.operation-entry {
  border-top: 1px solid var(--xy-border);
}

.operation-entry:last-child {
  border-bottom: 1px solid var(--xy-border);
}

.operation-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  gap: var(--xy-space-base);
  align-items: center;
  width: 100%;
  min-height: 5rem;
  padding: var(--xy-space-base);
  color: var(--xy-text);
  text-align: left;
  background: transparent;
  border: 0;
  cursor: pointer;
}

.operation-row:hover {
  background: color-mix(in srgb, var(--xy-primary) 6%, transparent);
}

.operation-row__main,
.operation-row__title-line {
  display: flex;
  min-width: 0;
}

.operation-row__main {
  flex-direction: column;
  gap: var(--xy-space-xs);
}

.operation-row__title-line {
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  align-items: center;
}

.operation-row__name {
  font-size: var(--xy-font-size-lg);
  font-weight: 700;
}

.operation-row__summary {
  max-width: 70ch;
  color: var(--xy-text-muted);
}

.operation-risk,
.operation-row__availability {
  display: inline-flex;
  gap: var(--xy-space-xs);
  align-items: center;
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
}

.operation-risk {
  padding: 0.15rem var(--xy-space-sm);
  color: var(--xy-warning-text);
  background: color-mix(in srgb, var(--xy-warning) 14%, var(--xy-surface-1));
  border-radius: var(--xy-radius-pill);
}

.operation-risk--routine {
  color: var(--xy-success-text);
  background: color-mix(in srgb, var(--xy-success) 14%, var(--xy-surface-1));
}

.operation-risk--irreversible {
  color: var(--xy-danger-text);
  background: color-mix(in srgb, var(--xy-danger) 14%, var(--xy-surface-1));
}

.operation-row__availability {
  color: var(--xy-success-text);
}

.operation-row__availability--disabled {
  color: var(--xy-text-muted);
}

.operation-row__chevron {
  color: var(--xy-text-muted);
}

.operation-expansion {
  padding: var(--xy-space-lg);
  margin: 0 var(--xy-space-base) var(--xy-space-lg);
  background: var(--xy-surface-1);
  border-radius: var(--xy-radius-lg);
  transform-origin: top;
}

.ledger-expand-enter-active,
.ledger-expand-leave-active {
  transition:
    opacity var(--xy-transition-fast),
    clip-path var(--xy-transition-base);
}

.ledger-expand-enter-from,
.ledger-expand-leave-to {
  opacity: 0;
  clip-path: inset(0 0 100% 0 round var(--xy-radius-lg));
}

.operation-unavailable {
  display: flex;
  gap: var(--xy-space-base);
  align-items: flex-start;
  color: var(--xy-text);
}

.operation-unavailable > .q-icon {
  color: var(--xy-warning);
  font-size: 1.5rem;
}

.operation-form {
  display: grid;
  gap: var(--xy-space-xl);
}

.operation-field {
  min-width: 0;
}

.operation-field__heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--xy-space-base);
}

.operation-field__heading label,
.operation-control > span {
  font-weight: 700;
}

.operation-field__heading > span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.operation-mode {
  display: inline-flex;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-xs);
  margin-top: var(--xy-space-base);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-sm);
}

.operation-mode__button,
.operations-button {
  min-height: 2.5rem;
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-muted);
  font-weight: 700;
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-sm);
  cursor: pointer;
}

.operation-mode__button[aria-pressed='true'] {
  color: var(--xy-text);
  background: var(--xy-primary-muted);
}

.operation-mode__button:disabled {
  cursor: not-allowed;
  opacity: 0.45;
}

.operation-control {
  display: grid;
  gap: var(--xy-space-xs);
  margin-top: var(--xy-space-base);
}

.operation-control input,
.operation-control select,
.operations-mobile-category select {
  width: 100%;
  min-height: var(--operations-control-height);
  padding: 0 var(--xy-space-base);
  color: var(--xy-text);
  color-scheme: dark;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
}

.operation-control input:focus,
.operation-control select:focus,
.operations-mobile-category select:focus {
  border-color: var(--xy-primary);
  outline: 2px solid color-mix(in srgb, var(--xy-primary) 45%, transparent);
  outline-offset: 2px;
}

.player-options {
  display: grid;
  max-height: 17rem;
  margin-top: var(--xy-space-sm);
  overflow-y: auto;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-sm);
  scrollbar-color: var(--xy-border-strong) var(--xy-surface-1);
  scrollbar-width: thin;
}

.player-option {
  display: grid;
  gap: var(--xy-space-xs);
  min-height: 3.5rem;
  padding: var(--xy-space-sm) var(--xy-space-base);
  color: var(--xy-text);
  text-align: left;
  background: transparent;
  border: 0;
  border-bottom: 1px solid var(--xy-border);
  cursor: pointer;
}

.player-option:last-of-type {
  border-bottom: 0;
}

.player-option:hover,
.player-option[aria-pressed='true'] {
  background: var(--xy-primary-muted);
}

.player-option__name {
  font-weight: 700;
}

.player-option__identity,
.operation-review dd {
  overflow-wrap: anywhere;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.operation-field__empty {
  padding: var(--xy-space-base);
  margin: 0;
  color: var(--xy-text-muted);
}

.operation-value-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-base);
}

.operation-check {
  display: flex;
  gap: var(--xy-space-sm);
  align-items: center;
  min-height: var(--operations-control-height);
}

.operation-field__error {
  margin: var(--xy-space-sm) 0 0;
  color: var(--xy-danger-text);
  font-weight: 600;
}

.operation-review {
  padding-top: var(--xy-space-lg);
  border-top: 1px solid var(--xy-border);
}

.operation-review__heading {
  display: flex;
  gap: var(--xy-space-sm);
  align-items: center;
}

.operation-review__heading > .q-icon {
  color: var(--xy-primary);
  font-size: var(--xy-font-size-lg);
}

.operation-review dl {
  display: grid;
  grid-template-columns: minmax(8rem, 12rem) minmax(0, 1fr);
  gap: var(--xy-space-sm) var(--xy-space-base);
  max-width: 60ch;
  margin: var(--xy-space-lg) 0;
}

.operation-review dt {
  color: var(--xy-text-muted);
}

.operation-review dd {
  margin: 0;
  color: var(--xy-text);
}

.operation-review__caution {
  display: flex;
  gap: var(--xy-space-sm);
  align-items: flex-start;
  max-width: 70ch;
}

.operation-review__caution {
  color: var(--xy-warning-text);
}

.operation-review__actions {
  margin-top: var(--xy-space-lg);
}

.operations-button--primary {
  color: var(--xy-text-on-bright);
  background: var(--xy-primary);
}

.operations-button--primary:hover:not(:disabled) {
  background: var(--xy-primary-hover);
}

.operations-button:disabled {
  cursor: wait;
  opacity: 0.65;
}

.operation-result {
  padding-top: var(--xy-space-lg);
  color: var(--xy-success-text);
  border-top: 1px solid var(--xy-border);
}

.operation-result--accepted-not-verified {
  color: var(--xy-warning-text);
}

.operation-result--failed {
  color: var(--xy-danger-text);
}

.operation-result__heading {
  display: flex;
  gap: var(--xy-space-sm);
  align-items: center;
  font-size: var(--xy-font-size-lg);
}

.operation-result > p {
  max-width: 70ch;
  color: var(--xy-text);
}

.operation-result details {
  max-width: 60ch;
  color: var(--xy-text-muted);
}

.operation-result summary {
  width: fit-content;
  cursor: pointer;
}

.operation-result dl {
  display: grid;
  grid-template-columns: 8rem minmax(0, 1fr);
  gap: var(--xy-space-xs) var(--xy-space-base);
}

.operation-result dd {
  margin: 0;
  color: var(--xy-text);
}

.operations-state {
  display: flex;
  gap: var(--xy-space-sm);
  align-items: center;
  justify-content: center;
  min-height: 10rem;
  color: var(--xy-text-muted);
}

.operations-state--error {
  color: var(--xy-danger-text);
}

.operations-button--secondary {
  color: var(--xy-text);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
}

.operation-row:focus-visible,
.operations-category:focus-visible,
.operation-mode__button:focus-visible,
.player-option:focus-visible,
.operations-button:focus-visible {
  outline: 2px solid var(--xy-primary);
  outline-offset: 2px;
}

.sr-only {
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

@media (max-width: 1023px) {
  .operations-ledger {
    grid-template-columns: minmax(0, 1fr);
  }

  .operations-categories {
    display: none;
  }

  .operations-mobile-category {
    display: grid;
    gap: var(--xy-space-xs);
    margin-bottom: var(--xy-space-lg);
    font-weight: 700;
  }
}

@media (max-width: 599px) {
  .operations-page {
    padding-inline: var(--xy-space-base);
  }

  .operation-row {
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--xy-space-sm);
    padding-inline: var(--xy-space-sm);
  }

  .operation-row__availability {
    grid-column: 1 / -1;
    grid-row: 2;
  }

  .operation-row__chevron {
    grid-column: 2;
    grid-row: 1;
  }

  .operation-expansion {
    padding: var(--xy-space-base);
    margin-inline: 0;
  }

  .operation-mode {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    width: 100%;
  }

  .operation-value-grid,
  .operation-review dl {
    grid-template-columns: minmax(0, 1fr);
  }

  .operation-review dl {
    gap: var(--xy-space-xs);
  }

  .operation-review dd {
    margin-bottom: var(--xy-space-sm);
  }
}

@media (prefers-reduced-motion: reduce) {
  .ledger-expand-enter-active,
  .ledger-expand-leave-active {
    transition: none;
  }
}
</style>
