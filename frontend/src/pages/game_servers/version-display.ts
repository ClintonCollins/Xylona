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
