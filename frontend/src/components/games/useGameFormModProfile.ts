import { computed, type Ref } from 'vue'
import { create } from '@bufbuild/protobuf'

import { ModProfileSchema, type Game, type ModSource, ModSourceSchema } from '@/proto/shared_pb'

import {
  getModSourceConfig,
  getModSourceOptions,
  isManagedModConfig,
  readModSourcePrimaryValue,
  writeModSourcePrimaryValue,
} from './game-form-provider-fields'

function createEmptyModSource() {
  return create(ModSourceSchema, {
    id: 'modrinth',
    searchParamsJson: '',
  })
}

function createEmptyModProfile() {
  return create(ModProfileSchema, {
    installPath: '',
    sources: [createEmptyModSource()],
  })
}

export function useGameFormModProfile(game: Ref<Game>) {
  const modSourceOptions = getModSourceOptions()

  const managedModConfig = computed(() => isManagedModConfig(game.value))

  const activeModSourceLabel = computed(() => {
    const sourceID = game.value.modProfile?.sources[0]?.id
    if (!sourceID) {
      return 'No provider'
    }

    return modSourceOptions.find((option) => option.value === sourceID)?.label ?? sourceID
  })

  function ensureModProfileSources(): void {
    if (game.value.modProfile && game.value.modProfile.sources.length === 0) {
      game.value.modProfile.sources.push(createEmptyModSource())
    }
  }

  function onModSourceProviderChanged(source: ModSource): void {
    source.searchParamsJson = ''
  }

  function readModSourceDisplayValue(source: ModSource): string {
    return readModSourcePrimaryValue(source.id, source.searchParamsJson)
  }

  function updateModSourceDisplayValue(source: ModSource, value: string | number | null): void {
    const nextValue = typeof value === 'string' ? value : value == null ? '' : String(value)
    source.searchParamsJson = writeModSourcePrimaryValue(
      source.id,
      source.searchParamsJson,
      nextValue,
    )
  }

  function addGameModProfile(): void {
    game.value.modProfile = createEmptyModProfile()
  }

  function clearGameModProfile(): void {
    game.value.modProfile = undefined
  }

  return {
    managedModConfig,
    modSourceOptions,
    activeModSourceLabel,
    ensureModProfileSources,
    addGameModProfile,
    clearGameModProfile,
    onModSourceProviderChanged,
    readModSourceDisplayValue,
    updateModSourceDisplayValue,
    getModSourceConfig,
  }
}
