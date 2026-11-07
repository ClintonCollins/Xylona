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

describe('useGameServerConsoleState command history', () => {
  function setupHistory() {
    const { state } = setup(true)
    for (const command of ['list', 'say Server restart in five minutes']) {
      state.serverInput.value = command
      state.recordConsoleInput()
    }
    return state
  }

  it('restores the exact unfinished command after browsing older commands', () => {
    const state = setupHistory()
    const draft = '  say Restart postponed until 18:30  '
    state.serverInput.value = draft

    state.navigateConsoleInputHistory('up')
    expect(state.serverInput.value).toBe('say Server restart in five minutes')
    state.navigateConsoleInputHistory('up')
    expect(state.serverInput.value).toBe('list')
    state.navigateConsoleInputHistory('up')
    expect(state.serverInput.value).toBe('list')
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe('say Server restart in five minutes')
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe(draft)
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe(draft)
  })

  it('keeps the draft separate from edits to a recalled command', () => {
    const state = setupHistory()
    state.serverInput.value = 'say Still typing'
    state.navigateConsoleInputHistory('up')
    state.serverInput.value = 'say Edited history entry'
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe('say Still typing')

    state.navigateConsoleInputHistory('up')
    expect(state.serverInput.value).toBe('say Server restart in five minutes')
  })

  it('captures edits to the restored draft on the next history visit', () => {
    const state = setupHistory()
    state.serverInput.value = 'say First draft'
    state.navigateConsoleInputHistory('up')
    state.navigateConsoleInputHistory('down')
    state.serverInput.value = 'say Revised draft'
    state.navigateConsoleInputHistory('up')
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe('say Revised draft')
  })

  it('starts with a fresh draft after sending a recalled command', () => {
    const state = setupHistory()
    state.serverInput.value = 'say Unsent draft'
    state.navigateConsoleInputHistory('up')
    state.serverInput.value = 'say Updated announcement'
    state.recordConsoleInput()
    expect(state.serverInput.value).toBe('')

    state.navigateConsoleInputHistory('up')
    expect(state.serverInput.value).toBe('say Updated announcement')
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe('')
  })

  it('does not change a draft when there is no history or the direction is invalid', () => {
    const { state } = setup(true)
    state.serverInput.value = 'say First command'
    state.navigateConsoleInputHistory('up')
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe('say First command')

    state.recordConsoleInput()
    state.serverInput.value = 'say Next command'
    state.navigateConsoleInputHistory('left')
    state.navigateConsoleInputHistory('down')
    expect(state.serverInput.value).toBe('say Next command')
  })
})
