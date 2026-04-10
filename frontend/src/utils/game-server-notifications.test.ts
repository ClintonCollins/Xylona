import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BackupProgressOperation, BackupProgressPhase, Status } from '@/proto/shared_pb'
import { StepStatus, type UpdateProgress, UpdateStep } from '@/proto/xylona_pb'
import {
  initGameServerNotificationService,
  recordLifecycleIntent,
  resetGameServerNotificationServiceForTests,
} from './game-server-notifications'

type EventHandler = (...args: unknown[]) => void

const mocks = vi.hoisted(() => {
  const listeners = new Map<string, Set<EventHandler>>()

  const eventBus = {
    on: vi.fn((event: string, handler: EventHandler) => {
      let handlers = listeners.get(event)
      if (!handlers) {
        handlers = new Set<EventHandler>()
        listeners.set(event, handlers)
      }
      handlers.add(handler)
    }),
    off: vi.fn((event: string, handler?: EventHandler) => {
      if (!handler) {
        listeners.delete(event)
        return
      }
      listeners.get(event)?.delete(handler)
    }),
    emit(event: string, ...args: unknown[]) {
      const handlers = listeners.get(event)
      if (!handlers) {
        return
      }
      for (const handler of handlers) {
        handler(...args)
      }
    },
    reset() {
      listeners.clear()
      this.on.mockClear()
      this.off.mockClear()
    },
  }

  return {
    notifyCreate: vi.fn(),
    eventBus,
  }
})

vi.mock('quasar', () => ({
  Notify: {
    create: mocks.notifyCreate,
  },
}))

vi.mock('@/utils/shared', () => ({
  XylonaEventBus: mocks.eventBus,
}))

function expectLatestToast(partial: Record<string, unknown>) {
  expect(mocks.notifyCreate).toHaveBeenCalled()
  const lastCall = mocks.notifyCreate.mock.calls.at(-1)?.[0]
  expect(lastCall).toEqual(expect.objectContaining(partial))
}

function expectAllToastsInTopRight() {
  for (const [options] of mocks.notifyCreate.mock.calls) {
    expect(options).toEqual(expect.objectContaining({ position: 'top-right' }))
  }
}

function emitStatus(serverID: string, serverName: string, status: Status): void {
  mocks.eventBus.emit('gameServerStatus', serverID, serverName, status)
}

function makeUpdateProgress(overrides: Partial<UpdateProgress> = {}): UpdateProgress {
  return {
    gameServerId: 'server-1',
    gameServerName: 'Alpha',
    step: UpdateStep.STOPPING,
    stepStatus: StepStatus.IN_PROGRESS,
    message: 'Working',
    ...overrides,
  } as UpdateProgress
}

describe('game-server-notifications', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    mocks.notifyCreate.mockReset()
    mocks.eventBus.reset()
    resetGameServerNotificationServiceForTests()
  })

  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('suppresses hydration status toasts and dedupes repeated status events', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('websocketConnected')
    emitStatus('server-1', 'Alpha', Status.ONLINE)

    expect(mocks.notifyCreate).not.toHaveBeenCalled()

    vi.advanceTimersByTime(2_000)

    emitStatus('server-1', 'Alpha', Status.ONLINE)
    expect(mocks.notifyCreate).not.toHaveBeenCalled()

    emitStatus('server-1', 'Alpha', Status.OFFLINE)
    expectLatestToast({
      type: 'xylona-info',
      caption: 'Alpha — Server stopped',
    })

    emitStatus('server-1', 'Alpha', Status.OFFLINE)
    expect(mocks.notifyCreate).toHaveBeenCalledTimes(1)

    emitStatus('server-1', 'Alpha', Status.ONLINE)
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Server started',
    })

    expectAllToastsInTopRight()
  })

  it('does not emit a started toast from a local update lifecycle intent', () => {
    recordLifecycleIntent('server-1', 'update')

    expect(mocks.notifyCreate).not.toHaveBeenCalled()
  })

  it('emits a started backup toast from live progress in observer windows', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('gameServerBackupProgress', {
      gameServerId: 'server-1',
      gameServerName: 'Alpha',
      backupId: 'backup-1',
      operation: BackupProgressOperation.CREATE,
      phase: BackupProgressPhase.PREPARING,
      percent: 0,
      sizeBytes: 0n,
      message: 'Preparing files',
    })

    expectLatestToast({
      type: 'xylona-info',
      caption: 'Alpha — Backup started',
    })

    mocks.eventBus.emit('gameServerBackupProgress', {
      gameServerId: 'server-1',
      gameServerName: 'Alpha',
      backupId: 'backup-1',
      operation: BackupProgressOperation.CREATE,
      phase: BackupProgressPhase.ARCHIVING,
      percent: 40,
      sizeBytes: 2048n,
      message: 'Archiving world data',
    })

    expect(mocks.notifyCreate).toHaveBeenCalledTimes(1)
    expectAllToastsInTopRight()
  })

  it('suppresses status toasts during updates and emits authoritative update toasts', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('websocketConnected')
    emitStatus('server-1', 'Alpha', Status.ONLINE)
    vi.advanceTimersByTime(2_000)

    recordLifecycleIntent('server-1', 'update')
    emitStatus('server-1', 'Alpha', Status.OFFLINE)
    expect(mocks.notifyCreate).not.toHaveBeenCalled()

    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      makeUpdateProgress({
        step: UpdateStep.STOPPING,
        stepStatus: StepStatus.IN_PROGRESS,
        message: 'Stopping server',
      }),
    )

    expectLatestToast({
      type: 'xylona-info',
      caption: 'Alpha — Update started',
    })

    emitStatus('server-1', 'Alpha', Status.ONLINE)
    expect(mocks.notifyCreate).toHaveBeenCalledTimes(1)

    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      makeUpdateProgress({
        step: UpdateStep.RESTARTING,
        stepStatus: StepStatus.COMPLETED,
        message: 'Restart complete',
      }),
    )

    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Update completed',
    })

    expectAllToastsInTopRight()
  })

  it('recovers status toasts if an update never emits a terminal event', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('websocketConnected')
    emitStatus('server-1', 'Alpha', Status.ONLINE)
    vi.advanceTimersByTime(2_000)

    recordLifecycleIntent('server-1', 'update')
    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      makeUpdateProgress({
        step: UpdateStep.STOPPING,
        stepStatus: StepStatus.IN_PROGRESS,
        message: 'Stopping server',
      }),
    )
    emitStatus('server-1', 'Alpha', Status.OFFLINE)

    expect(mocks.notifyCreate).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(10 * 60 * 1000)

    emitStatus('server-1', 'Alpha', Status.ONLINE)

    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Server started',
    })
    expectAllToastsInTopRight()
  })

  it('emits terminal update failures for observer tabs too', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('websocketConnected')
    emitStatus('server-1', 'Alpha', Status.ONLINE)
    vi.advanceTimersByTime(2_000)

    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      makeUpdateProgress({
        step: UpdateStep.STOPPING,
        stepStatus: StepStatus.IN_PROGRESS,
        message: 'Stopping server',
      }),
    )
    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      makeUpdateProgress({
        step: UpdateStep.BACKING_UP,
        stepStatus: StepStatus.FAILED,
        message: 'Disk full',
      }),
    )

    expectLatestToast({
      type: 'xylona-error',
      caption: 'Alpha — Update failed: Disk full',
      timeout: 0,
    })
    expectAllToastsInTopRight()
  })

  it('emits backup, restore, and install toasts from payload-supplied names', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('gameServerBackupProgress', {
      gameServerId: 'server-1',
      gameServerName: 'Alpha',
      backupId: 'backup-1',
      operation: BackupProgressOperation.CREATE,
      phase: BackupProgressPhase.COMPLETE,
      percent: 100,
      sizeBytes: 2048n,
      message: '',
    })
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Backup completed',
    })

    mocks.eventBus.emit('gameServerBackupProgress', {
      gameServerId: 'server-2',
      gameServerName: 'Bravo',
      backupId: 'backup-2',
      operation: BackupProgressOperation.RESTORE,
      phase: BackupProgressPhase.FAILED,
      percent: 100,
      sizeBytes: 2048n,
      message: 'Archive missing',
    })
    expectLatestToast({
      type: 'xylona-error',
      caption: 'Bravo — Restore failed: Archive missing',
      timeout: 0,
    })

    mocks.eventBus.emit('serverSoftwareInstall', 'server-1', 'Alpha', 'installing', '', 'paper')
    expectLatestToast({
      type: 'xylona-info',
      caption: 'Alpha — Install started',
    })

    mocks.eventBus.emit('serverSoftwareInstall', 'server-1', 'Alpha', 'complete', '', 'paper')
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Install completed',
    })

    mocks.eventBus.emit(
      'serverSoftwareInstall',
      'server-2',
      'Bravo',
      'failed',
      'SteamCMD failed',
      'paper',
    )
    expectLatestToast({
      type: 'xylona-error',
      caption: 'Bravo — Install failed: SteamCMD failed',
      timeout: 0,
    })

    expectAllToastsInTopRight()
  })

  it('falls back to result text when payloads omit the server name', () => {
    initGameServerNotificationService()

    mocks.eventBus.emit('gameServerBackupProgress', {
      gameServerId: 'server-1',
      gameServerName: '   ',
      backupId: 'backup-1',
      operation: BackupProgressOperation.CREATE,
      phase: BackupProgressPhase.COMPLETE,
      percent: 100,
      sizeBytes: 2048n,
      message: '',
    })
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Backup completed',
    })

    mocks.eventBus.emit('serverSoftwareInstall', 'server-1', '   ', 'complete', '', 'paper')
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Install completed',
    })
  })
})
