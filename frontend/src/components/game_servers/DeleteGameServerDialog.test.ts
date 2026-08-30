import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { GameServerSchema } from '@/proto/shared_pb'
import DeleteGameServerDialog from './DeleteGameServerDialog.vue'

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  removeGameServer: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ notify: mocks.notify }),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ removeGameServer: mocks.removeGameServer }),
}))

describe('DeleteGameServerDialog', () => {
  afterEach(() => {
    mocks.notify.mockReset()
    mocks.removeGameServer.mockReset()
  })

  it('continues after a failure, reports each result once, and ignores a double submit', async () => {
    mocks.removeGameServer.mockImplementation((request: { serverId: string }) => {
      if (request.serverId === 'server-2') {
        return Promise.reject(new Error('node unavailable'))
      }
      if (request.serverId === 'server-3') {
        return Promise.reject('permission denied')
      }
      return Promise.resolve({})
    })

    const wrapper = mount(DeleteGameServerDialog, {
      props: {
        gameServers: [
          create(GameServerSchema, { id: 'server-1', name: 'Alpha' }),
          create(GameServerSchema, { id: 'server-2', name: 'Bravo' }),
          create(GameServerSchema, { id: 'server-3', name: 'Charlie' }),
        ],
        showDialog: true,
      },
      global: {
        stubs: {
          'q-dialog': { template: '<div><slot /></div>' },
          'q-card': { template: '<div><slot /></div>' },
          'q-card-section': { template: '<div><slot /></div>' },
          'q-card-actions': { template: '<div><slot /></div>' },
          'q-btn': {
            props: ['label', 'disable'],
            emits: ['click'],
            template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
          },
        },
      },
    })

    const deleteButton = wrapper.findAll('button').find((button) => button.text() === 'Delete')
    if (!deleteButton) {
      throw new Error('expected delete button')
    }
    expect(wrapper.text()).toContain('DNS records remain at the provider')

    await Promise.all([deleteButton.trigger('click'), deleteButton.trigger('click')])
    await flushPromises()

    expect(mocks.removeGameServer).toHaveBeenCalledTimes(3)
    expect(wrapper.emitted('submit')).toEqual([
      [
        {
          succeeded: [{ id: 'server-1', name: 'Alpha' }],
          failed: [
            { id: 'server-2', name: 'Bravo', error: 'node unavailable' },
            { id: 'server-3', name: 'Charlie', error: 'permission denied' },
          ],
        },
      ],
    ])
    expect(mocks.notify).toHaveBeenCalledTimes(1)
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        caption: expect.stringContaining('Deleted: Alpha'),
        message: 'Deleted 1; 2 failed.',
        type: 'xylona-error',
      }),
    )
    expect(mocks.notify.mock.calls[0]?.[0].caption).toContain('Failed: Bravo — node unavailable')
    expect(mocks.notify.mock.calls[0]?.[0].caption).toContain('Failed: Charlie — permission denied')
  })
})
