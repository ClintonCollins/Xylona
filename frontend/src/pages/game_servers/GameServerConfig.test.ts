import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieSandboxComparisonState,
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'

import { useUserAuthStore } from '@/stores/xylona'

import GameServerConfig from './GameServerConfig.vue'

const mocks = vi.hoisted(() => ({
  routeState: {
    current: undefined as
      | {
          params?: {
            id?: string | string[]
          }
        }
      | undefined,
  },
  notify: vi.fn(),
  dialog: vi.fn(),
  getGameServerConfigFiles: vi.fn(),
  getGameServerConfigFile: vi.fn(),
  updateGameServerConfigFile: vi.fn(),
  generateGameServerConfigFile: vi.fn(),
  getSevenDaysToDieSandboxSettings: vi.fn(),
  getGameServer: vi.fn(),
  confirmSaved: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
      dialog: mocks.dialog,
    }),
  }
})

// Every toast goes through the shared helpers; record them on one mock.
vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notify,
  notifySuccess: mocks.notify,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => mocks.routeState.current,
    onBeforeRouteLeave: vi.fn(),
    onBeforeRouteUpdate: vi.fn(),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerConfigFiles: mocks.getGameServerConfigFiles,
    getGameServerConfigFile: mocks.getGameServerConfigFile,
    updateGameServerConfigFile: mocks.updateGameServerConfigFile,
    generateGameServerConfigFile: mocks.generateGameServerConfigFile,
    getSevenDaysToDieSandboxSettings: mocks.getSevenDaysToDieSandboxSettings,
    getGameServer: mocks.getGameServer,
  }),
  ConnectErrorToString: (err: unknown) => String(err),
}))

function mountConfig() {
  return mount(GameServerConfig, {
    global: {
      stubs: {
        'q-icon': { template: '<i />' },
        QSelect: true,
        'q-spinner-dots': { template: '<div class="spinner-stub" />' },
        'q-banner': { template: '<div><slot /><slot name="action" /></div>' },
        'q-btn': {
          props: ['label'],
          emits: ['click'],
          template: `<button @click="$emit('click')">{{ label }}</button>`,
        },
        ConfigFileSidebar: {
          name: 'ConfigFileSidebar',
          props: ['selectedPath', 'disabled'],
          emits: ['select'],
          template: '<div class="sidebar-stub" />',
        },
        ConfigFileEditor: {
          name: 'ConfigFileEditor',
          props: ['fields', 'advancedFields', 'filePath', 'saving', 'validationErrors'],
          emits: ['save', 'discard', 'generate'],
          data: () => ({ hasChanges: false }),
          methods: { confirmSaved: mocks.confirmSaved },
          template: '<div class="editor-stub" />',
        },
      },
    },
  })
}

const oneConfigFile = [
  { path: 'server.properties', category: 'Server', format: 'properties', existsOnDisk: true },
]

async function mountWithFile() {
  mocks.routeState.current = { params: { id: 'server-12' } }
  mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: oneConfigFile })
  mocks.getGameServerConfigFile.mockResolvedValue({
    fields: [{ key: 'motd', value: 'Hello' }],
    advancedFields: [],
  })
  mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'minecraft' } })
  const wrapper = mountConfig()
  await flushPromises()
  return wrapper
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

describe('GameServerConfig', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.routeState.current = undefined
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: '7_days_to_die' } })
  })

  afterEach(() => {
    mocks.notify.mockReset()
    mocks.dialog.mockReset()
    mocks.getGameServerConfigFiles.mockReset()
    mocks.getGameServerConfigFile.mockReset()
    mocks.updateGameServerConfigFile.mockReset()
    mocks.generateGameServerConfigFile.mockReset()
    mocks.getSevenDaysToDieSandboxSettings.mockReset()
    mocks.getGameServer.mockReset()
    mocks.confirmSaved.mockReset()
  })

  it('renders the Configuration page title', async () => {
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.find('h1.xy-page-title').text()).toBe('Configuration')
  })

  it('does not crash or fetch config files when the route context is unavailable', async () => {
    const mountComponent = () => mountConfig()

    expect(mountComponent).not.toThrow()

    const wrapper = mountComponent()
    await flushPromises()

    expect(mocks.getGameServerConfigFiles).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('No config files for this game yet')
  })

  it.each([
    {
      name: 'shows the inspector for SandboxCode metadata',
      fields: [{ key: 'SandboxCode', value: 'ABC' }],
      gameId: '7_days_to_die',
      visible: true,
    },
    {
      name: 'leaves other config workflows unchanged',
      fields: [{ key: 'ServerName', value: 'Example' }],
      gameId: '7_days_to_die',
      visible: false,
    },
    {
      name: 'does not show for another game with custom SandboxCode metadata',
      fields: [{ key: 'SandboxCode', value: 'CUSTOM' }],
      gameId: 'custom_game',
      visible: false,
    },
  ])('$name', async ({ fields, gameId, visible }) => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({
      configFiles: [
        { path: 'serverconfig.xml', category: 'General', format: 'xml', existsOnDisk: true },
      ],
    })
    mocks.getGameServerConfigFile.mockResolvedValue({ fields, advancedFields: [] })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId } })

    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.findComponent({ name: 'SevenDaysToDieSandboxInspector' }).exists()).toBe(visible)
    expect(wrapper.find('.editor-stub').exists()).toBe(true)
  })

  it('marks sandbox observations stale before post-save reloads finish', async () => {
    const configFiles = [
      { path: 'serverconfig.xml', category: 'General', format: 'xml', existsOnDisk: true },
    ]
    const reloadedFields = [{ key: 'SandboxCode', value: 'NEW-CODE' }]
    const fileReload = createDeferred<{ fields: typeof reloadedFields; advancedFields: [] }>()
    const fileListReload = createDeferred<{ configFiles: typeof configFiles }>()

    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles })
    mocks.getGameServerConfigFile.mockResolvedValue({
      fields: [{ key: 'SandboxCode', value: 'OLD-CODE' }],
      advancedFields: [],
    })
    mocks.getSevenDaysToDieSandboxSettings.mockResolvedValue({
      connectionState:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
      configuredCode: 'OLD-CODE',
      effectiveCode: 'OLD-CODE',
      settings: [],
    })
    mocks.updateGameServerConfigFile.mockResolvedValue({ success: true, errors: [] })

    const wrapper = mountConfig()
    await flushPromises()

    const inspector = wrapper.findComponent({ name: 'SevenDaysToDieSandboxInspector' })
    await inspector.find('.sandbox-summary').trigger('click')
    await flushPromises()
    expect(inspector.text()).toContain('Match')

    mocks.getGameServerConfigFile.mockReturnValueOnce(fileReload.promise)
    mocks.getGameServerConfigFiles.mockReturnValueOnce(fileListReload.promise)

    wrapper
      .findComponent({ name: 'ConfigFileEditor' })
      .vm.$emit('save', new Map([['SandboxCode', 'NEW-CODE']]))
    await flushPromises()

    expect(inspector.props('refreshKey')).toBe(1)
    expect(inspector.text()).toContain('Stale')
    expect(mocks.getGameServerConfigFile).toHaveBeenCalledTimes(2)
    expect(mocks.getGameServerConfigFiles).toHaveBeenCalledTimes(1)

    fileReload.resolve({ fields: reloadedFields, advancedFields: [] })
    await flushPromises()

    expect(mocks.getGameServerConfigFiles).toHaveBeenCalledTimes(2)
    expect(inspector.text()).toContain('Stale')

    fileListReload.resolve({ configFiles })
    await flushPromises()

    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
      reloadedFields,
    )
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('saving')).toBe(false)
  })

  it.each([
    {
      name: 'a rejected save keeps the edits and shows the errors',
      update: () =>
        mocks.updateGameServerConfigFile.mockResolvedValue({
          success: false,
          errors: [{ field: 'motd', message: 'Too long' }],
        }),
      errors: [{ field: 'motd', message: 'Too long' }],
    },
    {
      name: 'a failed request keeps the edits',
      update: () => mocks.updateGameServerConfigFile.mockRejectedValue(new Error('offline')),
      errors: [],
    },
  ])('$name', async ({ update, errors }) => {
    const wrapper = await mountWithFile()
    update()

    wrapper.findComponent({ name: 'ConfigFileEditor' }).vm.$emit('save', new Map([['motd', 'x']]))
    await flushPromises()

    expect(mocks.confirmSaved).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('validationErrors')).toEqual(
      errors,
    )
  })

  it('confirms the save to the editor once the file has reloaded', async () => {
    const wrapper = await mountWithFile()
    mocks.updateGameServerConfigFile.mockResolvedValue({ success: true, errors: [] })

    wrapper.findComponent({ name: 'ConfigFileEditor' }).vm.$emit('save', new Map([['motd', 'x']]))
    await flushPromises()

    expect(mocks.confirmSaved).toHaveBeenCalledTimes(1)
    expect(mocks.getGameServerConfigFile).toHaveBeenCalledTimes(2)
  })

  async function mountWithMultipleFiles() {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({
      configFiles: ['server.properties', 'other.properties', 'third.properties'].map((path) => ({
        path,
        category: 'Server',
        format: 'properties',
        existsOnDisk: true,
      })),
    })
    mocks.getGameServerConfigFile.mockResolvedValue({
      fields: [{ key: 'motd', value: 'Hello' }],
      advancedFields: [],
    })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'minecraft' } })
    const wrapper = mountConfig()
    await flushPromises()
    mocks.getGameServerConfigFile.mockClear()
    return wrapper
  }

  type ConfigWrapper = Awaited<ReturnType<typeof mountWithMultipleFiles>>

  async function selectFile(wrapper: ConfigWrapper, path: string) {
    wrapper.findComponent({ name: 'ConfigFileSidebar' }).vm.$emit('select', path, false)
    await flushPromises()
  }

  const otherFile = { fields: [{ key: 'other-setting', value: 'Other' }], advancedFields: [] }
  const thirdFile = { fields: [{ key: 'third-setting', value: 'Third' }], advancedFields: [] }

  it('hides the previous file fields until the selected file is loaded', async () => {
    const wrapper = await mountWithMultipleFiles()
    const pending = createDeferred<typeof otherFile>()
    mocks.getGameServerConfigFile.mockReturnValueOnce(pending.promise)
    await selectFile(wrapper, 'other.properties')
    expect(wrapper.find('.editor-stub').exists()).toBe(false)
    expect(wrapper.find('[data-test="config-file-loading"]').text()).toContain('other.properties')
    expect(wrapper.find('[data-test="config-file-loading"]').attributes('role')).toBe('status')
    pending.resolve(otherFile)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
      otherFile.fields,
    )
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('filePath')).toBe(
      'other.properties',
    )
    expect(wrapper.find('[data-test="config-file-loading"]').exists()).toBe(false)
  })

  it.each(['success', 'failure'] as const)(
    'ignores an older file response after a newer selection succeeds (%s)',
    async (outcome) => {
      const wrapper = await mountWithMultipleFiles()
      const pending = createDeferred<typeof otherFile>()
      mocks.getGameServerConfigFile
        .mockReturnValueOnce(pending.promise)
        .mockResolvedValueOnce(thirdFile)
      await selectFile(wrapper, 'other.properties')
      await selectFile(wrapper, 'third.properties')
      if (outcome === 'success') pending.resolve(otherFile)
      else pending.reject(new Error('old request failed'))
      await flushPromises()
      expect(wrapper.find('[data-test="config-file-load-error"]').exists()).toBe(false)
      expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
        thirdFile.fields,
      )
      expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('filePath')).toBe(
        'third.properties',
      )
      mocks.updateGameServerConfigFile.mockResolvedValueOnce({ success: true, errors: [] })
      wrapper
        .findComponent({ name: 'ConfigFileEditor' })
        .vm.$emit('save', new Map([['third-setting', 'Edited']]))
      await flushPromises()
      expect(mocks.updateGameServerConfigFile).toHaveBeenLastCalledWith(
        expect.objectContaining({
          filePath: 'third.properties',
          fields: [expect.objectContaining({ key: 'third-setting', value: 'Edited' })],
        }),
      )
    },
  )

  it('keeps the latest selection loading when an older response finishes first', async () => {
    const wrapper = await mountWithMultipleFiles()
    const older = createDeferred<typeof otherFile>()
    const newer = createDeferred<typeof thirdFile>()
    mocks.getGameServerConfigFile
      .mockReturnValueOnce(older.promise)
      .mockReturnValueOnce(newer.promise)
    await selectFile(wrapper, 'other.properties')
    await selectFile(wrapper, 'third.properties')
    older.resolve(otherFile)
    await flushPromises()
    expect(wrapper.find('.editor-stub').exists()).toBe(false)
    expect(wrapper.find('[data-test="config-file-loading"]').text()).toContain('third.properties')
    newer.resolve(thirdFile)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
      thirdFile.fields,
    )
  })

  it('does not reload the active file when it is selected again', async () => {
    const wrapper = await mountWithMultipleFiles()
    await selectFile(wrapper, 'server.properties')
    expect(mocks.getGameServerConfigFile).not.toHaveBeenCalled()
  })

  it('retries the current failed file without accepting an older result', async () => {
    const wrapper = await mountWithMultipleFiles()
    const older = createDeferred<typeof otherFile>()
    mocks.getGameServerConfigFile
      .mockReturnValueOnce(older.promise)
      .mockRejectedValueOnce(new Error('offline'))
    await selectFile(wrapper, 'other.properties')
    await selectFile(wrapper, 'third.properties')
    expect(wrapper.find('[data-test="config-file-load-error"]').text()).toContain(
      'third.properties',
    )
    older.resolve(otherFile)
    await flushPromises()
    expect(wrapper.find('[data-test="config-file-load-error"]').exists()).toBe(true)
    mocks.getGameServerConfigFile.mockResolvedValueOnce(thirdFile)
    await wrapper.find('[data-test="config-file-load-error"] button').trigger('click')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
      thirdFile.fields,
    )
  })

  it.each(['save', 'generate'] as const)(
    'holds the selected file during %s and its refresh',
    async (operation) => {
      const wrapper = await mountWithMultipleFiles()
      const pending = createDeferred<{ success: boolean; errors: [] }>()
      const reload = createDeferred<typeof otherFile>()
      const mutation =
        operation === 'save' ? mocks.updateGameServerConfigFile : mocks.generateGameServerConfigFile
      mutation.mockReturnValueOnce(pending.promise)
      mocks.getGameServerConfigFile.mockReturnValueOnce(reload.promise)
      const editor = wrapper.findComponent({ name: 'ConfigFileEditor' })
      editor.vm.$emit(operation, new Map([['motd', 'Updated']]))
      await flushPromises()
      expect(wrapper.findComponent({ name: 'ConfigFileSidebar' }).props('disabled')).toBe(true)
      await selectFile(wrapper, 'other.properties')
      expect(mocks.getGameServerConfigFile).not.toHaveBeenCalled()
      editor.vm.$emit(operation, new Map([['motd', 'Again']]))
      await flushPromises()
      expect(mutation).toHaveBeenCalledTimes(1)
      pending.resolve({ success: true, errors: [] })
      await flushPromises()
      await selectFile(wrapper, 'third.properties')
      expect(mocks.getGameServerConfigFile).toHaveBeenCalledTimes(1)
      expect(mocks.getGameServerConfigFile).toHaveBeenLastCalledWith(
        expect.objectContaining({ filePath: 'server.properties' }),
      )
      reload.resolve(otherFile)
      await flushPromises()
      expect(wrapper.findComponent({ name: 'ConfigFileSidebar' }).props('disabled')).toBe(false)
    },
  )

  it('keeps the current draft when switching files is cancelled', async () => {
    const wrapper = await mountWithMultipleFiles()
    const editor = wrapper.findComponent({ name: 'ConfigFileEditor' })
    await editor.setData({ hasChanges: true })
    const callbacks: { ok?: () => void; cancel?: () => void } = {}
    const dialog = {
      onOk(handler: () => void) {
        callbacks.ok = handler
        return dialog
      },
      onCancel(handler: () => void) {
        callbacks.cancel = handler
        return dialog
      },
      onDismiss() {
        return dialog
      },
    }
    mocks.dialog.mockReturnValue(dialog)
    await selectFile(wrapper, 'other.properties')
    expect(mocks.getGameServerConfigFile).not.toHaveBeenCalled()
    callbacks.cancel?.()
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).vm).toBe(editor.vm)
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('filePath')).toBe(
      'server.properties',
    )
    expect(mocks.getGameServerConfigFile).not.toHaveBeenCalled()
  })

  it('switches to the latest file when an older discard confirmation finishes late', async () => {
    const wrapper = await mountWithMultipleFiles()
    await wrapper.findComponent({ name: 'ConfigFileEditor' }).setData({ hasChanges: true })
    const confirmations: (() => void)[] = []
    mocks.dialog.mockImplementation(() => {
      const dialog = {
        onOk(handler: () => void) {
          confirmations.push(handler)
          return dialog
        },
        onCancel() {
          return dialog
        },
        onDismiss() {
          return dialog
        },
      }
      return dialog
    })
    await selectFile(wrapper, 'other.properties')
    await selectFile(wrapper, 'third.properties')
    mocks.getGameServerConfigFile.mockResolvedValueOnce(thirdFile)
    confirmations[1]?.()
    await flushPromises()
    confirmations[0]?.()
    await flushPromises()
    expect(mocks.getGameServerConfigFile).toHaveBeenCalledTimes(1)
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('filePath')).toBe(
      'third.properties',
    )
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
      thirdFile.fields,
    )
  })

  it('keeps the selected file if a save starts while discard confirmation is open', async () => {
    const wrapper = await mountWithMultipleFiles()
    const editor = wrapper.findComponent({ name: 'ConfigFileEditor' })
    await editor.setData({ hasChanges: true })
    let confirm: (() => void) | undefined
    const dialog = {
      onOk(handler: () => void) {
        confirm = handler
        return dialog
      },
      onCancel() {
        return dialog
      },
      onDismiss() {
        return dialog
      },
    }
    mocks.dialog.mockReturnValue(dialog)
    await selectFile(wrapper, 'other.properties')
    const pending = createDeferred<{ success: boolean; errors: [] }>()
    mocks.updateGameServerConfigFile.mockReturnValueOnce(pending.promise)
    editor.vm.$emit('save', new Map([['motd', 'Updated']]))
    await flushPromises()
    confirm?.()
    await flushPromises()
    expect(mocks.getGameServerConfigFile).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('filePath')).toBe(
      'server.properties',
    )
    pending.resolve({ success: true, errors: [] })
    await flushPromises()
    expect(mocks.getGameServerConfigFile).toHaveBeenLastCalledWith(
      expect.objectContaining({ filePath: 'server.properties' }),
    )
  })

  it('blocks stale editor events while a new file is loading', async () => {
    const wrapper = await mountWithMultipleFiles()
    const editor = wrapper.findComponent({ name: 'ConfigFileEditor' })
    const pending = createDeferred<typeof otherFile>()
    mocks.getGameServerConfigFile.mockReturnValueOnce(pending.promise)
    // Queue a save in the same turn that the selection hides the old editor.
    wrapper
      .findComponent({ name: 'ConfigFileSidebar' })
      .vm.$emit('select', 'other.properties', false)
    await Promise.resolve()
    editor.vm.$emit('save', new Map([['motd', 'Do not write to other.properties']]))
    editor.vm.$emit('generate')
    await flushPromises()
    expect(mocks.updateGameServerConfigFile).not.toHaveBeenCalled()
    expect(mocks.generateGameServerConfigFile).not.toHaveBeenCalled()
    pending.resolve(otherFile)
    await flushPromises()
  })

  it('uses the same safe loading flow for the mobile file picker', async () => {
    const wrapper = await mountWithMultipleFiles()
    const pending = createDeferred<typeof thirdFile>()
    mocks.getGameServerConfigFile.mockReturnValueOnce(pending.promise)
    wrapper.findComponent({ name: 'QSelect' }).vm.$emit('update:modelValue', 'third.properties')
    await flushPromises()
    expect(wrapper.find('.editor-stub').exists()).toBe(false)
    expect(wrapper.find('[data-test="config-file-loading"]').text()).toContain('third.properties')
    pending.resolve(thirdFile)
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('filePath')).toBe(
      'third.properties',
    )
  })

  it.each([
    { operation: 'save', verb: 'saved' },
    { operation: 'generate', verb: 'generated' },
  ] as const)(
    'confirms a pending $operation after leaving the page without refreshing it',
    async ({ operation, verb }) => {
      const wrapper = await mountWithMultipleFiles()
      const pending = createDeferred<{ success: boolean; errors: [] }>()
      const mutation =
        operation === 'save' ? mocks.updateGameServerConfigFile : mocks.generateGameServerConfigFile
      mutation.mockReturnValueOnce(pending.promise)
      wrapper
        .findComponent({ name: 'ConfigFileEditor' })
        .vm.$emit(operation, new Map([['motd', 'Updated']]))
      await flushPromises()
      wrapper.unmount()
      pending.resolve({ success: true, errors: [] })
      await flushPromises()
      expect(mocks.getGameServerConfigFile).not.toHaveBeenCalled()
      expect(mocks.notify).toHaveBeenCalledWith(`server.properties ${verb} successfully`)
      expect(mocks.confirmSaved).not.toHaveBeenCalled()
    },
  )

  it('clears save validation errors after a successful generate', async () => {
    const wrapper = await mountWithFile()
    mocks.updateGameServerConfigFile.mockResolvedValue({
      success: false,
      errors: [{ field: 'motd', message: 'Too long' }],
    })
    const editor = wrapper.findComponent({ name: 'ConfigFileEditor' })
    editor.vm.$emit('save', new Map([['motd', 'x']]))
    await flushPromises()
    expect(editor.props('validationErrors')).toHaveLength(1)

    mocks.generateGameServerConfigFile.mockResolvedValue({ success: true })
    editor.vm.$emit('generate')
    await flushPromises()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('validationErrors')).toEqual(
      [],
    )
  })

  it('offers Retry when the config files fail to load', async () => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.find('[data-test="config-load-error"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('No config files for this game yet')

    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: oneConfigFile })
    mocks.getGameServerConfigFile.mockResolvedValue({ fields: [], advancedFields: [] })
    await wrapper.find('[data-test="config-load-error"] button').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="config-load-error"]').exists()).toBe(false)
    expect(wrapper.find('.editor-stub').exists()).toBe(true)
  })

  it('shows a file load failure with Retry instead of an empty editor', async () => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: oneConfigFile })
    mocks.getGameServerConfigFile.mockRejectedValueOnce(new Error('node offline'))
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.find('[data-test="config-file-load-error"]').exists()).toBe(true)
    expect(wrapper.find('.editor-stub').exists()).toBe(false)
  })

  it.each([
    { name: 'links superusers to the game editor', superUser: true, linked: true },
    { name: 'leaves other users without the link', superUser: false, linked: false },
  ])('$name when the game has no config files', async ({ superUser, linked }) => {
    useUserAuthStore().user = { superUser } as never
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: [] })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'valheim' } })
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.text()).toContain('No config files for this game yet')
    expect(wrapper.find('[data-test="config-edit-game"]').exists()).toBe(linked)
  })
})
