import { onMounted, onUnmounted } from 'vue'
import { XylonaEventBus } from '@/utils/shared'

/**
 * Global composable that listens for server software install WebSocket events
 * and forwards them to an optional callback for component-specific reactions
 * (for example, recalculating game server tabs).
 */
export function useServerSoftwareInstall(
  onInstallEvent?: (gameServerId: string, status: string, softwareId: string) => void,
) {
  function handleEvent(
    gameServerId: string,
    _gameServerName: string,
    status: string,
    _error: string,
    softwareId: string,
  ) {
    onInstallEvent?.(gameServerId, status, softwareId)
  }

  onMounted(() => {
    XylonaEventBus.on('serverSoftwareInstall', handleEvent)
  })

  onUnmounted(() => {
    XylonaEventBus.off('serverSoftwareInstall', handleEvent)
  })
}
