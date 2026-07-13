import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  FileSchema,
  type ListDirectoryFilesResponse,
  ListDirectoryFilesResponseSchema,
  type File as XylonaFile,
} from '@/proto/gameserver_files_operations_pb'
import { GameServerSchema } from '@/proto/shared_pb'
import { GetGameServerResponseSchema } from '@/proto/xylona_pb'
import GameServerFiles from './GameServerFiles.vue'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  listDirectoryFiles: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      loading: { hide: vi.fn(), show: vi.fn() },
      notify: mocks.notify,
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
        }),
      }),
    )
  })

  afterEach(() => {
    mocks.getGameServer.mockReset()
    mocks.listDirectoryFiles.mockReset()
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
})
