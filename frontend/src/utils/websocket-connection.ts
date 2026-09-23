import { computed, readonly, ref } from 'vue'

export type WebsocketConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

const connectionStatus = ref<WebsocketConnectionStatus>('connecting')
const hasConnected = ref(false)
const browserOnline = ref(typeof navigator === 'undefined' ? true : navigator.onLine)
const connectionEpoch = ref(0)

export const websocketConnectionStatus = readonly(connectionStatus)
export const websocketHasConnected = readonly(hasConnected)
export const websocketBrowserOnline = readonly(browserOnline)
export const websocketConnectionEpoch = readonly(connectionEpoch)
export const websocketStateAuthoritative = computed(() => connectionStatus.value === 'connected')

/** How long a fresh connection may take before the page says it is still connecting. */
export const CONNECTING_NOTICE_DELAY_MS = 1_500

const connectingNoticeDue = ref(false)
let connectingNoticeTimer: ReturnType<typeof setTimeout> | undefined

/**
 * True while a fresh connection is still inside its quiet period. A normal page
 * load connects well within it, so a "connecting" notice would only flash.
 * Reconnecting and disconnected are never quiet.
 */
export const websocketConnectingQuietly = computed(
  () => connectionStatus.value === 'connecting' && !connectingNoticeDue.value,
)

/** Starts the quiet period for a fresh controller connection. */
export function restartConnectingNoticeDelay(): void {
  clearTimeout(connectingNoticeTimer)
  connectingNoticeDue.value = false
  connectingNoticeTimer = setTimeout(() => {
    connectingNoticeDue.value = true
  }, CONNECTING_NOTICE_DELAY_MS)
}

export function setWebsocketConnectionStatus(status: WebsocketConnectionStatus): boolean {
  if (connectionStatus.value === status) {
    return false
  }

  connectionStatus.value = status
  if (status === 'connected') {
    hasConnected.value = true
    connectionEpoch.value++
  }
  return true
}

export function setWebsocketBrowserOnline(online: boolean): boolean {
  if (browserOnline.value === online) {
    return false
  }

  browserOnline.value = online
  return true
}
