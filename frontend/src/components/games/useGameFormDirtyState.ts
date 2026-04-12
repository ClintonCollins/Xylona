import { computed, ref, type Ref } from 'vue'

import type { Game } from '@/proto/shared_pb'
import type { ConfigSchemaEntry } from './config-schema-types'
import type { StartArgBlock, StartArgBlocklistEntry } from '@/components/game_servers/start-args'

interface DownstreamImpactServer {
  name: string
  patchCount: number
}

interface UseGameFormDirtyStateOptions {
  game: Ref<Game>
  defaultPort: Ref<number | null>
  defaultQueryPort: Ref<number | null>
  configSchemas: Ref<ConfigSchemaEntry[]>
  linuxStartArgsTemplate: Ref<StartArgBlock[]>
  windowsStartArgsTemplate: Ref<StartArgBlock[]>
  startArgBlocklist: Ref<StartArgBlocklistEntry[]>
  downstreamImpactServers: Ref<DownstreamImpactServer[]>
}

export function useGameFormDirtyState(options: UseGameFormDirtyStateOptions) {
  const initialSnapshot = ref('')

  function takeSnapshot(): string {
    return JSON.stringify(
      {
        game: options.game.value,
        defaultPort: options.defaultPort.value,
        defaultQueryPort: options.defaultQueryPort.value,
        configSchemas: options.configSchemas.value,
        linuxStartArgsTemplate: options.linuxStartArgsTemplate.value,
        windowsStartArgsTemplate: options.windowsStartArgsTemplate.value,
        startArgBlocklist: options.startArgBlocklist.value,
        downstreamImpactServers: options.downstreamImpactServers.value,
      },
      (_key, value) => (typeof value === 'bigint' ? value.toString() : value),
    )
  }

  function commitSnapshot(): void {
    initialSnapshot.value = takeSnapshot()
  }

  const isDirty = computed(() => {
    if (!initialSnapshot.value) {
      return false
    }

    return takeSnapshot() !== initialSnapshot.value
  })

  return {
    isDirty,
    takeSnapshot,
    commitSnapshot,
  }
}
