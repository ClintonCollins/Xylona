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
import Notifications from './Notifications.vue'

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  listNotificationChannels: vi.fn(),
  listAlertRules: vi.fn(),
  getAlertHistory: vi.fn(),
  listGameServers: vi.fn(),
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

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    listNotificationChannels: mocks.listNotificationChannels,
    listAlertRules: mocks.listAlertRules,
    getAlertHistory: mocks.getAlertHistory,
    listGameServers: mocks.listGameServers,
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
      <slot name="body-cell-enabled" :row="row" />
      <slot name="body-cell-actions" :row="row" />
    </div>
    <slot />
  </div>`,
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

function mountNotifications(permissionIds: string[] = ['alerts.manage']) {
  // Set up default resolved values before mounting
  if (!mocks.listGameServers.mock.lastCall) {
    mocks.listGameServers.mockResolvedValue({ gameServers: [] })
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

  setAlertPermissions(permissionIds)

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
    Object.values(mocks).forEach((mock) => mock.mockReset())
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

  it('does not show test delivery actions while the backend endpoint is a stub', async () => {
    const channel = makeChannel({ name: 'Production Discord' })
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [channel] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    const wrapper = mountNotifications()
    await flushPromises()

    expect(wrapper.text()).not.toContain('send')
    expect(wrapper.text()).not.toContain('Test delivery')
  })

  it('shows error notification on failed channel load', async () => {
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockRejectedValueOnce(new Error('network failure'))
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    mountNotifications()
    await flushPromises()

    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-error',
      }),
    )
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

  it('parses and preserves extended threshold behavior when editing a rule', async () => {
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [makeChannel()] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })
    mocks.updateAlertRule.mockResolvedValueOnce({})
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })

    const wrapper = mountNotifications()
    await flushPromises()

    type NotificationsVM = {
      openRuleEditDialog: (rule: AlertRule) => void
      saveRule: () => Promise<void>
      ruleForm: {
        thresholdForSeconds: number
        thresholdRecoveryValue: number | null
        thresholdCooldownSeconds: number
        thresholdRepeatSeconds: number
        thresholdNoDataSeconds: number
      }
    }
    const vm = wrapper.vm as unknown as NotificationsVM
    vm.openRuleEditDialog(
      makeRule({
        eventType: AlertEventType.MEMORY_THRESHOLD,
        condition: JSON.stringify({
          operator: '>=',
          value: 85,
          for_seconds: 120,
          recovery_value: 75,
          cooldown_seconds: 300,
          repeat_seconds: 900,
          no_data_seconds: 180,
        }),
      }),
    )

    expect(vm.ruleForm).toMatchObject({
      thresholdForSeconds: 120,
      thresholdRecoveryValue: 75,
      thresholdCooldownSeconds: 300,
      thresholdRepeatSeconds: 900,
      thresholdNoDataSeconds: 180,
    })

    await vm.saveRule()
    await flushPromises()

    const request = mocks.updateAlertRule.mock.calls[0]?.[0] as { condition: string }
    expect(JSON.parse(request.condition)).toEqual({
      operator: '>=',
      value: 85,
      for_seconds: 120,
      recovery_value: 75,
      cooldown_seconds: 300,
      repeat_seconds: 900,
      no_data_seconds: 180,
    })
  })

  it('blocks invalid threshold timing and recovery values', async () => {
    mocks.listGameServers.mockResolvedValueOnce({ gameServers: [] })
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [makeChannel()] })
    mocks.listAlertRules.mockResolvedValueOnce({ rules: [] })
    mocks.getAlertHistory.mockResolvedValueOnce({ entries: [] })

    const wrapper = mountNotifications()
    await flushPromises()

    type NotificationsVM = {
      openRuleEditDialog: (rule: AlertRule) => void
      saveRule: () => Promise<void>
      ruleForm: {
        thresholdForSeconds: number
        thresholdRecoveryValue: number | null
      }
      canSaveRule: boolean
    }
    const vm = wrapper.vm as unknown as NotificationsVM
    vm.openRuleEditDialog(makeRule())
    vm.ruleForm.thresholdForSeconds = -1
    vm.ruleForm.thresholdRecoveryValue = 90

    expect(vm.canSaveRule).toBe(false)
    await vm.saveRule()

    expect(mocks.updateAlertRule).not.toHaveBeenCalled()
  })
})
