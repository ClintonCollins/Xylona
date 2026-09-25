import { create } from '@bufbuild/protobuf'
import { inject, onBeforeUnmount, onMounted, ref, watch, type InjectionKey, type Ref } from 'vue'

import {
  GetGameServerReadinessRequestSchema,
  type GameServerReadinessItem,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

export interface GameServerReadiness {
  /** Latest setup checks; actions that return fresh items assign them here. */
  items: Ref<GameServerReadinessItem[]>
  reload: () => Promise<void>
}

/** Shared by the server layout so every server page reads one copy. */
export const gameServerReadinessKey: InjectionKey<GameServerReadiness> =
  Symbol('gameServerReadiness')

const readinessLabels: Record<string, string> = {
  valheim_runtime: 'Valheim runtime',
  minecraft_eula: 'Minecraft EULA',
  steam_gslt: 'Steam GSLT',
  hytale_account: 'Hytale account',
  sunkenland_world: 'Sunkenland world',
  dragonwilds_config: 'Dragonwilds configuration',
}

/** Items fixed in the server's configuration files. */
const configReadinessKinds = new Set(['dragonwilds_config'])

export function readinessLabel(kind: string): string {
  return readinessLabels[kind] ?? 'Setup'
}

export function isConfigReadinessItem(item: GameServerReadinessItem): boolean {
  return configReadinessKinds.has(item.kind)
}

/** The first setup check the controller would refuse a start for, if any. */
export function findStartBlocker(
  items: GameServerReadinessItem[],
): GameServerReadinessItem | undefined {
  return items.find((item) => item.required && item.blocking)
}

/**
 * Loads a server's readiness when the id is known and again whenever the page
 * regains focus, since setup is often fixed elsewhere.
 */
export function createGameServerReadiness(serverId: Ref<string>): GameServerReadiness {
  const items = ref<GameServerReadinessItem[]>([])
  let requestSequence = 0

  async function reload(): Promise<void> {
    const id = serverId.value
    if (id === '') return
    const sequence = ++requestSequence
    try {
      const response = await GetXylonaClient().getGameServerReadiness(
        create(GetGameServerReadinessRequestSchema, { serverId: id }),
      )
      if (sequence === requestSequence) items.value = response.items
    } catch (error) {
      console.error(error)
    }
  }

  function onWindowFocus(): void {
    void reload()
  }

  watch(serverId, () => {
    items.value = []
    void reload()
  })

  onMounted(() => {
    window.addEventListener('focus', onWindowFocus)
    void reload()
  })

  onBeforeUnmount(() => {
    window.removeEventListener('focus', onWindowFocus)
  })

  return { items, reload }
}

/** The layout's shared readiness, or a local one when rendered on its own. */
export function useGameServerReadiness(serverId: Ref<string>): GameServerReadiness {
  return inject(gameServerReadinessKey, null) ?? createGameServerReadiness(serverId)
}
