import { Notify } from 'quasar'

import {
  BackupProgressOperation,
  BackupProgressPhase,
  Status,
  type BackupProgress,
} from '@/proto/shared_pb'
import { StepStatus, UpdateStep, type UpdateProgress } from '@/proto/xylona_pb'
import {
  applyUpdateProgress,
  buildUpdateSteps,
  isUpdateProgressTerminal,
} from '@/pages/game_servers/update-progress'
import { XylonaEventBus } from '@/utils/shared'

export type GameServerLifecycleIntent =
  | 'start'
  | 'stop'
  | 'update'
  | 'backup'
  | 'restore'
  | 'install'

const hydrationWindowMs = 1_500
const shortToastTimeoutMs = 3_000
const localIntentTtlMs = 30_000
const activeUpdateTtlMs = 10 * 60 * 1000

let initialized = false
let hydrating = true
let hydrationTimer: ReturnType<typeof setTimeout> | null = null

const lastKnownStatusByServer = new Map<string, Status>()
const activeOperationByServer = new Map<string, 'update'>()
const activeUpdateTimersByServer = new Map<string, ReturnType<typeof setTimeout>>()
const localIntentTimersByKey = new Map<string, ReturnType<typeof setTimeout>>()
const serverNameById = new Map<string, string>()
const updateStepsByServer = new Map<string, ReturnType<typeof buildUpdateSteps>>()

function enterHydrationWindow(): void {
  hydrating = true
  if (hydrationTimer !== null) {
    clearTimeout(hydrationTimer)
  }
  hydrationTimer = setTimeout(() => {
    hydrating = false
    hydrationTimer = null
  }, hydrationWindowMs)
}

function showToast(type: 'xylona-success' | 'xylona-error' | 'xylona-info', caption: string): void {
  const isError = type === 'xylona-error'
  Notify.create({
    type,
    caption,
    position: 'top-right',
    timeout: isError ? 0 : shortToastTimeoutMs,
    actions: isError
      ? [
          {
            label: 'Dismiss',
            color: 'white',
          },
        ]
      : undefined,
  })
}

function shortenServerID(serverID: string): string {
  if (serverID.length <= 8) {
    return serverID
  }
  return `${serverID.slice(0, 8)}...`
}

function resolveServerName(serverID: string): string {
  const knownName = serverNameById.get(serverID)?.trim()
  if (knownName) {
    return knownName
  }
  return shortenServerID(serverID)
}

function buildCaption(serverID: string, result: string): string {
  return `${resolveServerName(serverID)} — ${result}`
}

function buildFailureCaption(serverID: string, label: string, message: string): string {
  const trimmedMessage = message.trim()
  if (trimmedMessage === '') {
    return buildCaption(serverID, `${label} failed`)
  }
  return buildCaption(serverID, `${label} failed: ${trimmedMessage}`)
}

function localIntentKey(serverID: string, kind: GameServerLifecycleIntent): string {
  return `${serverID}:${kind}`
}

function trackLocalIntent(serverID: string, kind: GameServerLifecycleIntent): void {
  const key = localIntentKey(serverID, kind)
  const existingTimer = localIntentTimersByKey.get(key)
  if (existingTimer) {
    clearTimeout(existingTimer)
  }
  const nextTimer = setTimeout(() => {
    localIntentTimersByKey.delete(key)
  }, localIntentTtlMs)
  localIntentTimersByKey.set(key, nextTimer)
}

function clearActiveUpdate(serverID: string): void {
  activeOperationByServer.delete(serverID)
  updateStepsByServer.delete(serverID)

  const timer = activeUpdateTimersByServer.get(serverID)
  if (timer) {
    clearTimeout(timer)
  }
  activeUpdateTimersByServer.delete(serverID)
}

function markUpdateActive(serverID: string): void {
  activeOperationByServer.set(serverID, 'update')

  const existingTimer = activeUpdateTimersByServer.get(serverID)
  if (existingTimer) {
    clearTimeout(existingTimer)
  }

  const nextTimer = setTimeout(() => {
    clearActiveUpdate(serverID)
  }, activeUpdateTtlMs)
  activeUpdateTimersByServer.set(serverID, nextTimer)
}

function handleWebsocketConnected(): void {
  enterHydrationWindow()
}

function handleGameServerStatus(serverID: string, status: Status): void {
  if (serverID.trim() === '') {
    return
  }

  const previousStatus = lastKnownStatusByServer.get(serverID)
  lastKnownStatusByServer.set(serverID, status)

  if (hydrating) {
    return
  }
  if (previousStatus === status) {
    return
  }
  if (activeOperationByServer.get(serverID) === 'update') {
    return
  }

  if (status === Status.ONLINE) {
    showToast('xylona-success', buildCaption(serverID, 'Server started'))
  }
  if (status === Status.OFFLINE) {
    showToast('xylona-info', buildCaption(serverID, 'Server stopped'))
  }
}

function updateSucceeded(
  progress: UpdateProgress,
  steps: ReturnType<typeof buildUpdateSteps>,
): boolean {
  if (progress.step === UpdateStep.RESTARTING && progress.stepStatus === StepStatus.COMPLETED) {
    return true
  }

  return (
    progress.step === UpdateStep.INSTALLING &&
    progress.stepStatus === StepStatus.COMPLETED &&
    !steps.some((step) => step.step === UpdateStep.RESTARTING)
  )
}

function handleUpdateProgress(progress: UpdateProgress): void {
  const serverID = progress.gameServerId.trim()
  if (serverID === '') {
    return
  }

  markUpdateActive(serverID)

  const currentStatus = lastKnownStatusByServer.get(serverID) ?? Status.UNKNOWN
  const currentSteps = updateStepsByServer.get(serverID) ?? buildUpdateSteps(currentStatus)
  const nextSteps = applyUpdateProgress(currentSteps, progress)
  updateStepsByServer.set(serverID, nextSteps)

  if (!isUpdateProgressTerminal(progress, nextSteps)) {
    return
  }

  clearActiveUpdate(serverID)
  clearLifecycleIntent(serverID, 'update')

  if (updateSucceeded(progress, nextSteps)) {
    showToast('xylona-success', buildCaption(serverID, 'Update completed'))
    return
  }

  showToast('xylona-error', buildFailureCaption(serverID, 'Update', progress.message))
}

function handleBackupProgress(progress: BackupProgress): void {
  const serverID = progress.gameServerId.trim()
  if (serverID === '') {
    return
  }

  if (
    progress.phase !== BackupProgressPhase.COMPLETE &&
    progress.phase !== BackupProgressPhase.FAILED
  ) {
    return
  }

  const isBackup = progress.operation !== BackupProgressOperation.RESTORE
  const label = isBackup ? 'Backup' : 'Restore'
  const intent = isBackup ? 'backup' : 'restore'

  clearLifecycleIntent(serverID, intent)

  if (progress.phase === BackupProgressPhase.COMPLETE) {
    showToast('xylona-success', buildCaption(serverID, `${label} completed`))
    return
  }

  showToast('xylona-error', buildFailureCaption(serverID, label, progress.message))
}

function handleServerSoftwareInstall(
  serverID: string,
  status: string,
  error: string,
  _softwareID: string,
): void {
  const trimmedServerID = serverID.trim()
  if (trimmedServerID === '') {
    return
  }

  if (status === 'installing') {
    return
  }

  clearLifecycleIntent(trimmedServerID, 'install')

  if (status === 'complete') {
    showToast('xylona-success', buildCaption(trimmedServerID, 'Install completed'))
    return
  }

  if (status === 'failed') {
    showToast('xylona-error', buildFailureCaption(trimmedServerID, 'Install', error))
  }
}

export function initGameServerNotificationService(): void {
  if (initialized) {
    return
  }

  initialized = true
  enterHydrationWindow()

  XylonaEventBus.on('websocketConnected', handleWebsocketConnected)
  XylonaEventBus.on('gameServerStatus', handleGameServerStatus)
  XylonaEventBus.on('gameServerUpdateProgress', handleUpdateProgress)
  XylonaEventBus.on('gameServerBackupProgress', handleBackupProgress)
  XylonaEventBus.on('serverSoftwareInstall', handleServerSoftwareInstall)
}

export function recordLifecycleIntent(serverID: string, kind: GameServerLifecycleIntent): void {
  const trimmedServerID = serverID.trim()
  if (trimmedServerID === '') {
    return
  }

  trackLocalIntent(trimmedServerID, kind)

  switch (kind) {
    case 'update':
      markUpdateActive(trimmedServerID)
      showToast('xylona-info', buildCaption(trimmedServerID, 'Update started'))
      return
    case 'backup':
      showToast('xylona-info', buildCaption(trimmedServerID, 'Backup started'))
      return
    case 'restore':
      showToast('xylona-info', buildCaption(trimmedServerID, 'Restore started'))
      return
    case 'install':
      showToast('xylona-info', buildCaption(trimmedServerID, 'Install started'))
      return
    default:
      return
  }
}

export function clearLifecycleIntent(serverID: string, kind?: GameServerLifecycleIntent): void {
  const trimmedServerID = serverID.trim()
  if (trimmedServerID === '') {
    return
  }

  const keys =
    kind === undefined
      ? [...localIntentTimersByKey.keys()].filter((key) => key.startsWith(`${trimmedServerID}:`))
      : [localIntentKey(trimmedServerID, kind)]

  for (const key of keys) {
    const timer = localIntentTimersByKey.get(key)
    if (timer) {
      clearTimeout(timer)
    }
    localIntentTimersByKey.delete(key)
  }
}

export function registerServerContext(servers: Array<{ id: string; name: string }>): void {
  for (const server of servers) {
    registerServerName(server.id, server.name)
  }
}

export function registerServerName(serverID: string, name: string): void {
  const trimmedServerID = serverID.trim()
  const trimmedName = name.trim()
  if (trimmedServerID === '' || trimmedName === '') {
    return
  }
  serverNameById.set(trimmedServerID, trimmedName)
}

export function resetGameServerNotificationServiceForTests(): void {
  if (initialized) {
    XylonaEventBus.off('websocketConnected', handleWebsocketConnected)
    XylonaEventBus.off('gameServerStatus', handleGameServerStatus)
    XylonaEventBus.off('gameServerUpdateProgress', handleUpdateProgress)
    XylonaEventBus.off('gameServerBackupProgress', handleBackupProgress)
    XylonaEventBus.off('serverSoftwareInstall', handleServerSoftwareInstall)
  }

  initialized = false
  hydrating = true

  if (hydrationTimer !== null) {
    clearTimeout(hydrationTimer)
    hydrationTimer = null
  }

  for (const timer of localIntentTimersByKey.values()) {
    clearTimeout(timer)
  }

  lastKnownStatusByServer.clear()
  activeOperationByServer.clear()
  for (const timer of activeUpdateTimersByServer.values()) {
    clearTimeout(timer)
  }
  activeUpdateTimersByServer.clear()
  localIntentTimersByKey.clear()
  serverNameById.clear()
  updateStepsByServer.clear()
}
