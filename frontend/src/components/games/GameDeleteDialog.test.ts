import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'
import GameDeleteDialog from './GameDeleteDialog.vue'

const mocks = vi.hoisted(() => ({
  listGameServers: vi.fn(),
  notifyConnectError: vi.fn(),
  notifySuccess: vi.fn(),
  removeGame: vi.fn(),
}))

vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notifyConnectError,
  notifySuccess: mocks.notifySuccess,
}))

vi.mock('@/api/connect-client', () => ({
  getXylonaClient: () => ({
    listGameServers: mocks.listGameServers,
    removeGame: mocks.removeGame,
  }),
}))

function mountDialog(game = create(GameSchema, { id: 'game-1', name: 'Minecraft' })) {
  return mount(GameDeleteDialog, {
    props: {
      game,
      showDialog: true,
    },
    global: {
      stubs: {
        'q-dialog': { template: '<div><slot /></div>' },
        'q-card': { template: '<div><slot /></div>' },
        'q-card-section': { template: '<div><slot /></div>' },
        'q-card-actions': { template: '<div><slot /></div>' },
        'q-banner': { template: '<div v-bind="$attrs"><slot /></div>' },
        'q-icon': true,
        'q-btn': {
          props: ['label', 'disable'],
          emits: ['click'],
          template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
        },
      },
    },
  })
}

function deleteButton(wrapper: ReturnType<typeof mountDialog>) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text() === 'Delete')
  if (!button) {
    throw new Error('expected delete button')
  }
  return button
}

describe('GameDeleteDialog', () => {
  beforeEach(() => {
    mocks.listGameServers.mockResolvedValue({ gameServers: [] })
  })

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
    await flushPromises()

    expect(wrapper.text()).toContain('This action cannot be undone.')
    await deleteButton(wrapper).trigger('click')
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

  it('names the servers using the game and blocks the delete', async () => {
    mocks.listGameServers.mockResolvedValue({
      gameServers: [
        { gameId: 'game-1', name: 'Undead Legacy' },
        { gameId: 'other', name: 'Other Server' },
      ],
    })

    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.get('[data-test="game-in-use"]').text()).toContain(
      'Used by 1 game server: Undead Legacy.',
    )
    expect(wrapper.text()).not.toContain('Other Server')
    expect(deleteButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('says an official game comes back when the controller starts', async () => {
    const wrapper = mountDialog(
      create(GameSchema, { id: 'minecraft', name: 'Minecraft', xylonaOfficial: true }),
    )
    await flushPromises()

    expect(wrapper.get('[data-test="official-note"]').text()).toContain(
      'restored the next time the controller starts',
    )
    expect(wrapper.text()).not.toContain('cannot be undone')
    expect(deleteButton(wrapper).attributes('disabled')).toBeUndefined()
  })
})
