import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  FileSchema,
  type ListDirectoryFilesResponse,
  ListDirectoryFilesResponseSchema,
  type File as XylonaFile,
} from '@/proto/gameserver_files_operations_pb'
import PageHeader from '@/components/shared/PageHeader.vue'
import { GameServerSchema, type GameServer } from '@/proto/shared_pb'
import { GetGameServerResponseSchema } from '@/proto/xylona_pb'
import GameServerFiles from './GameServerFiles.vue'

const mocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  gameServerFilesDownloadFromURL: vi.fn(),
  getGameServer: vi.fn(),
  listDirectoryFiles: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    copyToClipboard: mocks.copyToClipboard,
    useQuasar: () => ({
      loading: { hide: vi.fn(), show: vi.fn() },
      notify: mocks.notify,
      platform: { has: { touch: false }, is: { mobile: false } },
      screen: { gt: { xs: true } },
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      gameServerFilesDownloadFromURL: mocks.gameServerFilesDownloadFromURL,
      getGameServer: mocks.getGameServer,
      listDirectoryFiles: mocks.listDirectoryFiles,
    }),
  }
})

describe('GameServerFiles', () => {
  beforeEach(() => {
    window.location.hash = ''
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: create(GameServerSchema, {
          id: 'server-1',
          directory: '/srv/server',
          effectivePermissions: ['game_server.files.view', 'game_server.files.edit'],
        }),
      }),
    )
    mocks.copyToClipboard.mockResolvedValue(undefined)
  })

  afterEach(() => {
    mocks.getGameServer.mockReset()
    mocks.gameServerFilesDownloadFromURL.mockReset()
    mocks.listDirectoryFiles.mockReset()
    mocks.copyToClipboard.mockReset()
    mocks.notify.mockReset()
    vi.unstubAllGlobals()
    window.location.hash = ''
  })

  it('invalidates selection immediately when directory navigation or path input begins', async () => {
    const configFile = create(FileSchema, { name: 'server.cfg' })
    const configDirectory = create(FileSchema, { name: 'config', isDirectory: true })
    mocks.listDirectoryFiles.mockResolvedValueOnce(
      create(ListDirectoryFilesResponseSchema, {
        files: [configFile, configDirectory],
      }),
    )

    let resolveNavigation: ((value: ListDirectoryFilesResponse) => void) | undefined
    const navigationResponse = new Promise<ListDirectoryFilesResponse>((resolve) => {
      resolveNavigation = resolve
    })
    mocks.listDirectoryFiles.mockReturnValueOnce(navigationResponse)
    mocks.listDirectoryFiles.mockResolvedValueOnce(
      create(ListDirectoryFilesResponseSchema, { files: [] }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      clickDirectory: (directory: XylonaFile) => Promise<void>
      directoryActionsEnabled: boolean
      downloadSelectedFiles: () => Promise<void>
      loadedPath: string
      path: string
      selectedFiles: XylonaFile[]
      updatePathFromInput: () => void
    }
    viewModel.selectedFiles = [configFile]

    const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    await viewModel.downloadSelectedFiles()
    expect(clickSpy).toHaveBeenCalledTimes(1)
    clickSpy.mockRestore()

    const navigation = viewModel.clickDirectory(configDirectory)

    expect(viewModel.selectedFiles).toEqual([])
    expect(viewModel.path).toBe('config')
    expect(viewModel.loadedPath).toBe('')
    expect(viewModel.directoryActionsEnabled).toBe(false)

    resolveNavigation?.(create(ListDirectoryFilesResponseSchema, { files: [] }))
    await navigation
    expect(viewModel.loadedPath).toBe('config')

    viewModel.selectedFiles = [configFile]
    viewModel.path = 'config/manual'
    viewModel.updatePathFromInput()

    expect(viewModel.selectedFiles).toEqual([])
    expect(viewModel.directoryActionsEnabled).toBe(false)
    wrapper.unmount()
  })

  it('rejects HTTP error bodies instead of opening them in the editor', async () => {
    const configFile = create(FileSchema, { name: 'server.cfg' })
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [configFile] }),
    )
    const readResponseBody = vi.fn().mockResolvedValue('upstream node unavailable')
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({
        ok: false,
        status: 502,
        statusText: 'Bad Gateway',
        text: readResponseBody,
      }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      editorModal: boolean
      readFileOctetStream: (fileName: string) => Promise<void>
    }

    await viewModel.readFileOctetStream('server.cfg')

    expect(viewModel.editorModal).toBe(false)
    expect(readResponseBody).not.toHaveBeenCalled()
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ caption: 'Error reading file server.cfg.' }),
    )
    wrapper.unmount()
  })

  it('renders the Files page title', async () => {
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [] }),
    )

    const wrapper = shallowMount(GameServerFiles, {
      global: {
        stubs: {
          QCardSection: { template: '<div><slot /></div>' },
        },
      },
    })
    await flushPromises()

    expect(wrapper.findComponent(PageHeader).props('title')).toBe('Files')
    wrapper.unmount()
  })

  it('clears selection and reloads safely for browser hash navigation', async () => {
    const configFile = create(FileSchema, { name: 'server.cfg' })
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [configFile] }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      loadedPath: string
      selectedFiles: XylonaFile[]
    }
    viewModel.selectedFiles = [configFile]

    window.location.hash = '#config'
    window.dispatchEvent(new Event('hashchange'))
    await flushPromises()

    expect(viewModel.selectedFiles).toEqual([])
    expect(viewModel.loadedPath).toBe('config')
    expect(mocks.listDirectoryFiles).toHaveBeenLastCalledWith(
      expect.objectContaining({ path: 'config' }),
    )
    wrapper.unmount()
  })

  it('applies explorer selection, context targeting, and folder navigation rules', async () => {
    const alpha = create(FileSchema, { name: 'alpha.cfg' })
    const beta = create(FileSchema, { name: 'beta.log' })
    const config = create(FileSchema, { name: 'config', isDirectory: true })
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [alpha, beta, config] }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      contextMenuIsBackground: boolean
      displayedEntries: XylonaFile[]
      loadedPath: string
      openBackgroundContextMenu: (event: Event) => void
      openItemContextMenu: (event: Event, entry: XylonaFile) => void
      selectEntry: (entry: XylonaFile, event: MouseEvent) => void
      selectEntryFromPointer: (entry: XylonaFile, event: MouseEvent) => void
      selectAllFiles: boolean
      selectedFiles: XylonaFile[]
    }

    viewModel.selectEntry(alpha, new MouseEvent('click'))
    viewModel.selectEntry(beta, new MouseEvent('click', { shiftKey: true }))
    expect(viewModel.selectedFiles.map((file) => file.name)).toEqual(['alpha.cfg', 'beta.log'])

    viewModel.selectEntry(alpha, new MouseEvent('click', { ctrlKey: true }))
    expect(viewModel.selectedFiles.map((file) => file.name)).toEqual(['beta.log'])

    viewModel.openItemContextMenu(new MouseEvent('contextmenu'), beta)
    expect(viewModel.selectedFiles.map((file) => file.name)).toEqual(['beta.log'])
    expect(viewModel.contextMenuIsBackground).toBe(false)

    viewModel.openItemContextMenu(new MouseEvent('contextmenu'), config)
    expect(viewModel.selectedFiles.map((file) => file.name)).toEqual(['config'])

    viewModel.openBackgroundContextMenu(new MouseEvent('contextmenu'))
    expect(viewModel.selectedFiles).toEqual([])
    expect(viewModel.contextMenuIsBackground).toBe(true)

    viewModel.selectEntryFromPointer(config, new MouseEvent('click'))
    await flushPromises()
    expect(viewModel.loadedPath).toBe('config')
    expect(viewModel.displayedEntries[0]?.name).toBe('..')

    viewModel.selectAllFiles = true
    expect(viewModel.selectedFiles.some((file) => file.name === '..')).toBe(false)
    viewModel.selectAllFiles = false

    const parentDirectory = viewModel.displayedEntries[0] as XylonaFile
    viewModel.selectEntryFromPointer(parentDirectory, new MouseEvent('click'))
    await flushPromises()
    expect(viewModel.loadedPath).toBe('')
    wrapper.unmount()
  })

  it('sorts and filters visible entries while keeping selection safe', async () => {
    const largeFile = create(FileSchema, { name: 'zeta.log', size: 20n })
    const smallFile = create(FileSchema, { name: 'alpha.cfg', size: 5n })
    const worlds = create(FileSchema, { name: 'worlds', size: 40n, isDirectory: true })
    const config = create(FileSchema, { name: 'config', size: 10n, isDirectory: true })
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, {
        files: [largeFile, worlds, smallFile, config],
      }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      displayedEntries: XylonaFile[]
      filterQuery: string
      selectedFiles: XylonaFile[]
      setSort: (column: 'name' | 'size' | 'modified') => void
    }

    expect(viewModel.displayedEntries.map((file) => file.name)).toEqual([
      'config',
      'worlds',
      'alpha.cfg',
      'zeta.log',
    ])

    viewModel.setSort('name')
    expect(viewModel.displayedEntries.map((file) => file.name)).toEqual([
      'worlds',
      'config',
      'zeta.log',
      'alpha.cfg',
    ])

    viewModel.setSort('size')
    expect(viewModel.displayedEntries.map((file) => file.name)).toEqual([
      'config',
      'worlds',
      'alpha.cfg',
      'zeta.log',
    ])

    viewModel.selectedFiles = [smallFile, largeFile]
    viewModel.filterQuery = 'zeta'
    await flushPromises()
    expect(viewModel.displayedEntries.map((file) => file.name)).toEqual(['zeta.log'])
    expect(viewModel.selectedFiles.map((file) => file.name)).toEqual(['zeta.log'])
    wrapper.unmount()
  })

  it('keeps directory history scoped and truncates its forward branch', async () => {
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [] }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      canNavigateForward: boolean
      listDirectoryFiles: (path: string, mode?: 'push' | 'replace' | 'none') => Promise<boolean>
      loadedPath: string
      navigateHistory: (offset: -1 | 1) => Promise<void>
      navigationHistory: string[]
      navigationHistoryIndex: number
      refreshFileList: () => Promise<void>
    }

    await viewModel.listDirectoryFiles('config')
    await viewModel.listDirectoryFiles('logs')
    expect(viewModel.navigationHistory).toEqual(['', 'config', 'logs'])

    await viewModel.navigateHistory(-1)
    expect(viewModel.loadedPath).toBe('config')
    expect(viewModel.navigationHistoryIndex).toBe(1)

    await viewModel.listDirectoryFiles('world')
    expect(viewModel.navigationHistory).toEqual(['', 'config', 'world'])
    expect(viewModel.canNavigateForward).toBe(false)

    await viewModel.refreshFileList()
    expect(viewModel.navigationHistory).toEqual(['', 'config', 'world'])
    wrapper.unmount()
  })

  it('gates mutations and copies full or relative paths', async () => {
    const configFile = create(FileSchema, { name: 'server.cfg' })
    const binaryFile = create(FileSchema, { name: 'server.jar' })
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [configFile, binaryFile] }),
    )

    const wrapper = shallowMount(GameServerFiles)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      canEditFiles: boolean
      copySelectedPaths: (fullPath: boolean) => Promise<void>
      deleteButtonEnabled: boolean
      downloadButtonEnabled: boolean
      editableSelectedFile?: XylonaFile
      gameServer: GameServer
      loadedPath: string
      path: string
      selectedFiles: XylonaFile[]
    }

    viewModel.loadedPath = 'config'
    viewModel.path = 'config'
    viewModel.selectedFiles = [configFile, binaryFile]
    await viewModel.copySelectedPaths(false)
    await viewModel.copySelectedPaths(true)

    expect(mocks.copyToClipboard).toHaveBeenNthCalledWith(1, 'config/server.cfg\nconfig/server.jar')
    expect(mocks.copyToClipboard).toHaveBeenNthCalledWith(
      2,
      '/srv/server/config/server.cfg\n/srv/server/config/server.jar',
    )
    expect(viewModel.deleteButtonEnabled).toBe(true)
    expect(viewModel.downloadButtonEnabled).toBe(true)

    viewModel.selectedFiles = [configFile]
    expect(viewModel.editableSelectedFile?.name).toBe('server.cfg')

    viewModel.gameServer = create(GameServerSchema, {
      id: 'server-1',
      directory: '/srv/server',
      effectivePermissions: ['game_server.files.view'],
    })
    expect(viewModel.canEditFiles).toBe(false)
    expect(viewModel.deleteButtonEnabled).toBe(false)
    expect(viewModel.editableSelectedFile).toBeUndefined()
    expect(viewModel.downloadButtonEnabled).toBe(true)
    wrapper.unmount()
  })

  it('opens the URL upload interface and downloads into the loaded directory', async () => {
    mocks.listDirectoryFiles.mockResolvedValue(
      create(ListDirectoryFilesResponseSchema, { files: [] }),
    )
    mocks.gameServerFilesDownloadFromURL.mockResolvedValue({ filePath: 'mods/server.jar' })

    const wrapper = shallowMount(GameServerFiles, {
      global: { renderStubDefaultSlot: true },
    })
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      fileUploaderDialog: boolean
      loadedPath: string
      path: string
      submitURLUpload: () => Promise<void>
      urlUpload: string
      urlUploadDialog: boolean
    }
    viewModel.loadedPath = 'mods'
    viewModel.path = 'mods'

    await wrapper.get('[data-testid="open-url-upload-dialog"]').trigger('click')
    expect(viewModel.urlUploadDialog).toBe(true)
    expect(viewModel.fileUploaderDialog).toBe(false)

    viewModel.urlUpload = 'file:///etc/passwd'
    await viewModel.submitURLUpload()
    expect(mocks.gameServerFilesDownloadFromURL).not.toHaveBeenCalled()

    viewModel.urlUpload = 'https://downloads.example.test/server.jar'
    await viewModel.submitURLUpload()

    expect(mocks.gameServerFilesDownloadFromURL).toHaveBeenCalledWith(
      expect.objectContaining({
        destinationBasePath: 'mods',
        gameServerId: 'server-1',
        url: 'https://downloads.example.test/server.jar',
      }),
    )
    expect(viewModel.urlUploadDialog).toBe(false)
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ message: 'server.jar uploaded from URL.' }),
    )
    wrapper.unmount()
  })
})
