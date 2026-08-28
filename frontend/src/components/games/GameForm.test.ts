import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, inject } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CommandProcessor,
  CommandType,
  EnvironmentVariableSchema,
  GameSchema,
  ModProfileSchema,
  ModSourceSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
  VariantSchema,
} from '@/proto/shared_pb'
import GameForm from './GameForm.vue'
import { gameFormContextKey } from './GameFormTypes'

const mocks = vi.hoisted(() => ({
  addGame: vi.fn(),
  editGame: vi.fn(),
  exportGame: vi.fn(),
  getGame: vi.fn(),
  getGameEnvironment: vi.fn(),
  listGameServers: vi.fn(),
  push: vi.fn(),
  updateGameStartArgBlocklist: vi.fn(),
  updateGameStartArgsTemplate: vi.fn(),
  updateGameEnvironment: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      addGame: mocks.addGame,
      editGame: mocks.editGame,
      exportGame: mocks.exportGame,
      getGame: mocks.getGame,
      getGameEnvironment: mocks.getGameEnvironment,
      listGameServers: mocks.listGameServers,
      updateGameEnvironment: mocks.updateGameEnvironment,
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

    return () => slots['default']?.()
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

function mountGameForm(
  stubOverrides: Record<string, unknown> = {},
  propOverrides: Record<string, unknown> = {},
) {
  return mount(GameForm, {
    props: {
      existingGameId: 'minecraft',
      ...propOverrides,
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
          template:
            '<button v-bind="$attrs" @click="$emit(\'click\')"><slot />{{ label }}</button>',
          props: ['label'],
          emits: ['click'],
        },
        'q-icon': true,
        'q-badge': { template: '<span><slot />{{ label }}</span>', props: ['label'] },
        'q-tooltip': true,
        'q-toggle': true,
        'q-spinner-dots': true,
        'router-link': { template: '<a><slot /></a>' },
        ConfigSchemaList: { template: '<div />' },
        StartArgsTemplateEditor: {
          props: ['mode'],
          template:
            '<div :data-testid="`start-args-template-editor-${mode || \'full\'}`"><slot /></div>',
        },
        BlocklistEditor: { template: '<div data-testid="blocklist-editor" />' },
        DownstreamImpactPanel: { template: '<div data-testid="downstream-impact-panel" />' },
        ...stubOverrides,
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

    window.history.replaceState({}, '', '/')

    mocks.addGame.mockReset()
    mocks.editGame.mockReset()
    mocks.exportGame.mockReset()
    mocks.getGame.mockReset()
    mocks.getGameEnvironment.mockReset()
    mocks.listGameServers.mockReset()
    mocks.push.mockReset()
    mocks.updateGameStartArgBlocklist.mockReset()
    mocks.updateGameStartArgsTemplate.mockReset()
    mocks.updateGameEnvironment.mockReset()
    mocks.getGameEnvironment.mockResolvedValue({
      defaultEnv: [],
      validationIssues: [],
    })
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

  it('summarizes per-variant mod support instead of claiming mods are off', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        updateProvider: create(UpdateProviderConfigSchema, {
          kind: UpdateProviderKind.MOJANG,
          sourceId: 'vanilla',
        }),
        variants: [
          create(VariantSchema, { id: 'vanilla', name: 'Vanilla' }),
          create(VariantSchema, {
            id: 'paper',
            name: 'Paper',
            modProfile: create(ModProfileSchema, {
              installPath: 'plugins/',
              sources: [
                create(ModSourceSchema, { id: 'modrinth', searchParamsJson: '{}' }),
                create(ModSourceSchema, { id: 'hangar', searchParamsJson: '{"platform":"PAPER"}' }),
              ],
            }),
          }),
        ],
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    expect(wrapper.text()).toContain('Variant Mod Support')
    expect(wrapper.text()).toContain('Paper')
    expect(wrapper.text()).toContain('plugins/')
    expect(wrapper.text()).toContain('No mod support')
    expect(wrapper.text()).not.toContain(
      'Mod support is off. Enable it to configure a download provider for this game.',
    )
  })

  it.each([
    { name: 'diverged official game shows banner', diverged: true, official: true, want: true },
    { name: 'clean official game hides banner', diverged: false, official: true, want: false },
    { name: 'diverged custom game hides banner', diverged: true, official: false, want: false },
  ])('$name', async ({ diverged, official, want }) => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        xylonaOfficial: official,
        officialDefinitionDiverged: diverged,
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    expect(wrapper.find('[data-testid="game-form-diverged-banner"]').exists()).toBe(want)
    if (want) {
      expect(wrapper.text()).toContain('Modified from official definition')
      expect(wrapper.text()).toContain('Restore official definition')
    }
  })

  it('persists structured start args through editGame without follow-up RPCs', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
        linuxStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"]}]',
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

    const saveButton = wrapper.findAll('button').find((button) => button.text().trim() === 'Save')

    expect(saveButton).toBeDefined()
    if (!saveButton) {
      throw new Error('expected Save button to exist')
    }

    await saveButton.trigger('click')
    await flushPromises()

    expect(mocks.editGame).toHaveBeenCalledTimes(1)
    expect(mocks.updateGameStartArgsTemplate).not.toHaveBeenCalled()
    expect(mocks.updateGameStartArgBlocklist).not.toHaveBeenCalled()
    expect(mocks.push).not.toHaveBeenCalled()

    const editGameCall = mocks.editGame.mock.calls[0]
    if (!editGameCall) {
      throw new Error('expected editGame to be called')
    }
    const request = editGameCall[0]
    expect(request.game.linuxStartArgsTemplate).toContain('"id":"jar"')
    expect(request.game.windowsStartArgsTemplate).toContain('"id":"jar"')
    expect(request.game.linuxBaseCommand).toBe('java')
    expect(request.game.windowsBaseCommand).toBe('java')
  })

  it('redirects newly created games into their edit page instead of the games list', async () => {
    mocks.addGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
      }),
    })

    const wrapper = mountGameForm({}, { existingGameId: '' })
    await flushPromises()

    const saveButton = wrapper.findAll('button').find((button) => button.text().trim() === 'Save')

    expect(saveButton).toBeDefined()
    if (!saveButton) {
      throw new Error('expected Save button to exist')
    }

    await saveButton.trigger('click')
    await flushPromises()

    expect(mocks.addGame).toHaveBeenCalledTimes(1)
    expect(mocks.push).toHaveBeenCalledWith({ path: '/games/minecraft/edit' })
  })

  it('organizes the editor into tabs and switches sections on demand', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
        linuxStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"]}]',
        windowsStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","tokens":["-jar","server.jar"]}]',
        linuxBaseCommand: 'java',
        windowsBaseCommand: 'java',
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    const overviewPanel = wrapper.get('[data-testid="game-form-tab-panel-overview"]')
    const runtimePanel = wrapper.get('[data-testid="game-form-tab-panel-runtime"]')
    const consoleCommandsPanel = wrapper.get('[data-testid="game-form-tab-panel-console-commands"]')

    expect(overviewPanel.attributes('style') ?? '').not.toContain('display: none')
    expect(runtimePanel.attributes('style') ?? '').toContain('display: none')
    expect(consoleCommandsPanel.attributes('style') ?? '').toContain('display: none')

    await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')

    expect(overviewPanel.attributes('style') ?? '').toContain('display: none')
    expect(runtimePanel.attributes('style') ?? '').not.toContain('display: none')

    await wrapper.get('[data-testid="game-form-tab-console-commands"]').trigger('click')

    expect(runtimePanel.attributes('style') ?? '').toContain('display: none')
    expect(consoleCommandsPanel.attributes('style') ?? '').not.toContain('display: none')
  })

  it('restores the last active tab from history state on reload', async () => {
    window.history.replaceState({ xylonaGameFormTab: 'runtime' }, '', '/')

    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    const overviewPanel = wrapper.get('[data-testid="game-form-tab-panel-overview"]')
    const runtimePanel = wrapper.get('[data-testid="game-form-tab-panel-runtime"]')

    expect(overviewPanel.attributes('style') ?? '').toContain('display: none')
    expect(runtimePanel.attributes('style') ?? '').not.toContain('display: none')
    expect(wrapper.get('[data-testid="game-form-tab-runtime"]').attributes('aria-selected')).toBe(
      'true',
    )
  })

  it('stores tab selections in history state', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')
    expect(window.history.state?.xylonaGameFormTab).toBe('runtime')

    await wrapper.get('[data-testid="game-form-tab-config"]').trigger('click')
    expect(window.history.state?.xylonaGameFormTab).toBe('config')
  })

  it('supports arrow-key navigation between editor tabs', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    const overviewTab = wrapper.get('[data-testid="game-form-tab-overview"]')
    const runtimeTab = wrapper.get('[data-testid="game-form-tab-runtime"]')
    const overviewPanel = wrapper.get('[data-testid="game-form-tab-panel-overview"]')
    const runtimePanel = wrapper.get('[data-testid="game-form-tab-panel-runtime"]')

    await overviewTab.trigger('keydown', { key: 'ArrowRight' })

    expect(overviewTab.attributes('aria-selected')).toBe('false')
    expect(runtimeTab.attributes('aria-selected')).toBe('true')
    expect(window.history.state?.xylonaGameFormTab).toBe('runtime')
    expect(overviewPanel.attributes('style') ?? '').toContain('display: none')
    expect(runtimePanel.attributes('style') ?? '').not.toContain('display: none')
  })

  it('supports Home and End keyboard navigation between editor tabs', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    const overviewTab = wrapper.get('[data-testid="game-form-tab-overview"]')
    const runtimeTab = wrapper.get('[data-testid="game-form-tab-runtime"]')
    const configTab = wrapper.get('[data-testid="game-form-tab-config"]')
    const runtimeTabElement = runtimeTab.element as HTMLButtonElement
    const configTabElement = configTab.element as HTMLButtonElement

    runtimeTabElement.focus()
    await runtimeTab.trigger('keydown', { key: 'End' })
    await flushPromises()

    expect(configTab.attributes('aria-selected')).toBe('true')
    expect(window.history.state?.xylonaGameFormTab).toBe('config')

    configTabElement.focus()
    await configTab.trigger('keydown', { key: 'Home' })
    await flushPromises()

    expect(overviewTab.attributes('aria-selected')).toBe('true')
    expect(window.history.state?.xylonaGameFormTab).toBe('overview')
  })

  it('treats runtime policy as an advanced section beneath the launch editor', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    const wrapper = mountGameForm()
    await flushPromises()

    await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')

    expect(wrapper.get('[data-testid="start-args-template-editor-preview"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="start-args-template-editor-advanced"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="runtime-policy-toggle"]').attributes('aria-expanded')).toBe(
      'false',
    )
    expect(wrapper.find('[data-testid="runtime-policy-panel"]').exists()).toBe(false)

    const runtimeHTML = wrapper.get('[data-testid="game-form-tab-panel-runtime"]').html()
    expect(runtimeHTML.indexOf('start-args-template-editor-preview')).toBeLessThan(
      runtimeHTML.indexOf('runtime-policy-toggle'),
    )
    expect(runtimeHTML.indexOf('runtime-policy-toggle')).toBeLessThan(
      runtimeHTML.indexOf('start-args-template-editor-advanced'),
    )

    await wrapper.get('[data-testid="runtime-policy-toggle"]').trigger('click')

    expect(wrapper.get('[data-testid="runtime-policy-toggle"]').attributes('aria-expanded')).toBe(
      'true',
    )
    expect(wrapper.get('[data-testid="runtime-policy-panel"]')).toBeTruthy()
    expect(wrapper.get('[data-testid="blocklist-editor"]')).toBeTruthy()
  })

  it('keeps runtime guardrails and the advanced runtime sequence mutually exclusive on compact viewports', async () => {
    const originalMatchMedia = window.matchMedia
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: query === '(max-width: 900px)',
        media: query,
        onchange: null,
        addEventListener: vi.fn(),
        addListener: vi.fn(),
        dispatchEvent: vi.fn(),
        removeEventListener: vi.fn(),
        removeListener: vi.fn(),
      })),
    })

    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    try {
      const wrapper = mountGameForm({
        StartArgsTemplateEditor: defineComponent({
          name: 'CompactViewportStartArgsTemplateEditorStub',
          props: {
            advancedExpanded: {
              type: Boolean,
              default: false,
            },
            mode: {
              type: String,
              default: 'full',
            },
          },
          emits: ['update:advanced-expanded'],
          template:
            '<div :data-testid="`start-args-template-editor-${mode || \'full\'}`" :data-advanced-expanded="String(advancedExpanded)">' +
            '<button :data-testid="`start-args-template-editor-${mode || \'full\'}-expand`" type="button" @click="$emit(\'update:advanced-expanded\', true)">Expand advanced sequence</button>' +
            '<button :data-testid="`start-args-template-editor-${mode || \'full\'}-collapse`" type="button" @click="$emit(\'update:advanced-expanded\', false)">Collapse advanced sequence</button>' +
            '</div>',
        }),
      })
      await flushPromises()

      await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')

      const advancedEditor = wrapper.get('[data-testid="start-args-template-editor-advanced"]')
      const runtimePolicyToggle = wrapper.get('[data-testid="runtime-policy-toggle"]')

      expect(advancedEditor.attributes('data-advanced-expanded')).toBe('false')
      expect(runtimePolicyToggle.attributes('aria-expanded')).toBe('false')

      await wrapper
        .get('[data-testid="start-args-template-editor-advanced-expand"]')
        .trigger('click')
      await flushPromises()

      expect(advancedEditor.attributes('data-advanced-expanded')).toBe('true')
      expect(runtimePolicyToggle.attributes('aria-expanded')).toBe('false')

      await runtimePolicyToggle.trigger('click')
      await flushPromises()

      expect(runtimePolicyToggle.attributes('aria-expanded')).toBe('true')
      expect(advancedEditor.attributes('data-advanced-expanded')).toBe('false')

      await wrapper
        .get('[data-testid="start-args-template-editor-advanced-expand"]')
        .trigger('click')
      await flushPromises()

      expect(advancedEditor.attributes('data-advanced-expanded')).toBe('true')
      expect(runtimePolicyToggle.attributes('aria-expanded')).toBe('false')
      expect(wrapper.find('[data-testid="runtime-policy-panel"]').exists()).toBe(false)
    } finally {
      Object.defineProperty(window, 'matchMedia', {
        configurable: true,
        writable: true,
        value: originalMatchMedia,
      })
    }
  })

  it('passes the loaded runtime baseline into the launch editor reset props', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
        linuxStartArgsTemplate:
          '[{"id":"jar","order":0,"ownership":"system","label":"Jar","tokens":["-jar","server.jar"]}]',
        windowsStartArgsTemplate:
          '[{"id":"nogui","order":0,"ownership":"editable","label":"No GUI","tokens":["nogui"]}]',
        linuxBaseCommand: 'java',
        windowsBaseCommand: 'javaw',
      }),
    })

    const wrapper = mountGameForm({
      StartArgsTemplateEditor: defineComponent({
        name: 'ResetAwareStartArgsTemplateEditorStub',
        props: {
          baselineLinuxBaseCommand: {
            type: String,
            default: '',
          },
          baselineLinuxTemplate: {
            type: Array,
            default: () => [],
          },
          baselineWindowsBaseCommand: {
            type: String,
            default: '',
          },
          baselineWindowsTemplate: {
            type: Array,
            default: () => [],
          },
          mode: {
            type: String,
            default: 'full',
          },
        },
        template:
          '<div :data-testid="`start-args-template-editor-${mode || \'full\'}`">' +
          '<span data-testid="baseline-linux-command">{{ baselineLinuxBaseCommand }}</span>' +
          '<span data-testid="baseline-windows-command">{{ baselineWindowsBaseCommand }}</span>' +
          '<span data-testid="baseline-linux-count">{{ baselineLinuxTemplate.length }}</span>' +
          '<span data-testid="baseline-windows-count">{{ baselineWindowsTemplate.length }}</span>' +
          '</div>',
      }),
    })
    await flushPromises()

    await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')

    expect(wrapper.get('[data-testid="baseline-linux-command"]').text()).toBe('java')
    expect(wrapper.get('[data-testid="baseline-windows-command"]').text()).toBe('javaw')
    expect(wrapper.get('[data-testid="baseline-linux-count"]').text()).toBe('1')
    expect(wrapper.get('[data-testid="baseline-windows-count"]').text()).toBe('1')
  })

  it('hides default environment editor for new and copy forms', async () => {
    const newWrapper = mountGameForm({}, { existingGameId: '' })
    await flushPromises()

    expect(newWrapper.find('[data-testid="game-default-environment-section"]').exists()).toBe(false)

    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    const copyWrapper = mountGameForm({}, { existingGameId: '', copyGameId: 'minecraft' })
    await flushPromises()

    expect(copyWrapper.find('[data-testid="game-default-environment-section"]').exists()).toBe(
      false,
    )
  })

  it('saves game default environment through the dedicated RPC', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })
    mocks.getGameEnvironment.mockResolvedValue({
      defaultEnv: [
        create(EnvironmentVariableSchema, {
          name: 'HYTALE_AUTH_MODE',
          value: 'refresh_token',
        }),
      ],
      validationIssues: [],
    })
    mocks.updateGameEnvironment.mockResolvedValue({
      defaultEnv: [
        create(EnvironmentVariableSchema, {
          name: 'HYTALE_AUTH_MODE',
          value: 'refresh_token',
        }),
      ],
      validationIssues: [],
    })

    const wrapper = mountGameForm()
    await flushPromises()

    await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')
    const saveButton = wrapper
      .findAll('button')
      .find((button) => button.text().trim() === 'Save Default Environment')

    expect(saveButton).toBeDefined()
    if (!saveButton) {
      throw new Error('expected Save Default Environment button to exist')
    }

    await saveButton.trigger('click')
    await flushPromises()

    expect(mocks.getGameEnvironment).toHaveBeenCalledTimes(1)
    expect(mocks.updateGameEnvironment).toHaveBeenCalledTimes(1)
    expect(mocks.updateGameEnvironment.mock.calls[0]?.[0]).toMatchObject({
      gameId: 'minecraft',
    })
    expect(mocks.updateGameEnvironment.mock.calls[0]?.[0]?.defaultEnv[0]).toMatchObject({
      name: 'HYTALE_AUTH_MODE',
      value: 'refresh_token',
    })
  })

  it('keeps game edits dirty when default environment finishes loading later', async () => {
    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        linuxSupport: true,
        windowsSupport: true,
      }),
    })

    let resolveEnvironment:
      ((value: { defaultEnv: never[]; validationIssues: never[] }) => void) | undefined
    mocks.getGameEnvironment.mockReturnValue(
      new Promise((resolve) => {
        resolveEnvironment = resolve
      }),
    )

    const wrapper = mountGameForm({
      GameFormOverviewTab: defineComponent({
        name: 'DirtyDuringEnvLoadOverviewStub',
        setup() {
          const ctx = inject(gameFormContextKey)
          if (!ctx) {
            throw new Error('expected game form context')
          }

          function mutateName(): void {
            ctx.game.value.name = 'Changed During Env Load'
          }

          return {
            mutateName,
          }
        },
        template:
          '<button data-testid="mutate-game-name" type="button" @click="mutateName">Mutate</button>',
      }),
    })
    await flushPromises()

    await wrapper.get('[data-testid="mutate-game-name"]').trigger('click')

    expect(wrapper.vm.isDirty).toBe(true)

    if (!resolveEnvironment) {
      throw new Error('expected delayed default environment request')
    }
    resolveEnvironment({
      defaultEnv: [],
      validationIssues: [],
    })
    await flushPromises()

    expect(wrapper.vm.isDirty).toBe(true)
  })
})
