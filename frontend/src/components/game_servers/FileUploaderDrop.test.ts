import { shallowMount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { nextTick } from 'vue'

import FileUploaderDrop from './FileUploaderDrop.vue'

describe('FileUploaderDrop', () => {
  it('queues files chosen with the native file and folder pickers', async () => {
    const wrapper = shallowMount(FileUploaderDrop, {
      props: {
        fileUploaderDialog: true,
        gameServerId: 'server-1',
        path: 'mods',
        pathSeparator: '/',
        uploadURL: '/api/file/upload',
      },
      global: { renderStubDefaultSlot: true },
    })

    const fileInput = wrapper.get('[data-testid="file-upload-picker"]')
    const folderInput = wrapper.get('[data-testid="folder-upload-picker"]')
    expect(fileInput.attributes()).toHaveProperty('multiple')
    expect(folderInput.attributes()).toHaveProperty('multiple')
    expect(folderInput.attributes()).toHaveProperty('webkitdirectory')

    const selectedFile = new File(['server'], 'server.jar')
    Object.defineProperty(fileInput.element, 'files', {
      configurable: true,
      value: [selectedFile],
    })
    await fileInput.trigger('change')

    const selectedFolderFile = new File(['world'], 'level.dat')
    Object.defineProperty(selectedFolderFile, 'webkitRelativePath', {
      configurable: true,
      value: 'world/level.dat',
    })
    Object.defineProperty(folderInput.element, 'files', {
      configurable: true,
      value: [selectedFolderFile],
    })
    await folderInput.trigger('change')

    const viewModel = wrapper.vm as unknown as {
      uploader: { files: Map<string, unknown> }
    }
    expect([...viewModel.uploader.files.keys()]).toEqual(['server.jar', 'world/level.dat'])
    wrapper.unmount()
  })

  it('names the destination and cannot be dismissed mid-upload', async () => {
    const wrapper = shallowMount(FileUploaderDrop, {
      props: {
        destination: '/srv/minecraft/mods',
        fileUploaderDialog: true,
        gameServerId: 'server-1',
        path: 'mods',
        pathSeparator: '/',
        uploadURL: '/api/file/upload',
      },
      global: { renderStubDefaultSlot: true },
    })

    expect(wrapper.get('.file-upload-destination').text()).toBe('/srv/minecraft/mods')
    const dialog = wrapper.findComponent({ name: 'QDialog' })
    expect(dialog.props('persistent')).toBe(false)

    const viewModel = wrapper.vm as unknown as { uploader: { isUploading: boolean } }
    viewModel.uploader.isUploading = true
    await nextTick()

    expect(dialog.props('persistent')).toBe(true)
    wrapper.unmount()
  })
})
