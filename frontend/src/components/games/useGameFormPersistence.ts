import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { nextTick, ref, type Ref } from 'vue'
import { useRouter } from 'vue-router'

import type { Game } from '@/proto/shared_pb'
import { parseStartArgsPatches } from '@/components/game_servers/start-args'
import {
  AddGameRequestSchema,
  EditGameRequestSchema,
  GetGameRequestSchema,
  ListGameServersRequestSchema,
  UpdateGameConfigSchemasRequestSchema,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

import { normalizeSteamAppID } from './game-form-normalization'
import type { ConfigSchemaEntry } from './config-schema-types'

export interface DownstreamImpactServer {
  name: string
  patchCount: number
}

interface GameFormValidationTarget {
  validate: () => boolean | Promise<boolean>
}

interface UseGameFormPersistenceOptions {
  formRef: Ref<GameFormValidationTarget | null>
  game: Ref<Game>
  gameID: Ref<string>
  existingGame: Ref<boolean>
  copyGame: Ref<boolean>
  defaultPort: Ref<number | null>
  defaultQueryPort: Ref<number | null>
  configSchemas: Ref<ConfigSchemaEntry[]>
  downstreamImpactServers: Ref<DownstreamImpactServer[]>
  savedSuccessfully: Ref<boolean>
  ensureTypedGameConfig: () => void
  syncSimpleGameConfig: () => void
  syncStructuredStartArgsFromGame: () => void
  captureRuntimeBaselineFromCurrentState: () => void
  syncStructuredStartArgsToGame: () => void
  syncActivePlatformFromGame: () => void
  commitSnapshot: () => void
}

export function useGameFormPersistence(options: UseGameFormPersistenceOptions) {
  const $q = useQuasar()
  const router = useRouter()
  const loading = ref(false)
  const submitting = ref(false)

  function syncConfigSchemas() {
    options.game.value.configSchemas =
      options.configSchemas.value.length > 0 ? JSON.stringify(options.configSchemas.value) : ''
  }

  function prepareGameForSave(): Game {
    options.game.value.steamAppid = normalizeSteamAppID(options.game.value.steamAppid)
    options.syncSimpleGameConfig()
    options.syncStructuredStartArgsToGame()
    options.game.value.defaultPort = BigInt(options.defaultPort.value ?? 0)
    options.game.value.defaultQueryPort = BigInt(options.defaultQueryPort.value ?? 0)
    return options.game.value
  }

  function parseConfigSchemas(configSchemasJson: string): ConfigSchemaEntry[] {
    if (!configSchemasJson) {
      return []
    }

    try {
      return JSON.parse(configSchemasJson) as ConfigSchemaEntry[]
    } catch {
      return []
    }
  }

  async function loadDownstreamImpact(gameId: string) {
    try {
      const response = await GetXylonaClient().listGameServers(
        create(ListGameServersRequestSchema, {}),
      )
      options.downstreamImpactServers.value = response.gameServers
        .filter((server) => server.gameId === gameId)
        .map((server) => ({
          name: server.name,
          patchCount: parseStartArgsPatches(server.startArgsPatches).length,
        }))
    } catch (unknownErr: unknown) {
      options.downstreamImpactServers.value = []
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-warning',
        caption: `Failed to load downstream impact: ${ConnectErrorToString(err)}`,
        position: 'top',
        timeout: 3500,
      })
    }
  }

  async function loadGameDetails() {
    loading.value = true
    const request = create(GetGameRequestSchema, {
      id: options.gameID.value,
    })

    try {
      const response = await GetXylonaClient().getGame(request)
      if (response.game === undefined) {
        return
      }

      options.game.value = response.game
      options.ensureTypedGameConfig()
      options.defaultPort.value = Number(response.game.defaultPort)
      options.defaultQueryPort.value = Number(response.game.defaultQueryPort)
      options.configSchemas.value = parseConfigSchemas(response.game.configSchemas)

      if (options.copyGame.value) {
        options.game.value.id = ''
        options.game.value.name = `${options.game.value.name} (Copy)`
      }

      options.syncStructuredStartArgsFromGame()
      options.captureRuntimeBaselineFromCurrentState()

      if (options.existingGame.value && !options.copyGame.value) {
        await loadDownstreamImpact(response.game.id)
      } else {
        options.downstreamImpactServers.value = []
      }

      options.syncActivePlatformFromGame()
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: `Failed to load game: ${ConnectErrorToString(err)}`,
        position: 'top',
        timeout: 5000,
      })
    } finally {
      loading.value = false
      await nextTick()
      options.commitSnapshot()
    }
  }

  async function persistConfigSchemasBeforeNavigation(gameId: string): Promise<boolean> {
    try {
      const request = create(UpdateGameConfigSchemasRequestSchema, {
        gameId,
        configSchemasJson: JSON.stringify(options.configSchemas.value),
      })
      await GetXylonaClient().updateGameConfigSchemas(request)
      return true
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: `Failed to save schemas before editing: ${ConnectErrorToString(err)}`,
        position: 'top',
        timeout: 5000,
      })
      return false
    }
  }

  async function navigateToSchemaEditor(fileIndex: number) {
    const id = options.existingGame.value ? options.gameID.value : ''
    if (!id) {
      return
    }

    const saved = await persistConfigSchemasBeforeNavigation(id)
    if (!saved) {
      return
    }

    options.commitSnapshot()
    await router.push({ path: `/games/${id}/config-schema/${fileIndex}` })
  }

  async function addNewGame() {
    const request = create(AddGameRequestSchema, {
      game: prepareGameForSave(),
    })

    try {
      const response = await GetXylonaClient().addGame(request)
      const savedGameID = response.game?.id || request.game?.id || options.game.value.id

      options.savedSuccessfully.value = true
      options.captureRuntimeBaselineFromCurrentState()
      options.commitSnapshot()
      $q.notify({
        caption: `${options.game.value.name} added successfully`,
        type: 'xylona-success',
        position: 'top',
        timeout: 5000,
      })

      if (savedGameID) {
        await router.push({ path: `/games/${savedGameID}/edit` })
      }
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        caption: `Error adding game: ${ConnectErrorToString(err)}`,
        type: 'xylona-error',
        position: 'top',
        timeout: 5000,
      })
    }
  }

  async function updateExistingGame() {
    const request = create(EditGameRequestSchema, {
      game: prepareGameForSave(),
    })

    try {
      await GetXylonaClient().editGame(request)
      options.savedSuccessfully.value = true
      options.captureRuntimeBaselineFromCurrentState()
      options.commitSnapshot()
      $q.notify({
        caption: `${options.game.value.name} updated successfully`,
        type: 'xylona-success',
        position: 'top',
        timeout: 5000,
      })
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        caption: `Error updating game: ${ConnectErrorToString(err)}`,
        type: 'xylona-error',
        position: 'top',
        timeout: 5000,
      })
    }
  }

  async function submit() {
    const valid = await options.formRef.value?.validate()
    if (!valid) {
      $q.notify({
        type: 'xylona-error',
        caption: 'Please fix the validation errors before saving.',
        position: 'top',
        timeout: 3000,
      })
      return
    }

    submitting.value = true
    syncConfigSchemas()

    try {
      if (options.existingGame.value) {
        await updateExistingGame()
      } else {
        await addNewGame()
      }
    } finally {
      submitting.value = false
    }
  }

  return {
    loading,
    submitting,
    loadGameDetails,
    navigateToSchemaEditor,
    submit,
  }
}
