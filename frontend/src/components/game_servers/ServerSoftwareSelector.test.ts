import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { Code, ConnectError } from '@connectrpc/connect'
import { nextTick } from 'vue'

import { ServerSoftwareOptionSchema, SoftwareVersionSchema } from '@/proto/shared_pb'
import ServerSoftwareSelector from './ServerSoftwareSelector.vue'

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  getServerSoftwareOptions: vi.fn(),
  getServerSoftwareVersions: vi.fn(),
  getServerSoftwareStatus: vi.fn(),
  setServerSoftware: vi.fn(),
  eventBusOn: vi.fn(),
  eventBusOff: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getServerSoftwareOptions: mocks.getServerSoftwareOptions,
    getServerSoftwareVersions: mocks.getServerSoftwareVersions,
    getServerSoftwareStatus: mocks.getServerSoftwareStatus,
    setServerSoftware: mocks.setServerSoftware,
  }),
  XylonaEventBus: {
    on: mocks.eventBusOn,
    off: mocks.eventBusOff,
  },
}))

const QUASAR_STUBS = {
  'q-dialog': {
    props: ['modelValue'],
    template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
  },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<section><slot /></section>' },
  'q-card-actions': { template: '<div><slot /></div>' },
  'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
  'q-select': {
    props: ['modelValue', 'options', 'label'],
    emits: ['update:modelValue'],
    template: '<div class="q-select-stub">{{ label }}</div>',
  },
  'q-btn': {
    props: ['label'],
    emits: ['click'],
    template: '<button @click="$emit(\'click\')">{{ label }}</button>',
  },
} as const

describe('ServerSoftwareSelector', () => {
  afterEach(() => {
    mocks.notify.mockReset()
    mocks.getServerSoftwareOptions.mockReset()
    mocks.getServerSoftwareVersions.mockReset()
    mocks.getServerSoftwareStatus.mockReset()
    mocks.setServerSoftware.mockReset()
    mocks.eventBusOn.mockReset()
    mocks.eventBusOff.mockReset()
  })

  it('emits a failed operation event with the selected software and version on synchronous apply failure', async () => {
    mocks.getServerSoftwareOptions.mockResolvedValueOnce({
      options: [
        create(ServerSoftwareOptionSchema, {
          id: 'paper',
          name: 'Paper',
          jarSource: 'papermc',
        }),
      ],
    })
    mocks.getServerSoftwareVersions.mockResolvedValue({
      versions: [
        create(SoftwareVersionSchema, {
          versionId: '1.21.5',
          versionString: 'Paper 1.21.5',
        }),
      ],
    })
    mocks.getServerSoftwareStatus.mockResolvedValueOnce({ status: 'idle' })
    mocks.setServerSoftware.mockRejectedValueOnce(
      new ConnectError('server must be stopped before changing software', Code.FailedPrecondition),
    )

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        currentSoftware: 'paper',
        currentVersion: 'Paper 1.21.4',
      },
      global: {
        stubs: QUASAR_STUBS,
      },
    })

    await Promise.resolve()
    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
    }

    vm.openChangeDialog()
    await nextTick()

    const buttons = wrapper.findAll('button')
    await buttons[1].trigger('click')
    await Promise.resolve()
    await Promise.resolve()

    const events = wrapper.emitted('software-operation-state')
    expect(events).toBeTruthy()
    expect(events?.[0]?.[0]).toEqual(
      expect.objectContaining({
        status: 'failed',
        softwareId: 'paper',
        softwareName: 'Paper',
        versionLabel: 'Paper 1.21.5',
        error: expect.stringContaining('server must be stopped before changing software'),
      }),
    )
    expect(mocks.notify).not.toHaveBeenCalledWith(
      expect.objectContaining({
        caption: expect.stringContaining('Failed to change server software'),
      }),
    )
  })
})
