import { describe, expect, it, vi } from 'vitest'
import { nextTick, ref } from 'vue'

import { useGameServerConsoleState } from './useGameServerConsoleState'

function setup(autoScroll: boolean) {
  const scrollToBottom = vi.fn()
  const storage = { getItem: vi.fn(() => String(autoScroll)), setItem: vi.fn() }
  const state = useGameServerConsoleState({ gameID: ref('minecraft'), scrollToBottom, storage })
  return { scrollToBottom, state, storage }
}

async function appendLine(state: ReturnType<typeof setup>['state']) {
  const before = state.consoleLines.value.length
  state.appendConsoleOutput('line\n')
  // Flushes are batched per animation frame; happy-dom runs them on a timer.
  await vi.waitFor(() => expect(state.consoleLines.value.length).toBeGreaterThan(before))
  await nextTick()
}

describe('useGameServerConsoleState following', () => {
  it.each([
    { label: 'follows new output at the bottom', autoScroll: true, away: false, follows: true },
    { label: 'pauses while scrolled up', autoScroll: true, away: true, follows: false },
    { label: 'never follows with auto scroll off', autoScroll: false, away: false, follows: false },
  ])('$label', async ({ autoScroll, away, follows }) => {
    const { scrollToBottom, state, storage } = setup(autoScroll)
    state.setConsoleScrolledAway(away)

    await appendLine(state)

    expect(scrollToBottom).toHaveBeenCalledTimes(follows ? 1 : 0)
    expect(state.unseenConsoleOutput.value).toBe(!follows)
    expect(storage.setItem).not.toHaveBeenCalled()
  })

  it('resumes following once the reader is back at the bottom', async () => {
    const { scrollToBottom, state } = setup(true)
    state.setConsoleScrolledAway(true)
    await appendLine(state)
    expect(state.unseenConsoleOutput.value).toBe(true)

    state.jumpToLatestOutput()
    await nextTick()
    expect(scrollToBottom).toHaveBeenCalledTimes(1)
    expect(state.unseenConsoleOutput.value).toBe(false)

    await appendLine(state)
    expect(scrollToBottom).toHaveBeenCalledTimes(2)
    expect(state.consoleAutoScroll.value).toBe(true)
  })
})
