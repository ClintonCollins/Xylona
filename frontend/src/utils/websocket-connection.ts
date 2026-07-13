import { computed, readonly, ref } from 'vue'

export type WebsocketConnectionStatus = 'connecting' | 'connected' | 'reconnecting' | 'disconnected'

const connectionStatus = ref<WebsocketConnectionStatus>('connecting')
const hasConnected = ref(false)

export const websocketConnectionStatus = readonly(connectionStatus)
export const websocketHasConnected = readonly(hasConnected)
export const websocketStateAuthoritative = computed(() => connectionStatus.value === 'connected')

export function setWebsocketConnectionStatus(status: WebsocketConnectionStatus): void {
  connectionStatus.value = status
  if (status === 'connected') {
    hasConnected.value = true
  }
}
