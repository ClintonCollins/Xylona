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
