import { create } from '@bufbuild/protobuf'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AlertRule, NotificationChannel } from '@/proto/shared_pb'
import {
  AlertEventType,
  AlertRuleSchema,
  NotificationChannelSchema,
  NotificationChannelType,
} from '@/proto/shared_pb'
import { CheckUserAuthenticatedResponseSchema, UserSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import AlertRuleDialog from '@/components/alerts/AlertRuleDialog.vue'
import Notifications from './Notifications.vue'

const mocks = vi.hoisted(() => ({
  route: { query: {} as Record<string, string> },
  notify: vi.fn(),
  notifySuccess: vi.fn(),
  notifyError: vi.fn(),
  notifyConnectError: vi.fn(),
  listNotificationChannels: vi.fn(),
  listAlertRules: vi.fn(),
  getAlertHistory: vi.fn(),
  listGameServers: vi.fn(),
  listNodes: vi.fn(),
  createNotificationChannel: vi.fn(),
  updateNotificationChannel: vi.fn(),
  deleteNotificationChannel: vi.fn(),
  testNotificationChannel: vi.fn(),
  updateAlertRule: vi.fn(),
  deleteAlertRule: vi.fn(),
  dialog: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
      screen: { lt: { md: false } },
      dialog: mocks.dialog,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => mocks.route,
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyError: mocks.notifyError,
  notifyConnectError: mocks.notifyConnectError,
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    listNotificationChannels: mocks.listNotificationChannels,
    listAlertRules: mocks.listAlertRules,
    getAlertHistory: mocks.getAlertHistory,
    listGameServers: mocks.listGameServers,
    listNodes: mocks.listNodes,
    createNotificationChannel: mocks.createNotificationChannel,
    updateNotificationChannel: mocks.updateNotificationChannel,
    deleteNotificationChannel: mocks.deleteNotificationChannel,
    testNotificationChannel: mocks.testNotificationChannel,
    updateAlertRule: mocks.updateAlertRule,
    deleteAlertRule: mocks.deleteAlertRule,
  }),
  ConnectErrorToString: (err: unknown) => String(err),
}))

function makeChannel(overrides: Partial<NotificationChannel> = {}): NotificationChannel {
  const channel = create(NotificationChannelSchema)
  Object.assign(channel, {
    id: 'chan-1',
    userId: 'user-1',
    name: 'My Discord Hook',
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
    serverId: 'server-1',
    serverNodeId: 'node-1',
    eventType: AlertEventType.CPU_THRESHOLD,
    condition: JSON.stringify({ operator: '>=', value: 80 }),
    notificationChannelId: 'chan-1',
    enabled: true,
    ...overrides,
  })
  return rule
}

const QTableStub = defineComponent({
  name: 'QTableStub',
  props: {
    rows: { type: Array, default: () => [] },
    columns: { type: Array, default: () => [] },
    loading: { type: Boolean, default: false },
    rowKey: { type: String, default: 'id' },
    flat: { type: Boolean, default: false },
    grid: { type: Boolean, default: false },
  },
  template: `<div class="q-table-stub">
    <div v-if="rows.length === 0"><slot name="no-data" /></div>
    <div v-for="(row, i) in rows" :key="i" class="q-table-row">
      {{ JSON.stringify(row) }}
      <span class="q-table-target"><slot name="body-cell-server" :row="row" /></span>
      <slot name="body-cell-enabled" :row="row" />
      <slot name="body-cell-actions" :row="row" />
    </div>
    <slot />
  </div>`,
})

function setAlertPermissions(permissionIds: string[] = ['alerts.manage'], superUser = false) {
  const store = useUserAuthStore()
  const user = create(UserSchema, {
    id: 'user-1',
    userName: 'owner',
    superUser,
  })
  store.user = user
  store.initialFetch = true
  store.initialResponse = create(CheckUserAuthenticatedResponseSchema, {
    authenticated: true,
    user,
    permissionIds,
  })
}

function mountNotifications(permissionIds: string[] = ['alerts.manage'], superUser = false) {
  // Set up default resolved values before mounting
  if (!mocks.listGameServers.mock.lastCall) {
    mocks.listGameServers.mockResolvedValue({ gameServers: [] })
  }
  if (!mocks.listNodes.mock.lastCall) {
    mocks.listNodes.mockResolvedValue({ nodes: [] })
  }
  if (!mocks.listNotificationChannels.mock.lastCall) {
    mocks.listNotificationChannels.mockResolvedValue({ channels: [] })
  }
  if (!mocks.listAlertRules.mock.lastCall) {
    mocks.listAlertRules.mockResolvedValue({ rules: [] })
  }
  if (!mocks.getAlertHistory.mock.lastCall) {
    mocks.getAlertHistory.mockResolvedValue({ entries: [] })
  }

  setAlertPermissions(permissionIds, superUser)

  return mount(Notifications, {
    global: {
      stubs: {
        'q-page': { template: '<div><slot /></div>' },
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
        'q-banner': { template: '<div class="q-banner-stub"><slot /><slot name="action" /></div>' },
        'q-tab-panels': {
          props: ['modelValue'],
          template: '<div class="q-tab-panels-stub"><slot /></div>',
        },
        'q-tab-panel': {
          props: ['name'],
          template: '<div class="q-tab-panel-stub" :data-panel-name="name"><slot /></div>',
        },
        'q-table': QTableStub,
        'q-td': { template: '<div><slot /></div>' },
        'q-btn': {
          props: ['label', 'icon', 'color', 'disable', 'loading', 'flat', 'dense'],
          emits: ['click'],
          template:
            '<button :disabled="disable" @click.stop="$emit(\'click\')">{{ label || icon }}<slot /></button>',
        },
        'q-badge': {
          props: ['color', 'label'],
          template: '<span class="q-badge-stub" :data-color="color">{{ label }}</span>',
        },
        'q-toggle': {
          props: ['modelValue', 'color', 'label'],
          template: '<div class="q-toggle-stub" />',
        },
        'q-tooltip': { template: '<span />' },
        'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
        'q-select': {
          props: ['modelValue', 'options', 'label'],
          template: '<div class="q-select-stub" />',
        },
        'q-space': { template: '<div />' },
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

describe('Notifications', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  afterEach(() => {
    const { route, ...fns } = mocks
    route.query = {}
    Object.values(fns).forEach((mock) => mock.mockReset())
  })

  it('loads channels on mount', async () => {
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    mountNotifications()
    await flushPromises()

    expect(mocks.listNotificationChannels).toHaveBeenCalledTimes(1)
  })

  it('shows channel data after successful load', async () => {
    const channel = makeChannel({ name: 'Production Discord' })
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [channel] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    const wrapper = mountNotifications()
    await flushPromises()

    // The QTableStub renders row data as JSON, so the channel name should appear
    expect(wrapper.text()).toContain('Production Discord')
  })

  it.each([
    {
      name: 'success',
      response: { success: true, error: '' },
      type: 'xylona-success',
      message: 'Test sent to Production Discord',
      caption: 'Check the channel for the test notification.',
    },
    {
      name: 'failure',
      response: { success: false, error: 'The webhook responded with HTTP 404' },
      type: 'xylona-error',
      message: 'Test to Production Discord failed',
      caption: 'The webhook responded with HTTP 404',
    },
  ])('tests a webhook channel and reports $name', async ({ response, type, message, caption }) => {
    const channel = makeChannel({ name: 'Production Discord' })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [channel] })
    mocks.testNotificationChannel.mockResolvedValueOnce(response)

    const wrapper = mountNotifications()
    await flushPromises()

    await wrapper.get('button[aria-label="Test Production Discord channel"]').trigger('click')
    await flushPromises()

    expect(mocks.testNotificationChannel).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'chan-1' }),
    )
    expect(mocks.notify).toHaveBeenCalledWith(expect.objectContaining({ type, message, caption }))
  })

  it('hides channel management actions for history-only users', async () => {
    const channel = makeChannel()
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [channel] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    const wrapper = mountNotifications(['alerts.view_history'])
    await flushPromises()

    expect(wrapper.text()).not.toContain('Add Channel')
    expect(wrapper.text()).not.toContain('send')
    expect(wrapper.text()).not.toContain('edit')
    expect(wrapper.text()).not.toContain('delete')
  })

  it.each([
    { name: 'opens Channels by default', query: {}, want: 'channels' },
    { name: 'opens Alert Rules from ?tab=rules', query: { tab: 'rules' }, want: 'rules' },
    { name: 'opens Alert History from ?tab=history', query: { tab: 'history' }, want: 'history' },
    { name: 'ignores an unknown tab', query: { tab: 'nope' }, want: 'channels' },
  ])('$name', async ({ query, want }) => {
    mocks.route.query = query
    const wrapper = mountNotifications()
    await flushPromises()

    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe(want)
  })

  const nodeRule = makeRule({
    serverId: undefined,
    serverNodeId: undefined,
    nodeId: 'node-1',
    eventType: AlertEventType.NODE_DISK_THRESHOLD,
  })

  it('keeps a non-superuser node rule listed with a note, without edit or toggle', async () => {
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [nodeRule] })

    const wrapper = mountNotifications()
    await flushPromises()

    const row = wrapper.get('.q-table-row')
    expect(row.text()).toContain("Won't send")
    expect(row.text()).toContain('Node alerts only go to superusers.')
    expect(row.find('button[aria-label="Edit Node Disk alert rule"]').exists()).toBe(false)
    expect(row.find('.q-toggle-stub').exists()).toBe(false)
    expect(row.find('button[aria-label="Delete Node Disk alert rule"]').exists()).toBe(true)
  })

  it('lets a superuser edit a node rule in the shared dialog', async () => {
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [nodeRule] })

    const wrapper = mountNotifications(['alerts.manage'], true)
    await flushPromises()

    const row = wrapper.get('.q-table-row')
    expect(row.text()).not.toContain("Won't send")
    await row.get('button[aria-label="Edit Node Disk alert rule"]').trigger('click')
    await flushPromises()

    const dialog = wrapper.getComponent(AlertRuleDialog)
    expect(dialog.props('rule')).toEqual(nodeRule)
    expect(dialog.props('modelValue')).toBe(true)
  })

  it('names the watched node as the target of node rules and history', async () => {
    mocks.listNodes.mockResolvedValueOnce({ nodes: [{ id: 'node-1', name: 'Rack A' }] })
    mocks.listAlertRules.mockResolvedValueOnce({
      rules: [
        makeRule({
          serverId: undefined,
          serverNodeId: undefined,
          nodeId: 'node-1',
          eventType: AlertEventType.NODE_DISK_THRESHOLD,
        }),
      ],
    })
    mocks.getAlertHistory.mockResolvedValueOnce({
      entries: [{ id: 'hist-1', nodeId: 'node-1', eventType: AlertEventType.NODE_DISK_THRESHOLD }],
    })

    const wrapper = mountNotifications()
    await flushPromises()

    const targets = wrapper.findAll('.q-table-target').map((cell) => cell.text())
    expect(targets.filter((target) => target === 'Rack A')).toHaveLength(2)
  })
})
