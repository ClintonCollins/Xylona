import { flushPromises, shallowMount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { DNSCredentialMode, DNSProviderKind, type DNSProviderConnection } from '@/proto/xylona_pb'
import DNSProviderSettings from './DNSProviderSettings.vue'

const mocks = vi.hoisted(() => ({
  getConnection: vi.fn(),
  listZones: vi.fn(),
  notify: vi.fn(),
  setConnection: vi.fn(),
}))

const QInputStub = defineComponent({
  inheritAttrs: false,
  props: ['modelValue'],
  emits: ['update:modelValue'],
  template:
    '<input v-bind="$attrs" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
})

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getDNSProviderConnection: mocks.getConnection,
    listDNSProviderZones: mocks.listZones,
    setDNSProviderConnection: mocks.setConnection,
  }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ notify: mocks.notify }),
  }
})

describe('DNSProviderSettings', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.getConnection.mockResolvedValue({ configured: false })
  })

  it('lists candidate zones and activates a tested connection without retaining typed secrets', async () => {
    mocks.listZones.mockResolvedValue({
      zones: [{ id: 'zone-1', name: 'example.com' }],
    })
    mocks.setConnection.mockResolvedValue({
      connection: {
        provider: DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE,
        zoneName: 'example.com',
        zoneId: 'zone-1',
        credentialMode: DNSCredentialMode.DNS_CREDENTIAL_MODE_CLOUDFLARE_API_TOKEN,
        credentialsConfigured: true,
      } satisfies DNSProviderConnection,
    })

    const wrapper = shallowMount(DNSProviderSettings, {
      global: { stubs: { 'q-input': QInputStub } },
    })
    await flushPromises()

    await wrapper.get('[data-testid="cloudflare-api-token"]').setValue('top-secret-token')
    await wrapper.get('[data-testid="list-dns-zones"]').trigger('click')
    await flushPromises()

    expect(mocks.listZones).toHaveBeenCalledWith(
      expect.objectContaining({
        candidate: expect.objectContaining({
          provider: DNSProviderKind.DNS_PROVIDER_KIND_CLOUDFLARE,
          cloudflareApiToken: 'top-secret-token',
        }),
      }),
    )

    await wrapper.get('[data-testid="dns-zone-name"]').setValue('example.com')
    await wrapper.get('[data-testid="dns-zone-id"]').setValue('zone-1')
    await wrapper.get('[data-testid="activate-dns-provider"]').trigger('click')
    await flushPromises()

    expect(mocks.setConnection).toHaveBeenCalledWith(
      expect.objectContaining({
        candidate: expect.objectContaining({
          zoneName: 'example.com',
          zoneId: 'zone-1',
        }),
      }),
    )
    expect(
      (wrapper.get('[data-testid="cloudflare-api-token"]').element as HTMLInputElement).value,
    ).toBe('')
    expect(wrapper.text()).not.toContain('top-secret-token')
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ message: 'DNS provider connection tested and activated.' }),
    )
  })

  it('keeps the stored Route 53 static-key mode when loading the active connection', async () => {
    mocks.getConnection.mockResolvedValue({
      configured: true,
      connection: {
        provider: DNSProviderKind.DNS_PROVIDER_KIND_ROUTE53,
        zoneName: 'example.com',
        zoneId: 'Z123',
        credentialMode: DNSCredentialMode.DNS_CREDENTIAL_MODE_AWS_ACCESS_KEY,
        credentialsConfigured: true,
      } satisfies DNSProviderConnection,
    })

    const wrapper = shallowMount(DNSProviderSettings, {
      global: { stubs: { 'q-input': QInputStub } },
    })
    await flushPromises()

    expect(wrapper.find('[aria-label="AWS access key ID"]').exists()).toBe(true)
    expect(wrapper.find('[aria-label="AWS secret access key"]').exists()).toBe(true)
  })
})
