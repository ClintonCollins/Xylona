export interface ReconnectingWebSocketOptions {
  connectionTimeoutMs?: number
  heartbeatIntervalMs?: number
  pongTimeoutMs?: number
  retryBaseDelayMs?: number
  retryMaxDelayMs?: number
  retryJitterRatio?: number
  startPaused?: boolean
}

interface OpenWaiter {
  resolve: () => void
  reject: (error: Error) => void
  timeoutId: ReturnType<typeof setTimeout>
}

const DEFAULT_CONNECTION_TIMEOUT_MS = 10_000
const DEFAULT_HEARTBEAT_INTERVAL_MS = 15_000
const DEFAULT_PONG_TIMEOUT_MS = 8_000
const DEFAULT_RETRY_BASE_DELAY_MS = 1_000
const DEFAULT_RETRY_MAX_DELAY_MS = 30_000
const DEFAULT_RETRY_JITTER_RATIO = 0.2

/**
 * A generation-safe WebSocket wrapper with liveness checks and bounded reconnection.
 */
export class ReconnectingWebSocket {
  public onopen: ((this: WebSocket, ev: Event) => void) | null = null
  public onmessage: ((this: WebSocket, ev: MessageEvent) => void) | null = null
  public onerror: ((this: WebSocket, ev: Event) => void) | null = null
  public onclose: ((this: WebSocket, ev: CloseEvent) => void) | null = null

  private _ws: WebSocket | null = null
  private readonly _url: string
  private readonly _protocols?: string | string[]
  private readonly _connectionTimeoutMs: number
  private readonly _heartbeatIntervalMs: number
  private readonly _pongTimeoutMs: number
  private readonly _retryBaseDelayMs: number
  private readonly _retryMaxDelayMs: number
  private readonly _retryJitterRatio: number

  private _generation = 0
  private _retryAttempt = 0
  private _paused: boolean
  private _disposed = false
  private _waitingForPong = false
  private _connectionTimer: ReturnType<typeof setTimeout> | null = null
  private _reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private _heartbeatTimer: ReturnType<typeof setTimeout> | null = null
  private _pongTimeoutTimer: ReturnType<typeof setTimeout> | null = null
  private _openWaiters: OpenWaiter[] = []

  constructor(
    url: string,
    protocols?: string | string[],
    optionsOrReconnectInterval: ReconnectingWebSocketOptions | number = {},
    legacyHeartbeatIntervalMs?: number,
  ) {
    const options =
      typeof optionsOrReconnectInterval === 'number'
        ? {
            retryBaseDelayMs: optionsOrReconnectInterval,
            heartbeatIntervalMs: legacyHeartbeatIntervalMs,
          }
        : optionsOrReconnectInterval

    this._url = url
    this._protocols = protocols
    this._connectionTimeoutMs = options.connectionTimeoutMs ?? DEFAULT_CONNECTION_TIMEOUT_MS
    this._heartbeatIntervalMs = options.heartbeatIntervalMs ?? DEFAULT_HEARTBEAT_INTERVAL_MS
    this._pongTimeoutMs = options.pongTimeoutMs ?? DEFAULT_PONG_TIMEOUT_MS
    this._retryBaseDelayMs = options.retryBaseDelayMs ?? DEFAULT_RETRY_BASE_DELAY_MS
    this._retryMaxDelayMs = options.retryMaxDelayMs ?? DEFAULT_RETRY_MAX_DELAY_MS
    this._retryJitterRatio = options.retryJitterRatio ?? DEFAULT_RETRY_JITTER_RATIO
    this._paused = options.startPaused ?? false

    if (!this._paused) {
      this._connect()
    }
  }

  public send(data: string | BufferSource | Blob): void {
    if (!this.isOpen() || this._ws === null) {
      throw new Error('WebSocket is not open')
    }
    this._ws.send(data)
  }

  public isOpen(): boolean {
    return (
      !this._paused &&
      !this._disposed &&
      this._ws !== null &&
      this._ws.readyState === WebSocket.OPEN
    )
  }

  public waitForOpen(timeoutMs: number = DEFAULT_CONNECTION_TIMEOUT_MS): Promise<void> {
    if (this.isOpen()) {
      return Promise.resolve()
    }
    if (this._disposed) {
      return Promise.reject(new Error('WebSocket has been disposed'))
    }
    if (this._paused) {
      return Promise.reject(new Error('WebSocket is paused'))
    }

    return new Promise<void>((resolve, reject) => {
      const timeoutId = setTimeout(() => {
        this._openWaiters = this._openWaiters.filter((waiter) => waiter.timeoutId !== timeoutId)
        reject(new Error('WebSocket did not open in time'))
      }, timeoutMs)

      this._openWaiters.push({ resolve, reject, timeoutId })
    })
  }

  /**
   * Retained for native WebSocket compatibility. A normal close pauses the client;
   * passing attemptReconnect starts a fresh connection immediately.
   */
  public close(code?: number, reason?: string, attemptReconnect: boolean = false): void {
    this._rejectOpenWaiters('WebSocket closed before open')
    if (attemptReconnect) {
      this.reconnectNow(code, reason)
      return
    }
    this.pause(code, reason)
  }

  public pause(code: number = 1000, reason: string = 'WebSocket paused'): void {
    if (this._disposed || this._paused) {
      return
    }

    this._paused = true
    this._clearReconnectTimer()
    this._rejectOpenWaiters('WebSocket paused before open')
    this._terminateCurrentSocket(code, reason, true)
  }

  public resume(): void {
    if (this._disposed) {
      return
    }

    this._paused = false
    if (this._ws !== null || this._reconnectTimer !== null) {
      return
    }
    this._connect()
  }

  public reconnectNow(code: number = 4002, reason: string = 'Reconnect requested'): void {
    if (this._disposed) {
      return
    }

    this._paused = false
    this._clearReconnectTimer()
    this._terminateCurrentSocket(code, reason, true)
    this._connect()
  }

  public probe(): void {
    if (this._disposed) {
      return
    }
    if (this._paused) {
      this.resume()
      return
    }
    if (this.isOpen()) {
      this._sendPing()
      return
    }
    this.reconnectNow(4002, 'Connection probe requested')
  }

  public dispose(code: number = 1000, reason: string = 'WebSocket disposed'): void {
    if (this._disposed) {
      return
    }

    this._disposed = true
    this._paused = true
    this._clearReconnectTimer()
    this._rejectOpenWaiters('WebSocket disposed before open')
    this._terminateCurrentSocket(code, reason, true)
    this.onopen = null
    this.onmessage = null
    this.onerror = null
    this.onclose = null
  }

  private _connect(): void {
    if (this._disposed || this._paused || this._ws !== null) {
      return
    }

    this._clearReconnectTimer()
    const generation = ++this._generation
    let socket: WebSocket
    try {
      socket = this._protocols
        ? new WebSocket(this._url, this._protocols)
        : new WebSocket(this._url)
    } catch {
      this._scheduleReconnect()
      return
    }
    this._ws = socket

    this._connectionTimer = setTimeout(() => {
      if (!this._isCurrent(socket, generation) || socket.readyState === WebSocket.OPEN) {
        return
      }
      this._terminateCurrentSocket(4001, 'Connection attempt timed out', true)
      this._scheduleReconnect()
    }, this._connectionTimeoutMs)

    socket.onopen = (event: Event) => {
      if (!this._isCurrent(socket, generation)) {
        return
      }
      this._clearConnectionTimer()
      this._resolveOpenWaiters()
      this._scheduleHeartbeat()
      this.onopen?.call(socket, event)
    }

    socket.onmessage = (event: MessageEvent) => {
      if (!this._isCurrent(socket, generation)) {
        return
      }
      this._markLiveness()
      this.onmessage?.call(socket, event)
    }

    socket.onerror = (event: Event) => {
      if (!this._isCurrent(socket, generation)) {
        return
      }
      this.onerror?.call(socket, event)
    }

    socket.onclose = (event: CloseEvent) => {
      if (!this._isCurrent(socket, generation)) {
        return
      }
      this._ws = null
      this._generation++
      this._clearConnectionTimers()
      this.onclose?.call(socket, event)
      this._scheduleReconnect()
    }
  }

  private _isCurrent(socket: WebSocket, generation: number): boolean {
    return (
      !this._disposed && !this._paused && this._ws === socket && this._generation === generation
    )
  }

  private _markLiveness(): void {
    this._retryAttempt = 0
    this._waitingForPong = false
    this._clearPongTimeout()
    this._scheduleHeartbeat()
  }

  private _scheduleReconnect(): void {
    if (this._disposed || this._paused || this._ws !== null || this._reconnectTimer !== null) {
      return
    }

    const delay = this._nextRetryDelay()
    this._reconnectTimer = setTimeout(() => {
      this._reconnectTimer = null
      this._connect()
    }, delay)
  }

  private _nextRetryDelay(): number {
    const attempt = this._retryAttempt++
    if (attempt === 0) {
      return 0
    }

    const exponentialDelay = this._retryBaseDelayMs * 2 ** (attempt - 1)
    const jitter = 1 + (Math.random() * 2 - 1) * this._retryJitterRatio
    return Math.round(Math.min(this._retryMaxDelayMs, exponentialDelay * jitter))
  }

  private _scheduleHeartbeat(): void {
    this._clearHeartbeatTimer()
    if (!this.isOpen()) {
      return
    }
    this._heartbeatTimer = setTimeout(() => {
      this._heartbeatTimer = null
      this._sendPing()
    }, this._heartbeatIntervalMs)
  }

  private _sendPing(): void {
    const socket = this._ws
    if (socket === null || !this.isOpen() || this._waitingForPong) {
      return
    }

    this._waitingForPong = true
    try {
      socket.send('ping')
    } catch {
      this._terminateCurrentSocket(4000, 'Heartbeat send failed', true)
      this._scheduleReconnect()
      return
    }

    this._pongTimeoutTimer = setTimeout(() => {
      if (!this._waitingForPong || this._ws !== socket) {
        return
      }
      this._terminateCurrentSocket(4000, 'No pong received', true)
      this._scheduleReconnect()
    }, this._pongTimeoutMs)
  }

  private _terminateCurrentSocket(code: number, reason: string, notifyClose: boolean): void {
    const socket = this._ws
    this._ws = null
    this._generation++
    this._clearConnectionTimers()
    if (socket === null) {
      return
    }

    if (notifyClose) {
      this.onclose?.call(socket, this._createCloseEvent(code, reason))
    }
    if (socket.readyState < WebSocket.CLOSING) {
      socket.close(code, reason)
    }
  }

  private _createCloseEvent(code: number, reason: string): CloseEvent {
    if (typeof CloseEvent === 'function') {
      return new CloseEvent('close', { code, reason, wasClean: false })
    }
    return Object.assign(new Event('close'), {
      code,
      reason,
      wasClean: false,
    }) as CloseEvent
  }

  private _clearConnectionTimers(): void {
    this._clearConnectionTimer()
    this._clearHeartbeatTimer()
    this._clearPongTimeout()
    this._waitingForPong = false
  }

  private _clearConnectionTimer(): void {
    if (this._connectionTimer === null) {
      return
    }
    clearTimeout(this._connectionTimer)
    this._connectionTimer = null
  }

  private _clearReconnectTimer(): void {
    if (this._reconnectTimer === null) {
      return
    }
    clearTimeout(this._reconnectTimer)
    this._reconnectTimer = null
  }

  private _clearHeartbeatTimer(): void {
    if (this._heartbeatTimer === null) {
      return
    }
    clearTimeout(this._heartbeatTimer)
    this._heartbeatTimer = null
  }

  private _clearPongTimeout(): void {
    if (this._pongTimeoutTimer === null) {
      return
    }
    clearTimeout(this._pongTimeoutTimer)
    this._pongTimeoutTimer = null
  }

  private _resolveOpenWaiters(): void {
    const waiters = this._openWaiters
    this._openWaiters = []
    for (const waiter of waiters) {
      clearTimeout(waiter.timeoutId)
      waiter.resolve()
    }
  }

  private _rejectOpenWaiters(message: string): void {
    const waiters = this._openWaiters
    this._openWaiters = []
    for (const waiter of waiters) {
      clearTimeout(waiter.timeoutId)
      waiter.reject(new Error(message))
    }
  }
}
