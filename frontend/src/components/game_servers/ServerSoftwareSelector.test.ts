import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'

import { UpdateProviderKind, VariantSchema } from '@/proto/shared_pb'
import ServerSoftwareSelector from './ServerSoftwareSelector.vue'

const mocks = vi.hoisted(() => ({
  getVariantOperationStatus: vi.fn(),
  getUpdateTargets: vi.fn(),
  setServerVariant: vi.fn(),
  recordLifecycleIntent: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getVariantOperationStatus: mocks.getVariantOperationStatus,
    getUpdateTargets: mocks.getUpdateTargets,
    setServerVariant: mocks.setServerVariant,
  }),
  XylonaEventBus: {
    on: vi.fn(),
    off: vi.fn(),
  },
}))

vi.mock('@/utils/game-server-notifications', () => ({
  recordLifecycleIntent: mocks.recordLifecycleIntent,
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
    }),
  }
})

const stubs = {
  'q-dialog': { template: '<div><slot /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-card-actions': { template: '<div><slot /></div>' },
  'q-btn': {
    template: '<button :data-disabled="String(!!disable)"><slot />{{ label }}</button>',
    props: ['label', 'disable'],
  },
  'q-icon': true,
  'q-select': { template: '<div><slot /></div>' },
  'q-toggle': {
    template: '<div data-testid="pin-toggle">{{ label }}</div>',
    props: ['label'],
  },
}

describe('ServerSoftwareSelector', () => {
  beforeEach(() => {
    mocks.getVariantOperationStatus.mockReset()
    mocks.getUpdateTargets.mockReset()
    mocks.setServerVariant.mockReset()
    mocks.recordLifecycleIntent.mockReset()
    mocks.getVariantOperationStatus.mockResolvedValue({ status: 'idle' })
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '',
      targets: [],
    })
  })

  it('exposes the current variant display name', async () => {
    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'paper',
        variants: [
          create(VariantSchema, {
            id: 'paper',
            name: 'Paper',
          }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      currentSoftwareDisplayName: string
    }
    expect(vm.currentSoftwareDisplayName).toBe('Paper')
  })

  it('submits the selected variant through the new RPC', async () => {
    mocks.setServerVariant.mockResolvedValue({
      status: 'complete',
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        variants: [
          create(VariantSchema, { id: 'vanilla', name: 'Vanilla' }),
          create(VariantSchema, { id: 'paper', name: 'Paper' }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedVariantId: string
      applyVariant: () => Promise<void>
    }

    vm.openChangeDialog()
    vm.selectedVariantId = 'paper'
    await vm.applyVariant()

    expect(mocks.setServerVariant).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-1',
        variantId: 'paper',
      }),
    )
    expect(wrapper.emitted('software-changed')).toBeTruthy()
  })

  it('records an install intent when the variant change continues asynchronously', async () => {
    mocks.setServerVariant.mockResolvedValue({
      status: 'installing',
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        variants: [
          create(VariantSchema, { id: 'vanilla', name: 'Vanilla' }),
          create(VariantSchema, { id: 'paper', name: 'Paper' }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedVariantId: string
      applyVariant: () => Promise<void>
    }

    vm.openChangeDialog()
    vm.selectedVariantId = 'paper'
    await vm.applyVariant()

    expect(mocks.recordLifecycleIntent).toHaveBeenCalledWith('server-1', 'install')
    expect(wrapper.emitted('software-changed')).toBeFalsy()
  })

  it('includes the selected target when variant target metadata is available', async () => {
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '1.21.2',
      targets: [
        {
          id: '1.21.2',
          label: '1.21.2',
          description: '',
          latestVersion: '1.21.2',
          isSelected: true,
        },
        {
          id: '1.21.1',
          label: '1.21.1',
          description: '',
          latestVersion: '1.21.1',
          isSelected: false,
        },
      ],
    })
    mocks.setServerVariant.mockResolvedValue({
      status: 'complete',
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        currentTarget: '1.21.2',
        currentTargetPinned: true,
        variants: [
          create(VariantSchema, {
            id: 'vanilla',
            name: 'Vanilla',
            updateProvider: { kind: UpdateProviderKind.MOJANG },
          }),
          create(VariantSchema, { id: 'paper', name: 'Paper' }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedVariantId: string
      selectedTarget: string
      selectedPinTarget: boolean
      applyVariant: () => Promise<void>
    }

    vm.openChangeDialog()
    await Promise.resolve()
    vm.selectedTarget = '1.21.2'
    vm.selectedPinTarget = true
    await vm.applyVariant()

    expect(mocks.getUpdateTargets).toHaveBeenCalled()
    expect(mocks.setServerVariant).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-1',
        variantId: 'vanilla',
        target: '1.21.2',
        pinTarget: true,
      }),
    )
  })

  it('enables apply when only the target changes on the current variant', async () => {
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '',
      targets: [
        {
          id: '1.21.4',
          label: '1.21.4',
          description: '',
          latestVersion: '1.21.4',
          isSelected: false,
        },
        {
          id: '1.21.3',
          label: '1.21.3',
          description: '',
          latestVersion: '1.21.3',
          isSelected: false,
        },
      ],
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        currentVersion: '1.21.4',
        currentInstalledVersion: '1.21.4',
        variants: [
          create(VariantSchema, {
            id: 'vanilla',
            name: 'Vanilla',
            updateProvider: { kind: UpdateProviderKind.MOJANG },
          }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedTarget: string
    }

    vm.openChangeDialog()
    await Promise.resolve()
    await nextTick()

    const buttons = wrapper.findAll('button')
    const applyButton = buttons.find((button) => button.text().includes('Apply'))
    expect(applyButton?.attributes('data-disabled')).toBe('true')

    vm.selectedTarget = '1.21.3'
    await nextTick()

    expect(applyButton?.attributes('data-disabled')).toBe('false')
  })

  it('shows the pin toggle only for Mojang and Paper variants', async () => {
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '',
      targets: [
        {
          id: '1.21.4',
          label: '1.21.4',
          description: '',
          latestVersion: '1.21.4',
          isSelected: false,
        },
      ],
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        currentInstalledVersion: '1.21.4',
        variants: [
          create(VariantSchema, {
            id: 'vanilla',
            name: 'Vanilla',
            updateProvider: { kind: UpdateProviderKind.MOJANG },
          }),
          create(VariantSchema, {
            id: 'steam',
            name: 'Steam',
            updateProvider: { kind: UpdateProviderKind.STEAMCMD },
          }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedVariantId: string
    }

    vm.openChangeDialog()
    await Promise.resolve()
    await nextTick()
    expect(wrapper.find('[data-testid="pin-toggle"]').exists()).toBe(true)

    vm.selectedVariantId = 'steam'
    await Promise.resolve()
    await nextTick()
    expect(wrapper.find('[data-testid="pin-toggle"]').exists()).toBe(false)
  })

  it('enables apply when only the pin mode changes', async () => {
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '',
      targets: [
        {
          id: '1.21.4',
          label: '1.21.4',
          description: '',
          latestVersion: '1.21.4',
          isSelected: false,
        },
      ],
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        currentInstalledVersion: '1.21.4',
        variants: [
          create(VariantSchema, {
            id: 'vanilla',
            name: 'Vanilla',
            updateProvider: { kind: UpdateProviderKind.MOJANG },
          }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedPinTarget: boolean
    }

    vm.openChangeDialog()
    await Promise.resolve()
    await nextTick()

    const buttons = wrapper.findAll('button')
    const applyButton = buttons.find((button) => button.text().includes('Apply'))
    expect(applyButton?.attributes('data-disabled')).toBe('true')

    vm.selectedPinTarget = true
    await nextTick()

    expect(applyButton?.attributes('data-disabled')).toBe('false')
  })

  it('preselects the installed version when tracking latest on the same variant', async () => {
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '',
      targets: [
        {
          id: '1.21.5',
          label: '1.21.5',
          description: '',
          latestVersion: '1.21.5',
          isSelected: false,
        },
        {
          id: '1.21.4',
          label: '1.21.4',
          description: '',
          latestVersion: '1.21.4',
          isSelected: false,
        },
      ],
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        currentInstalledVersion: '1.21.4',
        variants: [
          create(VariantSchema, {
            id: 'vanilla',
            name: 'Vanilla',
            updateProvider: { kind: UpdateProviderKind.MOJANG },
          }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedTarget: string
    }

    vm.openChangeDialog()
    await Promise.resolve()
    await nextTick()

    expect(vm.selectedTarget).toBe('1.21.4')
  })

  it('ignores stale stored targets when the server is tracking latest', async () => {
    mocks.getUpdateTargets.mockResolvedValue({
      currentTarget: '1.21.2',
      targets: [
        {
          id: '1.21.5',
          label: '1.21.5',
          description: '',
          latestVersion: '1.21.5',
          isSelected: false,
        },
        {
          id: '1.21.4',
          label: '1.21.4',
          description: '',
          latestVersion: '1.21.4',
          isSelected: false,
        },
      ],
    })

    const wrapper = mount(ServerSoftwareSelector, {
      props: {
        gameServerId: 'server-1',
        gameName: 'Minecraft',
        currentSoftware: 'vanilla',
        currentTarget: '1.21.2',
        currentTargetPinned: false,
        variants: [
          create(VariantSchema, {
            id: 'vanilla',
            name: 'Vanilla',
            updateProvider: { kind: UpdateProviderKind.MOJANG },
          }),
        ],
      },
      global: {
        stubs,
      },
    })

    await Promise.resolve()

    const vm = wrapper.vm as InstanceType<typeof ServerSoftwareSelector> & {
      openChangeDialog: () => void
      selectedTarget: string
    }

    vm.openChangeDialog()
    await Promise.resolve()
    await nextTick()

    expect(vm.selectedTarget).toBe('1.21.5')
  })
})
