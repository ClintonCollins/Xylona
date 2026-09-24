import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { AlertEventType } from '@/proto/shared_pb'
import NodeAlertRuleDialog from './NodeAlertRuleDialog.vue'

const mocks = vi.hoisted(() => ({
  listNotificationChannels: vi.fn(),
  createAlertRule: vi.fn(),
  notifySuccess: vi.fn(),
  notifyConnectError: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    listNotificationChannels: mocks.listNotificationChannels,
    createAlertRule: mocks.createAlertRule,
  }),
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
  notifyConnectError: mocks.notifyConnectError,
}))

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  props: ['modelValue', 'options', 'label'],
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
  'q-toggle': { template: '<div />' },
  'q-select': QSelectStub,
  'q-btn': {
    props: ['label', 'disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}</button>',
  },
  'router-link': { template: '<a><slot /></a>' },
}

function mountDialog() {
  return mount(NodeAlertRuleDialog, {
    props: {
      modelValue: true,
      'onUpdate:modelValue': vi.fn(),
      nodeId: 'node-1',
      nodeName: 'Rack A',
    },
    global: { stubs },
  })
}

function select(wrapper: ReturnType<typeof mountDialog>, label: string) {
  const found = wrapper
    .findAllComponents(QSelectStub)
    .find((candidate) => candidate.props('label') === label)
  if (!found) throw new Error(`select ${label} not found`)
  return found
}

function createButton(wrapper: ReturnType<typeof mountDialog>) {
  const found = wrapper.findAll('button').find((candidate) => candidate.text() === 'Create')
  if (!found) throw new Error('Create button not found')
  return found
}

describe('NodeAlertRuleDialog', () => {
  afterEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
  })

  it('offers only node event types and creates a rule for the node', async () => {
    mocks.listNotificationChannels.mockResolvedValue({ channels: [{ id: 'chan-1', name: 'Ops' }] })
    mocks.createAlertRule.mockResolvedValue({})
    const wrapper = mountDialog()
    await flushPromises()

    const eventTypes = select(wrapper, 'Event Type').props('options') as { value: number }[]
    expect(eventTypes.map((option) => option.value)).toEqual([
      AlertEventType.NODE_CPU_THRESHOLD,
      AlertEventType.NODE_MEMORY_THRESHOLD,
      AlertEventType.NODE_DISK_THRESHOLD,
    ])

    await createButton(wrapper).trigger('click')
    await flushPromises()

    const request = mocks.createAlertRule.mock.calls[0]?.[0] as {
      nodeId: string
      serverId?: string
      eventType: AlertEventType
      notificationChannelId: string
      condition: string
      enabled: boolean
    }
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
  })

  it('starts each resource at its high band', async () => {
    mocks.listNotificationChannels.mockResolvedValue({ channels: [{ id: 'chan-1', name: 'Ops' }] })
    mocks.createAlertRule.mockResolvedValue({})
    const wrapper = mountDialog()
    await flushPromises()

    select(wrapper, 'Event Type').vm.$emit('update:modelValue', AlertEventType.NODE_DISK_THRESHOLD)
    await createButton(wrapper).trigger('click')
    await flushPromises()

    const request = mocks.createAlertRule.mock.calls[0]?.[0] as {
      eventType: AlertEventType
      condition: string
    }
    expect(request.eventType).toBe(AlertEventType.NODE_DISK_THRESHOLD)
    expect(JSON.parse(request.condition)).toEqual({ operator: '>=', value: 80 })
  })

  it('points to channel setup and blocks Create when there are no channels', async () => {
    mocks.listNotificationChannels.mockResolvedValue({ channels: [] })
    const wrapper = mountDialog()
    await flushPromises()

    expect(wrapper.text()).toContain('No notification channels configured.')
    expect(createButton(wrapper).attributes('disabled')).toBeDefined()
  })

  it('keeps the dialog open when the save fails', async () => {
    mocks.listNotificationChannels.mockResolvedValue({ channels: [{ id: 'chan-1', name: 'Ops' }] })
    mocks.createAlertRule.mockRejectedValue(new Error('boom'))
    const wrapper = mountDialog()
    await flushPromises()

    await createButton(wrapper).trigger('click')
    await flushPromises()

    expect(mocks.notifyConnectError).toHaveBeenCalled()
    expect(mocks.notifySuccess).not.toHaveBeenCalled()
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })
})
