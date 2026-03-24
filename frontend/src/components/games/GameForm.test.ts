import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CommandProcessor,
  CommandType,
  GameSchema,
  ModProfileSchema,
  ModSourceSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
  VariantSchema,
} from '@/proto/shared_pb'
import GameForm from './GameForm.vue'

const mocks = vi.hoisted(() => ({
  getGame: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      getGame: mocks.getGame,
      addGame: vi.fn(),
      editGame: vi.fn(),
      updateGameConfigSchemas: vi.fn(),
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      back: vi.fn(),
      push: vi.fn(),
    }),
  }
})

const QInputStub = defineComponent({
  name: 'QInputStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<div class="q-input-stub" v-bind="$attrs">{{ label }}</div>',
})

function mountGameForm() {
  return mount(GameForm, {
    props: {
      existingGameId: 'minecraft',
    },
    global: {
      stubs: {
        'q-form': { template: '<form><slot /></form>' },
        'q-input': QInputStub,
        'q-select': {
          template: '<div class="q-select-stub" v-bind="$attrs">{{ label }}<slot /></div>',
          props: ['label'],
        },
        'q-btn': { template: '<button><slot />{{ label }}</button>', props: ['label'] },
        'q-icon': true,
        'q-badge': { template: '<span><slot />{{ label }}</span>', props: ['label'] },
        'q-spinner-dots': true,
        'router-link': { template: '<a><slot /></a>' },
        ConfigSchemaList: { template: '<div />' },
      },
    },
  })
}

describe('GameForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        disconnect() {}
        observe() {}
      },
    )

    mocks.getGame.mockReset()
  })

  it('keeps install controls editable for managed variant-backed games', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        linuxInstallType: CommandType.COMMAND,
        linuxInstallCommand: './install.sh',
        linuxInstallCommandProcessor: CommandProcessor.BASH,
        linuxUpdateType: CommandType.COMMAND,
        linuxUpdateCommand: './update.sh',
        linuxUpdateCommandProcessor: CommandProcessor.BASH,
        updateProvider: create(UpdateProviderConfigSchema, {
          kind: UpdateProviderKind.MOJANG,
          sourceId: 'vanilla',
        }),
        variants: [create(VariantSchema, { id: 'paper', name: 'Paper' })],
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    expect(wrapper.text()).toContain('Editing Minecraft')
    expect(wrapper.find('[data-testid="linux-install-type"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="linux-install-shell"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="linux-install-command"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="linux-update-type"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="linux-update-shell"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="linux-update-command"]').exists()).toBe(true)
  })

  it('keeps basic mod fields editable for managed variant-backed games with simple mod support', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        updateProvider: create(UpdateProviderConfigSchema, {
          kind: UpdateProviderKind.MOJANG,
          sourceId: 'vanilla',
        }),
        variants: [create(VariantSchema, { id: 'paper', name: 'Paper' })],
        modProfile: create(ModProfileSchema, {
          installPath: 'plugins/',
          sources: [
            create(ModSourceSchema, {
              id: 'hangar',
              searchParamsJson: '{"platform":"PAPER"}',
            }),
          ],
        }),
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    expect(wrapper.text()).toContain('Mod Support')
    expect(wrapper.text()).not.toContain(
      'This game uses managed install, update, or mod configuration outside the simple editor.',
    )
    expect(wrapper.text()).toContain('Install Path')
    expect(wrapper.text()).toContain('Mod Source')
    expect(wrapper.text()).toContain('Platform')
  })
})
