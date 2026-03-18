/**
 * A WebSocket wrapper that automatically reconnects and supports heartbeat checks.
 */
export class ReconnectingWebSocket {
  /**
   * Event handler for the `onopen` event.
   * @type {(this: WebSocket, ev: Event) => any | null}
   */
  public onopen: ((this: WebSocket, ev: Event) => any) | null = null

  /**
   * Event handler for the `onmessage` event.
   * @type {(this: WebSocket, ev: MessageEvent) => any | null}
   */
  public onmessage: ((this: WebSocket, ev: MessageEvent) => any) | null = null

  /**
   * Event handler for the `onerror` event.
   * @type {(this: WebSocket, ev: Event) => any | null}
   */
  public onerror: ((this: WebSocket, ev: Event) => any) | null = null

  /**
   * Event handler for the `onclose` event.
   * @type {(this: WebSocket, ev: CloseEvent) => any) | null}
   */
  public onclose: ((this: WebSocket, ev: CloseEvent) => any) | null = null

  private _ws: WebSocket | null = null
  private readonly _url: string
  private readonly _protocols?: string | string[]

  // Internal reconnection/heartbeat settings and timers
  private readonly _reconnectInterval: number
  private readonly _heartbeatInterval: number
  private _reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private _heartbeatTimer: ReturnType<typeof setInterval> | null = null

  // Ping–pong tracking
  private _waitingForPong: boolean = false
  private _pongTimeoutTimer: ReturnType<typeof setTimeout> | null = null

  // Track whether the close was initiated by the user
  private _manualClose: boolean = false

  /**
   * Creates an instance of ReconnectingWebSocket.
   * @param {string} url - The WebSocket URL.
   * @param {string | string[]} [protocols] - Optional protocols.
   * @param {number} [reconnectInterval=5000] - Interval in ms between reconnection attempts.
   * @param {number} [heartbeatInterval=15000] - Interval in ms between heartbeat checks.
   */
  constructor(
    url: string,
    protocols?: string | string[],
    reconnectInterval: number = 5000, // 5 seconds for reconnect attempts
    heartbeatInterval: number = 15000, // 15 seconds between "ping" checks
  ) {
    this._url = url
    this._protocols = protocols
    this._reconnectInterval = reconnectInterval
    this._heartbeatInterval = heartbeatInterval

    // Initiate the first connection
    this._connect()
  }

  /**
   * Send a message through the WebSocket.
   * If the WebSocket isn't open, this will throw (like native WebSocket).
   */
  public send(data: string | ArrayBuffer | Blob | ArrayBufferView): void {
    if (!this._ws || this._ws.readyState !== WebSocket.OPEN) {
      throw new Error('WebSocket is not open')
    }
    this._ws.send(data)
  }

  /**
   * Closes the WebSocket connection and prevents auto-reconnect.
   * @param {number} [code] - The status code explaining why the connection is being closed.
   * @param {string} [reason] - A human-readable string explaining why the connection is closing.
   * @param attemptReconnect - Whether to attempt to reconnect after closing.
   */
  public close(code?: number, reason?: string, attemptReconnect: boolean = false): void {
    this._manualClose = !attemptReconnect

    // Clear any pending reconnection
    if (this._reconnectTimer) {
      clearTimeout(this._reconnectTimer)
      this._reconnectTimer = null
    }

    // Stop heartbeat
    this._stopHeartbeat()

    // Close the actual WebSocket if it's open/connecting
    if (this._ws && this._ws.readyState < WebSocket.CLOSING) {
      this._ws.close(code, reason)
    }
  }

  /**
   * Establish the WebSocket connection and set up handlers.
   * Internal only.
   */
  private _connect(): void {
    this._ws = this._protocols
      ? new WebSocket(this._url, this._protocols)
      : new WebSocket(this._url)

    // Handlers
    this._ws.onopen = (event: Event) => {
      if (this.onopen) {
        this.onopen.call(this._ws, event)
      }
      // Start heartbeat once the connection is open
      this._startHeartbeat()
    }

    this._ws.onmessage = (event: MessageEvent) => {
      // Check for "pong"
      if (typeof event.data === 'string' && event.data === 'pong') {
        // Received expected pong
        this._clearPongTimeout()
        this._waitingForPong = false
      }

      if (this.onmessage) {
        this.onmessage.call(this._ws, event)
      }
    }

    this._ws.onerror = (event: Event) => {
      if (this.onerror) {
        this.onerror.call(this._ws, event)
      }
      // We rely on onclose to handle reconnection
    }

    this._ws.onclose = (event: CloseEvent) => {
      // Stop heartbeat
      this._stopHeartbeat()

      if (this.onclose) {
        this.onclose.call(this._ws, event)
      }

      // Attempt to reconnect only if it wasn't a manual close
      if (!this._manualClose) {
        this._scheduleReconnect()
      }
    }
  }

  /**
   * Schedules a reconnection after `_reconnectInterval` ms.
   */
  private _scheduleReconnect(): void {
    if (this._reconnectTimer) {
      clearTimeout(this._reconnectTimer)
    }
    this._reconnectTimer = setTimeout(() => {
      this._connect()
    }, this._reconnectInterval)
  }

  /**
   * Begin sending periodic ping messages to check the connection.
   */
  private _startHeartbeat(): void {
    this._stopHeartbeat() // ensure no duplicate timers

    // Send a ping every `_heartbeatInterval`
    this._heartbeatTimer = setInterval(() => {
      this._sendPing()
    }, this._heartbeatInterval)
  }

  /**
   * Stop sending the periodic heartbeat.
   */
  private _stopHeartbeat(): void {
    if (this._heartbeatTimer) {
      clearInterval(this._heartbeatTimer)
      this._heartbeatTimer = null
    }
    this._clearPongTimeout()
    this._waitingForPong = false
  }

  /**
   * Actually send a "ping" frame and set up a 10-second timeout for "pong".
   */
  private _sendPing(): void {
    if (!this._ws || this._ws.readyState !== WebSocket.OPEN) {
      return
    }

    // If we were still waiting for a pong from the previous ping,
    // it means the connection is unresponsive—force reconnect.
    if (this._waitingForPong) {
      this._forceReconnect()
      return
    }

    // Mark that we're expecting a pong soon
    this._waitingForPong = true

    // Send "ping"
    try {
      this._ws.send('ping')
    } catch (e) {
      // If sending fails, just try to reconnect
      this._forceReconnect()
      return
    }

    // Set up a 10-second timeout for receiving "pong"
    this._pongTimeoutTimer = setTimeout(() => {
      if (this._waitingForPong) {
        // We never got a pong—connection is stale
        this._forceReconnect()
      }
    }, 10000)
  }

  /**
   * Clears the timer waiting for the server's "pong".
   */
  private _clearPongTimeout(): void {
    if (this._pongTimeoutTimer) {
      clearTimeout(this._pongTimeoutTimer)
      this._pongTimeoutTimer = null
    }
  }

  /**
   * Force-close and schedule a reconnect (if not manually closed).
   */
  private _forceReconnect(): void {
    if (!this._ws) return

    // This will call onclose => scheduleReconnect if _manualClose = false
    this._ws.close(4000, 'No pong received')
  }
}
