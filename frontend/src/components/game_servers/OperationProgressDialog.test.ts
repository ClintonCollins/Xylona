import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { MountingOptions } from '@vue/test-utils'
import { UpdateStep, StepStatus } from '@/proto/xylona_pb'
import OperationProgressDialog from './OperationProgressDialog.vue'

type TestStubs = Exclude<
  NonNullable<NonNullable<MountingOptions<Record<string, never>>['global']>['stubs']>,
  string[]
>

const QUASAR_STUBS = {
  'q-dialog': {
    props: ['modelValue'],
    template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
  },
  'q-card': { template: '<div class="q-card-stub"><slot /></div>' },
  'q-card-section': { template: '<section><slot /></section>' },
  'q-card-actions': { template: '<div class="q-card-actions-stub"><slot /></div>' },
  'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
  'q-spinner': { props: ['color', 'size'], template: '<span class="q-spinner-stub" />' },
  'q-btn': {
    props: ['label'],
    emits: ['click'],
    template: '<button @click="$emit(\'click\')">{{ label }}</button>',
  },
} satisfies TestStubs

describe('OperationProgressDialog', () => {
  it('renders the timeline dialog with optional facts and output', () => {
    const wrapper = mount(OperationProgressDialog, {
      props: {
        modelValue: true,
        title: 'Changing Variant',
        subtitle: 'Xylona will apply the selected variant and refresh the detected version.',
        steps: [
          {
            step: 'variant-apply',
            label: 'Apply selected variant',
            status: StepStatus.IN_PROGRESS,
          },
        ],
        contextFacts: [
          { label: 'Current', value: 'Vanilla 1.21.1' },
          { label: 'Target', value: 'Paper 1.21.5-28' },
        ],
        outputLines: ['[Xylona] Downloading Paper build'],
      },
      global: { stubs: QUASAR_STUBS },
    })

    expect(wrapper.find('.q-dialog-stub').exists()).toBe(true)
    expect(wrapper.text()).toContain('Changing Variant')
    expect(wrapper.text()).toContain('Apply selected variant')
    expect(wrapper.text()).toContain('Current')
    expect(wrapper.text()).toContain('1 line available')
    expect(wrapper.text()).toContain('Hide')
  })

  it('emits close when the hide action is clicked', async () => {
    const wrapper = mount(OperationProgressDialog, {
      props: {
        modelValue: true,
        title: 'Updating Server',
        steps: [
          {
            step: UpdateStep.INSTALLING,
            label: 'Apply update',
            status: StepStatus.IN_PROGRESS,
          },
        ],
      },
      global: { stubs: QUASAR_STUBS },
    })

    await wrapper.get('button').trigger('click')

    expect(wrapper.emitted('update:modelValue')).toEqual([[false]])
  })

  it('switches to a close action for completed operations', () => {
    const wrapper = mount(OperationProgressDialog, {
      props: {
        modelValue: true,
        title: 'Update complete',
        complete: true,
        steps: [
          {
            step: UpdateStep.INSTALLING,
            label: 'Apply update',
            status: StepStatus.COMPLETED,
          },
        ],
      },
      global: { stubs: QUASAR_STUBS },
    })

    expect(wrapper.text()).toContain('Close')
    expect(wrapper.text()).not.toContain('Hide')
  })

  it('can reserve output space even before output exists', () => {
    const wrapper = mount(OperationProgressDialog, {
      props: {
        modelValue: true,
        title: 'Updating Server',
        showOutputArea: true,
        steps: [
          {
            step: UpdateStep.INSTALLING,
            label: 'Apply update',
            status: StepStatus.IN_PROGRESS,
          },
        ],
      },
      global: { stubs: QUASAR_STUBS },
    })

    expect(wrapper.text()).toContain('No operation output yet')
  })
})
