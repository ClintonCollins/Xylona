import type { GameServer, SteamBranch } from '@/proto/shared_pb'

export interface SteamBranchDialogItem {
  label: string
  value: string
  caption: string
}

export interface SteamBranchSelectionResult {
  metadataAvailable: boolean
  cancelled: boolean
  steamBranch: string
}

interface BranchLookupResponse {
  branches: SteamBranch[]
  currentBranch: string
}

interface ChooseSteamBranchForUpdateOptions {
  gameServerId: string
  gameServer: GameServer
  getBranches: (serverId: string) => Promise<BranchLookupResponse>
  openDialog: (options: {
    currentBranch: string
    items: SteamBranchDialogItem[]
    onOk: (value: string) => void
    onDismiss: () => void
  }) => void
}

export function normalizeSteamBranch(branch: string): string {
  const normalized = branch.trim()
  if (normalized === '') {
    return 'public'
  }
  return normalized
}

export function canSelectSteamBranch(gameServer: GameServer): boolean {
  const game = gameServer.game
  if (!game?.usesSteamcmd) {
    return false
  }

  return game.steamAppid.trim() !== ''
}

export function buildSteamBranchDialogItems(branches: SteamBranch[]): SteamBranchDialogItem[] {
  return branches.map((branch) => {
    const name = normalizeSteamBranch(branch.name)
    const label = branch.description.trim() || (name === 'public' ? 'Public' : name)
    const buildSuffix = branch.buildId.trim() === '' ? '' : ` - Build ${branch.buildId.trim()}`

    return {
      label,
      value: name,
      caption: `Branch ${name}${buildSuffix}`,
    }
  })
}

export async function chooseSteamBranchForUpdate(
  options: ChooseSteamBranchForUpdateOptions,
): Promise<SteamBranchSelectionResult> {
  if (!canSelectSteamBranch(options.gameServer)) {
    return {
      metadataAvailable: false,
      cancelled: false,
      steamBranch: '',
    }
  }

  let response: BranchLookupResponse
  try {
    response = await options.getBranches(options.gameServerId)
  } catch {
    return {
      metadataAvailable: false,
      cancelled: false,
      steamBranch: '',
    }
  }

  if (response.branches.length === 0) {
    return {
      metadataAvailable: false,
      cancelled: false,
      steamBranch: '',
    }
  }

  const currentBranch = normalizeSteamBranch(response.currentBranch || options.gameServer.branch)
  const items = buildSteamBranchDialogItems(response.branches)
  const selectedBranch = await new Promise<string | undefined>((resolve) => {
    options.openDialog({
      currentBranch,
      items,
      onOk: (value) => {
        resolve(normalizeSteamBranch(value))
      },
      onDismiss: () => {
        resolve(undefined)
      },
    })
  })

  if (selectedBranch === undefined) {
    return {
      metadataAvailable: true,
      cancelled: true,
      steamBranch: '',
    }
  }

  return {
    metadataAvailable: true,
    cancelled: false,
    steamBranch: selectedBranch,
  }
}
