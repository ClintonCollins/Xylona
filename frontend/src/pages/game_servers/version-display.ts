import type { VersionInfo } from '@/proto/shared_pb'
import { VersionStatus } from '@/proto/shared_pb'

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
