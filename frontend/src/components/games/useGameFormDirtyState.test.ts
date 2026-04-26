import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { EnvironmentVariableSchema, GameSchema } from '@/proto/shared_pb'
import type { ConfigSchemaEntry } from './config-schema-types'
import type { StartArgBlock, StartArgBlocklistEntry } from '@/components/game_servers/start-args'
import { useGameFormDirtyState } from './useGameFormDirtyState'

function createState() {
  const game = ref(create(GameSchema, { id: 'minecraft', name: 'Minecraft' }))
  const defaultPort = ref<number | null>(25565)
  const defaultQueryPort = ref<number | null>(25565)
  const configSchemas = ref<ConfigSchemaEntry[]>([])
  const defaultEnvRows = ref([
    create(EnvironmentVariableSchema, { name: 'JAVA_HOME', value: '/opt/java' }),
  ])
  const linuxStartArgsTemplate = ref<StartArgBlock[]>([])
  const windowsStartArgsTemplate = ref<StartArgBlock[]>([])
  const startArgBlocklist = ref<StartArgBlocklistEntry[]>([])
  const downstreamImpactServers = ref<Array<{ name: string; patchCount: number }>>([])

  return {
    game,
    defaultPort,
    defaultQueryPort,
    configSchemas,
    defaultEnvRows,
    linuxStartArgsTemplate,
    windowsStartArgsTemplate,
    startArgBlocklist,
    downstreamImpactServers,
  }
}

describe('useGameFormDirtyState', () => {
  it('stays clean until a baseline snapshot is committed', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    state.game.value.name = 'Changed'

    expect(dirtyState.isDirty.value).toBe(false)
  })

  it('marks the form dirty after nested runtime edits once a baseline exists', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    state.linuxStartArgsTemplate.value = [
      {
        id: 'jar',
        label: 'Jar',
        order: 0,
        ownership: 'system',
        tokens: ['-jar', 'server.jar'],
      },
    ]
    dirtyState.commitSnapshot()

    state.linuxStartArgsTemplate.value[0]?.tokens.push('nogui')

    expect(dirtyState.isDirty.value).toBe(true)
  })

  it('returns to clean after recommitting the current state', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    dirtyState.commitSnapshot()
    state.downstreamImpactServers.value = [{ name: 'Server One', patchCount: 2 }]

    expect(dirtyState.isDirty.value).toBe(true)

    dirtyState.commitSnapshot()

    expect(dirtyState.isDirty.value).toBe(false)
  })

  it('marks the form dirty when default environment rows change', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    dirtyState.commitSnapshot()
    state.defaultEnvRows.value.push(
      create(EnvironmentVariableSchema, { name: 'EULA', value: 'true' }),
    )

    expect(dirtyState.isDirty.value).toBe(true)
  })

  it('marks game edits dirty before default environment baseline is committed', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    dirtyState.commitFormSnapshot()
    state.game.value.name = 'Changed'

    expect(dirtyState.isDirty.value).toBe(true)
  })

  it('can recommit only default environment without clearing game edits', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    dirtyState.commitSnapshot()
    state.game.value.name = 'Changed'
    state.defaultEnvRows.value.push(
      create(EnvironmentVariableSchema, { name: 'EULA', value: 'true' }),
    )

    expect(dirtyState.isDirty.value).toBe(true)

    dirtyState.commitDefaultEnvSnapshot()

    expect(dirtyState.isDirty.value).toBe(true)
  })

  it('can recommit only game fields without clearing default environment edits', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    dirtyState.commitSnapshot()
    state.game.value.name = 'Changed'
    state.defaultEnvRows.value.push(
      create(EnvironmentVariableSchema, { name: 'EULA', value: 'true' }),
    )

    expect(dirtyState.isDirty.value).toBe(true)

    dirtyState.commitFormSnapshot()

    expect(dirtyState.isDirty.value).toBe(true)
  })

  it('serializes bigint-backed game fields without throwing', () => {
    const state = createState()
    const dirtyState = useGameFormDirtyState(state)

    state.game.value.defaultPort = 25565n

    expect(() => dirtyState.takeSnapshot()).not.toThrow()
    expect(dirtyState.takeSnapshot()).toContain('"25565"')
  })
})
