import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import Editor from './Editor.vue'

const mocks = vi.hoisted(() => ({
  editFile: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ notify: mocks.notify }),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ gameServersFileEdit: mocks.editFile }),
}))

describe('Editor', () => {
  afterEach(() => {
    mocks.editFile.mockReset()
    mocks.notify.mockReset()
  })

  it('keeps the editor content open after a failed save and emits submit only after success', async () => {
    const wrapper = shallowMount(Editor, {
      props: {
        codeInput: 'server-port=25565',
        fileName: 'server.properties',
        fullFilePath: 'config/server.properties',
        gameServerId: 'server-1',
      },
    })
    const viewModel = wrapper.vm as unknown as {
      codeInput: string
      saveError: string
      saveFile: () => Promise<void>
    }

    mocks.editFile.mockRejectedValueOnce(new Error('node unavailable'))
    await viewModel.saveFile()

    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(viewModel.codeInput).toBe('server-port=25565')
    expect(viewModel.saveError).toContain('node unavailable')

    mocks.editFile.mockResolvedValueOnce({})
    await viewModel.saveFile()

    expect(wrapper.emitted('submit')).toHaveLength(1)
    expect(viewModel.codeInput).toBe('server-port=25565')
    expect(viewModel.saveError).toBe('')
    expect(mocks.notify).toHaveBeenLastCalledWith(
      expect.objectContaining({ caption: 'File server.properties saved successfully.' }),
    )
    expect(mocks.notify.mock.calls.at(-1)?.[0]).not.toHaveProperty('html')

    wrapper.unmount()
  })
})
