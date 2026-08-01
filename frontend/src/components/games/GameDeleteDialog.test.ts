import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'
import GameDeleteDialog from './GameDeleteDialog.vue'

const mocks = vi.hoisted(() => ({
  notifyConnectError: vi.fn(),
  notifySuccess: vi.fn(),
  removeGame: vi.fn(),
}))

vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notifyConnectError,
  notifySuccess: mocks.notifySuccess,
}))

vi.mock('@/api/connect-client', () => ({
  getXylonaClient: () => ({ removeGame: mocks.removeGame }),
}))

function mountDialog() {
  return mount(GameDeleteDialog, {
    props: {
      game: create(GameSchema, { id: 'game-1', name: 'Minecraft' }),
      showDialog: true,
    },
    global: {
      stubs: {
        'q-dialog': { template: '<div><slot /></div>' },
        'q-card': { template: '<div><slot /></div>' },
        'q-card-section': { template: '<div><slot /></div>' },
        'q-card-actions': { template: '<div><slot /></div>' },
        'q-btn': {
          props: ['label'],
          emits: ['click'],
          template: '<button @click="$emit(\'click\')">{{ label }}</button>',
        },
      },
    },
  })
}

describe('GameDeleteDialog', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it.each([
    { name: 'closes and reports success', succeeds: true, submitError: false },
    { name: 'stays open and reports failure', succeeds: false, submitError: true },
  ])('$name', async ({ succeeds, submitError }) => {
    const error = new Error('delete failed')
    if (succeeds) {
      mocks.removeGame.mockResolvedValueOnce({})
    } else {
      mocks.removeGame.mockRejectedValueOnce(error)
    }

    const wrapper = mountDialog()
    const deleteButton = wrapper.findAll('button').find((button) => button.text() === 'Delete')
    if (!deleteButton) {
      throw new Error('expected delete button')
    }

    await deleteButton.trigger('click')
    await flushPromises()

    expect(mocks.removeGame).toHaveBeenCalledWith(expect.objectContaining({ gameId: 'game-1' }))
    expect(wrapper.emitted('submit')).toEqual([[submitError]])

    if (succeeds) {
      expect(mocks.notifySuccess).toHaveBeenCalledWith('Minecraft deleted successfully', {
        timeout: 5000,
      })
      expect(wrapper.emitted('update:showDialog')).toEqual([[false]])
      expect(mocks.notifyConnectError).not.toHaveBeenCalled()
    } else {
      expect(mocks.notifyConnectError).toHaveBeenCalledWith(error, 'Error deleting game')
      expect(wrapper.emitted('update:showDialog')).toBeUndefined()
      expect(mocks.notifySuccess).not.toHaveBeenCalled()
    }
  })
})
