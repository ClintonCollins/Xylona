import { inject, onScopeDispose, ref, watch, type InjectionKey, type Ref } from 'vue'

import { Status } from '@/proto/shared_pb'
import { formatTime } from '@/utils/format-timestamp'
import { XylonaEventBus } from '@/utils/shared'

/**
 * How long after a Start request an OFFLINE transition still counts as that
 * start failing. It covers the node's five-minute readiness wait: a server
 * that exits while Starting never came online. Reaching ONLINE clears it.
 */
export const startFailureWindowMs = 360_000

export interface StartFailure {
  at: number
  message: string
}

export interface LifecycleIntents {
  /** Epoch ms of the operator's last Start request on this page, or 0. */
  startRequestedAt: number
  /** Epoch ms of the operator's last Stop request on this page, or 0. */
  stopRequestedAt: number
}

export const processExitedMessage = 'The server process exited before it came online.'

/**
 * Decide whether a live status transition means the last Start request failed.
 * Returns the failure to show, `null` to clear any shown failure (server came
 * online), or `undefined` to leave the current state alone.
 */
export function detectStartFailure(
  status: Status,
  intents: LifecycleIntents,
  now: number,
): StartFailure | null | undefined {
  if (status === Status.ONLINE) {
    return null
  }
  if (status !== Status.OFFLINE || intents.startRequestedAt === 0) {
    return undefined
  }
  if (now - intents.startRequestedAt > startFailureWindowMs) {
    return undefined
  }
  if (intents.stopRequestedAt >= intents.startRequestedAt) {
    return undefined
  }
  // ponytail: only this workspace's own Start is tracked; a start issued from
  // another browser tab that dies shows as a plain Offline. Track intents
  // server-side if that matters.
  return { at: now, message: processExitedMessage }
}

export function formatFailureTime(at: number): string {
  return formatTime(new Date(at))
}

export interface GameServerLifecycle {
  /** The last failed Start, shown until dismissed, retried, or the server comes online. */
  lastStartFailure: Ref<StartFailure | null>
  intents: LifecycleIntents
  /** A Restart request from this workspace is in flight. */
  restarting: Ref<boolean>
}

export const gameServerLifecycleKey: InjectionKey<GameServerLifecycle> =
  Symbol('gameServerLifecycle')

/**
 * Start and Restart state for one server. The layout provides it, so a Start
 * pressed on the console and a crash seen from Files land in the same place
 * and the identity bar can say "Start failed" on every tab.
 */
export function createGameServerLifecycle(serverId: Ref<string>): GameServerLifecycle {
  const lifecycle: GameServerLifecycle = {
    lastStartFailure: ref(null),
    intents: { startRequestedAt: 0, stopRequestedAt: 0 },
    restarting: ref(false),
  }

  function onServerStatus(id: string, _name: string, status: Status): void {
    if (id !== serverId.value) return
    const failure = detectStartFailure(status, lifecycle.intents, Date.now())
    if (failure === undefined) return
    lifecycle.lastStartFailure.value = failure
    lifecycle.intents.startRequestedAt = 0
  }

  XylonaEventBus.on('gameServerStatus', onServerStatus)
  onScopeDispose(() => XylonaEventBus.off('gameServerStatus', onServerStatus))
  watch(serverId, () => {
    lifecycle.lastStartFailure.value = null
    lifecycle.intents.startRequestedAt = 0
    lifecycle.intents.stopRequestedAt = 0
  })
  return lifecycle
}

/** The layout's shared lifecycle state, or a local one when rendered on its own. */
export function useGameServerLifecycle(serverId: Ref<string>): GameServerLifecycle {
  return inject(gameServerLifecycleKey, null) ?? createGameServerLifecycle(serverId)
}
