import { afterEach, describe, expect, it, vi } from 'vitest'

import { ReconnectingWebSocket } from './websocket'

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static instances: FakeWebSocket[] = []

  readonly url: string
  readyState = FakeWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  constructor(url: string) {
    this.url = url
    FakeWebSocket.instances.push(this)
  }

  send(_data: string | ArrayBuffer | Blob | ArrayBufferView): void {}

  close(): void {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close'))
  }

  triggerOpen(): void {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }
}

describe('ReconnectingWebSocket.waitForOpen', () => {
  const originalWebSocket = globalThis.WebSocket

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    FakeWebSocket.instances = []
    vi.useRealTimers()
  })

  it('resolves immediately when the websocket is already open', async () => {
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket
    const socket = new ReconnectingWebSocket('ws://localhost/test')
    const fake = FakeWebSocket.instances[0]

    fake.triggerOpen()

    await expect(socket.waitForOpen()).resolves.toBeUndefined()
    socket.close()
  })

  it('waits for the websocket to open before resolving', async () => {
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket
    const socket = new ReconnectingWebSocket('ws://localhost/test')
    const fake = FakeWebSocket.instances[0]

    const waitForOpen = socket.waitForOpen()
    fake.triggerOpen()

    await expect(waitForOpen).resolves.toBeUndefined()
    socket.close()
  })

  it('rejects when the websocket does not open before the timeout', async () => {
    vi.useFakeTimers()
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket
    const socket = new ReconnectingWebSocket('ws://localhost/test')

    const waitForOpen = socket.waitForOpen(100)
    const assertion = expect(waitForOpen).rejects.toThrow('WebSocket did not open in time')
    await vi.advanceTimersByTimeAsync(101)

    await assertion
    socket.close()
  })

  it('rejects pending waiters when close is called before open', async () => {
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket
    const socket = new ReconnectingWebSocket('ws://localhost/test')

    const waitForOpen = socket.waitForOpen()
    const assertion = expect(waitForOpen).rejects.toThrow('WebSocket closed before open')
    socket.close()

    await assertion
  })

  it('rejects waiters from the previous connection cycle before reconnecting', async () => {
    vi.useFakeTimers()
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket
    const socket = new ReconnectingWebSocket('ws://localhost/test')

    const waitForOpen = socket.waitForOpen()
    const assertion = expect(waitForOpen).rejects.toThrow('WebSocket closed before open')
    socket.close(undefined, undefined, true)

    await vi.advanceTimersByTimeAsync(5000)
    expect(FakeWebSocket.instances).toHaveLength(2)

    FakeWebSocket.instances[1].triggerOpen()

    await assertion
  })
})
