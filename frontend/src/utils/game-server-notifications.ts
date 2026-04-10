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

function buildCaption(serverName: string, result: string): string {
  const name = serverName.trim()
  if (name === '') {
    return result
  }
  return `${name} — ${result}`
}

function buildFailureCaption(serverName: string, label: string, message: string): string {
  const trimmedMessage = message.trim()
  if (trimmedMessage === '') {
    return buildCaption(serverName, `${label} failed`)
  }
  return buildCaption(serverName, `${label} failed: ${trimmedMessage}`)
}

function showServerToast(
  type: 'xylona-success' | 'xylona-error' | 'xylona-info',
  serverName: string,
  result: string,
): void {
  showToast(type, buildCaption(serverName.trim(), result))
}

function showServerFailureToast(serverName: string, label: string, message: string): void {
  showToast('xylona-error', buildFailureCaption(serverName.trim(), label, message))
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

function hasTrackedIntent(serverID: string, kind: GameServerLifecycleIntent): boolean {
  return localIntentTimersByKey.has(localIntentKey(serverID, kind))
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

function handleGameServerStatus(serverID: string, serverName: string, status: Status): void {
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
    showServerToast('xylona-success', serverName, 'Server started')
  }
  if (status === Status.OFFLINE) {
    showServerToast('xylona-info', serverName, 'Server stopped')
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

  // Update started toasts come from the first authoritative progress event so
  // initiating tabs and observer tabs both see the same lifecycle signal.
  if (progress.stepStatus === StepStatus.IN_PROGRESS && !hasTrackedIntent(serverID, 'update')) {
    trackLocalIntent(serverID, 'update')
    showServerToast('xylona-info', progress.gameServerName, 'Update started')
  } else if (progress.stepStatus === StepStatus.IN_PROGRESS) {
    trackLocalIntent(serverID, 'update')
  }

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
    showServerToast('xylona-success', progress.gameServerName, 'Update completed')
    return
  }

  showServerFailureToast(progress.gameServerName, 'Update', progress.message)
}

function handleBackupProgress(progress: BackupProgress): void {
  const serverID = progress.gameServerId.trim()
  if (serverID === '') {
    return
  }

  const isBackup = progress.operation !== BackupProgressOperation.RESTORE
  const label = isBackup ? 'Backup' : 'Restore'
  const intent = isBackup ? 'backup' : 'restore'

  if (
    progress.phase !== BackupProgressPhase.COMPLETE &&
    progress.phase !== BackupProgressPhase.FAILED
  ) {
    if (!hasTrackedIntent(serverID, intent)) {
      trackLocalIntent(serverID, intent)
      showServerToast('xylona-info', progress.gameServerName, `${label} started`)
      return
    }
    trackLocalIntent(serverID, intent)
    return
  }

  clearLifecycleIntent(serverID, intent)

  if (progress.phase === BackupProgressPhase.COMPLETE) {
    showServerToast('xylona-success', progress.gameServerName, `${label} completed`)
    return
  }

  showServerFailureToast(progress.gameServerName, label, progress.message)
}

function handleServerSoftwareInstall(
  serverID: string,
  serverName: string,
  status: string,
  error: string,
  _softwareID: string,
): void {
  const trimmedServerID = serverID.trim()
  if (trimmedServerID === '') {
    return
  }

  if (status === 'installing') {
    if (!hasTrackedIntent(trimmedServerID, 'install')) {
      trackLocalIntent(trimmedServerID, 'install')
      showServerToast('xylona-info', serverName, 'Install started')
      return
    }
    trackLocalIntent(trimmedServerID, 'install')
    return
  }

  clearLifecycleIntent(trimmedServerID, 'install')

  if (status === 'complete') {
    showServerToast('xylona-success', serverName, 'Install completed')
    return
  }

  if (status === 'failed') {
    showServerFailureToast(serverName, 'Install', error)
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

export function recordLifecycleIntent(serverID: string, kind: 'update'): void {
  const trimmedServerID = serverID.trim()
  if (trimmedServerID === '') {
    return
  }

  // Update intents need eager suppression because their status transitions
  // can arrive before progress events. Backup, restore, and install started
  // toasts are driven by their first authoritative progress payload instead.
  if (kind === 'update') {
    markUpdateActive(trimmedServerID)
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
  updateStepsByServer.clear()
}
