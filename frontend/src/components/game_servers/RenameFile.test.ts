import { shallowMount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'

import RenameFile from './RenameFile.vue'

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ notify: vi.fn() }),
  }
})

describe('RenameFile', () => {
  it('prefills the selected file name whenever the dialog opens', async () => {
    const wrapper = shallowMount(RenameFile, {
      props: {
        gameServerId: 'server-1',
        gameServerPath: '/srv/server',
        oldFileName: 'server.cfg',
        path: 'config',
        showDialog: false,
      },
    })

    await wrapper.setProps({ showDialog: true })

    expect((wrapper.vm as unknown as { newFileName: string }).newFileName).toBe('server.cfg')
    wrapper.unmount()
  })
})
