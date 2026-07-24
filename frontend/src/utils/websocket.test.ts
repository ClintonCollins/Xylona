import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ReconnectingWebSocket, type ReconnectingWebSocketOptions } from './websocket'

class FakeWebSocket {
  static readonly CONNECTING = 0
  static readonly OPEN = 1
  static readonly CLOSING = 2
  static readonly CLOSED = 3
  static instances: FakeWebSocket[] = []

  readonly url: string
  readonly protocols?: string | string[]
  readonly sent: Array<string | ArrayBufferLike | Blob | ArrayBufferView> = []
  readonly closeCalls: Array<{ code?: number; reason?: string }> = []
  readyState = FakeWebSocket.CONNECTING
  onopen: ((event: Event) => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onerror: ((event: Event) => void) | null = null
  onclose: ((event: CloseEvent) => void) | null = null

  constructor(url: string, protocols?: string | string[]) {
    this.url = url
    this.protocols = protocols
    FakeWebSocket.instances.push(this)
  }

  send(data: string | ArrayBufferLike | Blob | ArrayBufferView): void {
    this.sent.push(data)
  }

  close(code?: number, reason?: string): void {
    this.closeCalls.push({ code, reason })
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close', { code, reason }))
  }

  triggerOpen(): void {
    this.readyState = FakeWebSocket.OPEN
    this.onopen?.(new Event('open'))
  }

  triggerMessage(data: unknown): void {
    this.onmessage?.(new MessageEvent('message', { data }))
  }

  triggerClose(code: number = 1006, reason: string = 'Connection lost'): void {
    this.readyState = FakeWebSocket.CLOSED
    this.onclose?.(new CloseEvent('close', { code, reason }))
  }
}

const originalWebSocket = globalThis.WebSocket
const clients: ReconnectingWebSocket[] = []

function createClient(options: ReconnectingWebSocketOptions = {}): ReconnectingWebSocket {
  const client = new ReconnectingWebSocket('ws://localhost/test', undefined, options)
  clients.push(client)
  return client
}

function getSocket(index: number = FakeWebSocket.instances.length - 1): FakeWebSocket {
  const socket = FakeWebSocket.instances[index]
  if (socket === undefined) {
    throw new Error(`expected websocket instance ${index} to exist`)
  }
  return socket
}

describe('ReconnectingWebSocket', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    globalThis.WebSocket = FakeWebSocket as unknown as typeof WebSocket
  })

  afterEach(() => {
    for (const client of clients.splice(0)) {
      client.dispose()
    }
    vi.clearAllTimers()
    vi.restoreAllMocks()
    vi.useRealTimers()
    globalThis.WebSocket = originalWebSocket
    FakeWebSocket.instances = []
  })

  it.each([
    {
      name: 'resolves immediately for an open socket',
      arrange: (client: ReconnectingWebSocket, socket: FakeWebSocket) => {
        socket.triggerOpen()
        return client.waitForOpen()
      },
    },
    {
      name: 'resolves a pending waiter on open',
      arrange: (client: ReconnectingWebSocket, socket: FakeWebSocket) => {
        const result = client.waitForOpen()
        socket.triggerOpen()
        return result
      },
    },
  ])('$name', async ({ arrange }) => {
    const client = createClient()
    await expect(arrange(client, getSocket())).resolves.toBeUndefined()
  })

  it('rejects a waiter that reaches its own timeout', async () => {
    const client = createClient()
    const waitForOpen = client.waitForOpen(100)
    const assertion = expect(waitForOpen).rejects.toThrow('WebSocket did not open in time')

    await vi.advanceTimersByTimeAsync(100)

    await assertion
  })

  it('times out a connection attempt after 10 seconds and retries immediately', async () => {
    const client = createClient()
    const onclose = vi.fn()
    client.onclose = onclose

    await vi.advanceTimersByTimeAsync(9_999)
    expect(FakeWebSocket.instances).toHaveLength(1)

    await vi.advanceTimersByTimeAsync(1)
    await vi.advanceTimersToNextTimerAsync()

    expect(onclose).toHaveBeenCalledOnce()
    expect(onclose.mock.calls[0]?.[0]).toMatchObject({
      code: 4001,
      reason: 'Connection attempt timed out',
    })
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('uses a 15-second idle heartbeat and reconnects after an 8-second pong timeout', async () => {
    createClient()
    const socket = getSocket()
    socket.triggerOpen()

    await vi.advanceTimersByTimeAsync(14_999)
    expect(socket.sent).toEqual([])

    await vi.advanceTimersByTimeAsync(1)
    expect(socket.sent).toEqual(['ping'])

    await vi.advanceTimersByTimeAsync(7_999)
    expect(FakeWebSocket.instances).toHaveLength(1)

    await vi.advanceTimersByTimeAsync(1)
    await vi.advanceTimersToNextTimerAsync()
    expect(socket.closeCalls).toContainEqual({ code: 4000, reason: 'No pong received' })
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('treats every received frame as liveness and restarts the idle heartbeat', async () => {
    createClient()
    const socket = getSocket()
    socket.triggerOpen()

    await vi.advanceTimersByTimeAsync(15_000)
    expect(socket.sent).toEqual(['ping'])

    socket.triggerMessage('application data')
    await vi.advanceTimersByTimeAsync(14_999)
    expect(socket.closeCalls).toEqual([])
    expect(socket.sent).toEqual(['ping'])

    await vi.advanceTimersByTimeAsync(1)
    expect(socket.sent).toEqual(['ping', 'ping'])
  })

  it('backs off only after repeated failures and resets only after received liveness', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.5)
    createClient({
      retryBaseDelayMs: 1_000,
      retryMaxDelayMs: 30_000,
      retryJitterRatio: 0.2,
    })

    getSocket().triggerClose()
    await vi.advanceTimersByTimeAsync(0)
    expect(FakeWebSocket.instances).toHaveLength(2)

    getSocket().triggerOpen()
    getSocket().triggerClose()
    await vi.advanceTimersByTimeAsync(999)
    expect(FakeWebSocket.instances).toHaveLength(2)
    await vi.advanceTimersByTimeAsync(1)
    expect(FakeWebSocket.instances).toHaveLength(3)

    getSocket().triggerOpen()
    getSocket().triggerClose()
    await vi.advanceTimersByTimeAsync(1_999)
    expect(FakeWebSocket.instances).toHaveLength(3)
    await vi.advanceTimersByTimeAsync(1)
    expect(FakeWebSocket.instances).toHaveLength(4)

    getSocket().triggerOpen()
    getSocket().triggerMessage('pong')
    getSocket().triggerClose()
    await vi.advanceTimersByTimeAsync(0)
    expect(FakeWebSocket.instances).toHaveLength(5)
  })

  it('caps exponential retry delays at 30 seconds', async () => {
    vi.spyOn(Math, 'random').mockReturnValue(0.5)
    createClient()
    const retryDelays = [0, 1_000, 2_000, 4_000, 8_000, 16_000, 30_000, 30_000]

    for (const [index, delay] of retryDelays.entries()) {
      getSocket().triggerClose()
      if (delay > 0) {
        await vi.advanceTimersByTimeAsync(delay - 1)
        expect(FakeWebSocket.instances).toHaveLength(index + 1)
        await vi.advanceTimersByTimeAsync(1)
      } else {
        await vi.advanceTimersByTimeAsync(0)
      }
      expect(FakeWebSocket.instances).toHaveLength(index + 2)
    }
  })

  it('ignores callbacks from sockets superseded by an explicit reconnect', () => {
    const client = createClient()
    const onopen = vi.fn()
    const onmessage = vi.fn()
    const onclose = vi.fn()
    client.onopen = onopen
    client.onmessage = onmessage
    client.onclose = onclose

    const staleSocket = getSocket()
    const staleOpen = staleSocket.onopen
    const staleMessage = staleSocket.onmessage
    const staleClose = staleSocket.onclose
    client.reconnectNow()
    expect(FakeWebSocket.instances).toHaveLength(2)
    expect(onclose).toHaveBeenCalledOnce()

    staleOpen?.(new Event('open'))
    staleMessage?.(new MessageEvent('message', { data: 'late' }))
    staleClose?.(new CloseEvent('close'))

    expect(onopen).not.toHaveBeenCalled()
    expect(onmessage).not.toHaveBeenCalled()
    expect(onclose).toHaveBeenCalledOnce()
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('keeps one active socket and no stale retry after repeated reconnect requests', async () => {
    const client = createClient()

    client.reconnectNow()
    client.reconnectNow()
    client.reconnectNow()
    await vi.advanceTimersByTimeAsync(30_000)

    const activeSockets = FakeWebSocket.instances.filter(
      (socket) => socket.readyState < FakeWebSocket.CLOSING,
    )
    expect(activeSockets).toHaveLength(1)
  })

  it('pauses retries and resumes with a fresh socket', async () => {
    const client = createClient()
    const firstSocket = getSocket()
    firstSocket.triggerOpen()

    client.pause()
    expect(client.isOpen()).toBe(false)
    await vi.advanceTimersByTimeAsync(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1)

    client.resume()
    expect(FakeWebSocket.instances).toHaveLength(2)
  })

  it('probes an open socket immediately', () => {
    const client = createClient()
    const socket = getSocket()
    socket.triggerOpen()

    client.probe()

    expect(socket.sent).toEqual(['ping'])
  })

  it('disposes all work and rejects later open waits', async () => {
    const client = createClient()
    client.dispose()

    await expect(client.waitForOpen()).rejects.toThrow('WebSocket has been disposed')
    await vi.advanceTimersByTimeAsync(60_000)
    expect(FakeWebSocket.instances).toHaveLength(1)
  })
})
