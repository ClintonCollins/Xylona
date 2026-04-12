import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { GameSchema, ModProfileSchema, ModSourceSchema } from '@/proto/shared_pb'

import { useGameFormModProfile } from './useGameFormModProfile'

describe('useGameFormModProfile', () => {
  it('creates and clears a default editable mod profile', () => {
    const game = ref(create(GameSchema, {}))
    const modProfile = useGameFormModProfile(game)

    modProfile.addGameModProfile()

    expect(game.value.modProfile?.installPath).toBe('')
    expect(game.value.modProfile?.sources).toHaveLength(1)
    expect(game.value.modProfile?.sources[0]?.id).toBe('modrinth')

    modProfile.clearGameModProfile()

    expect(game.value.modProfile).toBeUndefined()
  })

  it('hydrates an empty source list so the editor remains usable', () => {
    const game = ref(
      create(GameSchema, {
        modProfile: create(ModProfileSchema, {
          installPath: 'plugins/',
          sources: [],
        }),
      }),
    )
    const modProfile = useGameFormModProfile(game)

    modProfile.ensureModProfileSources()

    expect(game.value.modProfile?.sources).toHaveLength(1)
    expect(game.value.modProfile?.sources[0]?.id).toBe('modrinth')
  })

  it('derives the active provider label and resets provider payload state when changed', () => {
    const game = ref(
      create(GameSchema, {
        modProfile: create(ModProfileSchema, {
          installPath: 'plugins/',
          sources: [
            create(ModSourceSchema, {
              id: 'hangar',
              searchParamsJson: '{"platform":"PAPER"}',
            }),
          ],
        }),
      }),
    )
    const modProfile = useGameFormModProfile(game)
    const source = game.value.modProfile?.sources[0]

    expect(modProfile.activeModSourceLabel.value).toBe('Hangar')
    expect(source).toBeDefined()
    if (!source) {
      throw new Error('expected default source')
    }

    modProfile.onModSourceProviderChanged(source)

    expect(source.searchParamsJson).toBe('')
  })

  it('reads and writes provider display values through provider-specific helpers', () => {
    const game = ref(
      create(GameSchema, {
        modProfile: create(ModProfileSchema, {
          installPath: 'plugins/',
          sources: [
            create(ModSourceSchema, {
              id: 'steam_workshop',
              searchParamsJson: '{"app_id":"123"}',
            }),
          ],
        }),
      }),
    )
    const modProfile = useGameFormModProfile(game)
    const source = game.value.modProfile?.sources[0]

    expect(source).toBeDefined()
    if (!source) {
      throw new Error('expected default source')
    }

    expect(modProfile.readModSourceDisplayValue(source)).toBe('123')

    modProfile.updateModSourceDisplayValue(source, '456')

    expect(source.searchParamsJson).toBe('{"app_id":"456"}')
  })
})
