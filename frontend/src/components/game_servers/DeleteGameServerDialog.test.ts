import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import DeleteGameServerDialog from './DeleteGameServerDialog.vue'

const mocks = vi.hoisted(() => ({
  listGameServerBackups: vi.fn(),
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
  removeGameServer: vi.fn(),
}))

vi.mock('@/api/notifications', () => ({
  notifyError: mocks.notifyError,
  notifySuccess: mocks.notifySuccess,
}))

vi.mock('@/utils/shared', () => ({
  bytesToSize: (bytes: number) => `${bytes} B`,
  GetXylonaClient: () => ({
    listGameServerBackups: mocks.listGameServerBackups,
    removeGameServer: mocks.removeGameServer,
  }),
}))

const stubs = {
  'q-dialog': { template: '<div><slot /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-card-actions': { template: '<div><slot /></div>' },
  'q-btn': {
    props: ['label', 'disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
  },
  'q-checkbox': {
    props: ['modelValue', 'label', 'disable'],
    emits: ['update:modelValue'],
    template:
      '<label><input type="checkbox" :checked="modelValue" :disabled="disable" @change="$emit(\'update:modelValue\', $event.target.checked)" />{{ label }}</label>',
  },
  'router-link': { props: ['to'], template: '<a :href="to"><slot /></a>' },
}

function findButton(wrapper: ReturnType<typeof mount>, label: string) {
  const button = wrapper.findAll('button').find((candidate) => candidate.text() === label)
  if (!button) {
    throw new Error(`expected button ${label}`)
  }
  return button
}

describe('DeleteGameServerDialog', () => {
  afterEach(() => {
    mocks.listGameServerBackups.mockReset()
    mocks.notifyError.mockReset()
    mocks.notifySuccess.mockReset()
    mocks.removeGameServer.mockReset()
  })

  it('keeps backups by default and deletes them for every server only when checked', async () => {
    mocks.listGameServerBackups.mockImplementation((request: { gameServerId: string }) =>
      Promise.resolve({
        backups:
          request.gameServerId === 'server-1'
            ? [{ sizeBytes: 1000n }, { sizeBytes: 500n }]
            : [{ sizeBytes: 24n }],
      }),
    )
    mocks.removeGameServer.mockResolvedValue({})
    const wrapper = mount(DeleteGameServerDialog, {
      props: {
        gameServers: [
          { id: 'server-1', name: 'Alpha', canDeleteBackups: true },
          { id: 'server-2', name: 'Bravo', canDeleteBackups: true },
        ],
        showDialog: true,
      },
      global: { stubs },
    })
    await flushPromises()

    const checkbox = wrapper.get<HTMLInputElement>('input[type="checkbox"]')
    expect(checkbox.element.checked).toBe(false)
    expect(checkbox.element.disabled).toBe(false)
    expect(wrapper.text()).toContain('Also delete their backups')
    expect(wrapper.text()).toContain('3 backups · 1524 B')
    expect(wrapper.text()).toContain('Backup archives stay on disk')

    await checkbox.setValue(true)
    expect(wrapper.text()).toContain(
      'Their backup archives are permanently deleted before their folders.',
    )
    expect(wrapper.text()).toContain('its server is kept but stays stopped')
    expect(wrapper.text()).not.toContain('Backup archives stay on disk')

    await findButton(wrapper, 'Delete 2 servers').trigger('click')
    await flushPromises()

    expect(mocks.removeGameServer.mock.calls.map(([request]) => request)).toEqual([
      expect.objectContaining({ serverId: 'server-1', deleteBackups: true }),
      expect.objectContaining({ serverId: 'server-2', deleteBackups: true }),
    ])
  })

  it('uses singular copy for one server and describes the option by its summary', async () => {
    mocks.listGameServerBackups.mockResolvedValue({ backups: [{ sizeBytes: 24n }] })
    const wrapper = mount(DeleteGameServerDialog, {
      props: {
        gameServers: [{ id: 'server-1', name: 'Alpha', canDeleteBackups: true }],
        showDialog: true,
      },
      global: { stubs },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Also delete its backups')
    const describedBy = wrapper.get('label').attributes('aria-describedby')
    expect(wrapper.get(`#${describedBy}`).text()).toBe('1 backup · 24 B')

    await wrapper.get('input[type="checkbox"]').setValue(true)
    expect(wrapper.text()).toContain(
      'Its backup archives are permanently deleted before its folder.',
    )
    expect(wrapper.text()).toContain('the server is kept but stays stopped')
  })

  it('resets to keeping backups each time it opens', async () => {
    mocks.listGameServerBackups.mockResolvedValue({ backups: [] })
    mocks.removeGameServer.mockResolvedValue({})
    const wrapper = mount(DeleteGameServerDialog, {
      props: {
        gameServers: [{ id: 'server-1', name: 'Alpha', canDeleteBackups: true }],
        showDialog: true,
      },
      global: { stubs },
    })
    await flushPromises()
    expect(wrapper.text()).toContain('No backups recorded.')

    await wrapper.get('input[type="checkbox"]').setValue(true)
    await wrapper.setProps({ showDialog: false })
    await wrapper.setProps({ showDialog: true })
    await flushPromises()
    await findButton(wrapper, 'Delete server').trigger('click')
    await flushPromises()

    expect(mocks.removeGameServer).toHaveBeenCalledWith(
      expect.objectContaining({ serverId: 'server-1', deleteBackups: false }),
    )
  })

  it('disables the option and says why without the backup permission', async () => {
    const wrapper = mount(DeleteGameServerDialog, {
      props: {
        gameServers: [
          { id: 'server-1', name: 'Alpha', canDeleteBackups: true },
          { id: 'server-2', name: 'Bravo', canDeleteBackups: false },
        ],
        showDialog: true,
      },
      global: { stubs },
    })
    await flushPromises()

    expect(wrapper.get<HTMLInputElement>('input[type="checkbox"]').element.disabled).toBe(true)
    const describedBy = wrapper.get('label').attributes('aria-describedby')
    expect(wrapper.get(`#${describedBy}`).text()).toBe(
      'Needs the backup permission on every selected server.',
    )
    expect(mocks.listGameServerBackups).not.toHaveBeenCalled()
  })

  it('says it stops the server and erases its folder on the node, and links to Backups', () => {
    const wrapper = mount(DeleteGameServerDialog, {
      props: {
        gameServers: [
          { id: 'server-1', name: 'Alpha', nodeName: 'Local Node', directory: '/srv/alpha' },
        ],
        showDialog: true,
      },
      global: { stubs },
    })

    const text = wrapper.text()
    expect(text).toContain('Delete Game Server')
    expect(text).toContain('/srv/alpha')
    expect(text).toContain('on Local Node')
    expect(text).toContain('worlds, saves, configs and mods')
    expect(text).toContain('stops the server first if it is running')
    expect(text).toContain('Backup archives stay on disk')
    expect(wrapper.get('a').attributes('href')).toBe('/game-servers/server-1/backups')
    expect(wrapper.findAll('button').map((button) => button.text())).toContain('Delete server')
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
          { id: 'server-1', name: 'Alpha' },
          { id: 'server-2', name: 'Bravo' },
          { id: 'server-3', name: 'Charlie' },
        ],
        showDialog: true,
      },
      global: { stubs },
    })

    const deleteButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Delete 3 servers')
    if (!deleteButton) {
      throw new Error('expected delete button')
    }
    expect(wrapper.text()).toContain('DNS records stay at the provider')

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
    expect(mocks.notifySuccess).not.toHaveBeenCalled()
    expect(mocks.notifyError).toHaveBeenCalledTimes(1)
    expect(mocks.notifyError).toHaveBeenCalledWith(
      expect.stringContaining('Deleted: Alpha'),
      expect.objectContaining({
        message: 'Deleted 1; 2 failed.',
        timeout: 0,
        actions: [{ icon: 'close', 'aria-label': 'Dismiss' }],
      }),
    )
    expect(mocks.notifyError.mock.calls[0]?.[0]).toContain('Failed: Bravo — node unavailable')
    expect(mocks.notifyError.mock.calls[0]?.[0]).toContain('Failed: Charlie — permission denied')
  })
})
