import { onMounted, onUnmounted } from 'vue'
import { useQuasar } from 'quasar'
import { XylonaEventBus } from 'src/utils/shared'

/**
 * Global composable that listens for server software install WebSocket events
 * and shows Quasar notifications. Also accepts an optional callback for
 * component-specific reactions (e.g., tab recalculation).
 */
export function useServerSoftwareInstall(
  onInstallEvent?: (gameServerId: string, status: string, softwareId: string) => void,
) {
  const $q = useQuasar()

  function handleEvent(gameServerId: string, status: string, error: string, softwareId: string) {
    if (status === 'complete') {
      $q.notify({
        type: 'positive',
        message: `Server software updated to ${softwareId}`,
        timeout: 5000,
      })
    } else if (status === 'failed') {
      $q.notify({
        type: 'negative',
        message: `Server software installation failed: ${error}`,
        timeout: 8000,
      })
    }

    onInstallEvent?.(gameServerId, status, softwareId)
  }

  onMounted(() => {
    XylonaEventBus.on('serverSoftwareInstall', handleEvent)
  })

  onUnmounted(() => {
    XylonaEventBus.off('serverSoftwareInstall', handleEvent)
  })
}
