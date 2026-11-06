import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { readDroppedFiles } from '@/utils/dropped-files'
import FileUploaderDrop from './FileUploaderDrop.vue'

vi.mock('@/utils/dropped-files', () => ({ readDroppedFiles: vi.fn() }))

function mountUploader() {
  return shallowMount(FileUploaderDrop, {
    attachTo: document.body,
    props: {
      fileUploaderDialog: true,
      gameServerId: 'server-1',
      path: 'saves',
      pathSeparator: '/',
      uploadURL: '/api/file/upload',
    },
    global: { renderStubDefaultSlot: true },
  })
}

function droppedItems(entry: FileSystemEntry = {} as FileSystemEntry) {
  return { items: [{ kind: 'file', webkitGetAsEntry: () => entry }] }
}

type UploaderState = {
  uploader: {
    files: Map<string, { path: string }>
    isUploading: boolean
    addNativeFiles: (files: File[]) => void
  }
  addingFiles: boolean
}

afterEach(() => {
  vi.resetAllMocks()
  document.body.innerHTML = ''
})

describe('folder upload discovery', () => {
  it('keeps the dialog and queue stable until the whole dropped folder is ready', async () => {
    let finishReading!: (files: File[]) => void
    vi.mocked(readDroppedFiles).mockReturnValue(new Promise((resolve) => (finishReading = resolve)))
    const wrapper = mountUploader()
    const state = wrapper.vm as unknown as UploaderState
    const entry = {} as FileSystemEntry

    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems(entry) })

    expect(readDroppedFiles).toHaveBeenCalledWith([entry])
    expect(state.addingFiles).toBe(true)
    expect(state.uploader.files.size).toBe(0)
    expect(wrapper.findComponent({ name: 'QDialog' }).props('persistent')).toBe(true)
    expect(wrapper.get('[data-testid="folder-upload-picker"]').attributes()).toHaveProperty(
      'disabled',
    )
    const upload = wrapper
      .findAllComponents({ name: 'QBtn' })
      .find((button) => button.props('label') === 'Upload')
    expect(upload?.props('disable')).toBe(true)

    // A repeated drop must not start another discovery while the first is pending.
    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })
    expect(readDroppedFiles).toHaveBeenCalledTimes(1)

    finishReading([
      new File(['level'], 'world/level.dat'),
      new File(['region'], 'world/region/0.mca'),
    ])
    await flushPromises()

    expect(state.addingFiles).toBe(false)
    expect([...state.uploader.files.keys()]).toEqual(['world/level.dat', 'world/region/0.mca'])
    expect(state.uploader.files.get('world/region/0.mca')?.path).toBe('saves/world/region')
    expect(wrapper.findComponent({ name: 'QDialog' }).props('persistent')).toBe(false)
    expect(upload?.props('disable')).toBe(false)
    wrapper.unmount()
  })

  it('preserves existing queued files after a failed read and allows a fresh drop', async () => {
    vi.mocked(readDroppedFiles).mockRejectedValueOnce(new Error('Unreadable folder'))
    const wrapper = mountUploader()
    const state = wrapper.vm as unknown as UploaderState
    state.uploader.addNativeFiles([new File(['config'], 'server.properties')])

    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })
    await flushPromises()

    expect(state.addingFiles).toBe(false)
    expect([...state.uploader.files.keys()]).toEqual(['server.properties'])
    expect(wrapper.get('[role="alert"]').text()).toContain('Nothing from this drop was added.')
    expect(wrapper.findComponent({ name: 'QDialog' }).props('persistent')).toBe(false)

    vi.mocked(readDroppedFiles).mockResolvedValueOnce([new File(['level'], 'world/level.dat')])
    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })
    await flushPromises()

    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect([...state.uploader.files.keys()]).toEqual(['server.properties', 'world/level.dat'])
    wrapper.unmount()
  })

  it('recovers when the browser cannot provide a file entry', async () => {
    const wrapper = mountUploader()
    await wrapper.get('#fileUploaderCard').trigger('drop', {
      dataTransfer: { items: [{ kind: 'file', webkitGetAsEntry: () => null }] },
    })
    await flushPromises()

    expect(readDroppedFiles).not.toHaveBeenCalled()
    expect((wrapper.vm as unknown as UploaderState).addingFiles).toBe(false)
    expect(wrapper.get('[role="alert"]').text()).toContain('Choose folder')
    wrapper.unmount()
  })

  it("ignores a closed dialog's pending read without disturbing a newer drop", async () => {
    let finishOldRead!: (files: File[]) => void
    let finishNewRead!: (files: File[]) => void
    vi.mocked(readDroppedFiles)
      .mockReturnValueOnce(new Promise((resolve) => (finishOldRead = resolve)))
      .mockReturnValueOnce(new Promise((resolve) => (finishNewRead = resolve)))
    const wrapper = mountUploader()
    const state = wrapper.vm as unknown as UploaderState

    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })
    await wrapper.setProps({ fileUploaderDialog: false })
    expect(state.addingFiles).toBe(false)
    await wrapper.setProps({ fileUploaderDialog: true })
    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })

    finishOldRead([new File(['old'], 'old-world/level.dat')])
    await flushPromises()
    expect(state.uploader.files.size).toBe(0)
    expect(state.addingFiles).toBe(true)

    finishNewRead([new File(['new'], 'new-world/level.dat')])
    await flushPromises()
    expect([...state.uploader.files.keys()]).toEqual(['new-world/level.dat'])
    expect(state.addingFiles).toBe(false)
    wrapper.unmount()
  })

  it.each([{ path: 'another-folder' }, { gameServerId: 'another-server' }])(
    'does not show a stale read error after changing destination: %j',
    async (props) => {
      let failReading!: (error: Error) => void
      vi.mocked(readDroppedFiles).mockReturnValue(
        new Promise((_resolve, reject) => (failReading = reject)),
      )
      const wrapper = mountUploader()
      const state = wrapper.vm as unknown as UploaderState
      await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })
      await wrapper.setProps(props)

      failReading(new Error('Old directory is no longer readable'))
      await flushPromises()

      expect(state.uploader.files.size).toBe(0)
      expect(state.addingFiles).toBe(false)
      expect(wrapper.find('[role="alert"]').exists()).toBe(false)
      wrapper.unmount()
    },
  )

  it('does not add pending files after the uploader is unmounted', async () => {
    let finishReading!: (files: File[]) => void
    vi.mocked(readDroppedFiles).mockReturnValue(new Promise((resolve) => (finishReading = resolve)))
    const wrapper = mountUploader()
    const state = wrapper.vm as unknown as UploaderState
    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })
    wrapper.unmount()

    finishReading([new File(['old'], 'world/level.dat')])
    await flushPromises()

    expect(state.uploader.files.size).toBe(0)
    expect(state.addingFiles).toBe(false)
  })

  it('does not accept a new drop while files are being uploaded', async () => {
    const wrapper = mountUploader()
    const state = wrapper.vm as unknown as UploaderState
    state.uploader.isUploading = true
    await nextTick()

    await wrapper.get('#fileUploaderCard').trigger('drop', { dataTransfer: droppedItems() })

    expect(readDroppedFiles).not.toHaveBeenCalled()
    expect(state.uploader.files.size).toBe(0)
    wrapper.unmount()
  })
})
