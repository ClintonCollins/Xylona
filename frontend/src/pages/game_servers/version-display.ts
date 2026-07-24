import type { VersionInfo } from '@/proto/shared_pb'
import { UpdateProviderKind, VersionStatus } from '@/proto/shared_pb'

export interface CanonicalVersionDisplay {
  installedVersion: string
  latestVersion: string
  updateAvailable: boolean
  checking: boolean
  checked: boolean
}

export function resolveCanonicalVersionDisplay(
  version: string,
  versionInfo?: VersionInfo,
): CanonicalVersionDisplay {
  const checked = versionInfo?.status === VersionStatus.CHECKED
  const checking = versionInfo?.status === VersionStatus.CHECKING
  const installedVersion = (
    versionInfo?.installedVersionLabel ||
    versionInfo?.installedVersion ||
    version ||
    ''
  ).trim()
  const latestVersion =
    checked && versionInfo?.updateAvailable
      ? (versionInfo.latestVersionLabel || versionInfo.latestVersion || '').trim()
      : ''

  return {
    installedVersion,
    latestVersion,
    updateAvailable: checked && !!versionInfo?.updateAvailable && latestVersion !== '',
    checking,
    checked,
  }
}

export type VersionSectionState =
  | 'up-to-date'
  | 'update-available'
  | 'pinned-behind'
  | 'checking'
  | 'unknown'

export interface VersionSectionDisplay {
  installedVersion: string
  latestVersion: string
  state: VersionSectionState
  lastCheckedLabel: string
}

export interface VersionSectionInput {
  version: string
  versionInfo?: VersionInfo
  providerKind?: UpdateProviderKind
  selectedTarget: string
  selectedTargetPinned: boolean
  nowMs: number
}

export function resolveVersionSection(input: VersionSectionInput): VersionSectionDisplay {
  const display = resolveCanonicalVersionDisplay(input.version, input.versionInfo)
  const installedVersion = display.installedVersion
  const lastCheckedLabel = formatLastChecked(input.nowMs, input.versionInfo?.lastCheckTime ?? 0n)

  if (display.checking) {
    return { installedVersion, latestVersion: '', state: 'checking', lastCheckedLabel }
  }
  if (!display.checked) {
    return { installedVersion, latestVersion: '', state: 'unknown', lastCheckedLabel }
  }

  let latestVersion = (
    input.versionInfo?.latestVersionLabel ||
    input.versionInfo?.latestVersion ||
    ''
  ).trim()

  const behind = !!input.versionInfo?.updateAvailable && latestVersion !== ''
  if (!behind) {
    if (latestVersion === '') {
      latestVersion = installedVersion
    }
    return { installedVersion, latestVersion, state: 'up-to-date', lastCheckedLabel }
  }

  const pinnable =
    input.providerKind === UpdateProviderKind.MOJANG ||
    input.providerKind === UpdateProviderKind.PAPERMC
  const pinned = pinnable && input.selectedTargetPinned && input.selectedTarget.trim() !== ''

  return {
    installedVersion,
    latestVersion,
    state: pinned ? 'pinned-behind' : 'update-available',
    lastCheckedLabel,
  }
}

function formatLastChecked(nowMs: number, lastCheckTime: bigint): string {
  if (lastCheckTime <= 0n) {
    return ''
  }

  const elapsedSeconds = Math.max(0, Math.floor(nowMs / 1000) - Number(lastCheckTime))
  if (elapsedSeconds < 60) {
    return 'just now'
  }
  if (elapsedSeconds < 3600) {
    return `${Math.floor(elapsedSeconds / 60)}m ago`
  }
  if (elapsedSeconds < 86400) {
    return `${Math.floor(elapsedSeconds / 3600)}h ago`
  }
  return `${Math.floor(elapsedSeconds / 86400)}d ago`
}

export function resolveVariantTrackingLabel(
  providerKind: UpdateProviderKind | undefined,
  selectedTarget: string,
  selectedTargetPinned: boolean,
): string {
  if (providerKind !== UpdateProviderKind.MOJANG && providerKind !== UpdateProviderKind.PAPERMC) {
    return ''
  }

  if (!selectedTargetPinned) {
    return 'Tracking latest'
  }

  const normalizedTarget = selectedTarget.trim()
  if (normalizedTarget === '') {
    return 'Tracking latest'
  }

  return `Pinned to ${normalizedTarget}`
}
