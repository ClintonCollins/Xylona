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
  addGame: vi.fn(),
  editGame: vi.fn(),
  getGame: vi.fn(),
  listGameServers: vi.fn(),
  push: vi.fn(),
  updateGameStartArgBlocklist: vi.fn(),
  updateGameStartArgsTemplate: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      addGame: mocks.addGame,
      editGame: mocks.editGame,
      getGame: mocks.getGame,
      listGameServers: mocks.listGameServers,
      updateGameStartArgsTemplate: mocks.updateGameStartArgsTemplate,
      updateGameStartArgBlocklist: mocks.updateGameStartArgBlocklist,
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
      push: mocks.push,
    }),
  }
})

const QFormStub = defineComponent({
  name: 'QFormStub',
  setup(_, { slots, expose }) {
    expose({
      validate: async () => true,
    })

    return () => slots.default?.()
  },
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
        'q-form': QFormStub,
        'q-input': QInputStub,
        'q-select': {
          template: '<div class="q-select-stub" v-bind="$attrs">{{ label }}<slot /></div>',
          props: ['label'],
        },
        'q-btn': {
          template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot />{{ label }}</button>',
          props: ['label'],
          emits: ['click'],
        },
        'q-icon': true,
        'q-badge': { template: '<span><slot />{{ label }}</span>', props: ['label'] },
        'q-toggle': true,
        'q-spinner-dots': true,
        'router-link': { template: '<a><slot /></a>' },
        ConfigSchemaList: { template: '<div />' },
        StartArgsTemplateEditor: { template: '<div data-testid="start-args-template-editor" />' },
        BlocklistEditor: { template: '<div data-testid="blocklist-editor" />' },
        DownstreamImpactPanel: { template: '<div data-testid="downstream-impact-panel" />' },
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

    mocks.addGame.mockReset()
    mocks.editGame.mockReset()
    mocks.getGame.mockReset()
    mocks.listGameServers.mockReset()
    mocks.push.mockReset()
    mocks.updateGameStartArgBlocklist.mockReset()
    mocks.updateGameStartArgsTemplate.mockReset()
    mocks.listGameServers.mockResolvedValue({ gameServers: [] })
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

  it('persists structured start args through editGame without follow-up RPCs', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
        linuxStartArgsTemplate: '[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"]}]',
        windowsStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"]}]',
        linuxBaseCommand: 'java',
        windowsBaseCommand: 'java',
      }),
    })
    mocks.editGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    const saveButton = wrapper
      .findAll('button')
      .find((button) => button.text().trim() === 'Save')

    expect(saveButton).toBeDefined()
    if (!saveButton) {
      throw new Error('expected Save button to exist')
    }

    await saveButton.trigger('click')
    await flushPromises()

    expect(mocks.editGame).toHaveBeenCalledTimes(1)
    expect(mocks.updateGameStartArgsTemplate).not.toHaveBeenCalled()
    expect(mocks.updateGameStartArgBlocklist).not.toHaveBeenCalled()

    const request = mocks.editGame.mock.calls[0][0]
    expect(request.game.linuxStartArgsTemplate).toContain('"id":"jar"')
    expect(request.game.windowsStartArgsTemplate).toContain('"id":"jar"')
    expect(request.game.linuxBaseCommand).toBe('java')
    expect(request.game.windowsBaseCommand).toBe('java')
  })
})
