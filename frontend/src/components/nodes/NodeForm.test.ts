import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import NodeForm from './NodeForm.vue'

const mocks = vi.hoisted(() => ({
  copy: vi.fn(),
  notify: vi.fn(),
  routerBack: vi.fn(),
  routerPush: vi.fn(),
  generateNodePairingObject: vi.fn(),
}))

vi.mock('@vueuse/core', () => ({
  useClipboard: () => ({
    copy: mocks.copy,
  }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRouter: () => ({
    back: mocks.routerBack,
    push: mocks.routerPush,
  }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    generateNodePairingObject: mocks.generateNodePairingObject,
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
    'q-btn': {
      props: ['label'],
      emits: ['click'],
      template: '<button type="button" @click="$emit(\'click\')">{{ label }}</button>',
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
    expect(mocks.notify).toHaveBeenCalledWith({
      type: 'positive',
      message: 'Node join command copied to clipboard',
    })
  })
})
