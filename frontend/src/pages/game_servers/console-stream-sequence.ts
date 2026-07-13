export type ConsoleStreamAction = 'append' | 'replace' | 'ignore'

export interface ConsoleStreamChunk {
  reset: boolean
  sequence: bigint
}

export interface ConsoleStreamDecision {
  action: ConsoleStreamAction
  nextSequence: bigint
}

export function resolveConsoleStreamChunk(
  lastSequence: bigint,
  chunk: ConsoleStreamChunk,
): ConsoleStreamDecision {
  if (chunk.reset) {
    return {
      action: 'replace',
      nextSequence: chunk.sequence,
    }
  }

  if (chunk.sequence === 0n) {
    return {
      action: 'append',
      nextSequence: lastSequence,
    }
  }

  if (chunk.sequence <= lastSequence) {
    return {
      action: 'ignore',
      nextSequence: lastSequence,
    }
  }

  return {
    action: 'append',
    nextSequence: chunk.sequence,
  }
}
