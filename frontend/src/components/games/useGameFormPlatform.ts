import { computed, ref, type Ref } from 'vue'

import type { Game } from '@/proto/shared_pb'

import type { Platform } from './GameFormTypes'

export function useGameFormPlatform(game: Ref<Game>) {
  const activePlatform = ref<Platform>('windows')

  const activePlatformResolved = computed<Platform | null>(() => {
    if (game.value.windowsSupport && game.value.linuxSupport) {
      return activePlatform.value
    }
    if (game.value.windowsSupport) {
      return 'windows'
    }
    if (game.value.linuxSupport) {
      return 'linux'
    }

    return null
  })

  function syncActivePlatformFromGame(): void {
    if (game.value.windowsSupport) {
      activePlatform.value = 'windows'
      return
    }

    if (game.value.linuxSupport) {
      activePlatform.value = 'linux'
    }
  }

  return {
    activePlatform,
    activePlatformResolved,
    syncActivePlatformFromGame,
  }
}
