import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  CONNECTING_NOTICE_DELAY_MS,
  restartConnectingNoticeDelay,
  setWebsocketBrowserOnline,
  setWebsocketConnectionStatus,
  websocketBrowserOnline,
  websocketConnectingQuietly,
  websocketConnectionEpoch,
  websocketStateAuthoritative,
} from './websocket-connection'

describe('websocket connection state', () => {
  afterEach(() => {
    setWebsocketConnectionStatus('connecting')
    setWebsocketBrowserOnline(true)
  })

  it('increments the epoch only for a real transition to connected', () => {
    setWebsocketConnectionStatus('connecting')
    const initialEpoch = websocketConnectionEpoch.value

    expect(setWebsocketConnectionStatus('connected')).toBe(true)
    expect(websocketConnectionEpoch.value).toBe(initialEpoch + 1)
    expect(websocketStateAuthoritative.value).toBe(true)

    expect(setWebsocketConnectionStatus('connected')).toBe(false)
    expect(websocketConnectionEpoch.value).toBe(initialEpoch + 1)

    setWebsocketConnectionStatus('reconnecting')
    expect(websocketStateAuthoritative.value).toBe(false)
    setWebsocketConnectionStatus('connected')
    expect(websocketConnectionEpoch.value).toBe(initialEpoch + 2)
  })

  it('keeps a fresh connection quiet only while connecting and not yet overdue', () => {
    vi.useFakeTimers()
    try {
      setWebsocketConnectionStatus('connecting')
      restartConnectingNoticeDelay()
      expect(websocketConnectingQuietly.value).toBe(true)

      vi.advanceTimersByTime(CONNECTING_NOTICE_DELAY_MS - 1)
      expect(websocketConnectingQuietly.value).toBe(true)
      vi.advanceTimersByTime(1)
      expect(websocketConnectingQuietly.value).toBe(false)

      restartConnectingNoticeDelay()
      expect(websocketConnectingQuietly.value).toBe(true)
      setWebsocketConnectionStatus('reconnecting')
      expect(websocketConnectingQuietly.value).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it.each([
    { initial: true, online: false, expectedChange: true },
    { initial: false, online: false, expectedChange: false },
    { initial: false, online: true, expectedChange: true },
    { initial: true, online: true, expectedChange: false },
  ])(
    'sets browser online=$online with edge-triggered change=$expectedChange',
    ({ initial, online, expectedChange }) => {
      setWebsocketBrowserOnline(initial)
      expect(setWebsocketBrowserOnline(online)).toBe(expectedChange)
      expect(websocketBrowserOnline.value).toBe(online)
    },
  )
})
