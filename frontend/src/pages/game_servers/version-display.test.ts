import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import { VersionInfoSchema, VersionStatus } from '@/proto/shared_pb'

import { resolveCanonicalVersionDisplay } from './version-display'

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
})
