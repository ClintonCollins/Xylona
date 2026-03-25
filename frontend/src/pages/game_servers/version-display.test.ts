import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import { UpdateProviderKind, VersionInfoSchema, VersionStatus } from '@/proto/shared_pb'

import { resolveCanonicalVersionDisplay, resolveVariantTrackingLabel } from './version-display'

describe('resolveCanonicalVersionDisplay', () => {
  it('prefers checked installed and latest versions from version info', () => {
    const display = resolveCanonicalVersionDisplay('1.20.1', {
      ...create(VersionInfoSchema, {
        status: VersionStatus.CHECKED,
        installedVersion: '1.21.1',
        latestVersion: '1.21.3',
        updateAvailable: true,
        lastCheckTime: 0n,
        trackerType: 'minecraft',
      }),
    })

    expect(display).toMatchObject({
      installedVersion: '1.21.1',
      latestVersion: '1.21.3',
      updateAvailable: true,
      checked: true,
      checking: false,
    })
  })

  it('prefers display labels when they are available', () => {
    const display = resolveCanonicalVersionDisplay('21600865', {
      ...create(VersionInfoSchema, {
        status: VersionStatus.CHECKED,
        installedVersion: '21600865',
        installedVersionLabel: 'Public (21600865)',
        latestVersion: '22422094',
        latestVersionLabel: 'Unstable build (22422094)',
        updateAvailable: true,
        lastCheckTime: 0n,
        trackerType: 'steamcmd',
      }),
    })

    expect(display).toMatchObject({
      installedVersion: 'Public (21600865)',
      latestVersion: 'Unstable build (22422094)',
      updateAvailable: true,
    })
  })

  it('falls back to the raw version when checked installed version is empty', () => {
    const display = resolveCanonicalVersionDisplay('1.20.1', {
      ...create(VersionInfoSchema, {
        status: VersionStatus.CHECKED,
        installedVersion: '',
        latestVersion: '1.21.3',
        updateAvailable: false,
        lastCheckTime: 0n,
        trackerType: 'minecraft',
      }),
    })

    expect(display.installedVersion).toBe('1.20.1')
    expect(display.updateAvailable).toBe(false)
  })

  it('shows checking without inventing an update', () => {
    const display = resolveCanonicalVersionDisplay('1.20.1', {
      ...create(VersionInfoSchema, {
        status: VersionStatus.CHECKING,
        installedVersion: '',
        latestVersion: '',
        updateAvailable: false,
        lastCheckTime: 0n,
        trackerType: 'minecraft',
      }),
    })

    expect(display).toMatchObject({
      installedVersion: '1.20.1',
      latestVersion: '',
      updateAvailable: false,
      checked: false,
      checking: true,
    })
  })

  it('falls back cleanly for error and no-tracker states', () => {
    expect(
      resolveCanonicalVersionDisplay('1.18.2', {
        ...create(VersionInfoSchema, {
          status: VersionStatus.ERROR,
          installedVersion: '',
          latestVersion: '',
          updateAvailable: false,
          lastCheckTime: 0n,
          trackerType: 'minecraft',
        }),
      }).installedVersion,
    ).toBe('1.18.2')

    expect(
      resolveCanonicalVersionDisplay('', {
        ...create(VersionInfoSchema, {
          status: VersionStatus.NO_TRACKER,
          installedVersion: '',
          latestVersion: '',
          updateAvailable: false,
          lastCheckTime: 0n,
          trackerType: '',
        }),
      }),
    ).toMatchObject({
      installedVersion: '',
      latestVersion: '',
      updateAvailable: false,
      checked: false,
      checking: false,
    })
  })

  it('falls back from empty labels to the raw versions', () => {
    const display = resolveCanonicalVersionDisplay('21600865', {
      ...create(VersionInfoSchema, {
        status: VersionStatus.CHECKED,
        installedVersion: '21600865',
        installedVersionLabel: '',
        latestVersion: '22422094',
        latestVersionLabel: '',
        updateAvailable: true,
        lastCheckTime: 0n,
        trackerType: 'steamcmd',
      }),
    })

    expect(display).toMatchObject({
      installedVersion: '21600865',
      latestVersion: '22422094',
      updateAvailable: true,
    })
  })

  it('reports tracking latest for Mojang and Paper servers that are not pinned', () => {
    expect(resolveVariantTrackingLabel(UpdateProviderKind.MOJANG, '', false)).toBe(
      'Tracking latest',
    )
    expect(resolveVariantTrackingLabel(UpdateProviderKind.PAPERMC, '1.21.4', false)).toBe(
      'Tracking latest',
    )
  })

  it('reports the pinned target for Mojang and Paper servers', () => {
    expect(resolveVariantTrackingLabel(UpdateProviderKind.MOJANG, '1.21.4', true)).toBe(
      'Pinned to 1.21.4',
    )
  })

  it('returns an empty label for non-Minecraft providers', () => {
    expect(resolveVariantTrackingLabel(UpdateProviderKind.STEAMCMD, 'public', false)).toBe('')
  })
})
