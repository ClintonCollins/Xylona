import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'

import ScheduledTaskForm from './ScheduledTaskForm.vue'
import { ScheduledTaskSchema } from '@/proto/shared_pb'

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
    }),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    createScheduledTask: vi.fn(),
    updateScheduledTask: vi.fn(),
  }),
  ConnectErrorToString: (err: unknown) => String(err),
}))

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  props: {
    label: { type: String, default: '' },
    options: { type: Array, default: () => [] },
  },
  emits: ['update:modelValue', 'filter'],
  template: `<div class="q-select-stub">
    <div v-if="label === 'Task Type'" class="task-type-options">
      <span
        v-for="option in options"
        :key="typeof option === 'object' && option !== null && 'value' in option ? String(option.value) : String(option)">
        {{
          typeof option === 'object' && option !== null && 'label' in option
            ? String(option.label)
            : String(option)
        }}
      </span>
    </div>
  </div>`,
})

describe('ScheduledTaskForm', () => {
  it('includes backup in task type options', () => {
    const wrapper = mount(ScheduledTaskForm, {
      props: {
        showDialog: true,
        gameServerId: 'server-1',
      },
      global: {
        stubs: {
          'q-btn': { template: '<button><slot /></button>' },
          'q-card': { template: '<div><slot /></div>' },
          'q-card-actions': { template: '<div><slot /></div>' },
          'q-card-section': { template: '<div><slot /></div>' },
          'q-dialog': { template: '<div><slot /></div>' },
          'q-icon': { template: '<i />' },
          'q-input': { template: '<input />' },
          'q-select': QSelectStub,
          'q-toggle': { template: '<input type="checkbox" />' },
        },
      },
    })

    expect(wrapper.find('.task-type-options').text()).toContain('Backup Server')
  })

  it('requires an unsupported existing backup schedule to be disabled or converted', async () => {
    const wrapper = mount(ScheduledTaskForm, {
      props: {
        showDialog: false,
        gameServerId: 'server-1',
        backupOperationsAllowed: false,
        backupDisabledReason: 'Backups are not supported on this platform.',
        existingTask: create(ScheduledTaskSchema, {
          id: 'task-1',
          gameServerId: 'server-1',
          name: 'Nightly backup',
          taskType: 'backup',
          cronExpression: '0 2 * * *',
          timezone: 'UTC',
          enabled: true,
        }),
      },
      global: {
        stubs: {
          'q-banner': { template: '<div v-bind="$attrs"><slot /></div>' },
          'q-btn': { template: '<button><slot /></button>' },
          'q-card': { template: '<div><slot /></div>' },
          'q-card-actions': { template: '<div><slot /></div>' },
          'q-card-section': { template: '<div><slot /></div>' },
          'q-dialog': { template: '<div><slot /></div>' },
          'q-icon': { template: '<i />' },
          'q-input': { template: '<input />' },
          'q-select': QSelectStub,
          'q-toggle': { template: '<input type="checkbox" />' },
        },
      },
    })

    await wrapper.setProps({ showDialog: true })

    expect(wrapper.get('[data-testid="backup-schedule-unsupported"]').text()).toContain(
      'Backups are not supported on this platform.',
    )
  })

  describe('new schedules', () => {
    type FormVM = {
      form: { name: string; taskType: string; timezone: string; cronExpression: string }
      builder: { frequency: string; weekdays: number[] }
      useAdvancedCron: boolean
      isFormValid: boolean
      toggleAdvancedCron: () => void
    }

    async function openNewForm(initialTaskType?: string) {
      const wrapper = mount(ScheduledTaskForm, {
        props: {
          showDialog: false,
          gameServerId: 'server-1',
          initialTaskType,
          backupOperationsAllowed: true,
        },
        global: {
          stubs: {
            'q-banner': { template: '<div><slot /></div>' },
            'q-btn': { template: '<button><slot /></button>' },
            'q-card': { template: '<div><slot /></div>' },
            'q-card-actions': { template: '<div><slot /></div>' },
            'q-card-section': { template: '<div><slot /></div>' },
            'q-dialog': { template: '<div><slot /></div>' },
            'q-icon': { template: '<i />' },
            'q-input': { template: '<input />' },
            'q-select': QSelectStub,
            'q-toggle': { template: '<input type="checkbox" />' },
          },
        },
      })
      await wrapper.setProps({ showDialog: true })
      return { wrapper, vm: wrapper.vm as unknown as FormVM }
    }

    const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone

    it('default to daily at 03:00 in the browser zone and preview the next run', async () => {
      const { wrapper, vm } = await openNewForm()

      expect(vm.form.taskType).toBe('restart')
      expect(vm.form.timezone).toBe(browserZone)
      expect(vm.builder.frequency).toBe('daily')
      const preview = wrapper.get('[data-testid="schedule-preview"]').text()
      expect(preview).toContain('At 03:00')
      expect(preview).toContain(`(${browserZone})`)
      expect(preview).toMatch(/Next run \w{3} \d+, \d{4} 03:00:00 \S+/)
    })

    it('start on the requested task type', async () => {
      const { vm } = await openNewForm('backup')

      expect(vm.form.taskType).toBe('backup')
    })

    it('block saving an invalid cron expression', async () => {
      const { wrapper, vm } = await openNewForm()
      vm.form.name = 'Broken'
      vm.toggleAdvancedCron()
      vm.form.cronExpression = '99 3 * * *'
      await wrapper.vm.$nextTick()

      expect(wrapper.get('[data-testid="schedule-preview"]').text()).toBe('Invalid cron expression')
      expect(vm.isFormValid).toBe(false)
    })

    it('report a weekly schedule without days in the preview', async () => {
      const { wrapper, vm } = await openNewForm()
      vm.form.name = 'Weekly'
      vm.builder.frequency = 'weekly'
      await wrapper.vm.$nextTick()

      expect(wrapper.get('[data-testid="schedule-preview"]').text()).toBe('Select at least one day')
      expect(vm.isFormValid).toBe(false)
    })

    it('reopen in the visual builder after the advanced editor was used', async () => {
      const { wrapper, vm } = await openNewForm()
      vm.toggleAdvancedCron()
      vm.form.cronExpression = '*/7 * * * *'
      await wrapper.setProps({ showDialog: false })
      await wrapper.setProps({ showDialog: true })

      expect(vm.useAdvancedCron).toBe(false)
      expect(vm.builder.frequency).toBe('daily')
    })
  })
})
