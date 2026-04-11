import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'

import { useGameFormPlatform } from './useGameFormPlatform'

describe('useGameFormPlatform', () => {
  it('resolves to the selected platform when both platforms are supported', () => {
    const game = ref(
      create(GameSchema, {
        windowsSupport: true,
        linuxSupport: true,
      }),
    )

    const platform = useGameFormPlatform(game)

    expect(platform.activePlatformResolved.value).toBe('windows')

    platform.activePlatform.value = 'linux'

    expect(platform.activePlatformResolved.value).toBe('linux')
  })

  it('resolves to the only supported platform and syncs selection from the game', () => {
    const windowsOnlyGame = ref(
      create(GameSchema, {
        windowsSupport: true,
        linuxSupport: false,
      }),
    )
    const linuxOnlyGame = ref(
      create(GameSchema, {
        windowsSupport: false,
        linuxSupport: true,
      }),
    )

    const windowsPlatform = useGameFormPlatform(windowsOnlyGame)
    const linuxPlatform = useGameFormPlatform(linuxOnlyGame)

    windowsPlatform.activePlatform.value = 'linux'
    linuxPlatform.activePlatform.value = 'windows'

    windowsPlatform.syncActivePlatformFromGame()
    linuxPlatform.syncActivePlatformFromGame()

    expect(windowsPlatform.activePlatformResolved.value).toBe('windows')
    expect(windowsPlatform.activePlatform.value).toBe('windows')
    expect(linuxPlatform.activePlatformResolved.value).toBe('linux')
    expect(linuxPlatform.activePlatform.value).toBe('linux')
  })

  it('resolves to null when no platform is supported', () => {
    const game = ref(
      create(GameSchema, {
        windowsSupport: false,
        linuxSupport: false,
      }),
    )

    const platform = useGameFormPlatform(game)

    expect(platform.activePlatformResolved.value).toBeNull()
  })
})
