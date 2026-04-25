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
  exportGame: vi.fn(),
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
      exportGame: mocks.exportGame,
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

  it('organizes the editor into tabs and switches to runtime details on demand', async () => {
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

    expect(overviewPanel.attributes('style') ?? '').not.toContain('display: none')
    expect(runtimePanel.attributes('style') ?? '').toContain('display: none')

    await wrapper.get('[data-testid="game-form-tab-runtime"]').trigger('click')

    expect(overviewPanel.attributes('style') ?? '').toContain('display: none')
    expect(runtimePanel.attributes('style') ?? '').not.toContain('display: none')
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

  it('uses a more instructive empty state for mod support setup', async () => {
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

    await wrapper.get('[data-testid="game-form-tab-mods"]').trigger('click')

    expect(wrapper.text()).toContain(
      'Mod support is off. Enable it to configure a download provider for this game.',
    )
    expect(wrapper.text()).toContain('Enable Mod Support')
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

  it('gives the advanced runtime policy toggle a concise accessible label', async () => {
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

    const toggle = wrapper.get('[data-testid="runtime-policy-toggle"]')
    expect(toggle.attributes('aria-label')).toBe('Expand runtime guardrails')
    expect(toggle.attributes('aria-describedby')).toBe('runtime-policy-assistive-summary')

    await toggle.trigger('click')

    expect(toggle.attributes('aria-label')).toBe('Collapse runtime guardrails')
  })

  it('removes the extra runtime header chrome and focuses on the launch editor', async () => {
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

    expect(wrapper.find('.game-form-tab-note').exists()).toBe(false)

    const runtimePanel = wrapper.get('[data-testid="game-form-tab-panel-runtime"]')
    expect(runtimePanel.find('.section-header').exists()).toBe(false)
    expect(runtimePanel.find('.section-help').exists()).toBe(false)
    expect(runtimePanel.find('.game-form-sr-only').text()).toContain('Runtime')
    expect(runtimePanel.get('[data-testid="start-args-template-editor-preview"]')).toBeTruthy()
    expect(runtimePanel.get('[data-testid="start-args-template-editor-advanced"]')).toBeTruthy()
  })

  it('connects tabs and panels with accessible relationships and headings', async () => {
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

    expect(wrapper.get('h1').text()).toContain('Editing Minecraft')

    const panels = [
      ['overview', 'Identity'],
      ['runtime', 'Runtime'],
      ['mods', 'Mods'],
      ['config', 'Configuration Files'],
    ] as const

    for (const [tabId, sectionHeading] of panels) {
      const tab = wrapper.get(`[data-testid="game-form-tab-${tabId}"]`)
      const panel = wrapper.get(`[data-testid="game-form-tab-panel-${tabId}"]`)

      expect(tab.attributes('id')).toBe(`game-form-tab-${tabId}`)
      expect(tab.attributes('aria-controls')).toBe(`game-form-tab-panel-${tabId}`)
      expect(panel.attributes('id')).toBe(`game-form-tab-panel-${tabId}`)
      expect(panel.attributes('role')).toBe('tabpanel')
      expect(panel.attributes('aria-labelledby')).toBe(`game-form-tab-${tabId}`)
      expect(wrapper.findAll('h2').some((heading) => heading.text().includes(sectionHeading))).toBe(
        true,
      )
    }
  })

  it('keeps install and update fields on the overview tab', async () => {
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

    expect(wrapper.find('[data-testid="game-form-tab-install"]').exists()).toBe(false)

    const overviewPanel = wrapper.get('[data-testid="game-form-tab-panel-overview"]')
    expect(overviewPanel.text()).toContain('Install & Update')
  })
})
