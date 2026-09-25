import { create } from '@bufbuild/protobuf'
import { shallowMount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import { FileSchema } from '@/proto/gameserver_files_operations_pb'
import MoveFiles from './MoveFiles.vue'

function mountMove(path: string) {
  return shallowMount(MoveFiles, {
    props: {
      gameServerId: 'server-1',
      gameServerPath: '/srv/server',
      neighboringDirectoriesInPath: ['world', 'plugins', 'config'],
      path,
      selectedFiles: [create(FileSchema, { name: 'plugins', isDirectory: true })],
      showDialog: true,
    },
    global: { renderStubDefaultSlot: true },
  })
}

describe('MoveFiles', () => {
  it('offers the parent folder and leaves out the items being moved', () => {
    const wrapper = mountMove('server')

    expect(wrapper.text()).toContain('Move 1 item from server')
    const options = wrapper.findComponent({ name: 'QSelect' }).props('options')
    expect(options).toEqual([
      { label: '.. (parent folder)', value: '..' },
      { label: 'config', value: 'config' },
      { label: 'world', value: 'world' },
    ])
    wrapper.unmount()
  })

  it('has no parent option at the server root', () => {
    const wrapper = mountMove('')

    expect(wrapper.text()).toContain('Move 1 item from the server root')
    const options = wrapper.findComponent({ name: 'QSelect' }).props('options')
    expect(options.map((option: { value: string }) => option.value)).toEqual(['config', 'world'])
    wrapper.unmount()
  })
})
