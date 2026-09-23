import { nextTick, type Ref } from 'vue'

import { CommandType, type Game } from '@/proto/shared_pb'

interface GameCreateWizardState {
  name?: string
  slug?: string
  steamAppId?: string
  usesSteamcmd?: boolean
  windowsSupport?: boolean
  linuxSupport?: boolean
  linuxBaseCommand?: string
  windowsBaseCommand?: string
  linuxStartArgsTemplate?: string
  windowsStartArgsTemplate?: string
}

interface DownstreamImpactServer {
  name: string
  patchCount: number
}

interface UseGameFormNewGameSetupOptions {
  game: Ref<Game>
  downstreamImpactServers: Ref<DownstreamImpactServer[]>
  ensureTypedGameConfig: () => void
  syncSimpleGameConfig: () => void
  syncStructuredStartArgsFromGame: () => void
  captureRuntimeBaselineFromCurrentState: () => void
  syncActivePlatformFromGame: () => void
  commitSnapshot: () => void
}

function readWizardState(): GameCreateWizardState | null {
  const state = history.state?.wizardState
  if (!state || typeof state !== 'object') {
    return null
  }

  return state as GameCreateWizardState
}

export function useGameFormNewGameSetup(options: UseGameFormNewGameSetupOptions) {
  function applyWizardPrefill(wizardState: GameCreateWizardState) {
    options.game.value.name = wizardState.name || ''
    options.game.value.id = wizardState.slug || ''
    options.game.value.steamAppid = wizardState.steamAppId || ''
    options.game.value.usesSteamcmd = wizardState.usesSteamcmd ?? false
    options.game.value.windowsSupport = wizardState.windowsSupport ?? false
    options.game.value.linuxSupport = wizardState.linuxSupport ?? false

    // A SteamCMD game installs and updates through SteamCMD on each supported platform;
    // syncSimpleGameConfig then builds the commands from the Steam AppID.
    if (options.game.value.usesSteamcmd) {
      if (options.game.value.linuxSupport) {
        options.game.value.linuxInstallType = CommandType.STEAMCMD
        options.game.value.linuxUpdateType = CommandType.STEAMCMD
      }
      if (options.game.value.windowsSupport) {
        options.game.value.windowsInstallType = CommandType.STEAMCMD
        options.game.value.windowsUpdateType = CommandType.STEAMCMD
      }
    }

    if (wizardState.linuxBaseCommand) {
      options.game.value.linuxBaseCommand = wizardState.linuxBaseCommand
    }
    if (wizardState.windowsBaseCommand) {
      options.game.value.windowsBaseCommand = wizardState.windowsBaseCommand
    }
    if (wizardState.linuxStartArgsTemplate) {
      options.game.value.linuxStartArgsTemplate = wizardState.linuxStartArgsTemplate
    }
    if (wizardState.windowsStartArgsTemplate) {
      options.game.value.windowsStartArgsTemplate = wizardState.windowsStartArgsTemplate
    }
  }

  async function initializeNewGameForm() {
    const wizardState = readWizardState()

    if (wizardState) {
      applyWizardPrefill(wizardState)
      options.ensureTypedGameConfig()
      options.syncSimpleGameConfig()
      options.syncActivePlatformFromGame()
    }

    options.ensureTypedGameConfig()
    options.syncStructuredStartArgsFromGame()
    options.captureRuntimeBaselineFromCurrentState()
    options.downstreamImpactServers.value = []

    await nextTick()
    options.commitSnapshot()
  }

  return {
    applyWizardPrefill,
    initializeNewGameForm,
  }
}
