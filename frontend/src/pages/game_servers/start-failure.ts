import { Status } from '@/proto/shared_pb'

/** How long after a Start request an OFFLINE transition still counts as that start failing. */
export const startFailureWindowMs = 120_000

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
  // ponytail: only this page's own Start is tracked; a start issued from
  // another tab that dies shows as a plain Offline. Track intents server-side
  // if that matters.
  return { at: now, message: processExitedMessage }
}

export function formatFailureTime(at: number): string {
  return new Date(at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}
