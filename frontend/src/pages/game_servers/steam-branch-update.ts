import { UpdateProviderKind, type GameServer, type UpdateTargetOption } from '@/proto/shared_pb'

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
  targets: UpdateTargetOption[]
  currentTarget: string
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
  return normalizeTargetValue(UpdateProviderKind.STEAMCMD, branch)
}

export function canSelectSteamBranch(gameServer: GameServer): boolean {
  const kind = gameServer.resolvedUpdateProvider?.kind
  if (kind === UpdateProviderKind.STEAMCMD) {
    return (gameServer.game?.steamAppid?.trim() ?? '') !== ''
  }
  return kind === UpdateProviderKind.PAPERMC || kind === UpdateProviderKind.MOJANG
}

export function buildSteamBranchDialogItems(
  targets: UpdateTargetOption[],
): SteamBranchDialogItem[] {
  return targets.map((target) => {
    const value = target.id.trim()
    const buildSuffix =
      target.latestVersion.trim() === '' ? '' : ` - Build ${target.latestVersion.trim()}`

    return {
      label: target.label.trim() || value,
      value,
      caption: target.description.trim() || `Track ${value}${buildSuffix}`,
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

  if (response.targets.length === 0) {
    return {
      metadataAvailable: false,
      cancelled: false,
      steamBranch: '',
    }
  }

  const providerKind = options.gameServer.resolvedUpdateProvider?.kind ?? UpdateProviderKind.NONE
  const currentBranch = normalizeSteamBranch(
    normalizeTargetValue(providerKind, response.currentTarget || options.gameServer.selectedTarget),
  )
  const items = buildSteamBranchDialogItems(response.targets)
  const selectedBranch = await new Promise<string | undefined>((resolve) => {
    options.openDialog({
      currentBranch,
      items,
      onOk: (value) => {
        resolve(normalizeTargetValue(providerKind, value))
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

function normalizeTargetValue(kind: UpdateProviderKind, value: string): string {
  const normalized = value.trim()
  if (kind === UpdateProviderKind.STEAMCMD && normalized === '') {
    return 'public'
  }
  return normalized
}
