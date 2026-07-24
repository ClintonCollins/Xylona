import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import type { VersionInfo } from '@/proto/shared_pb'
import { UpdateProviderKind, VersionInfoSchema, VersionStatus } from '@/proto/shared_pb'

import {
  resolveCanonicalVersionDisplay,
  resolveVariantTrackingLabel,
  resolveVersionSection,
  type VersionSectionState,
} from './version-display'

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

describe('resolveVersionSection', () => {
  const nowMs = 1_700_000_000_000

  function buildVersionInfo(overrides: Partial<VersionInfo> = {}): VersionInfo {
    return create(VersionInfoSchema, {
      status: VersionStatus.CHECKED,
      installedVersion: '1.21.5',
      latestVersion: '1.21.8',
      updateAvailable: true,
      lastCheckTime: 0n,
      trackerType: 'minecraft',
      ...overrides,
    })
  }

  const cases: {
    name: string
    versionInfo?: VersionInfo
    providerKind?: UpdateProviderKind
    selectedTarget?: string
    selectedTargetPinned?: boolean
    wantState: VersionSectionState
    wantInstalled?: string
    wantLatest?: string
  }[] = [
    {
      name: 'update available while tracking latest',
      versionInfo: buildVersionInfo(),
      providerKind: UpdateProviderKind.MOJANG,
      wantState: 'update-available',
      wantInstalled: '1.21.5',
      wantLatest: '1.21.8',
    },
    {
      name: 'pinned server behind latest stays quiet',
      versionInfo: buildVersionInfo(),
      providerKind: UpdateProviderKind.MOJANG,
      selectedTarget: '1.21.5',
      selectedTargetPinned: true,
      wantState: 'pinned-behind',
      wantLatest: '1.21.8',
    },
    {
      name: 'pinned flag without a target still tracks latest',
      versionInfo: buildVersionInfo(),
      providerKind: UpdateProviderKind.PAPERMC,
      selectedTarget: '',
      selectedTargetPinned: true,
      wantState: 'update-available',
    },
    {
      name: 'steam providers cannot pin and still report updates',
      versionInfo: buildVersionInfo({ trackerType: 'steamcmd' }),
      providerKind: UpdateProviderKind.STEAMCMD,
      selectedTarget: 'public',
      selectedTargetPinned: true,
      wantState: 'update-available',
    },
    {
      name: 'checked and current reports up to date',
      versionInfo: buildVersionInfo({ updateAvailable: false, latestVersion: '1.21.5' }),
      providerKind: UpdateProviderKind.MOJANG,
      wantState: 'up-to-date',
      wantLatest: '1.21.5',
    },
    {
      name: 'up to date with empty latest falls back to installed',
      versionInfo: buildVersionInfo({ updateAvailable: false, latestVersion: '' }),
      providerKind: UpdateProviderKind.MOJANG,
      wantState: 'up-to-date',
      wantLatest: '1.21.5',
    },
    {
      name: 'checking state',
      versionInfo: buildVersionInfo({ status: VersionStatus.CHECKING }),
      wantState: 'checking',
      wantLatest: '',
    },
    {
      name: 'no tracker means unknown',
      versionInfo: buildVersionInfo({ status: VersionStatus.NO_TRACKER }),
      wantState: 'unknown',
      wantLatest: '',
    },
    {
      name: 'missing version info means unknown',
      wantState: 'unknown',
      wantInstalled: '1.20.0',
      wantLatest: '',
    },
    {
      name: 'prefers display labels for both rows',
      versionInfo: buildVersionInfo({
        installedVersionLabel: 'Public (21600865)',
        latestVersionLabel: 'Public (22422094)',
      }),
      wantState: 'update-available',
      wantInstalled: 'Public (21600865)',
      wantLatest: 'Public (22422094)',
    },
  ]

  it.each(cases)('$name', (testCase) => {
    const section = resolveVersionSection({
      version: '1.20.0',
      versionInfo: testCase.versionInfo,
      providerKind: testCase.providerKind,
      selectedTarget: testCase.selectedTarget ?? '',
      selectedTargetPinned: testCase.selectedTargetPinned ?? false,
      nowMs,
    })

    expect(section.state).toBe(testCase.wantState)
    if (testCase.wantInstalled !== undefined) {
      expect(section.installedVersion).toBe(testCase.wantInstalled)
    }
    if (testCase.wantLatest !== undefined) {
      expect(section.latestVersion).toBe(testCase.wantLatest)
    }
  })

  it.each([
    { name: 'never checked', lastCheckTime: 0n, want: '' },
    { name: 'just now', lastCheckTime: BigInt(Math.floor(nowMs / 1000) - 30), want: 'just now' },
    { name: 'minutes', lastCheckTime: BigInt(Math.floor(nowMs / 1000) - 5 * 60), want: '5m ago' },
    { name: 'hours', lastCheckTime: BigInt(Math.floor(nowMs / 1000) - 2 * 3600), want: '2h ago' },
    { name: 'days', lastCheckTime: BigInt(Math.floor(nowMs / 1000) - 3 * 86400), want: '3d ago' },
  ])('formats last checked: $name', ({ lastCheckTime, want }) => {
    const section = resolveVersionSection({
      version: '',
      versionInfo: buildVersionInfo({ lastCheckTime }),
      selectedTarget: '',
      selectedTargetPinned: false,
      nowMs,
    })

    expect(section.lastCheckedLabel).toBe(want)
  })
})
