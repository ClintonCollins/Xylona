import { create } from '@bufbuild/protobuf'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { Quasar, QTable, QTd } from 'quasar'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AlertHistoryEntry, AlertRule, NotificationChannel } from '@/proto/shared_pb'
import {
  AlertEventType,
  AlertHistoryEntrySchema,
  AlertRuleSchema,
  DeliveryStatus,
  NotificationChannelSchema,
  NotificationChannelType,
} from '@/proto/shared_pb'
import { CheckUserAuthenticatedResponseSchema, UserSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import GameServerAlerts from './GameServerAlerts.vue'

const quasarScreen = vi.hoisted(() => ({ lt: { md: false } }))

const mocks = vi.hoisted(() => ({
  notifyConnectError: vi.fn(),
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
  dialog: vi.fn(),
  getGameServer: vi.fn(),
  listNotificationChannels: vi.fn(),
  listAlertRules: vi.fn(),
  getAlertHistory: vi.fn(),
  createAlertRule: vi.fn(),
  updateAlertRule: vi.fn(),
  deleteAlertRule: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      screen: quasarScreen,
      dialog: mocks.dialog,
    }),
  }
})

vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notifyConnectError,
  notifyError: mocks.notifyError,
  notifySuccess: mocks.notifySuccess,
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    listNotificationChannels: mocks.listNotificationChannels,
    listAlertRules: mocks.listAlertRules,
    getAlertHistory: mocks.getAlertHistory,
    createAlertRule: mocks.createAlertRule,
    updateAlertRule: mocks.updateAlertRule,
    deleteAlertRule: mocks.deleteAlertRule,
  }),
  ConnectErrorToString: (err: unknown) => String(err),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    params: { id: 'test-server-123' },
    path: '/game-servers/test-server-123/alerts',
  }),
}))

function makeChannel(overrides: Partial<NotificationChannel> = {}): NotificationChannel {
  const channel = create(NotificationChannelSchema)
  Object.assign(channel, {
    id: 'chan-1',
    userId: 'user-1',
    name: 'Discord Alerts',
    channelType: NotificationChannelType.WEBHOOK_DISCORD,
    config: JSON.stringify({ url: 'https://discord.com/api/webhooks/test' }),
    enabled: true,
    ...overrides,
  })
  return channel
}

function makeRule(overrides: Partial<AlertRule> = {}): AlertRule {
  const rule = create(AlertRuleSchema)
  Object.assign(rule, {
    id: 'rule-1',
    userId: 'user-1',
    serverId: 'test-server-123',
    eventType: AlertEventType.CRASH,
    condition: '',
    notificationChannelId: 'chan-1',
    enabled: true,
    ...overrides,
  })
  return rule
}

function makeHistoryEntry(overrides: Partial<AlertHistoryEntry> = {}): AlertHistoryEntry {
  const entry = create(AlertHistoryEntrySchema)
  Object.assign(entry, {
    id: 'hist-1',
    userId: 'user-1',
    serverId: 'test-server-123',
    eventType: AlertEventType.CRASH,
    eventData: 'Server crashed unexpectedly',
    channelType: NotificationChannelType.WEBHOOK_DISCORD,
    deliveryStatus: DeliveryStatus.SENT,
    ...overrides,
  })
  return entry
}

const QTableStub = defineComponent({
  name: 'QTableStub',
  props: {
    rows: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false },
    rowKey: { type: String, default: 'id' },
    flat: { type: Boolean, default: false },
    noDataLabel: { type: String, default: '' },
    pagination: { type: Object, default: () => ({}) },
  },
  template: `<div class="q-table-stub">
    <div v-if="rows.length === 0" class="q-table-no-data"><slot name="no-data">{{ noDataLabel }}</slot></div>
    <div v-for="(row, i) in rows" :key="i" class="q-table-row">
      {{ JSON.stringify(row) }}
      <slot name="body-cell-enabled" :row="row" />
      <slot name="body-cell-actions" :row="row" />
      <slot name="body-cell-deliveryStatus" :row="row" />
    </div>
    <slot />
  </div>`,
})

const QBadgeStub = defineComponent({
  name: 'QBadgeStub',
  props: {
    color: { type: String, default: '' },
    label: { type: String, default: '' },
  },
  template: '<span class="q-badge-stub" :data-color="color">{{ label }}</span>',
})

function setAlertPermissions(permissionIds: string[] = ['alerts.manage']) {
  const store = useUserAuthStore()
  const user = create(UserSchema, {
    id: 'user-1',
    userName: 'owner',
    superUser: false,
  })
  store.user = user
  store.initialFetch = true
  store.initialResponse = create(CheckUserAuthenticatedResponseSchema, {
    authenticated: true,
    user,
    permissionIds,
  })
}

function mountAlerts(permissionIds: string[] = ['alerts.manage'], realTable = false) {
  setAlertPermissions(permissionIds)

  return mount(GameServerAlerts, {
    global: {
      plugins: realTable ? [Quasar] : [],
      stubs: {
        'q-tabs': {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template: '<div class="q-tabs-stub"><slot /></div>',
        },
        'q-tab': {
          props: ['name', 'label'],
          template: '<div class="q-tab-stub" :data-tab-name="name">{{ label }}</div>',
        },
        'q-separator': { template: '<hr />' },
        'q-tab-panels': {
          props: ['modelValue'],
          template: '<div class="q-tab-panels-stub"><slot /></div>',
        },
        'q-tab-panel': {
          props: ['name'],
          template: '<div class="q-tab-panel-stub" :data-panel-name="name"><slot /></div>',
        },
        'q-table': realTable ? QTable : QTableStub,
        'q-td': realTable ? QTd : { template: '<div><slot /></div>' },
        'q-btn': {
          name: 'QBtn',
          props: ['label', 'icon', 'color', 'disable', 'loading', 'flat', 'dense', 'size'],
          emits: ['click'],
          template:
            '<button :disabled="disable" @click.stop="$emit(\'click\')">{{ label || icon }}<slot /></button>',
        },
        'q-badge': QBadgeStub,
        'q-toggle': {
          props: ['modelValue', 'color', 'label'],
          template: '<div class="q-toggle-stub" />',
        },
        'q-tooltip': { template: '<span />' },
        'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
        'q-select': {
          name: 'QSelect',
          props: ['modelValue', 'options', 'label'],
          template: '<div class="q-select-stub" />',
        },
        'q-space': { template: '<div />' },
        'q-banner': {
          template: '<div class="q-banner-stub"><slot /><slot name="avatar" /></div>',
        },
        'q-dialog': { template: '<div><slot /></div>' },
        'q-card': { template: '<div><slot /></div>' },
        'q-card-section': { template: '<div><slot /></div>' },
        'q-card-actions': { template: '<div><slot /></div>' },
        'q-expansion-item': {
          props: ['label'],
          template: '<section><span>{{ label }}</span><slot /></section>',
        },
        'q-input': { template: '<input />' },
        'q-checkbox': true,
        'router-link': { template: '<a><slot /></a>' },
      },
    },
  })
}

function historyFilter(wrapper: ReturnType<typeof mountAlerts>) {
  const filter = wrapper
    .findAllComponents({ name: 'QSelect' })
    .find((component) => component.props('label') === 'Filter by event')
  if (!filter) throw new Error('Expected event filter')
  return filter
}

function historyTable(wrapper: ReturnType<typeof mountAlerts>) {
  const table = wrapper
    .findAllComponents(QTableStub)
    .find((component) => component.attributes('aria-label') === 'Alert history')
  if (!table) throw new Error('Expected alert history table')
  return table
}

function loadOlderButton(wrapper: ReturnType<typeof mountAlerts>) {
  const button = wrapper
    .findAllComponents({ name: 'QBtn' })
    .find((component) => component.props('label') === 'Load older events')
  if (!button) throw new Error('Expected Load older events button')
  return button
}

describe('GameServerAlerts', () => {
  beforeEach(() => {
    quasarScreen.lt.md = false
    setActivePinia(createPinia())
  })

  afterEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
  })

  function setupDefaultMocks(
    overrides: {
      nodeId?: string
      channels?: NotificationChannel[]
      rules?: AlertRule[]
      entries?: AlertHistoryEntry[]
    } = {},
  ) {
    mocks.getGameServer.mockResolvedValueOnce({
      gameServer: {
        id: 'test-server-123',
        nodeId: overrides.nodeId ?? 'node-local',
      },
    })
    mocks.listNotificationChannels.mockResolvedValueOnce({
      channels: overrides.channels ?? [],
    })
    mocks.listAlertRules.mockResolvedValueOnce({
      rules: overrides.rules ?? [],
    })
    mocks.getAlertHistory.mockResolvedValueOnce({
      entries: overrides.entries ?? [],
    })
  }

  it('loads rules, history, and channels on mount', async () => {
    setupDefaultMocks()
    mountAlerts()
    await flushPromises()

    expect(mocks.getGameServer).toHaveBeenCalledTimes(1)
    expect(mocks.listNotificationChannels).toHaveBeenCalledTimes(1)
    expect(mocks.listAlertRules).toHaveBeenCalledTimes(1)
    expect(mocks.getAlertHistory).toHaveBeenCalledTimes(1)
  })

  it('passes the current server node id to rules and history requests', async () => {
    setupDefaultMocks({ nodeId: 'node-remote-42' })

    mountAlerts()
    await flushPromises()

    expect(mocks.listAlertRules).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: 'test-server-123',
        serverNodeId: 'node-remote-42',
      }),
    )
    expect(mocks.getAlertHistory).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: 'test-server-123',
        serverNodeId: 'node-remote-42',
        limit: 100,
        offset: 0,
      }),
    )
  })

  it('shows "Create Rule" button that is disabled when no channels exist', async () => {
    setupDefaultMocks({ channels: [] })
    const wrapper = mountAlerts()
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const createBtn = buttons.find((b) => b.text().includes('Create Rule'))
    expect(createBtn).toBeDefined()
    if (createBtn) {
      expect(createBtn.attributes('disabled')).toBeDefined()
    }
  })

  it('shows "Create Rule" button that is enabled when channels exist', async () => {
    setupDefaultMocks({ channels: [makeChannel()] })
    const wrapper = mountAlerts()
    await flushPromises()

    const buttons = wrapper.findAll('button')
    const createBtn = buttons.find((b) => b.text().includes('Create Rule'))
    expect(createBtn).toBeDefined()
    if (createBtn) {
      expect(createBtn.attributes('disabled')).toBeUndefined()
    }
  })

  it('shows rule data after successful load', async () => {
    const channel = makeChannel({ id: 'chan-1', name: 'Discord Alerts' })
    const rule = makeRule({
      eventType: AlertEventType.CPU_THRESHOLD,
      condition: JSON.stringify({ operator: '>=', value: 90 }),
      notificationChannelId: 'chan-1',
    })
    setupDefaultMocks({ channels: [channel], rules: [rule] })
    const wrapper = mountAlerts()
    await flushPromises()

    // The QTableStub serializes rows as JSON -- check that rule data is present
    const tableRows = wrapper.findAll('.q-table-row')
    expect(tableRows.length).toBeGreaterThanOrEqual(1)
  })

  it('labels rule toggles by event type and condition', async () => {
    setupDefaultMocks({
      channels: [makeChannel()],
      rules: [
        makeRule({
          id: 'rule-cpu',
          eventType: AlertEventType.CPU_THRESHOLD,
          condition: JSON.stringify({ operator: '>=', value: 90 }),
        }),
        makeRule({ id: 'rule-status', eventType: AlertEventType.STATUS_CHANGE }),
      ],
    })
    const wrapper = mountAlerts()
    await flushPromises()

    expect(
      wrapper
        .findAll('.q-table-row .q-toggle-stub')
        .map((toggle) => toggle.attributes('aria-label')),
    ).toEqual(['Enable CPU Threshold alert: >= 90%', 'Enable Status Change alert: Any status'])
  })

  it('names row actions after their rule', async () => {
    setupDefaultMocks({
      channels: [makeChannel()],
      rules: [makeRule({ id: 'rule-status', eventType: AlertEventType.STATUS_CHANGE })],
    })
    const wrapper = mountAlerts()
    await flushPromises()

    expect(
      wrapper.findAll('.q-table-row .xy-row-actions button').map((b) => b.attributes('aria-label')),
    ).toEqual(['Edit Status Change alert: Any status', 'Delete Status Change alert: Any status'])
  })

  it('shows a delivery error as text under its status', async () => {
    setupDefaultMocks({ entries: [makeHistoryEntry({ deliveryError: 'webhook returned 404' })] })
    const wrapper = mountAlerts()
    await flushPromises()

    expect(wrapper.find('.alerts-delivery-error').text()).toBe('webhook returned 404')
  })

  it('describes alert history details instead of showing raw event data', async () => {
    setupDefaultMocks({
      entries: [
        makeHistoryEntry({
          eventType: AlertEventType.CPU_THRESHOLD,
          eventData: '{"current_value":92.345,"threshold":90,"direction":"entered"}',
        }),
      ],
    })

    const wrapper = mountAlerts()
    await flushPromises()

    type Column = { name: string; field: (row: AlertHistoryEntry) => string }
    type AlertsVM = { historyColumns: Column[]; alertHistory: AlertHistoryEntry[] }
    const vm = wrapper.vm as unknown as AlertsVM
    const details = vm.historyColumns.find((column) => column.name === 'eventData')
    expect(details?.field(vm.alertHistory[0] as AlertHistoryEntry)).toBe('92.3% (threshold 90%)')
  })

  it('shows a load error instead of empty states when loading rules fails', async () => {
    mocks.getGameServer.mockResolvedValueOnce({
      gameServer: { id: 'test-server-123', nodeId: 'node-local' },
    })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [] })
    mocks.listAlertRules.mockRejectedValueOnce(new Error('load rules failed'))
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    const wrapper = mountAlerts()
    await flushPromises()

    expect(wrapper.text()).toContain('Alerts could not be loaded.')
    expect(wrapper.text()).toContain('load rules failed')
    expect(
      wrapper.findAllComponents(QTableStub)[0].findComponent({ name: 'EmptyState' }).exists(),
    ).toBe(false)
  })

  it('does not claim there are no channels when channels fail to load', async () => {
    mocks.getGameServer.mockResolvedValueOnce({
      gameServer: { id: 'test-server-123', nodeId: 'node-local' },
    })
    mocks.listNotificationChannels.mockRejectedValueOnce(new Error('channels unavailable'))
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    const wrapper = mountAlerts()
    await flushPromises()

    expect(wrapper.text()).toContain('channels unavailable')
    expect(wrapper.text()).not.toContain('No notification channels configured')
  })

  it('shows a load error when loading history fails', async () => {
    mocks.getGameServer.mockResolvedValueOnce({
      gameServer: {
        id: 'test-server-123',
        nodeId: 'node-local',
      },
    })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockRejectedValueOnce(new Error('load history failed'))

    const wrapper = mountAlerts()
    await flushPromises()

    expect(wrapper.text()).toContain('load history failed')
  })

  it('creates a rule with the current server node id', async () => {
    mocks.getGameServer.mockResolvedValueOnce({
      gameServer: {
        id: 'test-server-123',
        nodeId: 'node-local',
      },
    })
    mocks.listNotificationChannels.mockResolvedValue({
      channels: [makeChannel()],
    })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })
    mocks.createAlertRule.mockResolvedValueOnce({})
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })

    const wrapper = mountAlerts()
    await flushPromises()

    const createRuleButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('Create Rule'))
    if (!createRuleButton) {
      throw new Error('expected Create Rule button')
    }
    await createRuleButton.trigger('click')
    await flushPromises()

    const dialogCreateButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Create')
    if (!dialogCreateButton) {
      throw new Error('expected dialog Create button')
    }
    await dialogCreateButton.trigger('click')
    await flushPromises()

    expect(mocks.createAlertRule).toHaveBeenCalledWith(
      expect.objectContaining({
        serverId: 'test-server-123',
        serverNodeId: 'node-local',
      }),
    )
    expect(mocks.notifySuccess).toHaveBeenCalledWith('Alert rule created')
    // Saving reloads the rules list.
    expect(mocks.listAlertRules).toHaveBeenCalledTimes(2)
  })

  it('loads more history when additional server entries are available', async () => {
    const firstPageEntries = Array.from({ length: 100 }, (_, idx) =>
      makeHistoryEntry({ id: `hist-${idx + 1}` }),
    )
    const secondPageEntry = makeHistoryEntry({ id: 'hist-101' })

    mocks.getGameServer.mockResolvedValueOnce({
      gameServer: {
        id: 'test-server-123',
        nodeId: 'node-local',
      },
    })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: firstPageEntries })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [secondPageEntry] })

    const wrapper = mountAlerts()
    await flushPromises()

    const loadMoreButton = wrapper
      .findAll('button')
      .find((button) => button.text().includes('Load older events'))
    if (!loadMoreButton) {
      throw new Error('expected Load older events button')
    }
    await loadMoreButton.trigger('click')
    await flushPromises()

    expect(mocks.getAlertHistory).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({
        serverId: 'test-server-123',
        serverNodeId: 'node-local',
        limit: 100,
        offset: 100,
      }),
    )

    type AlertsVM = {
      alertHistory: AlertHistoryEntry[]
    }
    const vm = wrapper.vm as unknown as AlertsVM
    expect(vm.alertHistory).toHaveLength(101)
  })

  it('distinguishes an empty history from a filter with no matches', async () => {
    setupDefaultMocks()
    const wrapper = mountAlerts()
    await flushPromises()

    expect(wrapper.text()).toContain('No alert history for this server')
    expect(wrapper.find('[data-testid="history-scope"]').text()).toBe('0 events loaded')
    expect(wrapper.find('[data-testid="clear-history-filter"]').exists()).toBe(false)

    wrapper.unmount()
  })

  it.each([1, 100])(
    'explains the scope of a filter with no matches in %i loaded events',
    async (count) => {
      setupDefaultMocks({
        entries: Array.from({ length: count }, (_, index) =>
          makeHistoryEntry({ id: `hist-${index}`, eventType: AlertEventType.STATUS_CHANGE }),
        ),
      })
      const wrapper = mountAlerts()
      await flushPromises()
      const filter = historyFilter(wrapper)
      filter.vm.$emit('update:modelValue', AlertEventType.CRASH)
      await flushPromises()

      expect(wrapper.text()).not.toContain('No alert history for this server')
      expect(wrapper.text()).toContain('No matching events in loaded history')
      expect(wrapper.find('[data-testid="history-scope"]').text()).toContain(
        `0 matching of ${count} loaded`,
      )
      expect(wrapper.text().includes('Older events may contain a match.')).toBe(count === 100)
      expect(wrapper.find('[data-testid="clear-history-filter"]').exists()).toBe(true)

      wrapper.unmount()
    },
  )

  it('clears the history filter and returns to the first page without changing page size', async () => {
    setupDefaultMocks({ entries: [makeHistoryEntry({ eventType: AlertEventType.STATUS_CHANGE })] })
    const wrapper = mountAlerts()
    await flushPromises()
    const table = historyTable(wrapper)
    table.vm.$emit('update:pagination', { page: 3, rowsPerPage: 10 })
    await flushPromises()
    const filter = historyFilter(wrapper)
    filter.vm.$emit('update:modelValue', AlertEventType.CRASH)
    await flushPromises()

    expect(table.props('pagination')).toMatchObject({ page: 1, rowsPerPage: 10 })
    await wrapper.get('[data-testid="clear-history-filter"]').trigger('click')
    await flushPromises()

    expect(filter.props('modelValue')).toBeNull()
    expect(table.props('rows')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('No matching events in loaded history')
    expect(mocks.getAlertHistory).toHaveBeenCalledTimes(1)

    wrapper.unmount()
  })

  it('finds an older matching event without duplicating repeated load requests', async () => {
    setupDefaultMocks({
      entries: Array.from({ length: 100 }, (_, index) =>
        makeHistoryEntry({ id: `hist-${index}`, eventType: AlertEventType.STATUS_CHANGE }),
      ),
    })
    let resolveOlder!: (response: { entries: AlertHistoryEntry[] }) => void
    mocks.getAlertHistory.mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOlder = resolve
        }),
    )
    const wrapper = mountAlerts()
    await flushPromises()
    const filter = historyFilter(wrapper)
    filter.vm.$emit('update:modelValue', AlertEventType.CRASH)
    await flushPromises()

    const loadOlder = loadOlderButton(wrapper)
    loadOlder.vm.$emit('click')
    loadOlder.vm.$emit('click')
    await flushPromises()
    expect(mocks.getAlertHistory).toHaveBeenCalledTimes(2)
    expect(mocks.getAlertHistory).toHaveBeenLastCalledWith(
      expect.objectContaining({ limit: 100, offset: 100 }),
    )
    expect(wrapper.text()).not.toContain('No matching events in loaded history')

    resolveOlder({ entries: [makeHistoryEntry({ id: 'older-crash' })] })
    await flushPromises()

    const table = historyTable(wrapper)
    expect(table.props('rows')).toEqual([expect.objectContaining({ id: 'older-crash' })])
    expect(wrapper.find('[data-testid="history-scope"]').text()).toBe(
      '1 matching of 101 loaded events',
    )
    expect(wrapper.text()).not.toContain('Older events may be available')
    expect(wrapper.text()).not.toContain('Load older events')

    wrapper.unmount()
  })

  it('retries a failed older-history request without hiding it behind an empty state', async () => {
    setupDefaultMocks({
      entries: Array.from({ length: 100 }, (_, index) =>
        makeHistoryEntry({ id: `hist-${index}`, eventType: AlertEventType.STATUS_CHANGE }),
      ),
    })
    mocks.getAlertHistory.mockRejectedValueOnce(new Error('Older history unavailable'))
    mocks.getAlertHistory.mockResolvedValueOnce({
      entries: [makeHistoryEntry({ id: 'older-crash' })],
    })
    const wrapper = mountAlerts()
    await flushPromises()
    historyFilter(wrapper).vm.$emit('update:modelValue', AlertEventType.CRASH)
    await flushPromises()
    loadOlderButton(wrapper).vm.$emit('click')
    await flushPromises()

    expect(wrapper.text()).toContain('Older history unavailable')
    expect(wrapper.text()).not.toContain('No matching events in loaded history')
    expect(wrapper.find('[data-testid="history-scope"]').exists()).toBe(false)

    loadOlderButton(wrapper).vm.$emit('click')
    await flushPromises()

    expect(mocks.getAlertHistory).toHaveBeenLastCalledWith(expect.objectContaining({ offset: 100 }))
    expect(historyTable(wrapper).props('rows')).toHaveLength(1)
    expect(wrapper.text()).not.toContain('Older history unavailable')
    expect(wrapper.find('[data-testid="history-scope"]').text()).toBe(
      '1 matching of 101 loaded events',
    )
    wrapper.unmount()
  })

  it.each([false, true])(
    'keeps real-table pagination, filter, and page size coherent after refresh (mobile: %s)',
    async (mobile) => {
      quasarScreen.lt.md = mobile
      const rowSelector = mobile ? '.alerts-mobile-card' : 'tbody tr'
      setupDefaultMocks({
        entries: Array.from({ length: 60 }, (_, index) =>
          makeHistoryEntry({ id: `hist-${index}`, eventData: `Crash ${index}` }),
        ),
      })
      mocks.getAlertHistory.mockResolvedValueOnce({
        entries: [makeHistoryEntry({ id: 'refreshed-crash' })],
      })
      const wrapper = mountAlerts(['alerts.view_history'], true)
      await flushPromises()
      const table = wrapper
        .findAllComponents(QTable)
        .find((component) => component.attributes('aria-label') === 'Alert history')
      if (!table) throw new Error('Expected real alert history table')
      historyFilter(wrapper).vm.$emit('update:modelValue', AlertEventType.CRASH)
      await flushPromises()
      table.vm.$emit('update:pagination', { page: 3, rowsPerPage: 10 })
      await flushPromises()
      expect(table.findAll(rowSelector)).toHaveLength(10)

      await wrapper.get('[aria-label="Refresh history"]').trigger('click')
      await flushPromises()

      expect(table.props('pagination')).toMatchObject({ page: 1, rowsPerPage: 10 })
      expect(historyFilter(wrapper).props('modelValue')).toBe(AlertEventType.CRASH)
      expect(table.findAll(rowSelector)).toHaveLength(1)
      expect(wrapper.find('[data-testid="history-scope"]').text()).toBe(
        '1 matching of 1 loaded event',
      )

      historyFilter(wrapper).vm.$emit('update:modelValue', AlertEventType.STATUS_CHANGE)
      await flushPromises()
      expect(table.text()).toContain('No matching events in loaded history')
      await wrapper.get('[data-testid="clear-history-filter"]').trigger('click')
      await flushPromises()
      expect(table.findAll(rowSelector)).toHaveLength(1)
      wrapper.unmount()
    },
  )

  it('hides alert rule write actions for history-only users', async () => {
    setupDefaultMocks({
      channels: [makeChannel()],
      rules: [makeRule()],
    })

    const wrapper = mountAlerts(['alerts.view_history'])
    await flushPromises()

    expect(wrapper.text()).not.toContain('Create Rule')
    expect(wrapper.text()).not.toContain('edit')
    expect(wrapper.text()).not.toContain('delete')
  })
})
