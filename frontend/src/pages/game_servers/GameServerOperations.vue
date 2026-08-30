<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import OperationCatalogInput from '@/components/game_servers/OperationCatalogInput.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import {
  ExecuteGameServerOperationRequestSchema,
  GameOperationFieldType,
  GameOperationResultClassification,
  GameOperationResultSchema,
  GameOperationRisk,
  GameOperationValueSchema,
  GetGameServerRequestSchema,
  GetSevenDaysToDieWebAPIStatusRequestSchema,
  ListGameServerOperationsRequestSchema,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import { StartGameServerRequestSchema, Status } from '@/proto/shared_pb'
import type {
  GameOperationDescriptor,
  GameOperationField,
  GameOperationFieldOption,
  GameOperationResult,
  GameOperationValue,
  SevenDaysToDieWebAPIStatus,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { canStartServer as isStartableStatus } from './server-list-actions'

type OperationValue = string | number | boolean
type OperationCategory = 'players' | 'access' | 'messages' | 'world'
type PermissionMode = 'moderator' | 'administrator' | 'custom'

interface WorkbenchState {
  activeCategory: OperationCategory
  activeOperationID: string
  administratorPermissionMode: PermissionMode
  administratorCustomPermissionLevel: number
  commandPermissionMode: PermissionMode
  commandCustomPermissionLevel: number
  itemAmount: number
  experienceAmount: number
  temperatureUnit: string
  weatherPreset: string
  exactWorldDay: number
  exactWorldHour: number
  exactWorldMinute: number
}

interface PendingOperation {
  operation: GameOperationDescriptor
  values: Record<string, OperationValue>
}

interface LatestResult {
  operation: GameOperationDescriptor
  result: GameOperationResult
}

interface OperationPanelDefinition {
  id: string
  label: string
  summary: string
  operationIDs: readonly string[]
}

interface OperationPanel extends OperationPanelDefinition {
  category: OperationCategory
  detail: string
  operationID: string
}

const worldStatusPollMilliseconds = 30_000
const playerAccessOperationIDs = [
  'player_access.add_administrator',
  'player_access.remove_administrator',
  'player_access.allowlist_add',
  'player_access.allowlist_remove',
] as const
const playerOperationIDs = [
  ...playerAccessOperationIDs,
  'player_moderation.kick',
  'player_moderation.ban',
  'player_moderation.unban',
] as const
const playerAssistanceOperationIDs = [
  'player_assistance.teleport_to_player',
  'player_assistance.give_item',
  'player_assistance.give_experience',
  'player_assistance.apply_buff',
  'player_assistance.remove_buff',
] as const
const commandPermissionOperationIDs = [
  'permissions.set_command_permission',
  'permissions.reset_command_permission',
] as const
const communicationOperationIDs = [
  'communication.message_player',
  'communication.broadcast_message',
] as const
const worldOperationIDs = [
  'server_control.save_world',
  'server_control.set_temperature_unit',
  'server_control.set_game_time',
] as const
const worldControlOperationIDs = [...worldOperationIDs, 'world_events.set_weather'] as const
const spawnEventOperationIDs = [
  'world_events.spawn_airdrop',
  'world_events.spawn_wandering_horde',
] as const
const worldEventOperationIDs = [...spawnEventOperationIDs, 'world_events.set_weather'] as const
const sharedOperationPanelDefinitions: OperationPanelDefinition[] = [
  {
    id: 'player-access',
    label: 'Player access',
    summary: 'Manage administrator and allowlist access for a player.',
    operationIDs: playerAccessOperationIDs,
  },
  {
    id: 'player-assistance',
    label: 'Player assistance',
    summary: 'Teleport players and grant items, experience, buffs, or debuffs.',
    operationIDs: playerAssistanceOperationIDs,
  },
  {
    id: 'command-permissions',
    label: 'Command permissions',
    summary: 'Set or reset the permission level for server commands.',
    operationIDs: commandPermissionOperationIDs,
  },
  {
    id: 'messaging',
    label: 'Messaging',
    summary: 'Send a message to one player or broadcast to everyone.',
    operationIDs: communicationOperationIDs,
  },
  {
    id: 'world-controls',
    label: 'World controls',
    summary: 'Manage weather, time, temperature display, and world saves.',
    operationIDs: worldControlOperationIDs,
  },
  {
    id: 'spawn-events',
    label: 'Spawn events',
    summary: 'Spawn an airdrop or wandering horde in the active world.',
    operationIDs: spawnEventOperationIDs,
  },
]
const operationPanelDefinitionByOperationID = new Map(
  sharedOperationPanelDefinitions.flatMap((panel) =>
    panel.operationIDs.map((operationID) => [operationID, panel] as const),
  ),
)
const commonOperationIDs = new Set([
  ...playerOperationIDs,
  ...playerAssistanceOperationIDs,
  ...commandPermissionOperationIDs,
  ...communicationOperationIDs,
  ...worldOperationIDs,
  ...worldEventOperationIDs,
])
const operationCategories: { id: OperationCategory; label: string }[] = [
  { id: 'players', label: 'Players' },
  { id: 'access', label: 'Access' },
  { id: 'messages', label: 'Messages' },
  { id: 'world', label: 'World' },
]
const defaultWorkbenchState: WorkbenchState = {
  activeCategory: 'players',
  activeOperationID: '',
  administratorPermissionMode: 'moderator',
  administratorCustomPermissionLevel: 200,
  commandPermissionMode: 'moderator',
  commandCustomPermissionLevel: 200,
  itemAmount: 1,
  experienceAmount: 1000,
  temperatureUnit: 'F',
  weatherPreset: 'natural',
  exactWorldDay: 1,
  exactWorldHour: 12,
  exactWorldMinute: 0,
}

const route = useRoute()
const operations = ref<GameOperationDescriptor[]>([])
const gameServerName = ref('')
const loading = ref(true)
const loadError = ref('')
const formError = ref('')
const operationSearch = ref('')
const activeCategory = ref<OperationCategory>('players')
const activeOperationID = ref('')
const playerIdentity = ref('')
const destinationPlayerIdentity = ref('')
const administratorPermissionMode = ref<PermissionMode>('moderator')
const administratorCustomPermissionLevel = ref<OperationValue>(200)
const moderationReason = ref('')
const itemName = ref('')
const itemAmount = ref<OperationValue>(1)
const experienceAmount = ref<OperationValue>(1000)
const buffName = ref('')
const commandName = ref('')
const commandPermissionMode = ref<PermissionMode>('moderator')
const commandCustomPermissionLevel = ref<OperationValue>(200)
const message = ref('')
const temperatureUnit = ref('F')
const weatherPreset = ref('natural')
const exactWorldDay = ref<OperationValue>(1)
const exactWorldHour = ref<OperationValue>(12)
const exactWorldMinute = ref<OperationValue>(0)
const executingOperationIDs = ref(new Set<string>())
const pendingOperation = ref<PendingOperation | null>(null)
const latestResult = ref<LatestResult | null>(null)
const worldStatus = ref<SevenDaysToDieWebAPIStatus | null>(null)
const worldStatusLoading = ref(true)
const worldStatusError = ref(false)
const lifecycleStatus = ref(Status.UNKNOWN)
const lifecycleStatusAuthoritative = ref(false)
const canStartServer = ref(false)
const startingServer = ref(false)
const startRequested = ref(false)
const startError = ref('')
const operationResult = ref<HTMLElement | null>(null)
let worldStatusPollTimer: ReturnType<typeof setInterval> | undefined
let worldStatusRequestInFlight = false

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const workbenchStorageKey = computed(
  () => `xylona:game-server:${gameServerID.value}:operations-workbench`,
)
const operationByID = computed(
  () => new Map(operations.value.map((operation) => [operation.id, operation])),
)
const spawnEventOperations = computed(() =>
  spawnEventOperationIDs.flatMap((operationID) => {
    const operation = operationByID.value.get(operationID)
    return operation ? [operation] : []
  }),
)
const hasCommonOperations = computed(() =>
  operations.value.some((operation) => commonOperationIDs.has(operation.id)),
)
const catalogOperations = computed(() =>
  operations.value.filter((operation) => commonOperationIDs.has(operation.id)),
)
const operationPanels = computed<OperationPanel[]>(() => {
  const panels = new Map<string, OperationPanel>()
  for (const operation of catalogOperations.value) {
    const definition = operationPanelDefinitionByOperationID.get(operation.id)
    const id = definition?.id ?? operation.id
    if (panels.has(id)) continue
    const operationIDs = definition?.operationIDs ?? [operation.id]
    const operationID =
      operationIDs.find((candidate) => operationByID.value.get(candidate)?.available) ??
      operationIDs.find((candidate) => operationByID.value.has(candidate)) ??
      operation.id
    const operationCount = operationIDs.filter((candidate) =>
      operationByID.value.has(candidate),
    ).length
    panels.set(id, {
      id,
      label: definition?.label ?? operation.name,
      summary: definition?.summary ?? operation.summary,
      operationIDs,
      operationID,
      category: operationCategory(operationID),
      detail: definition ? `${operationCount} actions` : operation.category,
    })
  }
  return [...panels.values()]
})
const filteredOperationPanels = computed(() => {
  const query = operationSearch.value.trim().toLocaleLowerCase()
  return operationPanels.value.filter((panel) => {
    if (query === '') return panel.category === activeCategory.value
    const operationsText = panel.operationIDs
      .flatMap((operationID) => {
        const operation = operationByID.value.get(operationID)
        return operation ? [operation.name, operation.summary, operation.category] : []
      })
      .join(' ')
    return `${panel.label} ${panel.summary} ${operationsText}`.toLocaleLowerCase().includes(query)
  })
})
const activeOperation = computed(() => operationByID.value.get(activeOperationID.value) ?? null)
const activeOperationPanel = computed(
  () =>
    operationPanels.value.find((panel) => panel.operationIDs.includes(activeOperationID.value)) ??
    null,
)
const activeOperationPanelRisk = computed(() =>
  activeOperationPanel.value?.operationIDs.some(
    (operationID) => operationByID.value.get(operationID)?.risk !== GameOperationRisk.ROUTINE,
  )
    ? GameOperationRisk.CAUTION
    : GameOperationRisk.ROUTINE,
)
const activeOperationPanelUnavailableReason = computed(() => {
  const panelOperations =
    activeOperationPanel.value?.operationIDs.flatMap((operationID) => {
      const operation = operationByID.value.get(operationID)
      return operation ? [operation] : []
    }) ?? []
  if (panelOperations.some((operation) => operation.available)) return ''
  return (
    panelOperations
      .map((operation) => operation.availabilityReasonText.trim())
      .find((reason) => reason !== '') ?? 'This operation panel is currently unavailable.'
  )
})
const activeOperationUsesPlayer = computed(() => {
  if (
    playerAccessOperationIDs.some((operationID) => operationID === activeOperationID.value) ||
    playerAssistanceOperationIDs.some((operationID) => operationID === activeOperationID.value)
  ) {
    return true
  }
  if (communicationOperationIDs.some((operationID) => operationID === activeOperationID.value)) {
    return operationByID.value.has('communication.message_player')
  }
  return activeOperation.value?.fields.some(
    (field) => field.type === GameOperationFieldType.PLAYER_IDENTITY && field.id === 'player',
  )
})
const administratorPermissionLevel = computed<OperationValue>(() =>
  permissionLevel(administratorPermissionMode.value, administratorCustomPermissionLevel.value),
)
const commandPermissionLevel = computed<OperationValue>(() =>
  permissionLevel(commandPermissionMode.value, commandCustomPermissionLevel.value),
)
const serverStopped = computed(
  () => lifecycleStatusAuthoritative.value && isStartableStatus(lifecycleStatus.value),
)
const knownPlayers = computed(() => {
  const players = new Map<string, GameOperationFieldOption>()
  for (const operation of operations.value) {
    for (const field of operation.fields) {
      if (field.type !== GameOperationFieldType.PLAYER_IDENTITY) continue
      for (const option of field.options) players.set(option.value, option)
    }
  }
  return [...players.values()]
})
const onlinePlayers = computed(
  () =>
    operationByID.value
      .get('player_assistance.teleport_to_player')
      ?.fields.find((field) => field.id === 'destination')?.options ?? [],
)
const onlinePlayerValues = computed(
  () => new Set(onlinePlayers.value.map((player) => player.value)),
)
const selectedPlayer = computed(() => {
  const value = playerIdentity.value.trim()
  if (value === '') return null
  const option = knownPlayers.value.find((player) => player.value === value)
  const label = option?.label ?? value
  const words = label.split(/\s+/).filter(Boolean)
  let initials = label.slice(0, 2)
  if (words.length > 1) {
    initials = `${words.at(0)?.at(0) ?? ''}${words.at(-1)?.at(0) ?? ''}`
  }
  initials = initials.toUpperCase()
  return {
    initials,
    known: option !== undefined,
    label,
    online: option !== undefined && onlinePlayerValues.value.has(value),
    value,
  }
})
const teleportPlayers = computed(() =>
  knownPlayers.value.map((player) => ({
    label: onlinePlayerValues.value.has(player.value) ? player.label : `${player.label} (offline)`,
    value: player.value,
  })),
)
const destinationPlayerIsKnownOffline = computed(() => {
  const destination = destinationPlayerIdentity.value.trim()
  return (
    destination !== '' &&
    knownPlayers.value.some((player) => player.value === destination) &&
    !onlinePlayerValues.value.has(destination)
  )
})
const itemOptions = computed(() => operationCatalogOptions(['player_assistance.give_item'], 'item'))
const selectedItem = computed(() => {
  const value = itemName.value.trim()
  if (value === '') return null
  const option = itemOptions.value.find((item) => item.value === value)
  return {
    category: option?.category ?? '',
    iconUrl: option?.iconUrl ?? '',
    known: option !== undefined,
    label: option?.label ?? value,
    value,
  }
})
const buffOptions = computed(() =>
  operationCatalogOptions(
    ['player_assistance.apply_buff', 'player_assistance.remove_buff'],
    'buff',
  ),
)
const commandOptions = computed(() =>
  operationCatalogOptions(commandPermissionOperationIDs, 'command'),
)
const activeAvailabilityNotice = computed(() => activeOperationPanelUnavailableReason.value)
const availabilityNotices = computed(() => [
  ...new Set(
    operations.value
      .filter((operation) => commonOperationIDs.has(operation.id) && !operation.available)
      .map((operation) => operation.availabilityReasonText.trim())
      .filter((notice) => notice !== '' && notice !== activeAvailabilityNotice.value),
  ),
])
const confirmationOpen = computed({
  get: () => pendingOperation.value !== null,
  set: (open: boolean) => {
    if (!open) pendingOperation.value = null
  },
})
const pendingValues = computed(() => {
  const pending = pendingOperation.value
  if (!pending) return []
  return pending.operation.fields
    .map((field) => ({ label: field.label, value: displayValue(field, pending.values[field.id]) }))
    .filter((entry) => entry.value !== '')
})
const worldTimeLabel = computed(() => {
  const status = worldStatus.value
  if (
    status?.worldTimeState ===
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE &&
    status.worldTime
  ) {
    return `Day ${status.worldTime.day}, ${String(status.worldTime.hour).padStart(2, '0')}:${String(status.worldTime.minute).padStart(2, '0')}`
  }
  switch (status?.worldTimeState) {
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED:
      return 'Not supported by this server'
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED:
      return 'Access denied by the game server'
    default:
      return worldStatusError.value ? 'Unable to load' : 'Unavailable'
  }
})

onMounted(() => {
  restoreWorkbenchState()
  void loadOperations()
  void loadLifecycleState()
  void loadWorldStatus()
  worldStatusPollTimer = setInterval(() => void loadWorldStatus(), worldStatusPollMilliseconds)
})

watch(
  [
    activeCategory,
    activeOperationID,
    administratorPermissionMode,
    administratorCustomPermissionLevel,
    commandPermissionMode,
    commandCustomPermissionLevel,
    itemAmount,
    experienceAmount,
    temperatureUnit,
    weatherPreset,
    exactWorldDay,
    exactWorldHour,
    exactWorldMinute,
  ],
  persistWorkbenchState,
)

onBeforeUnmount(() => {
  if (worldStatusPollTimer !== undefined) clearInterval(worldStatusPollTimer)
})

async function loadOperations() {
  loading.value = true
  loadError.value = ''
  try {
    const response = await GetXylonaClient().listGameServerOperations(
      create(ListGameServerOperationsRequestSchema, { gameServerId: gameServerID.value }),
    )
    gameServerName.value = response.gameServerName
    operations.value = response.operations
    ensureActiveOperation()
    openLinkedOperation()
  } catch {
    loadError.value =
      'The administration controls could not be loaded. Check the server connection and retry.'
  } finally {
    loading.value = false
  }
}

async function loadLifecycleState() {
  lifecycleStatusAuthoritative.value = false
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerID.value }),
    )
    const gameServer = response.gameServer
    if (!gameServer || gameServer.status === Status.UNKNOWN) return
    lifecycleStatus.value = gameServer.status
    canStartServer.value = gameServer.effectivePermissions.includes('game_server.start')
    lifecycleStatusAuthoritative.value = true
  } catch {
    lifecycleStatus.value = Status.UNKNOWN
    canStartServer.value = false
  }
}

async function startGameServer() {
  if (!serverStopped.value || !canStartServer.value || startingServer.value) return
  startingServer.value = true
  startError.value = ''
  startRequested.value = false
  try {
    await GetXylonaClient().startGameServer(
      create(StartGameServerRequestSchema, { serverId: gameServerID.value }),
    )
    startRequested.value = true
  } catch (unknownError) {
    startError.value = `The game server could not be started: ${ConnectErrorToString(ConnectError.from(unknownError))}`
  } finally {
    startingServer.value = false
  }
}

async function loadWorldStatus() {
  if (gameServerID.value === '') {
    worldStatusLoading.value = false
    return
  }
  if (worldStatusRequestInFlight) return
  worldStatusRequestInFlight = true
  worldStatusLoading.value = true
  try {
    const response = await GetXylonaClient().getSevenDaysToDieWebAPIStatus(
      create(GetSevenDaysToDieWebAPIStatusRequestSchema, { gameServerId: gameServerID.value }),
    )
    worldStatus.value = response.status ?? null
    worldStatusError.value = false
  } catch {
    worldStatus.value = null
    worldStatusError.value = true
  } finally {
    worldStatusRequestInFlight = false
    worldStatusLoading.value = false
  }
}

function routeQueryValue(name: string) {
  const value = route.query?.[name]
  return Array.isArray(value) ? (value[0] ?? '') : String(value ?? '')
}

function openLinkedOperation() {
  const operationID = routeQueryValue('operation')
  const operation = operationByID.value.get(operationID)
  if (!operation || !commonOperationIDs.has(operationID)) return
  selectOperation(operationID)
  const linkedPlayer = routeQueryValue('player')
  if (linkedPlayer !== '') playerIdentity.value = linkedPlayer
  requestOperation(operationID, { player: linkedPlayer }, true)
}

function operationCategory(operationID: string): OperationCategory {
  if (
    operationID.startsWith('player_assistance.') ||
    operationID.startsWith('player_moderation.')
  ) {
    return 'players'
  }
  if (operationID.startsWith('player_access.') || operationID.startsWith('permissions.')) {
    return 'access'
  }
  if (operationID.startsWith('communication.')) return 'messages'
  return 'world'
}

function operationCategoryCount(category: OperationCategory) {
  return operationPanels.value.filter((panel) => panel.category === category).length
}

function selectCategory(category: OperationCategory) {
  activeCategory.value = category
  operationSearch.value = ''
  const first = operationPanels.value.find((panel) => panel.category === category)
  if (first) activeOperationID.value = first.operationID
  formError.value = ''
}

function selectOperation(operationID: string) {
  const operation = operationByID.value.get(operationID)
  if (!operation || !commonOperationIDs.has(operationID)) return
  activeOperationID.value = operationID
  activeCategory.value = operationCategory(operationID)
  formError.value = ''
}

function ensureActiveOperation() {
  if (activeOperationPanel.value) return
  const categoryMatch = operationPanels.value.find(
    (panel) => panel.category === activeCategory.value,
  )
  const first = categoryMatch ?? operationPanels.value[0]
  if (!first) return
  activeOperationID.value = first.operationID
  activeCategory.value = first.category
}

function permissionLevel(mode: PermissionMode, customValue: OperationValue): OperationValue {
  if (mode === 'moderator') return 200
  if (mode === 'administrator') return 0
  return customValue
}

function permissionPresetAvailable(operationID: string, level: number) {
  const field = operationByID.value
    .get(operationID)
    ?.fields.find((candidate) => candidate.id === 'permission_level')
  if (!field) return false
  return (
    (field.minValue === undefined || level >= field.minValue) &&
    (field.maxValue === undefined || level <= field.maxValue)
  )
}

function permissionMinimum(operationID: string) {
  return (
    operationByID.value
      .get(operationID)
      ?.fields.find((candidate) => candidate.id === 'permission_level')?.minValue ?? 0
  )
}

function permissionMaximum(operationID: string) {
  return (
    operationByID.value
      .get(operationID)
      ?.fields.find((candidate) => candidate.id === 'permission_level')?.maxValue ?? 1000
  )
}

function isPermissionMode(value: unknown): value is PermissionMode {
  return value === 'moderator' || value === 'administrator' || value === 'custom'
}

function isOperationCategory(value: unknown): value is OperationCategory {
  return value === 'players' || value === 'access' || value === 'messages' || value === 'world'
}

function finiteNumber(value: unknown, fallback: number) {
  return typeof value === 'number' && Number.isFinite(value) ? value : fallback
}

function restoreWorkbenchState() {
  try {
    const raw = window.localStorage.getItem(workbenchStorageKey.value)
    if (raw === null) return
    const stored = JSON.parse(raw) as Partial<WorkbenchState>
    if (isOperationCategory(stored.activeCategory)) activeCategory.value = stored.activeCategory
    if (typeof stored.activeOperationID === 'string')
      activeOperationID.value = stored.activeOperationID
    if (isPermissionMode(stored.administratorPermissionMode)) {
      administratorPermissionMode.value = stored.administratorPermissionMode
    }
    administratorCustomPermissionLevel.value = finiteNumber(
      stored.administratorCustomPermissionLevel,
      defaultWorkbenchState.administratorCustomPermissionLevel,
    )
    if (isPermissionMode(stored.commandPermissionMode)) {
      commandPermissionMode.value = stored.commandPermissionMode
    }
    commandCustomPermissionLevel.value = finiteNumber(
      stored.commandCustomPermissionLevel,
      defaultWorkbenchState.commandCustomPermissionLevel,
    )
    itemAmount.value = finiteNumber(stored.itemAmount, defaultWorkbenchState.itemAmount)
    experienceAmount.value = finiteNumber(
      stored.experienceAmount,
      defaultWorkbenchState.experienceAmount,
    )
    if (stored.temperatureUnit === 'F' || stored.temperatureUnit === 'C') {
      temperatureUnit.value = stored.temperatureUnit
    }
    if (['natural', 'rain', 'snow'].includes(stored.weatherPreset ?? '')) {
      weatherPreset.value = stored.weatherPreset ?? defaultWorkbenchState.weatherPreset
    }
    exactWorldDay.value = finiteNumber(stored.exactWorldDay, defaultWorkbenchState.exactWorldDay)
    exactWorldHour.value = finiteNumber(stored.exactWorldHour, defaultWorkbenchState.exactWorldHour)
    exactWorldMinute.value = finiteNumber(
      stored.exactWorldMinute,
      defaultWorkbenchState.exactWorldMinute,
    )
  } catch {
    // Workbench preferences are best-effort; use safe defaults when storage is unavailable.
  }
}

function persistWorkbenchState() {
  const state: WorkbenchState = {
    activeCategory: activeCategory.value,
    activeOperationID: activeOperationID.value,
    administratorPermissionMode: administratorPermissionMode.value,
    administratorCustomPermissionLevel: Number(administratorCustomPermissionLevel.value),
    commandPermissionMode: commandPermissionMode.value,
    commandCustomPermissionLevel: Number(commandCustomPermissionLevel.value),
    itemAmount: Number(itemAmount.value),
    experienceAmount: Number(experienceAmount.value),
    temperatureUnit: temperatureUnit.value,
    weatherPreset: weatherPreset.value,
    exactWorldDay: Number(exactWorldDay.value),
    exactWorldHour: Number(exactWorldHour.value),
    exactWorldMinute: Number(exactWorldMinute.value),
  }
  try {
    window.localStorage.setItem(workbenchStorageKey.value, JSON.stringify(state))
  } catch {
    // Persisting non-sensitive preferences is best-effort.
  }
}

function operationCatalogOptions(operationIDs: readonly string[], fieldID: string) {
  const options = new Map<string, GameOperationFieldOption>()
  for (const operationID of operationIDs) {
    const field = operationByID.value.get(operationID)?.fields.find((entry) => entry.id === fieldID)
    for (const option of field?.options ?? []) options.set(option.value, option)
  }
  return [...options.values()]
}

function hideBrokenImage(event: Event) {
  const image = event.currentTarget
  if (image instanceof HTMLImageElement) image.hidden = true
}

function isExecuting(operationID: string) {
  return executingOperationIDs.value.has(operationID)
}

function riskLabel(risk: GameOperationRisk) {
  return risk === GameOperationRisk.ROUTINE ? 'Routine' : 'Review required'
}

function operationDisabled(operationID: string, inputReady = true) {
  const operation = operationByID.value.get(operationID)
  return !operation || !operation.available || !inputReady || isExecuting(operationID)
}

function fieldError(field: GameOperationField, value: OperationValue | undefined) {
  if (field.required && (value === '' || value === undefined)) return `${field.label} is required.`
  if (typeof value === 'string' && value !== '' && field.validationPattern !== '') {
    try {
      if (!new RegExp(field.validationPattern).test(value)) {
        return `${field.label} contains an unsupported value.`
      }
    } catch {
      return `${field.label} validation is unavailable.`
    }
  }
  if (field.type === GameOperationFieldType.INTEGER && value !== '') {
    const numericValue = Number(value)
    if (!Number.isInteger(numericValue)) return `${field.label} must be a whole number.`
    if (field.minValue !== undefined && numericValue < field.minValue) {
      return `${field.label} must be ${field.minValue} or greater.`
    }
    if (field.maxValue !== undefined && numericValue > field.maxValue) {
      return `${field.label} must be ${field.maxValue} or less.`
    }
  }
  return ''
}

function operationValues(
  operation: GameOperationDescriptor,
  supplied: Record<string, OperationValue>,
) {
  const values: Record<string, OperationValue> = {}
  for (const field of operation.fields) {
    values[field.id] =
      supplied[field.id] ??
      (field.type === GameOperationFieldType.BOOLEAN
        ? field.defaultValue === 'true'
        : field.defaultValue || field.options[0]?.value || '')
  }
  return values
}

function requestOperation(
  operationID: string,
  supplied: Record<string, OperationValue> = {},
  forceConfirmation = false,
) {
  const operation = operationByID.value.get(operationID)
  if (!operation || !operation.available || isExecuting(operationID)) return
  const values = operationValues(operation, supplied)
  const error = operation.fields.map((field) => fieldError(field, values[field.id])).find(Boolean)
  if (error) {
    latestResult.value = null
    formError.value = error
    return
  }
  formError.value = ''
  if (forceConfirmation || operation.risk !== GameOperationRisk.ROUTINE) {
    pendingOperation.value = { operation, values }
    return
  }
  void executeOperation(operation, values)
}

function playerOperation(operationID: string) {
  requestOperation(operationID, {
    player: playerIdentity.value.trim(),
    permission_level: administratorPermissionLevel.value,
    reason: moderationReason.value.trim(),
  })
}

function playerAssistanceOperation(operationID: string) {
  requestOperation(operationID, {
    player: playerIdentity.value.trim(),
    destination: destinationPlayerIdentity.value.trim(),
    item: itemName.value.trim(),
    amount: itemAmount.value,
    experience: experienceAmount.value,
    buff: buffName.value.trim(),
  })
}

function commandPermissionOperation(operationID: string) {
  requestOperation(operationID, {
    command: commandName.value.trim(),
    permission_level: commandPermissionLevel.value,
  })
}

function communicationOperation(operationID: string) {
  requestOperation(operationID, {
    player: playerIdentity.value.trim(),
    message: message.value.trim(),
  })
}

function worldOperation(operationID: string, values: Record<string, OperationValue> = {}) {
  requestOperation(operationID, values)
}

function exactWorldTimeReady() {
  const day = Number(exactWorldDay.value)
  const hour = Number(exactWorldHour.value)
  const minute = Number(exactWorldMinute.value)
  return (
    Number.isInteger(day) &&
    day >= 1 &&
    Number.isInteger(hour) &&
    hour >= 0 &&
    hour <= 23 &&
    Number.isInteger(minute) &&
    minute >= 0 &&
    minute <= 59
  )
}

function setExactWorldTime() {
  worldOperation('server_control.set_game_time', {
    time: `${exactWorldDay.value} ${exactWorldHour.value} ${exactWorldMinute.value}`,
  })
}

function displayValue(field: GameOperationField, value: OperationValue | undefined) {
  if (value === undefined || value === '') return ''
  const option = field.options.find((candidate) => candidate.value === String(value))
  if (field.type === GameOperationFieldType.PLAYER_IDENTITY && option) {
    return `${option.label} (${option.value})`
  }
  return option?.label ?? String(value)
}

function typedOperationValues(
  operation: GameOperationDescriptor,
  values: Record<string, OperationValue>,
): GameOperationValue[] {
  return operation.fields.map((field) => {
    const rawValue = values[field.id]
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

async function executeOperation(
  operation: GameOperationDescriptor,
  values: Record<string, OperationValue>,
) {
  if (isExecuting(operation.id)) return
  pendingOperation.value = null
  executingOperationIDs.value = new Set(executingOperationIDs.value).add(operation.id)
  try {
    const response = await GetXylonaClient().executeGameServerOperation(
      create(ExecuteGameServerOperationRequestSchema, {
        gameServerId: gameServerID.value,
        operationId: operation.id,
        values: typedOperationValues(operation, values),
      }),
    )
    const result =
      response.result ?? failedOperationResult('The server returned no operation result.')
    latestResult.value = { operation, result }
  } catch (unknownError) {
    const error = ConnectError.from(unknownError)
    latestResult.value = {
      operation,
      result: failedOperationResult(
        `The operation could not be completed: ${ConnectErrorToString(error)}`,
      ),
    }
  } finally {
    await nextTick()
    operationResult.value?.scrollIntoView?.({ block: 'nearest' })
    const executing = new Set(executingOperationIDs.value)
    executing.delete(operation.id)
    executingOperationIDs.value = executing
  }
}

function confirmPendingOperation() {
  const pending = pendingOperation.value
  if (pending) void executeOperation(pending.operation, pending.values)
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
      return 'Command issued'
    default:
      return 'Failed'
  }
}

function resultMessage(result: GameOperationResult) {
  return result.classification === GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED
    ? 'The command was sent to the game server.'
    : result.message
}

function resultIcon(classification: GameOperationResultClassification) {
  switch (classification) {
    case GameOperationResultClassification.CONFIRMED:
      return 'check_circle'
    case GameOperationResultClassification.ACCEPTED_BUT_UNVERIFIED:
      return 'send'
    default:
      return 'error_outline'
  }
}
</script>

<template>
  <div class="operations-page xy-page-content">
    <page-header
      icon="manage_accounts"
      :subtitle="
        'Find one structured server task and execute it with confidence for ' +
        (gameServerName || 'this server') +
        '.'
      "
      title="Operations workbench" />

    <section class="server-strip" aria-label="Current server state">
      <div class="server-strip__identity">
        <span class="server-strip__icon"><q-icon aria-hidden="true" name="schedule" /></span>
        <div>
          <strong>World time</strong>
          <span>Updates automatically</span>
        </div>
      </div>
      <div class="server-strip__readout">
        <div class="server-strip__time" :aria-busy="worldStatusLoading" aria-live="polite">
          <q-spinner v-if="worldStatusLoading" color="primary" size="1.25rem" />
          <strong v-else>{{ worldTimeLabel }}</strong>
          <button
            aria-label="Refresh world time"
            class="icon-button"
            :disabled="worldStatusLoading"
            type="button"
            @click="loadWorldStatus">
            <q-icon aria-hidden="true" name="refresh" />
          </button>
        </div>
      </div>
    </section>

    <section
      v-if="availabilityNotices.length > 0 && !serverStopped"
      aria-labelledby="availability-summary-title"
      class="availability-summary"
      role="status">
      <q-icon aria-hidden="true" name="info_outline" />
      <div>
        <strong id="availability-summary-title">Some operations are currently unavailable</strong>
        <p v-for="notice in availabilityNotices" :key="notice">{{ notice }}</p>
      </div>
    </section>

    <datalist id="teleport-player-identities">
      <option
        v-for="player in teleportPlayers"
        :key="player.value"
        :label="player.label"
        :value="player.value" />
    </datalist>

    <div v-if="loading" aria-live="polite" class="operations-state">
      <q-spinner color="primary" size="2rem" />
      <span>Loading administration controls…</span>
    </div>

    <div v-else-if="loadError" class="operations-state operations-state--error" role="alert">
      <q-icon aria-hidden="true" name="error_outline" />
      <span>{{ loadError }}</span>
      <button class="action-button action-button--quiet" type="button" @click="loadOperations">
        Retry
      </button>
    </div>

    <div v-else-if="!hasCommonOperations" class="operations-state">
      <q-icon aria-hidden="true" name="visibility_off" />
      <span>No administration controls are available for your permissions.</span>
    </div>

    <section v-else-if="serverStopped" class="recovery-panel" aria-labelledby="recovery-title">
      <span class="recovery-panel__icon"><q-icon aria-hidden="true" name="play_arrow" /></span>
      <div>
        <h2 id="recovery-title">
          Start {{ gameServerName || 'this game server' }} to run operations
        </h2>
        <p>
          Operations require an authoritative running server state. Your saved workbench preferences
          remain available after startup.
        </p>
        <p v-if="startRequested" class="recovery-panel__success" role="status">
          Start requested. Open Overview to follow the lifecycle state.
        </p>
        <p v-if="startError" class="recovery-panel__error" role="alert">{{ startError }}</p>
      </div>
      <div class="recovery-panel__actions">
        <button
          v-if="canStartServer"
          class="action-button"
          data-testid="start-server"
          :disabled="startingServer || startRequested"
          type="button"
          @click="startGameServer">
          {{ startingServer ? 'Starting…' : startRequested ? 'Start requested' : 'Start server' }}
        </button>
        <router-link
          class="action-button action-button--quiet"
          :to="`/game-servers/${gameServerID}`">
          Open Overview
        </router-link>
        <p v-if="!canStartServer" class="recovery-panel__permission">
          Starting this game server requires start permission.
        </p>
      </div>
    </section>

    <div v-else class="admin-console" data-testid="operations-workbench">
      <aside class="operation-finder" aria-label="Find an operation">
        <div class="operation-finder__header">
          <h2>Find an operation</h2>
          <span
            >{{ filteredOperationPanels.length }} {{ operationSearch ? 'matches' : 'panels' }}</span
          >
        </div>
        <label class="operation-search">
          <span class="sr-only">Search operations</span>
          <q-icon aria-hidden="true" name="search" />
          <input
            v-model="operationSearch"
            data-testid="operation-search"
            placeholder="Search operations"
            type="search" />
        </label>
        <div class="category-list" aria-label="Operation categories">
          <button
            v-for="category in operationCategories"
            :key="category.id"
            :aria-pressed="activeCategory === category.id"
            class="category-button"
            :data-testid="`operation-category-${category.id}`"
            type="button"
            @click="selectCategory(category.id)">
            <span>{{ category.label }}</span>
            <code>{{ operationCategoryCount(category.id) }}</code>
          </button>
        </div>
        <h3 class="operation-finder__label">Operation panels</h3>
        <div class="operation-list">
          <button
            v-for="panel in filteredOperationPanels"
            :key="panel.id"
            :aria-current="panel.operationIDs.includes(activeOperationID) ? 'true' : undefined"
            class="operation-button"
            :data-testid="`operation-option-${panel.id}`"
            type="button"
            @click="selectOperation(panel.operationID)">
            <span>
              <strong>{{ panel.label }}</strong>
              <small>{{ panel.detail }}</small>
            </span>
            <q-icon aria-hidden="true" name="chevron_right" />
          </button>
          <p v-if="filteredOperationPanels.length === 0" class="operation-list__empty">
            No matching operations. Try a player, access, message, or world task.
          </p>
        </div>
      </aside>

      <section
        v-if="activeOperation && activeOperationPanel"
        class="active-task"
        aria-labelledby="active-operation-title">
        <header class="active-task__header">
          <div>
            <h2 id="active-operation-title">{{ activeOperationPanel.label }}</h2>
            <p>{{ activeOperationPanel.summary }}</p>
          </div>
          <span class="risk-badge">{{ riskLabel(activeOperationPanelRisk) }}</span>
        </header>

        <p
          v-if="activeOperationPanelUnavailableReason"
          class="active-task__unavailable"
          role="status">
          {{ activeOperationPanelUnavailableReason }}
        </p>
        <p v-if="formError" class="form-error" role="alert">{{ formError }}</p>

        <div v-if="activeOperationUsesPlayer" class="player-context">
          <operation-catalog-input
            v-model="playerIdentity"
            label="Player"
            :options="knownPlayers"
            placeholder="Choose a known player or paste a stable ID"
            test-id="player-identity" />
          <div
            v-if="selectedPlayer"
            class="player-identity"
            data-testid="selected-player"
            aria-live="polite">
            <span class="player-identity__avatar" aria-hidden="true">{{
              selectedPlayer.initials
            }}</span>
            <span class="player-identity__copy">
              <strong>{{ selectedPlayer.label }}</strong>
              <code>{{ selectedPlayer.value }}</code>
            </span>
            <span
              class="player-identity__status"
              :class="{
                'player-identity__status--online': selectedPlayer.online,
                'player-identity__status--manual': !selectedPlayer.known,
              }">
              <span aria-hidden="true" class="player-identity__status-dot"></span>
              {{
                selectedPlayer.known
                  ? selectedPlayer.online
                    ? 'Online'
                    : 'Offline'
                  : 'Manual identity'
              }}
            </span>
          </div>
        </div>

        <div
          v-if="playerAccessOperationIDs.some((operationID) => operationID === activeOperationID)"
          class="operation-action-grid">
          <div
            v-if="operationByID.has('player_access.add_administrator')"
            class="control-group control-group--wide">
            <div class="control-group__copy">
              <h3>Administration level</h3>
              <p>
                Choose the least access this player needs. Lower native numbers grant more
                authority.
              </p>
            </div>
            <div class="permission-options">
              <label class="permission-option">
                <input
                  v-model="administratorPermissionMode"
                  :disabled="!permissionPresetAvailable('player_access.add_administrator', 200)"
                  name="administrator-permission"
                  type="radio"
                  value="moderator" />
                <strong>Moderator</strong><em>Recommended</em>
                <span>Level 200 · moderation and player support without full server control.</span>
              </label>
              <label class="permission-option">
                <input
                  v-model="administratorPermissionMode"
                  :disabled="!permissionPresetAvailable('player_access.add_administrator', 0)"
                  name="administrator-permission"
                  type="radio"
                  value="administrator" />
                <strong>Full administrator</strong>
                <span>Level 0 · unrestricted access to supported administration commands.</span>
              </label>
            </div>
            <details class="advanced-permission" :open="administratorPermissionMode === 'custom'">
              <summary @click.prevent="administratorPermissionMode = 'custom'">
                Advanced custom numeric
              </summary>
              <label class="compact-field">
                <span>Native permission level</span>
                <input
                  v-model.number="administratorCustomPermissionLevel"
                  data-testid="administrator-permission-level"
                  :max="permissionMaximum('player_access.add_administrator')"
                  :min="permissionMinimum('player_access.add_administrator')"
                  step="1"
                  type="number" />
              </label>
            </details>
            <div class="form-actions">
              <button
                class="action-button"
                data-testid="add-administrator"
                :disabled="
                  operationDisabled('player_access.add_administrator', playerIdentity.trim() !== '')
                "
                type="button"
                @click="playerOperation('player_access.add_administrator')">
                {{ isExecuting('player_access.add_administrator') ? 'Adding…' : 'Review change' }}
              </button>
            </div>
          </div>

          <div v-if="operationByID.has('player_access.remove_administrator')" class="control-group">
            <div class="control-group__copy">
              <h3>Administrator access</h3>
              <p>Remove this player's administrator entry without changing allowlist access.</p>
            </div>
            <div class="form-actions">
              <button
                class="action-button action-button--warning"
                data-testid="remove-administrator"
                :disabled="
                  operationDisabled(
                    'player_access.remove_administrator',
                    playerIdentity.trim() !== '',
                  )
                "
                type="button"
                @click="playerOperation('player_access.remove_administrator')">
                {{
                  isExecuting('player_access.remove_administrator') ? 'Removing…' : 'Review removal'
                }}
              </button>
            </div>
          </div>

          <div
            v-if="
              operationByID.has('player_access.allowlist_add') ||
              operationByID.has('player_access.allowlist_remove')
            "
            class="control-group">
            <div class="control-group__copy">
              <h3>Allowlist access</h3>
              <p>Control whether this player may join an allowlist-only server.</p>
            </div>
            <div class="button-cluster">
              <button
                v-if="operationByID.has('player_access.allowlist_add')"
                class="action-button"
                data-testid="allowlist-add"
                :disabled="
                  operationDisabled('player_access.allowlist_add', playerIdentity.trim() !== '')
                "
                type="button"
                @click="playerOperation('player_access.allowlist_add')">
                {{
                  isExecuting('player_access.allowlist_add')
                    ? 'Adding…'
                    : operationByID.get('player_access.allowlist_add')?.name
                }}
              </button>
              <button
                v-if="operationByID.has('player_access.allowlist_remove')"
                class="action-button action-button--warning"
                data-testid="allowlist-remove"
                :disabled="
                  operationDisabled('player_access.allowlist_remove', playerIdentity.trim() !== '')
                "
                type="button"
                @click="playerOperation('player_access.allowlist_remove')">
                {{
                  isExecuting('player_access.allowlist_remove')
                    ? 'Removing…'
                    : operationByID.get('player_access.allowlist_remove')?.name
                }}
              </button>
            </div>
          </div>
        </div>

        <div
          v-else-if="
            ['player_moderation.kick', 'player_moderation.ban', 'player_moderation.unban'].includes(
              activeOperationID,
            )
          "
          class="control-group">
          <label
            v-if="activeOperationID !== 'player_moderation.unban'"
            class="control-field control-field--flush">
            <span>Reason</span>
            <input
              v-model="moderationReason"
              autocomplete="off"
              placeholder="Optional"
              type="text" />
          </label>
          <div class="form-actions">
            <button
              class="action-button"
              :class="
                activeOperationID === 'player_moderation.ban'
                  ? 'action-button--danger'
                  : 'action-button--warning'
              "
              :data-testid="
                activeOperationID === 'player_moderation.ban'
                  ? 'ban-player'
                  : activeOperationID === 'player_moderation.unban'
                    ? 'unban-player'
                    : undefined
              "
              :disabled="operationDisabled(activeOperationID, playerIdentity.trim() !== '')"
              type="button"
              @click="playerOperation(activeOperationID)">
              {{ isExecuting(activeOperationID) ? 'Submitting…' : activeOperation.name }}
            </button>
          </div>
        </div>

        <div
          v-else-if="
            playerAssistanceOperationIDs.some((operationID) => operationID === activeOperationID)
          "
          class="operation-action-grid">
          <div
            v-if="operationByID.has('player_assistance.teleport_to_player')"
            class="control-group">
            <h3>Teleport</h3>
            <label class="compact-field">
              <span>Destination player</span>
              <input
                v-model="destinationPlayerIdentity"
                autocomplete="off"
                data-testid="teleport-destination"
                list="teleport-player-identities"
                placeholder="Choose an online player"
                spellcheck="false"
                type="text" />
              <small v-if="destinationPlayerIsKnownOffline" class="compact-field__hint"
                >This saved player is offline.</small
              >
            </label>
            <div class="form-actions">
              <button
                class="action-button"
                data-testid="teleport-player"
                :disabled="
                  operationDisabled(
                    'player_assistance.teleport_to_player',
                    playerIdentity.trim() !== '' &&
                      destinationPlayerIdentity.trim() !== '' &&
                      !destinationPlayerIsKnownOffline &&
                      playerIdentity.trim() !== destinationPlayerIdentity.trim(),
                  )
                "
                type="button"
                @click="playerAssistanceOperation('player_assistance.teleport_to_player')">
                {{
                  isExecuting('player_assistance.teleport_to_player')
                    ? 'Teleporting…'
                    : 'Review teleport'
                }}
              </button>
            </div>
          </div>

          <div v-if="operationByID.has('player_assistance.give_experience')" class="control-group">
            <h3>Experience</h3>
            <label class="compact-field">
              <span>Amount</span>
              <input
                v-model.number="experienceAmount"
                data-testid="experience-amount"
                max="1000000"
                min="1"
                step="1"
                type="number" />
            </label>
            <div class="form-actions">
              <button
                class="action-button"
                data-testid="give-experience"
                :disabled="
                  operationDisabled(
                    'player_assistance.give_experience',
                    playerIdentity.trim() !== '',
                  )
                "
                type="button"
                @click="playerAssistanceOperation('player_assistance.give_experience')">
                {{
                  isExecuting('player_assistance.give_experience')
                    ? 'Giving XP…'
                    : 'Review XP grant'
                }}
              </button>
            </div>
          </div>

          <div
            v-if="operationByID.has('player_assistance.give_item')"
            class="control-group control-group--wide">
            <h3>Item grant</h3>
            <div class="assist-fields">
              <operation-catalog-input
                v-model="itemName"
                label="Item"
                :options="itemOptions"
                placeholder="Search server items or enter an exact name"
                test-id="item-name" />
              <label class="compact-field compact-field--amount">
                <span>Amount</span>
                <input
                  v-model.number="itemAmount"
                  data-testid="item-amount"
                  max="1000"
                  min="1"
                  step="1"
                  type="number" />
              </label>
            </div>
            <div
              v-if="selectedItem"
              class="catalog-selection"
              data-testid="selected-item"
              aria-live="polite">
              <span class="catalog-selection__media">
                <img
                  v-if="selectedItem.iconUrl"
                  alt=""
                  height="56"
                  :src="selectedItem.iconUrl"
                  width="56"
                  @error="hideBrokenImage" />
                <q-icon v-else aria-hidden="true" name="inventory_2" />
              </span>
              <span class="catalog-selection__copy">
                <strong>{{ selectedItem.label }}</strong
                ><code>{{ selectedItem.value }}</code>
                <span v-if="selectedItem.category" class="catalog-selection__category">{{
                  selectedItem.category
                }}</span>
              </span>
              <span
                class="catalog-selection__status"
                :class="{ 'catalog-selection__status--known': selectedItem.known }">
                <span aria-hidden="true" class="catalog-selection__status-dot"></span>
                {{ selectedItem.known ? 'Server catalog' : 'Manual exact name' }}
              </span>
            </div>
            <div class="form-actions">
              <button
                class="action-button"
                data-testid="give-item"
                :disabled="
                  operationDisabled(
                    'player_assistance.give_item',
                    playerIdentity.trim() !== '' && itemName.trim() !== '',
                  )
                "
                type="button"
                @click="playerAssistanceOperation('player_assistance.give_item')">
                {{
                  isExecuting('player_assistance.give_item') ? 'Giving item…' : 'Review item grant'
                }}
              </button>
            </div>
          </div>

          <div
            v-if="
              operationByID.has('player_assistance.apply_buff') ||
              operationByID.has('player_assistance.remove_buff')
            "
            class="control-group control-group--wide">
            <h3>Buffs and debuffs</h3>
            <operation-catalog-input
              v-model="buffName"
              label="Buff or debuff"
              :options="buffOptions"
              placeholder="Search server effects or enter an exact name"
              test-id="buff-name" />
            <div class="button-cluster">
              <button
                v-if="operationByID.has('player_assistance.apply_buff')"
                class="action-button"
                data-testid="apply-buff"
                :disabled="
                  operationDisabled(
                    'player_assistance.apply_buff',
                    playerIdentity.trim() !== '' && buffName.trim() !== '',
                  )
                "
                type="button"
                @click="playerAssistanceOperation('player_assistance.apply_buff')">
                {{
                  isExecuting('player_assistance.apply_buff')
                    ? 'Applying…'
                    : operationByID.get('player_assistance.apply_buff')?.name
                }}
              </button>
              <button
                v-if="operationByID.has('player_assistance.remove_buff')"
                class="action-button action-button--warning"
                data-testid="remove-buff"
                :disabled="
                  operationDisabled(
                    'player_assistance.remove_buff',
                    playerIdentity.trim() !== '' && buffName.trim() !== '',
                  )
                "
                type="button"
                @click="playerAssistanceOperation('player_assistance.remove_buff')">
                {{
                  isExecuting('player_assistance.remove_buff')
                    ? 'Removing…'
                    : operationByID.get('player_assistance.remove_buff')?.name
                }}
              </button>
            </div>
          </div>
        </div>

        <div
          v-else-if="
            commandPermissionOperationIDs.some((operationID) => operationID === activeOperationID)
          "
          class="control-group">
          <h3>Command permission</h3>
          <operation-catalog-input
            v-model="commandName"
            label="Command"
            :options="commandOptions"
            placeholder="Search known commands or enter an exact name"
            test-id="command-name" />
          <template v-if="operationByID.has('permissions.set_command_permission')">
            <div class="permission-options">
              <label class="permission-option">
                <input
                  v-model="commandPermissionMode"
                  :disabled="!permissionPresetAvailable('permissions.set_command_permission', 200)"
                  name="command-permission"
                  type="radio"
                  value="moderator" />
                <strong>Moderator</strong><em>Recommended</em>
                <span>Level 200 · suitable for routine moderation commands.</span>
              </label>
              <label class="permission-option">
                <input
                  v-model="commandPermissionMode"
                  :disabled="!permissionPresetAvailable('permissions.set_command_permission', 0)"
                  name="command-permission"
                  type="radio"
                  value="administrator" />
                <strong>Full administrator</strong>
                <span>Level 0 · exposes this command to full administrators.</span>
              </label>
            </div>
            <details class="advanced-permission" :open="commandPermissionMode === 'custom'">
              <summary @click.prevent="commandPermissionMode = 'custom'">
                Advanced custom numeric
              </summary>
              <label class="compact-field">
                <span>Native permission level</span>
                <input
                  v-model.number="commandCustomPermissionLevel"
                  data-testid="command-permission-level"
                  :max="permissionMaximum('permissions.set_command_permission')"
                  :min="permissionMinimum('permissions.set_command_permission')"
                  step="1"
                  type="number" />
              </label>
            </details>
          </template>
          <div class="button-cluster">
            <button
              v-if="operationByID.has('permissions.set_command_permission')"
              class="action-button"
              data-testid="set-command-permission"
              :disabled="
                operationDisabled('permissions.set_command_permission', commandName.trim() !== '')
              "
              type="button"
              @click="commandPermissionOperation('permissions.set_command_permission')">
              {{
                isExecuting('permissions.set_command_permission')
                  ? 'Setting…'
                  : operationByID.get('permissions.set_command_permission')?.name
              }}
            </button>
            <button
              v-if="operationByID.has('permissions.reset_command_permission')"
              class="action-button action-button--warning"
              data-testid="reset-command-permission"
              :disabled="
                operationDisabled('permissions.reset_command_permission', commandName.trim() !== '')
              "
              type="button"
              @click="commandPermissionOperation('permissions.reset_command_permission')">
              {{
                isExecuting('permissions.reset_command_permission')
                  ? 'Resetting…'
                  : operationByID.get('permissions.reset_command_permission')?.name
              }}
            </button>
          </div>
        </div>

        <div
          v-else-if="
            communicationOperationIDs.some((operationID) => operationID === activeOperationID)
          "
          class="control-group">
          <h3>Message</h3>
          <label class="control-field control-field--flush">
            <span>Message</span>
            <textarea
              v-model="message"
              data-testid="operation-message"
              maxlength="1024"
              placeholder="Write a server message"
              rows="3" />
          </label>
          <div class="button-cluster">
            <button
              v-if="operationByID.has('communication.message_player')"
              class="action-button"
              data-testid="message-player"
              :disabled="
                operationDisabled(
                  'communication.message_player',
                  message.trim() !== '' && playerIdentity.trim() !== '',
                )
              "
              type="button"
              @click="communicationOperation('communication.message_player')">
              {{
                isExecuting('communication.message_player')
                  ? 'Sending…'
                  : operationByID.get('communication.message_player')?.name
              }}
            </button>
            <button
              v-if="operationByID.has('communication.broadcast_message')"
              class="action-button action-button--warning"
              data-testid="broadcast-message"
              :disabled="
                operationDisabled('communication.broadcast_message', message.trim() !== '')
              "
              type="button"
              @click="communicationOperation('communication.broadcast_message')">
              {{
                isExecuting('communication.broadcast_message')
                  ? 'Sending…'
                  : operationByID.get('communication.broadcast_message')?.name
              }}
            </button>
          </div>
        </div>

        <div
          v-else-if="
            worldControlOperationIDs.some((operationID) => operationID === activeOperationID)
          "
          class="operation-action-grid">
          <div v-if="operationByID.has('world_events.set_weather')" class="control-group">
            <h3>Weather</h3>
            <label class="compact-field">
              <span>Preset</span>
              <select v-model="weatherPreset" data-testid="weather-preset">
                <option value="natural">Natural</option>
                <option value="rain">Rain</option>
                <option value="snow">Snow</option>
              </select>
            </label>
            <div class="form-actions">
              <button
                class="action-button"
                data-testid="set-weather"
                :disabled="operationDisabled('world_events.set_weather')"
                type="button"
                @click="worldOperation('world_events.set_weather', { weather: weatherPreset })">
                {{
                  isExecuting('world_events.set_weather') ? 'Changing…' : 'Review weather change'
                }}
              </button>
            </div>
          </div>

          <div v-if="operationByID.has('server_control.save_world')" class="control-group">
            <div class="control-group__copy">
              <h3>World save</h3>
              <p>Request an immediate world save without stopping the server.</p>
            </div>
            <div class="form-actions">
              <button
                class="action-button"
                data-testid="save-world"
                :disabled="operationDisabled('server_control.save_world')"
                type="button"
                @click="worldOperation('server_control.save_world')">
                {{ isExecuting('server_control.save_world') ? 'Saving…' : 'Save now' }}
              </button>
            </div>
          </div>

          <div
            v-if="operationByID.has('server_control.set_game_time')"
            class="control-group control-group--wide">
            <div class="control-group__copy">
              <h3>World time</h3>
              <p>Use a shortcut or set a precise day and clock value.</p>
            </div>
            <div class="button-cluster">
              <button
                class="action-button action-button--quiet"
                data-testid="set-day"
                :disabled="operationDisabled('server_control.set_game_time')"
                type="button"
                @click="worldOperation('server_control.set_game_time', { time: 'day' })">
                Set day
              </button>
              <button
                class="action-button action-button--quiet"
                data-testid="set-night"
                :disabled="operationDisabled('server_control.set_game_time')"
                type="button"
                @click="worldOperation('server_control.set_game_time', { time: 'night' })">
                Set night
              </button>
            </div>
            <div class="exact-time-fields">
              <label class="compact-field"
                ><span>Day</span
                ><input
                  v-model.number="exactWorldDay"
                  data-testid="exact-world-day"
                  min="1"
                  step="1"
                  type="number"
              /></label>
              <label class="compact-field"
                ><span>Hour</span
                ><input
                  v-model.number="exactWorldHour"
                  data-testid="exact-world-hour"
                  max="23"
                  min="0"
                  step="1"
                  type="number"
              /></label>
              <label class="compact-field"
                ><span>Minute</span
                ><input
                  v-model.number="exactWorldMinute"
                  data-testid="exact-world-minute"
                  max="59"
                  min="0"
                  step="1"
                  type="number"
              /></label>
              <button
                class="action-button"
                data-testid="set-exact-world-time"
                :disabled="operationDisabled('server_control.set_game_time', exactWorldTimeReady())"
                type="button"
                @click="setExactWorldTime">
                {{ isExecuting('server_control.set_game_time') ? 'Setting…' : 'Review exact time' }}
              </button>
            </div>
          </div>

          <div
            v-if="operationByID.has('server_control.set_temperature_unit')"
            class="control-group control-group--wide">
            <h3>Temperature display</h3>
            <div class="assistance-form">
              <label class="compact-field"
                ><span>Unit</span
                ><select v-model="temperatureUnit">
                  <option value="F">Fahrenheit</option>
                  <option value="C">Celsius</option>
                </select></label
              >
              <button
                class="action-button"
                data-testid="set-temperature-unit"
                :disabled="operationDisabled('server_control.set_temperature_unit')"
                type="button"
                @click="
                  worldOperation('server_control.set_temperature_unit', { unit: temperatureUnit })
                ">
                {{ isExecuting('server_control.set_temperature_unit') ? 'Applying…' : 'Apply' }}
              </button>
            </div>
          </div>
        </div>

        <div
          v-else-if="
            ['world_events.spawn_airdrop', 'world_events.spawn_wandering_horde'].includes(
              activeOperationID,
            )
          "
          class="control-group">
          <p>The active world decides where this event unfolds.</p>
          <div class="button-cluster">
            <button
              v-for="operation in spawnEventOperations"
              :key="operation.id"
              class="action-button action-button--warning"
              :data-testid="
                operation.id === 'world_events.spawn_airdrop'
                  ? 'spawn-airdrop'
                  : 'spawn-wandering-horde'
              "
              :disabled="operationDisabled(operation.id)"
              type="button"
              @click="worldOperation(operation.id)">
              {{ isExecuting(operation.id) ? 'Spawning…' : operation.name }}
            </button>
          </div>
        </div>
      </section>
    </div>

    <section
      v-if="latestResult"
      ref="operationResult"
      class="operation-result"
      :class="
        'operation-result--' +
        resultLabel(latestResult.result.classification)
          .toLowerCase()
          .replaceAll(/[^a-z]+/g, '-')
      "
      data-testid="operation-result"
      :role="
        latestResult.result.classification === GameOperationResultClassification.FAILED
          ? 'alert'
          : 'status'
      ">
      <q-icon aria-hidden="true" :name="resultIcon(latestResult.result.classification)" />
      <div>
        <strong>
          {{ latestResult.operation.name }} — {{ resultLabel(latestResult.result.classification) }}
        </strong>
        <p>{{ resultMessage(latestResult.result) }}</p>
      </div>
      <button
        aria-label="Dismiss operation result"
        class="icon-button operation-result__dismiss"
        type="button"
        @click="latestResult = null">
        <q-icon aria-hidden="true" name="close" />
      </button>
    </section>

    <q-dialog v-model="confirmationOpen" aria-labelledby="operation-confirmation-title">
      <q-card v-if="pendingOperation" class="confirmation-dialog">
        <q-card-section>
          <h2 id="operation-confirmation-title">
            {{ pendingOperation.operation.review?.title || pendingOperation.operation.name }}
          </h2>
          <p>
            {{ pendingOperation.operation.review?.effect || pendingOperation.operation.summary }}
          </p>
          <dl v-if="pendingValues.length > 0" class="confirmation-values">
            <template v-for="entry in pendingValues" :key="entry.label">
              <dt>{{ entry.label }}</dt>
              <dd>{{ entry.value }}</dd>
            </template>
          </dl>
          <p v-if="pendingOperation.operation.review?.caution" class="confirmation-caution">
            <q-icon aria-hidden="true" name="warning" />
            {{ pendingOperation.operation.review.caution }}
          </p>
        </q-card-section>
        <q-card-actions class="confirmation-actions" align="right">
          <button
            class="action-button action-button--quiet"
            type="button"
            @click="confirmationOpen = false">
            Cancel
          </button>
          <button
            class="action-button"
            :class="
              pendingOperation.operation.risk === GameOperationRisk.IRREVERSIBLE
                ? 'action-button--danger'
                : 'action-button--warning'
            "
            data-testid="confirm-operation"
            type="button"
            @click="confirmPendingOperation">
            {{ pendingOperation.operation.name }}
          </button>
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<style scoped>
.operations-page {
  --control-height: 2.75rem;
  color: var(--xy-text-primary);
}

.operations-page ::selection {
  color: var(--xy-text-primary);
  background: var(--xy-primary-muted);
}

.server-strip {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-lg);
  min-height: 4.25rem;
  padding: var(--xy-space-base) var(--xy-space-md);
  margin-bottom: var(--xy-space-lg);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.server-strip__identity,
.server-strip__readout {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
}

.server-strip__readout {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.server-strip__time,
.server-strip__actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.server-strip__actions {
  flex-wrap: wrap;
  justify-content: flex-end;
}

.server-strip__identity > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.server-strip__identity span,
.control-panel__header p,
.control-group__copy p {
  color: var(--xy-text-muted);
}

.server-strip__icon {
  display: grid;
  width: 2.5rem;
  height: 2.5rem;
  place-items: center;
  color: var(--xy-accent-hover);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-md);
}

.server-strip__time > strong {
  font-family: var(--xy-font-mono);
  font-variant-numeric: tabular-nums;
}

.availability-summary {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-base);
  padding: var(--xy-space-base) var(--xy-space-md);
  margin: calc(var(--xy-space-lg) * -0.5) 0 var(--xy-space-lg);
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
}

.availability-summary > .q-icon {
  margin-top: 0.125rem;
  font-size: 1.25rem;
}

.availability-summary p {
  margin: var(--xy-space-2xs) 0 0;
}

.admin-console {
  display: grid;
  grid-template-columns: minmax(15rem, 18rem) minmax(28rem, 1fr);
  align-items: start;
  min-height: 36rem;
  overflow: clip;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.operation-finder {
  min-width: 0;
  height: 100%;
  padding: var(--xy-space-base);
  background: var(--xy-surface-1);
  border-right: 1px solid var(--xy-border);
}

.operation-finder__header,
.active-task__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-base);
}

.operation-finder__header h2,
.active-task__header h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  line-height: var(--xy-line-height-tight);
}

.operation-finder__header > span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  white-space: nowrap;
}

.operation-search {
  position: relative;
  display: block;
  margin: var(--xy-space-sm) 0 var(--xy-space-base);
}

.operation-search > .q-icon {
  position: absolute;
  top: 50%;
  left: var(--xy-space-sm);
  color: var(--xy-text-muted);
  transform: translateY(-50%);
  pointer-events: none;
}

.operation-search input {
  width: 100%;
  min-height: var(--control-height);
  padding: 0 var(--xy-space-sm) 0 calc(var(--xy-space-xl) + var(--xy-space-xs));
  color: var(--xy-text-primary);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.operation-search input::placeholder {
  color: var(--xy-text-muted);
  opacity: 1;
}

.category-list,
.operation-list {
  display: grid;
  gap: var(--xy-space-2xs);
}

.category-button,
.operation-button {
  display: flex;
  width: 100%;
  min-height: var(--control-height);
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
}

.category-button:hover,
.operation-button:hover,
.operation-button[aria-current='true'] {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
}

.category-button[aria-pressed='true'] {
  color: var(--xy-text-primary);
  background: var(--xy-primary-muted);
  border-color: var(--xy-border-active);
}

.category-button code {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.operation-finder__label {
  margin: var(--xy-space-md) 0 var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-2xs);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.operation-button {
  min-height: 3.25rem;
}

.operation-button > span {
  display: grid;
  min-width: 0;
}

.operation-button strong,
.operation-button small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.operation-button small {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.operation-list__empty {
  padding: var(--xy-space-base) var(--xy-space-sm);
  margin: 0;
  color: var(--xy-text-muted);
}

.active-task {
  min-width: 0;
  padding: var(--xy-space-md);
}

.active-task__header {
  padding-bottom: var(--xy-space-base);
  border-bottom: 1px solid var(--xy-border);
}

.active-task__header p {
  max-width: 70ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-muted);
}

.risk-badge {
  flex: 0 0 auto;
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-warning-hover);
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-pill);
}

.active-task__unavailable {
  padding: var(--xy-space-base);
  margin: var(--xy-space-base) 0 0;
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
}

.active-task > .control-group {
  padding-inline: 0;
  border-top: 0;
}

.control-field--flush {
  margin: 0;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: var(--xy-space-sm);
  padding-top: var(--xy-space-base);
}

.permission-options {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-sm);
}

.permission-option {
  position: relative;
  display: grid;
  gap: var(--xy-space-2xs);
  min-height: 5.5rem;
  padding: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
  cursor: pointer;
}

.permission-option:has(input:checked) {
  color: var(--xy-text-primary);
  background: var(--xy-primary-muted);
  border-color: var(--xy-border-active);
}

.permission-option:has(input:disabled) {
  cursor: not-allowed;
  opacity: 0.5;
}

.permission-option input {
  position: absolute;
  opacity: 0;
}

.permission-option strong {
  padding-right: 5.5rem;
}

.permission-option em {
  position: absolute;
  top: var(--xy-space-sm);
  right: var(--xy-space-sm);
  color: var(--xy-accent-hover);
  font-size: var(--xy-font-size-2xs);
  font-style: normal;
  font-weight: 700;
  text-transform: uppercase;
}

.permission-option span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.advanced-permission {
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.advanced-permission summary {
  display: flex;
  min-height: var(--control-height);
  align-items: center;
  padding: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  cursor: pointer;
}

.advanced-permission .compact-field {
  padding: 0 var(--xy-space-sm) var(--xy-space-sm);
}

.recovery-panel {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: var(--xy-space-base);
  align-items: center;
  padding: var(--xy-space-md);
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-lg);
}

.recovery-panel__icon {
  display: grid;
  width: var(--control-height);
  height: var(--control-height);
  place-items: center;
  background: var(--xy-warning-bg-soft);
  border-radius: var(--xy-radius-md);
}

.recovery-panel h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  line-height: var(--xy-line-height-tight);
}

.recovery-panel p {
  max-width: 70ch;
  margin: var(--xy-space-xs) 0 0;
}

.recovery-panel__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  justify-content: flex-end;
}

.recovery-panel__permission {
  flex-basis: 100%;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  text-align: right;
}

.recovery-panel__success {
  color: var(--xy-success-hover);
}

.recovery-panel__error {
  color: var(--xy-danger-hover);
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

.control-panel--players {
  overflow: clip;
}

.player-context {
  position: sticky;
  top: var(--xy-header-stack-height, var(--xy-toolbar-height));
  z-index: var(--xy-z-sticky);
  display: grid;
  grid-template-columns: minmax(16rem, 1fr) minmax(18rem, auto);
  gap: var(--xy-space-base);
  align-items: end;
  padding: var(--xy-space-base) var(--xy-space-md);
  background: var(--xy-surface-raised-subtle);
  border-bottom: 1px solid var(--xy-border);
  box-shadow: var(--xy-shadow-sticky-lg);
}

.player-identity {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: var(--xy-space-sm);
  align-items: center;
  min-height: var(--control-height);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border-active);
  border-radius: var(--xy-radius-md);
}

.player-identity__avatar {
  display: grid;
  width: 2rem;
  height: 2rem;
  place-items: center;
  color: var(--xy-accent-hover);
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  background: var(--xy-accent-muted);
  border: 1px solid var(--xy-accent-border-soft);
  border-radius: var(--xy-radius-md);
}

.player-identity__copy {
  display: grid;
  min-width: 0;
}

.player-identity__copy strong,
.player-identity__copy code {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.player-identity__copy code {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.player-identity__status,
.catalog-selection__status {
  display: inline-flex;
  gap: var(--xy-space-xs);
  align-items: center;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  white-space: nowrap;
}

.player-identity__status-dot,
.catalog-selection__status-dot {
  width: 0.45rem;
  height: 0.45rem;
  background: var(--xy-text-muted);
  border-radius: var(--xy-radius-pill);
}

.player-identity__status--online,
.catalog-selection__status--known {
  color: var(--xy-success-text-soft);
}

.player-identity__status--online .player-identity__status-dot,
.catalog-selection__status--known .catalog-selection__status-dot {
  background: var(--xy-success);
}

.player-identity__status--manual {
  color: var(--xy-warning-hover);
}

.player-identity__status--manual .player-identity__status-dot {
  background: var(--xy-warning);
}

.operation-action-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  background: var(--xy-border);
  border-top: 1px solid var(--xy-border);
}

.operation-action-grid > .control-group {
  background: var(--xy-surface-1);
  border-top: 0;
}

.operation-action-grid > .control-group:only-child {
  grid-column: 1 / -1;
}

.operation-action-grid .control-row {
  grid-template-columns: 1fr;
}

.operation-action-grid .control-group--moderation {
  grid-column: 1 / -1;
}

.operation-action-grid .control-group--wide {
  grid-column: 1 / -1;
}

.operation-action-grid .control-group--moderation .control-row {
  grid-template-columns: minmax(12rem, 1fr) auto;
}

.assistance-form,
.exact-time-fields {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: var(--xy-space-base);
  align-items: end;
}

.assistance-form + .assistance-form {
  padding-top: var(--xy-space-base);
  border-top: 1px solid var(--xy-border);
}

.assistance-form--catalog {
  grid-template-columns: minmax(0, 1fr);
}

.assistance-form--catalog > .action-button {
  justify-self: start;
}

.catalog-selection {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  gap: var(--xy-space-base);
  align-items: center;
  min-height: 4.5rem;
  padding: var(--xy-space-sm) var(--xy-space-base);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.catalog-selection__media {
  display: grid;
  width: 3.5rem;
  height: 3.5rem;
  place-items: center;
  color: var(--xy-accent-hover);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-md);
}

.catalog-selection__media img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.catalog-selection__media .q-icon {
  font-size: 1.75rem;
}

.catalog-selection__copy {
  display: grid;
  min-width: 0;
  gap: var(--xy-space-2xs);
}

.catalog-selection__copy strong,
.catalog-selection__copy code {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.catalog-selection__copy code {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.catalog-selection__category {
  width: fit-content;
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-2xs);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-pill);
}

.assist-fields {
  display: grid;
  grid-template-columns: minmax(12rem, 1fr) 6rem;
  gap: var(--xy-space-sm);
}

.exact-time-fields {
  grid-template-columns: repeat(3, minmax(4.5rem, 1fr)) auto;
}

.control-panel {
  overflow: hidden;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.control-panel__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-lg);
  padding: var(--xy-space-md);
  background: var(--xy-surface-2);
  border-bottom: 1px solid var(--xy-border);
}

.control-panel__header h2,
.control-group h3,
.confirmation-dialog h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
}

.control-panel__header h2 {
  font-size: var(--xy-font-size-lg);
}

.control-panel__header p,
.control-group__copy p {
  margin: var(--xy-space-xs) 0 0;
}

.control-panel__header > .q-icon {
  color: var(--xy-accent-hover);
  font-size: 1.5rem;
}

.control-field,
.compact-field {
  display: grid;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.control-field {
  margin: var(--xy-space-md);
}

.control-field > span,
.compact-field > span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  font-weight: 700;
}

.compact-field__hint {
  color: var(--xy-warning);
  font-size: var(--xy-font-size-xs);
}

.control-field input,
.control-field textarea,
.compact-field input,
.compact-field select {
  width: 100%;
  min-height: var(--control-height);
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-primary);
  color-scheme: dark;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.control-field textarea {
  min-height: 5.5rem;
  padding-block: var(--xy-space-sm);
  resize: vertical;
}

.control-field input::placeholder,
.control-field textarea::placeholder,
.compact-field input::placeholder {
  color: var(--xy-text-muted);
  opacity: 1;
}

.control-field input:focus,
.control-field textarea:focus,
.compact-field input:focus,
.compact-field select:focus,
.operation-search input:focus,
.category-button:focus-visible,
.operation-button:focus-visible,
.advanced-permission summary:focus-visible,
.action-button:focus-visible,
.icon-button:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 2px;
}

.control-group {
  display: grid;
  gap: var(--xy-space-base);
  padding: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.control-group h3 {
  font-size: var(--xy-font-size-base);
}

.control-row,
.paired-fields {
  display: grid;
  grid-template-columns: minmax(8rem, 0.8fr) minmax(15rem, 1.2fr);
  gap: var(--xy-space-base);
  align-items: end;
}

.paired-fields {
  padding: var(--xy-space-md);
  grid-template-columns: minmax(0, 1fr) 8rem;
}

.paired-fields .control-field {
  margin: 0;
}

.operations-page :deep(.q-field--outlined .q-field__control) {
  min-height: var(--control-height);
  color: var(--xy-text-primary);
  background: var(--xy-surface-0);
  border-radius: var(--xy-radius-md);
}

.operations-page :deep(.q-field--outlined .q-field__control::before) {
  border-color: var(--xy-border);
}

.operations-page :deep(.q-field--outlined.q-field--focused .q-field__control::after) {
  border-color: var(--xy-focus-ring);
}

.operations-page :deep(.q-field__label),
.operations-page :deep(.q-field__native),
.operations-page :deep(.q-field__input) {
  color: var(--xy-text-primary);
}

.compact-field--wide {
  min-width: 12rem;
}

.button-cluster {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  align-items: center;
}

.button-cluster--end {
  justify-content: flex-end;
  padding: 0 var(--xy-space-md) var(--xy-space-md);
}

.action-button {
  min-height: var(--control-height);
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-on-bright);
  font-weight: 700;
  background: var(--xy-primary);
  border: 1px solid transparent;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  text-decoration: none;
  transition:
    background var(--xy-transition-fast),
    border-color var(--xy-transition-fast),
    color var(--xy-transition-fast);
}

.action-button:hover:not(:disabled) {
  background: var(--xy-primary-hover);
}

.action-button--quiet {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  border-color: var(--xy-border);
}

.action-button--quiet:hover:not(:disabled) {
  background: var(--xy-surface-3);
  border-color: var(--xy-border-hover);
}

.action-button--warning {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
  border-color: var(--xy-warning-border);
}

.action-button--warning:hover:not(:disabled) {
  color: var(--xy-text-on-bright);
  background: var(--xy-warning-hover);
  border-color: var(--xy-warning-hover);
}

.action-button--danger {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
  border-color: var(--xy-danger-border);
}

.action-button--danger:hover:not(:disabled) {
  color: var(--xy-text-on-bright);
  background: var(--xy-danger-hover);
  border-color: var(--xy-danger-hover);
}

.action-button:disabled,
.icon-button:disabled {
  cursor: not-allowed;
  opacity: 0.5;
}

.icon-button {
  display: grid;
  width: var(--control-height);
  height: var(--control-height);
  place-items: center;
  color: var(--xy-text-muted);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
}

.icon-button:hover:not(:disabled) {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
}

.form-error {
  margin: var(--xy-space-base) 0 0;
  color: var(--xy-warning-hover);
  font-size: var(--xy-font-size-sm);
}

.form-error {
  padding: var(--xy-space-base) var(--xy-space-md);
  background: var(--xy-warning-bg);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
}

.operations-state {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--xy-space-base);
  min-height: 16rem;
  color: var(--xy-text-muted);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.operations-state--error {
  color: var(--xy-danger-hover);
}

.operation-result {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-base);
  padding: var(--xy-space-base) var(--xy-space-md);
  margin-top: var(--xy-space-base);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.operation-result > .q-icon {
  margin-top: 0.15rem;
  font-size: var(--xy-font-size-lg);
}

.operation-result p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
}

.operation-result__dismiss {
  flex: 0 0 auto;
  margin-left: auto;
}

.operation-result--confirmed {
  color: var(--xy-success-hover);
  background: var(--xy-success-bg);
  border-color: var(--xy-success-border);
}

.operation-result--command-issued {
  color: var(--xy-info);
  background: var(--xy-info-bg);
  border-color: var(--xy-info-border);
}

.operation-result--failed {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
  border-color: var(--xy-danger-border);
}

.confirmation-dialog {
  --control-height: 2.75rem;
  width: min(34rem, calc(100vw - 2rem));
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  box-shadow: var(--xy-shadow-xl);
}

.confirmation-dialog h2 {
  font-size: var(--xy-font-size-lg);
}

.confirmation-dialog p {
  color: var(--xy-text-secondary);
}

.confirmation-actions {
  gap: var(--xy-space-sm);
  padding: var(--xy-space-base);
  background: var(--xy-surface-2);
  border-top: 1px solid var(--xy-border);
}

.confirmation-values {
  display: grid;
  grid-template-columns: max-content minmax(0, 1fr);
  gap: var(--xy-space-xs) var(--xy-space-base);
  padding: var(--xy-space-base);
  background: var(--xy-surface-0);
  border-radius: var(--xy-radius-md);
}

.confirmation-values dt {
  color: var(--xy-text-muted);
}

.confirmation-values dd {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
}

.confirmation-caution {
  display: flex;
  gap: var(--xy-space-sm);
  align-items: flex-start;
  color: var(--xy-warning-hover) !important;
}

@media (max-width: 63.9375rem) {
  .admin-console {
    grid-template-columns: 1fr;
    min-height: 0;
  }

  .operation-finder {
    border-right: 0;
    border-bottom: 1px solid var(--xy-border);
  }

  .category-list,
  .operation-list {
    display: flex;
    gap: var(--xy-space-xs);
    overflow-x: auto;
    padding-bottom: var(--xy-space-xs);
    scroll-snap-type: x proximity;
  }

  .category-button,
  .operation-button {
    flex: 0 0 auto;
    width: auto;
    scroll-snap-align: start;
  }

  .operation-button {
    flex-basis: 13rem;
  }
}

@media (max-width: 37.4375rem) {
  .server-strip,
  .control-row,
  .paired-fields,
  .assist-fields,
  .catalog-selection,
  .player-context,
  .player-identity,
  .operation-action-grid,
  .operation-action-grid .control-group--moderation .control-row,
  .assistance-form,
  .exact-time-fields {
    grid-template-columns: 1fr;
  }

  .player-context {
    top: var(--xy-header-stack-height, var(--xy-toolbar-height));
  }

  .player-identity__status,
  .catalog-selection__status {
    justify-self: start;
  }

  .server-strip {
    display: grid;
  }

  .server-strip__readout {
    align-items: stretch;
    justify-content: stretch;
  }

  .server-strip__time {
    justify-content: space-between;
  }

  .server-strip__actions,
  .server-strip__actions .button-cluster {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .operation-action-grid .control-group--moderation {
    grid-column: auto;
  }

  .operation-action-grid .control-group--wide {
    grid-column: auto;
  }

  .active-task__header {
    display: grid;
  }

  .risk-badge {
    justify-self: start;
  }

  .permission-options,
  .recovery-panel {
    grid-template-columns: 1fr;
  }

  .recovery-panel__actions {
    grid-column: 1 / -1;
    justify-content: stretch;
  }

  .recovery-panel__actions .action-button {
    flex: 1 1 9rem;
  }

  .recovery-panel__permission {
    text-align: left;
  }

  .form-actions {
    position: sticky;
    bottom: 0;
    z-index: var(--xy-z-sticky);
    margin: var(--xy-space-base) calc(var(--xy-space-md) * -1) calc(var(--xy-space-md) * -1);
    padding: var(--xy-space-sm) var(--xy-space-md)
      max(var(--xy-space-sm), env(safe-area-inset-bottom));
    background: var(--xy-surface-1);
    border-top: 1px solid var(--xy-border);
    box-shadow: var(--xy-shadow-sticky-lg);
  }

  .button-cluster,
  .button-cluster--end {
    display: grid;
    grid-template-columns: 1fr;
    padding-inline: 0;
  }

  .button-cluster--end {
    padding: 0 var(--xy-space-md) var(--xy-space-md);
  }

  .action-button {
    width: 100%;
  }
}
</style>
