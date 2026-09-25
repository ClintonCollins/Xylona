import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { NodeSchema } from '@/proto/shared_pb'
import NodeForm from './NodeForm.vue'

const mocks = vi.hoisted(() => ({
  copy: vi.fn(),
  notify: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
  routerBack: vi.fn(),
  routerPush: vi.fn(),
  generateNodePairingObject: vi.fn(),
  getNode: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    copyToClipboard: mocks.copy,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyError: mocks.notifyError,
}))

vi.mock('vue-router', () => ({
  useRouter: () => ({
    back: mocks.routerBack,
    push: mocks.routerPush,
  }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    generateNodePairingObject: mocks.generateNodePairingObject,
    getNode: mocks.getNode,
  }),
}))

const globalStubs = {
  stubs: {
    'q-card': { template: '<div><slot /></div>' },
    'q-card-section': { template: '<div><slot /></div>' },
    'q-form': { template: '<form><slot /></form>' },
    'q-space': true,
    'q-input': {
      props: ['modelValue'],
      template: '<textarea readonly :value="modelValue"></textarea>',
    },
    'q-banner': { template: '<div><slot /><slot name="action" /></div>' },
    'q-icon': true,
    'q-btn': {
      props: ['label', 'disable'],
      emits: ['click'],
      template:
        '<button type="button" :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
    },
  },
}

function trimmedPanelURL() {
  return window.location.origin.trim().replace(/\/+$/, '')
}

describe('NodeForm', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('generates and copies a full node join command with the panel URL and join token', async () => {
    mocks.generateNodePairingObject.mockResolvedValueOnce({
      baseUrl: 'https://public-panel.example.com',
      pairingToken: 'join-token-123',
    })

    const wrapper = mount(NodeForm, { global: globalStubs })

    const generateButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Generate Join Command')
    if (!generateButton) {
      throw new Error('expected generate button to exist')
    }

    await generateButton.trigger('click')
    await flushPromises()

    expect(mocks.generateNodePairingObject).toHaveBeenCalledWith(
      expect.objectContaining({
        targetUrl: trimmedPanelURL(),
      }),
    )

    const expectedCommand =
      'xylona-node --controller-url https://public-panel.example.com --join-token join-token-123'
    expect(wrapper.find('textarea').element.value).toBe(expectedCommand)

    const copyButton = wrapper.findAll('button').find((button) => button.text() === 'Copy')
    if (!copyButton) {
      throw new Error('expected copy button to exist')
    }

    await copyButton.trigger('click')
    await flushPromises()

    expect(mocks.copy).toHaveBeenCalledWith(expectedCommand)
    expect(mocks.notifySuccess).toHaveBeenCalledWith('Node join command copied to clipboard')
  })

  it('replaces the Listen URL with a note for the in-process controller node', async () => {
    mocks.getNode.mockResolvedValueOnce({
      node: create(NodeSchema, { id: 'node-local', name: 'Controller', local: true }),
    })

    const wrapper = mount(NodeForm, {
      global: globalStubs,
      props: { existingNodeId: 'node-local' },
    })
    await flushPromises()

    expect(wrapper.findAll('textarea')).toHaveLength(1)
    expect(wrapper.text()).toContain('Runs inside the controller, so it needs no listen URL.')
  })

  it('keeps Save disabled and offers Retry when the node fails to load', async () => {
    mocks.getNode.mockRejectedValueOnce(new ConnectError('controller down', Code.Unavailable))

    const wrapper = mount(NodeForm, {
      global: globalStubs,
      props: { existingNodeId: 'node-1' },
    })
    await flushPromises()

    const button = (label: string) =>
      wrapper.findAll('button').find((candidate) => candidate.text() === label)
    expect(wrapper.text()).toContain('Node details could not be loaded.')
    expect(button('Save')?.attributes('disabled')).toBeDefined()

    mocks.getNode.mockResolvedValueOnce({
      node: create(NodeSchema, { id: 'node-1', name: 'Rack A' }),
    })
    await button('Retry')?.trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('Node details could not be loaded.')
    expect(button('Save')?.attributes('disabled')).toBeUndefined()
  })

  it('leaves the add form for the node list instead of browser history', async () => {
    const wrapper = mount(NodeForm, { global: globalStubs })

    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Back')
      ?.trigger('click')

    expect(mocks.routerPush).toHaveBeenCalledWith('/nodes')
    expect(mocks.routerBack).not.toHaveBeenCalled()
  })
})
