import { computed, onScopeDispose, ref, watch, type Ref } from 'vue'

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

  // Serializing the whole game on every keystroke is slow, so an edit marks the form changed
  // straight away and the snapshot comparison waits until typing pauses, clearing the flag when
  // the edit was undone.
  const formChanged = ref(false)
  let recheckTimer: ReturnType<typeof setTimeout> | undefined

  watch(
    takeFormSnapshotValue,
    () => {
      formChanged.value = true
      clearTimeout(recheckTimer)
      recheckTimer = setTimeout(() => {
        formChanged.value = takeFormSnapshot() !== initialFormSnapshot.value
      }, 300)
    },
    { deep: true, flush: 'sync' },
  )
  onScopeDispose(() => clearTimeout(recheckTimer), true)

  function commitSnapshot(): void {
    commitFormSnapshot()
    initialDefaultEnvSnapshot.value = takeDefaultEnvSnapshot()
  }

  function commitFormSnapshot(): void {
    clearTimeout(recheckTimer)
    initialFormSnapshot.value = takeFormSnapshot()
    formChanged.value = false
  }

  function commitDefaultEnvSnapshot(): void {
    initialDefaultEnvSnapshot.value = takeDefaultEnvSnapshot()
  }

  const defaultEnvDirty = computed(
    () =>
      initialDefaultEnvSnapshot.value !== '' &&
      takeDefaultEnvSnapshot() !== initialDefaultEnvSnapshot.value,
  )

  const isDirty = computed(
    () => (initialFormSnapshot.value !== '' && formChanged.value) || defaultEnvDirty.value,
  )

  return {
    defaultEnvDirty,
    isDirty,
    takeSnapshot,
    commitSnapshot,
    commitFormSnapshot,
    commitDefaultEnvSnapshot,
  }
}
