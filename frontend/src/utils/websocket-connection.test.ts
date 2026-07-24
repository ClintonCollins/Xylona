import { afterEach, describe, expect, it } from 'vitest'

import {
  setWebsocketBrowserOnline,
  setWebsocketConnectionStatus,
  websocketBrowserOnline,
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
