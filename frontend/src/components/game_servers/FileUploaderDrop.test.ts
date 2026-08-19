import { shallowMount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

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
})
