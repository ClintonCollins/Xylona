import { computed, ref, type Ref } from 'vue'

import type { EnvironmentVariable, Game } from '@/proto/shared_pb'
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
  defaultEnvRows: Ref<EnvironmentVariable[]>
  linuxStartArgsTemplate: Ref<StartArgBlock[]>
  windowsStartArgsTemplate: Ref<StartArgBlock[]>
  startArgBlocklist: Ref<StartArgBlocklistEntry[]>
  downstreamImpactServers: Ref<DownstreamImpactServer[]>
}

export function useGameFormDirtyState(options: UseGameFormDirtyStateOptions) {
  const initialFormSnapshot = ref('')
  const initialDefaultEnvSnapshot = ref('')

  function serializeSnapshot(value: unknown): string {
    return JSON.stringify(value, (_key, snapshotValue) =>
      typeof snapshotValue === 'bigint' ? snapshotValue.toString() : snapshotValue,
    )
  }

  function takeFormSnapshotValue() {
    return {
      game: options.game.value,
      defaultPort: options.defaultPort.value,
      defaultQueryPort: options.defaultQueryPort.value,
      configSchemas: options.configSchemas.value,
      linuxStartArgsTemplate: options.linuxStartArgsTemplate.value,
      windowsStartArgsTemplate: options.windowsStartArgsTemplate.value,
      startArgBlocklist: options.startArgBlocklist.value,
      downstreamImpactServers: options.downstreamImpactServers.value,
    }
  }

  function takeFormSnapshot(): string {
    return serializeSnapshot(takeFormSnapshotValue())
  }

  function takeDefaultEnvSnapshot(): string {
    return serializeSnapshot(options.defaultEnvRows.value)
  }

  function takeSnapshot(): string {
    return JSON.stringify(
      {
        form: takeFormSnapshotValue(),
        defaultEnvRows: options.defaultEnvRows.value,
      },
      (_key, value) => (typeof value === 'bigint' ? value.toString() : value),
    )
  }

  function commitSnapshot(): void {
    initialFormSnapshot.value = takeFormSnapshot()
    initialDefaultEnvSnapshot.value = takeDefaultEnvSnapshot()
  }

  function commitFormSnapshot(): void {
    initialFormSnapshot.value = takeFormSnapshot()
  }

  function commitDefaultEnvSnapshot(): void {
    initialDefaultEnvSnapshot.value = takeDefaultEnvSnapshot()
  }

  const isDirty = computed(() => {
    const formDirty =
      initialFormSnapshot.value !== '' && takeFormSnapshot() !== initialFormSnapshot.value
    const defaultEnvDirty =
      initialDefaultEnvSnapshot.value !== '' &&
      takeDefaultEnvSnapshot() !== initialDefaultEnvSnapshot.value

    return formDirty || defaultEnvDirty
  })

  return {
    isDirty,
    takeSnapshot,
    commitSnapshot,
    commitFormSnapshot,
    commitDefaultEnvSnapshot,
  }
}
