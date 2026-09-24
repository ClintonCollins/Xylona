import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import type { AlertRule } from '@/proto/shared_pb'
import { AlertEventType, AlertRuleSchema } from '@/proto/shared_pb'
import { UserSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import AlertRuleDialog from './AlertRuleDialog.vue'

const mocks = vi.hoisted(() => ({
  listNotificationChannels: vi.fn(),
  createAlertRule: vi.fn(),
  updateAlertRule: vi.fn(),
  notifySuccess: vi.fn(),
  notifyConnectError: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    listNotificationChannels: mocks.listNotificationChannels,
    createAlertRule: mocks.createAlertRule,
    updateAlertRule: mocks.updateAlertRule,
  }),
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyConnectError: mocks.notifyConnectError,
}))

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  props: ['modelValue', 'options', 'label', 'displayValue'],
  emits: ['update:modelValue'],
  template: '<div class="q-select-stub" :data-label="label" />',
})

const stubs = {
  'q-dialog': { props: ['modelValue'], template: '<div><slot v-if="modelValue" /></div>' },
  'q-card': { template: '<div><slot /></div>' },
  'q-card-section': { template: '<div><slot /></div>' },
  'q-card-actions': { template: '<div><slot /></div>' },
  'q-banner': { template: '<div class="q-banner-stub"><slot /></div>' },
  'q-expansion-item': { template: '<section><slot /></section>' },
  'q-input': { template: '<input />' },
  'q-checkbox': true,
  'q-toggle': { template: '<div />' },
  'q-select': QSelectStub,
  'q-btn': {
    props: ['label', 'disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
  },
  'router-link': {
    props: ['to'],
    template: '<a :data-to="JSON.stringify(to)"><slot /></a>',
  },
}

type RuleRequest = {
  id?: string
  serverId?: string
  serverNodeId?: string
  nodeId?: string
  eventType: AlertEventType
  notificationChannelId: string
  condition: string
  enabled: boolean
}

type DialogVM = {
  form: {
    eventType: AlertEventType
    operator: string
    threshold: number
    forSeconds: number
    recoveryValue: number | null
    cooldownSeconds: number
    repeatSeconds: number
    statusOnline: boolean
    statusOffline: boolean
  }
  canSave: boolean
  save: () => Promise<void>
  saving: boolean
  thresholdOperators: { label: string; value: string }[]
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

function mountDialog(props: Record<string, unknown> = {}, superUser = true) {
  useUserAuthStore().user = create(UserSchema, { id: 'user-1', userName: 'owner', superUser })
  return mount(AlertRuleDialog, {
    props: { modelValue: true, 'onUpdate:modelValue': vi.fn(), ...props },
    global: { stubs },
  })
}

const serverScope = { serverId: 'server-1', serverNodeId: 'node-1' }
const nodeScope = { nodeId: 'node-1', nodeName: 'Rack A' }

function select(wrapper: ReturnType<typeof mountDialog>, label: string) {
  const found = wrapper
    .findAllComponents(QSelectStub)
    .find((candidate) => candidate.props('label') === label)
  if (!found) throw new Error(`select ${label} not found`)
  return found
}

function offeredEventTypes(wrapper: ReturnType<typeof mountDialog>): AlertEventType[] {
  const options = select(wrapper, 'Event Type').props('options') as { value: AlertEventType }[]
  return options.map((option) => option.value)
}

function button(wrapper: ReturnType<typeof mountDialog>, label: string) {
  const found = wrapper.findAll('button').find((candidate) => candidate.text() === label)
  if (!found) throw new Error(`${label} button not found`)
  return found
}

function lastRequest(mock: ReturnType<typeof vi.fn>): RuleRequest {
  return mock.mock.calls.at(-1)?.[0] as RuleRequest
}

const nodeEventTypes = [
  AlertEventType.NODE_CPU_THRESHOLD,
  AlertEventType.NODE_MEMORY_THRESHOLD,
  AlertEventType.NODE_DISK_THRESHOLD,
]

describe('AlertRuleDialog', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.listNotificationChannels.mockResolvedValue({ channels: [{ id: 'chan-1', name: 'Ops' }] })
  })

  afterEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
  })

  describe('node scope', () => {
    it('offers only node event types and creates a rule for the node', async () => {
      mocks.createAlertRule.mockResolvedValue({})
      const wrapper = mountDialog(nodeScope)
      await flushPromises()

      expect(offeredEventTypes(wrapper)).toEqual(nodeEventTypes)

      await button(wrapper, 'Create').trigger('click')
      await flushPromises()

      const request = lastRequest(mocks.createAlertRule)
      expect(request).toMatchObject({
        nodeId: 'node-1',
        eventType: AlertEventType.NODE_CPU_THRESHOLD,
        notificationChannelId: 'chan-1',
        enabled: true,
      })
      expect(request.serverId).toBeUndefined()
      expect(JSON.parse(request.condition)).toEqual({ operator: '>=', value: 85 })
      expect(mocks.notifySuccess).toHaveBeenCalledWith('Alert rule created')
      expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
      expect(wrapper.emitted('saved')).toHaveLength(1)
    })

    it('starts each resource at its high band', async () => {
      mocks.createAlertRule.mockResolvedValue({})
      const wrapper = mountDialog(nodeScope)
      await flushPromises()

      select(wrapper, 'Event Type').vm.$emit(
        'update:modelValue',
        AlertEventType.NODE_DISK_THRESHOLD,
      )
      await button(wrapper, 'Create').trigger('click')
      await flushPromises()

      const request = lastRequest(mocks.createAlertRule)
      expect(request.eventType).toBe(AlertEventType.NODE_DISK_THRESHOLD)
      expect(JSON.parse(request.condition)).toEqual({ operator: '>=', value: 80 })
    })

    it('names the node and links to the Alert Rules tab', async () => {
      const wrapper = mountDialog(nodeScope)
      await flushPromises()

      expect(wrapper.text()).toContain('Alerts for Rack A')
      const link = wrapper.findAll('a').find((a) => a.text() === 'Notifications → Alert Rules')
      expect(JSON.parse(link?.attributes('data-to') ?? '{}')).toEqual({
        path: '/notifications',
        query: { tab: 'rules' },
      })
    })

    it('offers nothing and cannot save for a non-superuser', async () => {
      const wrapper = mountDialog(nodeScope, false)
      await flushPromises()

      expect(offeredEventTypes(wrapper)).toEqual([])
      expect(button(wrapper, 'Create').attributes('disabled')).toBeDefined()
    })
  })

  describe('server scope', () => {
    it('offers only server event types and creates a rule for the server', async () => {
      mocks.createAlertRule.mockResolvedValue({})
      const wrapper = mountDialog(serverScope)
      await flushPromises()

      expect(offeredEventTypes(wrapper)).toEqual([
        AlertEventType.CRASH,
        AlertEventType.STATUS_CHANGE,
        AlertEventType.CPU_THRESHOLD,
        AlertEventType.MEMORY_THRESHOLD,
        AlertEventType.DISK_THRESHOLD,
        AlertEventType.PLAYER_COUNT_THRESHOLD,
      ])
      expect(wrapper.text()).not.toContain('Alerts for')

      await button(wrapper, 'Create').trigger('click')
      await flushPromises()

      expect(lastRequest(mocks.createAlertRule)).toMatchObject({
        serverId: 'server-1',
        serverNodeId: 'node-1',
        eventType: AlertEventType.CRASH,
        condition: '',
      })
      expect(lastRequest(mocks.createAlertRule).nodeId).toBeUndefined()
    })

    it('serializes meaningful advanced threshold behavior fields', async () => {
      mocks.createAlertRule.mockResolvedValue({})
      const wrapper = mountDialog(serverScope)
      await flushPromises()

      const vm = wrapper.vm as unknown as DialogVM
      Object.assign(vm.form, {
        eventType: AlertEventType.MEMORY_THRESHOLD,
        operator: '>=',
        threshold: 85,
        forSeconds: 120,
        recoveryValue: 75,
        cooldownSeconds: 300,
        repeatSeconds: 900,
      })
      await vm.save()

      expect(JSON.parse(lastRequest(mocks.createAlertRule).condition)).toEqual({
        operator: '>=',
        value: 85,
        for_seconds: 120,
        recovery_value: 75,
        cooldown_seconds: 300,
        repeat_seconds: 900,
      })
    })

    it('requires at least one status for a status change rule', async () => {
      const wrapper = mountDialog(serverScope)
      await flushPromises()

      const vm = wrapper.vm as unknown as DialogVM
      vm.form.eventType = AlertEventType.STATUS_CHANGE
      vm.form.statusOnline = false
      vm.form.statusOffline = false
      await flushPromises()

      expect(vm.canSave).toBe(false)
      expect(wrapper.text()).toContain('Select at least one status')

      vm.form.statusOffline = true
      expect(vm.canSave).toBe(true)
    })

    it.each([
      { name: 'recovery above a >= trigger', operator: '>=', recoveryValue: 85 },
      { name: 'recovery on an equality rule', operator: '==', recoveryValue: 70 },
      { name: 'negative sustain', operator: '>=', recoveryValue: null, forSeconds: -1 },
    ])('does not save $name', async ({ operator, recoveryValue, forSeconds = 0 }) => {
      const wrapper = mountDialog(serverScope)
      await flushPromises()

      const vm = wrapper.vm as unknown as DialogVM
      Object.assign(vm.form, {
        eventType: AlertEventType.CPU_THRESHOLD,
        operator,
        threshold: 80,
        recoveryValue,
        forSeconds,
      })

      expect(vm.canSave).toBe(false)
      await vm.save()
      expect(mocks.createAlertRule).not.toHaveBeenCalled()
    })

    it('stores = as the == operator', () => {
      const vm = mountDialog(serverScope).vm as unknown as DialogVM
      expect(vm.thresholdOperators.find((operator) => operator.label === '=')?.value).toBe('==')
    })

    it('keeps the dialog open when saving fails and ignores a second click', async () => {
      let rejectCreate: (reason: unknown) => void = () => undefined
      mocks.createAlertRule.mockReturnValueOnce(
        new Promise((_resolve, reject) => {
          rejectCreate = reject
        }),
      )
      const wrapper = mountDialog(serverScope)
      await flushPromises()

      const vm = wrapper.vm as unknown as DialogVM
      const firstSave = vm.save()
      await vm.save()
      expect(vm.saving).toBe(true)
      rejectCreate(new Error('channel rejected'))
      await firstSave
      await flushPromises()

      expect(mocks.createAlertRule).toHaveBeenCalledTimes(1)
      expect(mocks.notifyConnectError).toHaveBeenCalledTimes(1)
      expect(mocks.notifySuccess).not.toHaveBeenCalled()
      expect(vm.saving).toBe(false)
      expect(wrapper.emitted('update:modelValue')).toBeUndefined()
      expect(wrapper.emitted('saved')).toBeUndefined()
    })
  })

  describe('editing', () => {
    it('keeps the rule target and preserves extended threshold behavior', async () => {
      mocks.updateAlertRule.mockResolvedValue({})
      const condition = {
        operator: '>=',
        value: 85,
        for_seconds: 120,
        recovery_value: 75,
        cooldown_seconds: 300,
        repeat_seconds: 900,
        no_data_seconds: 180,
      }
      const wrapper = mountDialog({
        rule: makeRule({
          eventType: AlertEventType.MEMORY_THRESHOLD,
          condition: JSON.stringify(condition),
        }),
      })
      await flushPromises()

      expect((wrapper.vm as unknown as DialogVM).form).toMatchObject({
        threshold: 85,
        forSeconds: 120,
        recoveryValue: 75,
        cooldownSeconds: 300,
        repeatSeconds: 900,
      })

      await button(wrapper, 'Save').trigger('click')
      await flushPromises()

      const request = lastRequest(mocks.updateAlertRule)
      expect(request).toMatchObject({ id: 'rule-1', serverId: 'server-1', serverNodeId: 'node-1' })
      expect(JSON.parse(request.condition)).toEqual(condition)
      expect(mocks.notifySuccess).toHaveBeenCalledWith('Alert rule updated')
    })

    it('keeps legacy threshold rules operator/value-only', async () => {
      mocks.updateAlertRule.mockResolvedValue({})
      const wrapper = mountDialog({
        rule: makeRule({ condition: JSON.stringify({ operator: '>=', value: 90 }) }),
      })
      await flushPromises()

      await (wrapper.vm as unknown as DialogVM).save()

      expect(JSON.parse(lastRequest(mocks.updateAlertRule).condition)).toEqual({
        operator: '>=',
        value: 90,
      })
    })

    it.each([
      {
        name: 'a node rule offers node types',
        rule: { serverId: undefined, serverNodeId: undefined, nodeId: 'node-1' },
        eventType: AlertEventType.NODE_DISK_THRESHOLD,
        want: (offered: AlertEventType[]) => expect(offered).toEqual(nodeEventTypes),
      },
      {
        name: 'a server rule offers no node types',
        rule: {},
        eventType: AlertEventType.CRASH,
        want: (offered: AlertEventType[]) => {
          expect(offered).not.toContain(AlertEventType.NODE_CPU_THRESHOLD)
          expect(offered).toContain(AlertEventType.PLAYER_COUNT_THRESHOLD)
        },
      },
    ])('stays in scope: $name', async ({ rule, eventType, want }) => {
      const wrapper = mountDialog({ rule: makeRule({ ...rule, eventType }) })
      await flushPromises()
      want(offeredEventTypes(wrapper))
    })
  })

  it('reloads channels on every opening instead of showing the last list', async () => {
    mocks.listNotificationChannels.mockResolvedValueOnce({ channels: [] })
    const wrapper = mountDialog(serverScope)
    await flushPromises()
    expect(wrapper.text()).toContain('No notification channels configured.')

    await wrapper.setProps({ modelValue: false })
    let resolveChannels: (value: unknown) => void = () => undefined
    mocks.listNotificationChannels.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveChannels = resolve
      }),
    )
    await wrapper.setProps({ modelValue: true })

    // While the new list loads, the old "no channels" state is gone.
    expect(wrapper.text()).not.toContain('No notification channels configured.')
    expect(select(wrapper, 'Notification Channel').props('options')).toEqual([])

    resolveChannels({ channels: [{ id: 'chan-2', name: 'New' }] })
    await flushPromises()
    expect(select(wrapper, 'Notification Channel').props('modelValue')).toBe('chan-2')
    expect(mocks.listNotificationChannels).toHaveBeenCalledTimes(2)
  })

  it('shows the channel name, never its ID, while editing a rule', async () => {
    let resolveChannels: (value: unknown) => void = () => undefined
    mocks.listNotificationChannels.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveChannels = resolve
      }),
    )
    const wrapper = mountDialog({ rule: makeRule() })
    const channelSelect = () => select(wrapper, 'Notification Channel')

    expect(channelSelect().props('modelValue')).toBe('chan-1')
    expect(channelSelect().props('displayValue')).toBe('Loading channels…')

    resolveChannels({ channels: [{ id: 'chan-1', name: 'Ops' }] })
    await flushPromises()
    expect(channelSelect().props('displayValue')).toBe('Ops')
  })
})
