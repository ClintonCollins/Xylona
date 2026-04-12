import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'

import { useGameFormStartArgsState } from './useGameFormStartArgsState'

describe('useGameFormStartArgsState', () => {
  it('hydrates parsed templates and blocklists from the game model', () => {
    const game = ref(
      create(GameSchema, {
        linuxStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","label":"Jar","tokens":["-jar","server.jar"]}]',
        windowsStartArgsTemplate:
          '[{"id":"nogui","order":0,"ownership":"editable","label":"No GUI","tokens":["nogui"]}]',
        startArgBlocklist: '[{"pattern":"nogui","reason":"Managed by defaults"}]',
      }),
    )

    const state = useGameFormStartArgsState(game)

    state.syncStructuredStartArgsFromGame()

    expect(state.linuxStartArgsTemplate.value).toHaveLength(1)
    expect(state.linuxStartArgsTemplate.value[0]?.id).toBe('jar')
    expect(state.windowsStartArgsTemplate.value).toHaveLength(1)
    expect(state.windowsStartArgsTemplate.value[0]?.id).toBe('nogui')
    expect(state.startArgBlocklist.value).toHaveLength(1)
    expect(state.startArgBlocklist.value[0]?.pattern).toBe('nogui')
  })

  it('captures baseline values as clones so later edits do not mutate reset state', () => {
    const game = ref(
      create(GameSchema, {
        linuxBaseCommand: 'java',
        windowsBaseCommand: 'javaw',
        linuxStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","label":"Jar","tokens":["-jar","server.jar"]}]',
        windowsStartArgsTemplate:
          '[{"id":"nogui","order":0,"ownership":"editable","label":"No GUI","tokens":["nogui"]}]',
      }),
    )

    const state = useGameFormStartArgsState(game)

    state.syncStructuredStartArgsFromGame()
    state.captureRuntimeBaselineFromCurrentState()

    state.linuxStartArgsTemplate.value[0]?.tokens.push('extra-token')
    const windowsBlock = state.windowsStartArgsTemplate.value[0]
    expect(windowsBlock).toBeDefined()
    if (!windowsBlock) {
      throw new Error('expected windows baseline block')
    }
    windowsBlock.label = 'Changed'

    expect(state.baselineLinuxBaseCommand.value).toBe('java')
    expect(state.baselineWindowsBaseCommand.value).toBe('javaw')
    expect(state.baselineLinuxStartArgsTemplate.value[0]?.tokens).toEqual(['-jar', 'server.jar'])
    expect(state.baselineWindowsStartArgsTemplate.value[0]?.label).toBe('No GUI')
  })

  it('serializes edited templates and blocklists back into the game model', () => {
    const game = ref(create(GameSchema, {}))
    const state = useGameFormStartArgsState(game)

    state.linuxStartArgsTemplate.value = [
      {
        id: 'jar',
        label: 'Jar',
        order: 0,
        ownership: 'system',
        tokens: ['-jar', 'server.jar'],
      },
    ]
    state.windowsStartArgsTemplate.value = [
      {
        id: 'nogui',
        label: 'No GUI',
        order: 0,
        ownership: 'editable',
        tokens: ['nogui'],
      },
    ]
    state.startArgBlocklist.value = [
      {
        pattern: 'nogui',
        reason: 'Managed by defaults',
      },
    ]

    state.syncStructuredStartArgsToGame()

    expect(game.value.linuxStartArgsTemplate).toContain('"id":"jar"')
    expect(game.value.windowsStartArgsTemplate).toContain('"id":"nogui"')
    expect(game.value.startArgBlocklist).toContain('"pattern":"nogui"')
  })
})
