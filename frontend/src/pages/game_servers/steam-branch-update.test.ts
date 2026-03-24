import { create } from '@bufbuild/protobuf'
import { describe, expect, it, vi } from 'vitest'

import {
  GameSchema,
  GameServerSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
  UpdateTargetOptionSchema,
} from '@/proto/shared_pb'

import {
  buildSteamBranchDialogItems,
  chooseSteamBranchForUpdate,
  normalizeSteamBranch,
} from './steam-branch-update'

describe('normalizeSteamBranch', () => {
  it('normalizes empty values to public', () => {
    expect(normalizeSteamBranch('')).toBe('public')
    expect(normalizeSteamBranch('  ')).toBe('public')
    expect(normalizeSteamBranch('latest_experimental')).toBe('latest_experimental')
  })
})

describe('buildSteamBranchDialogItems', () => {
  it('uses the backend target labels and build ids for dialog options', () => {
    const items = buildSteamBranchDialogItems([
      create(UpdateTargetOptionSchema, {
        id: 'public',
        label: 'Public',
        latestVersion: '21600865',
      }),
      create(UpdateTargetOptionSchema, {
        id: 'latest_experimental',
        label: 'Unstable build',
        latestVersion: '22422094',
      }),
    ])

    expect(items).toEqual([
      {
        label: 'Public',
        value: 'public',
        caption: 'Track public - Build 21600865',
      },
      {
        label: 'Unstable build',
        value: 'latest_experimental',
        caption: 'Track latest_experimental - Build 22422094',
      },
    ])
  })
})

describe('chooseSteamBranchForUpdate', () => {
  it('skips target lookup for non-SteamCMD servers', async () => {
    const getBranches = vi.fn()
    const dialog = vi.fn()

    const result = await chooseSteamBranchForUpdate({
      gameServerId: 'server-1',
      gameServer: create(GameServerSchema, {
        id: 'server-1',
      }),
      getBranches,
      openDialog: dialog,
    })

    expect(result).toEqual({
      metadataAvailable: false,
      cancelled: false,
      steamBranch: '',
    })
    expect(getBranches).not.toHaveBeenCalled()
    expect(dialog).not.toHaveBeenCalled()
  })

  it('returns the selected target when metadata is available', async () => {
    const getBranches = vi.fn().mockResolvedValue({
      currentTarget: 'public',
      targets: [
        create(UpdateTargetOptionSchema, {
          id: 'public',
          label: 'Public',
          latestVersion: '21600865',
        }),
        create(UpdateTargetOptionSchema, {
          id: 'latest_experimental',
          label: 'Unstable build',
          latestVersion: '22422094',
        }),
      ],
    })

    const dialog = vi.fn().mockImplementation(({ onOk }: { onOk: (value: string) => void }) => {
      onOk('latest_experimental')
    })

    const result = await chooseSteamBranchForUpdate({
      gameServerId: 'server-1',
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        selectedTarget: 'public',
        resolvedUpdateProvider: create(UpdateProviderConfigSchema, {
          kind: UpdateProviderKind.STEAMCMD,
        }),
        game: create(GameSchema, {
          id: 'game-1',
          steamAppid: '294420',
        }),
      }),
      getBranches,
      openDialog: dialog,
    })

    expect(result).toEqual({
      metadataAvailable: true,
      cancelled: false,
      steamBranch: 'latest_experimental',
    })
    expect(getBranches).toHaveBeenCalledWith('server-1')
    expect(dialog).toHaveBeenCalledWith(
      expect.objectContaining({
        currentBranch: 'public',
        items: expect.arrayContaining([
          expect.objectContaining({
            value: 'latest_experimental',
            label: 'Unstable build',
          }),
        ]),
      }),
    )
  })

  it('falls back cleanly when target metadata cannot be loaded', async () => {
    const getBranches = vi.fn().mockRejectedValue(new Error('boom'))
    const dialog = vi.fn()

    const result = await chooseSteamBranchForUpdate({
      gameServerId: 'server-1',
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        resolvedUpdateProvider: create(UpdateProviderConfigSchema, {
          kind: UpdateProviderKind.STEAMCMD,
        }),
        game: create(GameSchema, {
          id: 'game-1',
          steamAppid: '294420',
        }),
      }),
      getBranches,
      openDialog: dialog,
    })

    expect(result).toEqual({
      metadataAvailable: false,
      cancelled: false,
      steamBranch: '',
    })
    expect(dialog).not.toHaveBeenCalled()
  })

  it('cancels the update when the picker is dismissed', async () => {
    const getBranches = vi.fn().mockResolvedValue({
      currentTarget: 'public',
      targets: [
        create(UpdateTargetOptionSchema, {
          id: 'public',
          label: 'Public',
          latestVersion: '21600865',
        }),
      ],
    })

    const dialog = vi.fn().mockImplementation(({ onDismiss }: { onDismiss: () => void }) => {
      onDismiss()
    })

    const result = await chooseSteamBranchForUpdate({
      gameServerId: 'server-1',
      gameServer: create(GameServerSchema, {
        id: 'server-1',
        resolvedUpdateProvider: create(UpdateProviderConfigSchema, {
          kind: UpdateProviderKind.STEAMCMD,
        }),
        game: create(GameSchema, {
          id: 'game-1',
          steamAppid: '294420',
        }),
      }),
      getBranches,
      openDialog: dialog,
    })

    expect(result).toEqual({
      metadataAvailable: true,
      cancelled: true,
      steamBranch: '',
    })
  })
})
