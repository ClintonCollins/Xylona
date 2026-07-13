import { nextTick, ref, type Ref } from 'vue'

import { parseConsole } from '@/utils/console'

import { splitConsoleChunk, trimConsoleLines, type ConsoleLine } from './console-buffer'

const autoScrollStorageKey = 'xylona_console_autoscroll'
const defaultMaxConsoleCharacters = 100000

type ConsoleStateOptions = {
  gameID: Ref<string>
  maxCharacters?: number
  scrollToBottom: () => void
  storage?: Pick<Storage, 'getItem' | 'setItem'> | null
}

export function useGameServerConsoleState(options: ConsoleStateOptions) {
  const maxConsoleCharacters = options.maxCharacters ?? defaultMaxConsoleCharacters
  const storage = resolveConsoleStorage(options.storage)
  const consoleLines = ref<ConsoleLine[]>([])
  const consoleTruncated = ref(false)
  const consoleAutoScroll = ref(storage?.getItem(autoScrollStorageKey) !== 'false')
  const serverInput = ref('')
  const consoleHistory = ref<string[]>([])
  const consoleHistoryCurrentIndex = ref(0)

  let consoleLineIdCounter = 0
  let pendingConsoleChunks: string[] = []
  let consoleRafId: number | null = null

  function appendConsoleOutput(rawOutput: string) {
    const parsed = parseConsole(options.gameID.value, rawOutput)
    if (parsed.length === 0) return

    pendingConsoleChunks.push(parsed)
    scheduleConsoleFlush()
  }

  function replaceConsoleOutput(rawOutput: string) {
    cancelPendingConsoleFlush()

    const parsed = parseConsole(options.gameID.value, rawOutput)
    const replacementLines = splitConsoleChunk(parsed).map((lineHtml) => ({
      id: consoleLineIdCounter++,
      html: lineHtml,
    }))
    const trimmedConsole = trimConsoleLines(replacementLines, maxConsoleCharacters)
    consoleLines.value = trimmedConsole.lines
    consoleTruncated.value = trimmedConsole.truncated

    if (consoleAutoScroll.value) {
      void nextTick(options.scrollToBottom)
    }
  }

  function scheduleConsoleFlush() {
    if (consoleRafId !== null) return

    if (typeof requestAnimationFrame !== 'function') {
      flushConsolePending()
      return
    }

    consoleRafId = requestAnimationFrame(flushConsolePending)
  }

  function flushConsolePending() {
    consoleRafId = null
    if (pendingConsoleChunks.length === 0) return

    const newLines = pendingConsoleChunks.flatMap((html) =>
      splitConsoleChunk(html).map((lineHtml) => ({
        id: consoleLineIdCounter++,
        html: lineHtml,
      })),
    )
    pendingConsoleChunks = []

    if (newLines.length === 0) {
      return
    }

    consoleLines.value.push(...newLines)

    const trimmedConsole = trimConsoleLines(consoleLines.value, maxConsoleCharacters)
    consoleLines.value = trimmedConsole.lines
    if (trimmedConsole.truncated) {
      consoleTruncated.value = true
    }

    if (consoleAutoScroll.value) {
      void nextTick(options.scrollToBottom)
    }
  }

  function toggleConsoleAutoScroll() {
    consoleAutoScroll.value = !consoleAutoScroll.value
    storage?.setItem(autoScrollStorageKey, String(consoleAutoScroll.value))
    if (consoleAutoScroll.value) {
      void nextTick(options.scrollToBottom)
    }
  }

  function navigateConsoleInputHistory(direction: string) {
    const normalizedDirection = direction.toLowerCase()
    if (normalizedDirection !== 'up' && normalizedDirection !== 'down') return
    if (consoleHistory.value.length === 0) return
    if (consoleHistoryCurrentIndex.value > consoleHistory.value.length) return

    const historyDirection = normalizedDirection === 'up' ? -1 : 1
    const newIndex = consoleHistoryCurrentIndex.value + historyDirection
    if (newIndex < 0 || newIndex > consoleHistory.value.length) return

    consoleHistoryCurrentIndex.value = newIndex
    serverInput.value =
      newIndex === consoleHistory.value.length ? '' : (consoleHistory.value[newIndex] ?? '')
  }

  function recordConsoleInput() {
    consoleHistory.value.push(serverInput.value)
    consoleHistoryCurrentIndex.value = consoleHistory.value.length
    serverInput.value = ''
  }

  function cancelPendingConsoleFlush() {
    if (consoleRafId !== null && typeof cancelAnimationFrame === 'function') {
      cancelAnimationFrame(consoleRafId)
    }
    consoleRafId = null
    pendingConsoleChunks = []
  }

  return {
    appendConsoleOutput,
    cancelPendingConsoleFlush,
    consoleAutoScroll,
    consoleLines,
    consoleTruncated,
    navigateConsoleInputHistory,
    recordConsoleInput,
    replaceConsoleOutput,
    serverInput,
    toggleConsoleAutoScroll,
  }
}

function resolveConsoleStorage(storage: Pick<Storage, 'getItem' | 'setItem'> | null | undefined) {
  if (storage !== undefined) return storage
  if (typeof window === 'undefined') return null
  return window.localStorage
}
