import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { DNSRecordType, type DNSBinding } from '@/proto/xylona_pb'
import GameServerDNSBindingSettings from './GameServerDNSBindingSettings.vue'

const mocks = vi.hoisted(() => ({
  adoptBinding: vi.fn(),
  dialog: vi.fn(),
  getBinding: vi.fn(),
  notify: vi.fn(),
  removeBinding: vi.fn(),
  setBinding: vi.fn(),
  syncBinding: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    adoptDNSBindingRecord: mocks.adoptBinding,
    getDNSBinding: mocks.getBinding,
    removeDNSBinding: mocks.removeBinding,
    setDNSBinding: mocks.setBinding,
    syncDNSBinding: mocks.syncBinding,
  }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ dialog: mocks.dialog, notify: mocks.notify }),
  }
})

const binding: DNSBinding = {
  $typeName: 'xylona.DNSBinding',
  gameServerId: 'server-1',
  relativeName: 'play',
  fullyQualifiedName: 'play.example.com',
  recordType: DNSRecordType.DNS_RECORD_TYPE_A,
  bindAddress: '10.0.0.8',
  privateAddress: true,
  ttlSeconds: 300,
  owned: false,
  ownedFullyQualifiedName: '',
  ownedRecordType: DNSRecordType.DNS_RECORD_TYPE_UNSPECIFIED,
  ownedValue: '',
  ownedTtlSeconds: 0,
}

const QInputStub = defineComponent({
  inheritAttrs: false,
  props: ['modelValue'],
  emits: ['update:modelValue'],
  template:
    '<input v-bind="$attrs" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
})

describe('GameServerDNSBindingSettings', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.getBinding.mockResolvedValue({ configured: true, binding })
    mocks.setBinding.mockResolvedValue({ binding: { ...binding, relativeName: 'game' } })
    mocks.adoptBinding.mockResolvedValue({ binding: { ...binding, owned: true } })
    mocks.removeBinding.mockResolvedValue({})
  })

  it('keeps save local, confirms record adoption after a sync conflict, and warns before removal', async () => {
    const dialogHandlers: Array<() => void | Promise<void>> = []
    mocks.dialog.mockImplementation(() => ({
      onOk(handler: () => void | Promise<void>) {
        dialogHandlers.push(handler)
        return this
      },
    }))
    mocks.syncBinding.mockRejectedValue(
      new ConnectError('An existing DNS record requires adoption.', Code.AlreadyExists),
    )

    const wrapper = shallowMount(GameServerDNSBindingSettings, {
      props: { gameServerId: 'server-1' },
      global: { stubs: { 'q-input': QInputStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('play.example.com')
    expect(wrapper.find('.private-address-warning').exists()).toBe(true)

    await wrapper.get('[data-testid="dns-relative-name"]').setValue('game')
    await wrapper.get('[data-testid="save-dns-binding"]').trigger('click')
    await flushPromises()
    expect(mocks.setBinding).toHaveBeenCalledWith(
      expect.objectContaining({ gameServerId: 'server-1', relativeName: 'game' }),
    )
    expect(mocks.syncBinding).not.toHaveBeenCalled()

    await wrapper.get('[data-testid="sync-dns-binding"]').trigger('click')
    await flushPromises()
    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Adopt existing DNS record?',
        message: expect.stringContaining('without changing DNS'),
      }),
    )
    await dialogHandlers[0]?.()
    expect(mocks.adoptBinding).toHaveBeenCalledWith(
      expect.objectContaining({ gameServerId: 'server-1' }),
    )

    await wrapper.get('[data-testid="remove-dns-binding"]').trigger('click')
    expect(mocks.dialog).toHaveBeenLastCalledWith(
      expect.objectContaining({ message: expect.stringContaining('DNS records remain') }),
    )
    await dialogHandlers[1]?.()
    expect(mocks.removeBinding).toHaveBeenCalledWith(
      expect.objectContaining({ gameServerId: 'server-1' }),
    )
  })

  it('distinguishes a retained previous record from ownership of the desired record', async () => {
    mocks.getBinding.mockResolvedValue({
      configured: true,
      binding: {
        ...binding,
        owned: true,
        ownedFullyQualifiedName: 'old.example.com',
        ownedRecordType: DNSRecordType.DNS_RECORD_TYPE_AAAA,
        ownedValue: '2001:db8::1',
        ownedTtlSeconds: 300,
      },
    })

    const wrapper = shallowMount(GameServerDNSBindingSettings, {
      props: { gameServerId: 'server-1' },
      global: { stubs: { 'q-input': QInputStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Previous record retained')
    expect(wrapper.text()).toContain('old.example.com')
    expect(wrapper.text()).not.toContain('Owned by this binding')
  })

  it('refreshes the visible ownership state when drift releases ownership', async () => {
    mocks.getBinding.mockResolvedValue({
      configured: true,
      binding: {
        ...binding,
        owned: true,
        ownedFullyQualifiedName: binding.fullyQualifiedName,
        ownedRecordType: binding.recordType,
        ownedValue: binding.bindAddress,
        ownedTtlSeconds: 300,
      },
    })
    mocks.syncBinding.mockRejectedValue(
      new ConnectError('DNS record changed externally; ownership was released.', Code.Aborted),
    )

    const wrapper = shallowMount(GameServerDNSBindingSettings, {
      props: { gameServerId: 'server-1' },
      global: { stubs: { 'q-input': QInputStub } },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Owned by this binding')
    await wrapper.get('[data-testid="sync-dns-binding"]').trigger('click')
    await flushPromises()
    expect(wrapper.text()).toContain('Not owned')
    expect(wrapper.text()).not.toContain('Owned by this binding')
  })
})
