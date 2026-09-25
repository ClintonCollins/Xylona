import { shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import Editor from './Editor.vue'

const mocks = vi.hoisted(() => ({
  uploadFormData: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ platform: { is: { mac: false } } }),
  }
})

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyError: mocks.notifyError,
}))

vi.mock('@/utils/upload', () => ({
  uploadFormData: mocks.uploadFormData,
}))

describe('Editor', () => {
  afterEach(() => {
    mocks.uploadFormData.mockReset()
    mocks.notifySuccess.mockReset()
    mocks.notifyError.mockReset()
  })

  it('keeps the editor content open after a failed save and emits submit only after success', async () => {
    const wrapper = shallowMount(Editor, {
      props: {
        codeInput: 'server-port=25565',
        fileName: 'server.properties',
        fullFilePath: 'config/server.properties',
        gameServerId: 'server-1',
      },
      global: { renderStubDefaultSlot: true },
    })
    const viewModel = wrapper.vm as unknown as {
      codeInput: string
      saveError: string
      saveFile: (options: { close: boolean }) => Promise<void>
    }

    mocks.uploadFormData.mockRejectedValueOnce(new Error('node unavailable'))
    await viewModel.saveFile({ close: true })

    expect(wrapper.emitted('submit')).toBeUndefined()
    expect(viewModel.codeInput).toBe('server-port=25565')
    expect(viewModel.saveError).toContain('node unavailable')
    // The inline alert is the only announcement; a toast would say it twice.
    await wrapper.vm.$nextTick()
    const alert = wrapper.get('[role="alert"]')
    expect(alert.classes()).toContain('xy-banner-negative')
    expect(alert.text()).toContain('node unavailable')
    expect(mocks.notifyError).not.toHaveBeenCalled()

    mocks.uploadFormData.mockResolvedValueOnce(undefined)
    await viewModel.saveFile({ close: true })

    expect(wrapper.emitted('submit')).toHaveLength(1)
    expect(viewModel.codeInput).toBe('server-port=25565')
    expect(viewModel.saveError).toBe('')
    expect(mocks.notifySuccess).toHaveBeenLastCalledWith(
      'File server.properties saved successfully.',
    )
    expect(mocks.uploadFormData).toHaveBeenLastCalledWith('/api/file/upload', expect.any(FormData))
    const savedForm = mocks.uploadFormData.mock.calls.at(-1)?.[1] as FormData
    expect(savedForm.get('gameServerId')).toBe('server-1')
    expect(savedForm.get('path')).toBe('config')
    const savedFile = savedForm.get('file')
    expect(savedFile).toBeInstanceOf(File)
    expect((savedFile as File).name).toBe('server.properties')

    // Ctrl/Cmd+S saves in place: it reports 'saved' and keeps the editor open.
    mocks.uploadFormData.mockResolvedValueOnce(undefined)
    await viewModel.saveFile({ close: false })

    expect(wrapper.emitted('submit')).toHaveLength(1)
    expect(wrapper.emitted('saved')).toHaveLength(1)

    wrapper.unmount()
  })
})
