import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { FileSchema } from '@/proto/gameserver_files_operations_pb'
import ArchiveFiles from './ArchiveFiles.vue'
import Create from './Create.vue'
import DeleteGameServerFilesDialog from './DeleteGameServerFilesDialog.vue'
import ExtractFiles from './ExtractFiles.vue'

const mocks = vi.hoisted(() => ({
  gameServerFileCreate: vi.fn(),
  gameServerFilesArchive: vi.fn(),
  gameServerFilesDelete: vi.fn(),
  gameServerFilesExtract: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ notify: mocks.notify }),
  }
})

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      gameServerFilesDelete: mocks.gameServerFilesDelete,
      gameServersFileOrDirectoryCreate: mocks.gameServerFileCreate,
    }),
    GetXylonaClientCallback: () => ({
      gameServerFilesArchive: mocks.gameServerFilesArchive,
      gameServerFilesExtract: mocks.gameServerFilesExtract,
    }),
  }
})

describe('file operation hardening', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('submits file creation once and emits the created file name as a string', async () => {
    let resolveCreate: (() => void) | undefined
    mocks.gameServerFileCreate.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveCreate = resolve
      }),
    )
    const wrapper = shallowMount(Create, {
      props: {
        gameServerId: 'server-1',
        gameServerPath: '/srv/server',
        path: 'config',
        showDialog: true,
      },
    })
    const viewModel = wrapper.vm as unknown as {
      createFileOrDirectory: () => Promise<void>
      fileName: string
    }
    viewModel.fileName = 'server.cfg'

    const firstSubmit = viewModel.createFileOrDirectory()
    const secondSubmit = viewModel.createFileOrDirectory()
    expect(mocks.gameServerFileCreate).toHaveBeenCalledTimes(1)

    resolveCreate?.()
    await Promise.all([firstSubmit, secondSubmit])
    expect(wrapper.emitted('submit')?.[0]?.[1]).toEqual({
      fileName: 'server.cfg',
      fullFilePath: 'config/server.cfg',
      isDir: false,
    })
    wrapper.unmount()
  })

  it('prevents repeated deletion and keeps the confirmation open after failure', async () => {
    mocks.gameServerFilesDelete.mockRejectedValue(new Error('node unavailable'))
    const wrapper = shallowMount(DeleteGameServerFilesDialog, {
      props: {
        currentPath: 'config',
        filesToDelete: [create(FileSchema, { name: 'server.cfg' })],
        gameServerID: 'server-1',
        pathSeparator: '/',
        showDialog: true,
      },
    })
    const viewModel = wrapper.vm as unknown as {
      deleteFiles: () => Promise<void>
      showDialog: boolean
    }

    await Promise.all([viewModel.deleteFiles(), viewModel.deleteFiles()])

    expect(mocks.gameServerFilesDelete).toHaveBeenCalledTimes(1)
    expect(viewModel.showDialog).toBe(true)
    expect(wrapper.emitted('filesDeleted')).toBeUndefined()
    wrapper.unmount()
  })

  it('preserves archive input when the operation fails', async () => {
    const wrapper = shallowMount(ArchiveFiles, {
      props: {
        archiveName: 'server-backup',
        gameServerId: 'server-1',
        path: 'world',
        selectedFiles: [create(FileSchema, { name: 'level.dat' })],
        showDialog: true,
      },
    })
    const viewModel = wrapper.vm as unknown as {
      archiveFiles: () => Promise<void>
      archiveName: string
      archiveSubmitting: boolean
      showDialog: boolean
    }

    await viewModel.archiveFiles()
    const complete = mocks.gameServerFilesArchive.mock.calls[0]?.[2] as
      ((error?: ConnectError) => void) | undefined
    complete?.(new ConnectError('node unavailable'))
    await flushPromises()

    expect(viewModel.archiveName).toBe('server-backup')
    expect(viewModel.archiveSubmitting).toBe(false)
    expect(viewModel.showDialog).toBe(true)
    wrapper.unmount()
  })

  it('preserves the extraction destination when the operation fails', async () => {
    const wrapper = shallowMount(ExtractFiles, {
      props: {
        fullArchivePath: 'world/server-backup.zip',
        gameServerId: 'server-1',
        gameServerPath: '/srv/server',
        path: 'world',
        showDialog: true,
      },
    })
    const viewModel = wrapper.vm as unknown as {
      extractFiles: () => Promise<void>
      extractSubmitting: boolean
      fullDestinationPath: string
      showDialog: boolean
    }
    viewModel.fullDestinationPath = 'restored'

    await viewModel.extractFiles()
    const complete = mocks.gameServerFilesExtract.mock.calls[0]?.[2] as
      ((error?: ConnectError) => void) | undefined
    complete?.(new ConnectError('node unavailable'))
    await flushPromises()

    expect(viewModel.fullDestinationPath).toBe('restored')
    expect(viewModel.extractSubmitting).toBe(false)
    expect(viewModel.showDialog).toBe(true)
    wrapper.unmount()
  })
})
