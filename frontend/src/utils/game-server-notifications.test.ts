import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { BackupProgressOperation, BackupProgressPhase, Status } from '@/proto/shared_pb'
import { StepStatus, UpdateStep, type UpdateProgress } from '@/proto/xylona_pb'
import {
  initGameServerNotificationService,
  recordLifecycleIntent,
  registerServerContext,
  registerServerName,
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

function makeUpdateProgress(overrides: Partial<UpdateProgress> = {}): UpdateProgress {
  return {
    gameServerId: 'server-1',
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
    registerServerName('server-1', 'Alpha')

    mocks.eventBus.emit('websocketConnected')
    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)

    expect(mocks.notifyCreate).not.toHaveBeenCalled()

    vi.advanceTimersByTime(2_000)

    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)
    expect(mocks.notifyCreate).not.toHaveBeenCalled()

    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.OFFLINE)
    expectLatestToast({
      type: 'xylona-info',
      caption: 'Alpha — Server stopped',
    })

    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.OFFLINE)
    expect(mocks.notifyCreate).toHaveBeenCalledTimes(1)

    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Server started',
    })

    expectAllToastsInTopRight()
  })

  it('emits actor-scoped started toasts for long-running lifecycle actions', () => {
    registerServerContext([{ id: 'server-1', name: 'Alpha' }])

    recordLifecycleIntent('server-1', 'update')
    recordLifecycleIntent('server-1', 'backup')
    recordLifecycleIntent('server-1', 'restore')
    recordLifecycleIntent('server-1', 'install')

    expect(mocks.notifyCreate).toHaveBeenCalledTimes(4)
    expect(mocks.notifyCreate.mock.calls.map(([options]) => options.caption)).toEqual([
      'Alpha — Update started',
      'Alpha — Backup started',
      'Alpha — Restore started',
      'Alpha — Install started',
    ])

    expectAllToastsInTopRight()
  })

  it('suppresses status toasts during updates and emits a terminal completion toast', () => {
    initGameServerNotificationService()
    registerServerName('server-1', 'Alpha')

    mocks.eventBus.emit('websocketConnected')
    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)
    vi.advanceTimersByTime(2_000)

    recordLifecycleIntent('server-1', 'update')
    expectLatestToast({
      type: 'xylona-info',
      caption: 'Alpha — Update started',
    })

    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.OFFLINE)
    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      makeUpdateProgress({
        step: UpdateStep.STOPPING,
        stepStatus: StepStatus.IN_PROGRESS,
        message: 'Stopping server',
      }),
    )
    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)

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

  it('recovers start and stop toasts if an update never emits a terminal event', () => {
    initGameServerNotificationService()
    registerServerName('server-1', 'Alpha')

    mocks.eventBus.emit('websocketConnected')
    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)
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
    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.OFFLINE)

    expect(mocks.notifyCreate).toHaveBeenCalledTimes(1)

    vi.advanceTimersByTime(10 * 60 * 1000)

    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)

    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Server started',
    })
    expectAllToastsInTopRight()
  })

  it('emits terminal update failures for observer tabs too', () => {
    initGameServerNotificationService()
    registerServerName('server-1', 'Alpha')

    mocks.eventBus.emit('websocketConnected')
    mocks.eventBus.emit('gameServerStatus', 'server-1', Status.ONLINE)
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

  it('emits backup, restore, and install terminal toasts with fallback names', () => {
    initGameServerNotificationService()
    registerServerName('server-1', 'Alpha')

    mocks.eventBus.emit('gameServerBackupProgress', {
      gameServerId: 'server-1',
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
      gameServerId: 'server-1234567890',
      backupId: 'backup-2',
      operation: BackupProgressOperation.RESTORE,
      phase: BackupProgressPhase.FAILED,
      percent: 100,
      sizeBytes: 2048n,
      message: 'Archive missing',
    })
    expectLatestToast({
      type: 'xylona-error',
      caption: 'server-1... — Restore failed: Archive missing',
      timeout: 0,
    })

    mocks.eventBus.emit('serverSoftwareInstall', 'server-1', 'installing', '', 'paper')
    expect(mocks.notifyCreate).toHaveBeenCalledTimes(2)

    mocks.eventBus.emit('serverSoftwareInstall', 'server-1', 'complete', '', 'paper')
    expectLatestToast({
      type: 'xylona-success',
      caption: 'Alpha — Install completed',
    })

    mocks.eventBus.emit(
      'serverSoftwareInstall',
      'server-1234567890',
      'failed',
      'SteamCMD failed',
      'paper',
    )
    expectLatestToast({
      type: 'xylona-error',
      caption: 'server-1... — Install failed: SteamCMD failed',
      timeout: 0,
    })

    expectAllToastsInTopRight()
  })
})
