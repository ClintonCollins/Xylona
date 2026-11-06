import { enableAutoUnmount, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import Editor from './Editor.vue'

const mocks = vi.hoisted(() => ({
  uploadFormData: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
  dialog: vi.fn(),
  leaveGuard: undefined as undefined | (() => boolean | Promise<boolean>),
  updateGuard: undefined as
    undefined | ((to: { path: string }, from: { path: string }) => boolean | Promise<boolean>),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ dialog: mocks.dialog, platform: { is: { mac: false } } }),
  }
})

vi.mock('vue-router', () => ({
  onBeforeRouteLeave: (guard: () => boolean | Promise<boolean>) => {
    mocks.leaveGuard = guard
  },
  onBeforeRouteUpdate: (
    guard: (to: { path: string }, from: { path: string }) => boolean | Promise<boolean>,
  ) => {
    mocks.updateGuard = guard
  },
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyError: mocks.notifyError,
}))

vi.mock('@/utils/upload', () => ({
  uploadFormData: mocks.uploadFormData,
}))

enableAutoUnmount(afterEach)

function answerDialog(answer: 'ok' | 'cancel') {
  mocks.dialog.mockImplementation(() => {
    const chain = {
      onOk: (handler: () => void) => {
        if (answer === 'ok') handler()
        return chain
      },
      onCancel: (handler: () => void) => {
        if (answer === 'cancel') handler()
        return chain
      },
      onDismiss: () => chain,
    }
    return chain
  })
}

function mountEditor() {
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
    dirty: boolean
    requestClose: () => Promise<void>
    saveFile: (options: { close: boolean }) => Promise<void>
  }
  return { wrapper, viewModel }
}

describe('Editor', () => {
  beforeEach(() => {
    // These tests exercise save/navigation state, not Monaco's delayed startup.
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
    mocks.dialog.mockReset()
    mocks.leaveGuard = undefined
    mocks.updateGuard = undefined
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

  it('protects edits when navigating away or switching servers, but not a clean editor', async () => {
    const { wrapper } = mountEditor()

    await expect(Promise.resolve(mocks.leaveGuard?.())).resolves.toBe(true)
    expect(mocks.dialog).not.toHaveBeenCalled()

    await wrapper.setProps({ codeInput: 'server-port=25566' })
    answerDialog('cancel')
    await expect(Promise.resolve(mocks.leaveGuard?.())).resolves.toBe(false)
    await expect(
      Promise.resolve(
        mocks.updateGuard?.(
          { path: '/game-servers/server-2/files' },
          { path: '/game-servers/server-1/files' },
        ),
      ),
    ).resolves.toBe(false)
    expect(wrapper.props('codeInput')).toBe('server-port=25566')

    answerDialog('ok')
    await expect(Promise.resolve(mocks.leaveGuard?.())).resolves.toBe(true)
  })

  it('protects browser reload until edits are saved and removes the guard on unmount', async () => {
    const { wrapper, viewModel } = mountEditor()
    await wrapper.setProps({ codeInput: 'server-port=25566' })
    const dirtyEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(dirtyEvent)
    expect(dirtyEvent.defaultPrevented).toBe(true)

    mocks.uploadFormData.mockResolvedValueOnce(undefined)
    await viewModel.saveFile({ close: false })
    const cleanEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(cleanEvent)
    expect(cleanEvent.defaultPrevented).toBe(false)

    await wrapper.setProps({ codeInput: 'server-port=25567' })
    wrapper.unmount()
    const unmountedEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unmountedEvent)
    expect(unmountedEvent.defaultPrevented).toBe(false)
  })

  it('keeps edits after Cancel and closes only when discarding is confirmed', async () => {
    const { wrapper, viewModel } = mountEditor()
    await wrapper.setProps({ codeInput: 'server-port=25566' })

    answerDialog('cancel')
    await viewModel.requestClose()
    expect(wrapper.emitted('close')).toBeUndefined()
    expect(viewModel.dirty).toBe(true)

    answerDialog('ok')
    await viewModel.requestClose()
    expect(wrapper.emitted('close')).toHaveLength(1)
    expect(mocks.dialog).toHaveBeenLastCalledWith(
      expect.objectContaining({
        message:
          'You have unsaved changes to server.properties. Discard them and close the editor?',
      }),
    )
  })

  it.each([true, false])(
    'keeps newer edits open and unsaved when an earlier upload completes (close=%s)',
    async (close) => {
      const { wrapper, viewModel } = mountEditor()
      await wrapper.setProps({ codeInput: 'server-port=25566' })
      let finishUpload!: () => void
      mocks.uploadFormData.mockImplementationOnce(
        () => new Promise<void>((resolve) => (finishUpload = resolve)),
      )

      const saving = viewModel.saveFile({ close })
      // A repeated save must not race the in-flight request.
      await viewModel.saveFile({ close })
      expect(mocks.uploadFormData).toHaveBeenCalledOnce()
      await wrapper.setProps({ codeInput: 'server-port=25567' })
      finishUpload()
      await saving

      const savedForm = mocks.uploadFormData.mock.calls[0]?.[1] as FormData
      expect(await (savedForm.get('file') as File).text()).toBe('server-port=25566')
      expect(wrapper.props('codeInput')).toBe('server-port=25567')
      expect(viewModel.dirty).toBe(true)
      expect(wrapper.emitted('submit')).toBeUndefined()
      expect(wrapper.emitted('saved')).toHaveLength(1)
      expect(mocks.notifySuccess).toHaveBeenLastCalledWith(
        'File server.properties saved. Newer edits are still unsaved.',
      )

      mocks.uploadFormData.mockResolvedValueOnce(undefined)
      await viewModel.saveFile({ close: true })
      expect(viewModel.dirty).toBe(false)
      expect(wrapper.emitted('submit')).toHaveLength(1)
    },
  )

  it('protects an in-flight save even if the operator restores the original text', async () => {
    const { wrapper, viewModel } = mountEditor()
    await wrapper.setProps({ codeInput: 'server-port=25566' })
    let finishUpload!: () => void
    mocks.uploadFormData.mockImplementationOnce(
      () => new Promise<void>((resolve) => (finishUpload = resolve)),
    )
    const saving = viewModel.saveFile({ close: true })
    await wrapper.setProps({ codeInput: 'server-port=25565' })
    expect(viewModel.dirty).toBe(false)

    const pendingEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(pendingEvent)
    expect(pendingEvent.defaultPrevented).toBe(true)
    await viewModel.requestClose()
    expect(wrapper.emitted('close')).toBeUndefined()

    finishUpload()
    await saving
    expect(viewModel.dirty).toBe(true)
    expect(wrapper.emitted('submit')).toBeUndefined()
  })
})
