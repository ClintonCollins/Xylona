import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import type { MountingOptions } from '@vue/test-utils'
import { UpdateStep, StepStatus } from '@/proto/xylona_pb'
import type { OperationContextFact, StepState } from './UpdateProgressPanel.types'
import UpdateProgressPanel from './UpdateProgressPanel.vue'

type TestStubs = Exclude<
  NonNullable<NonNullable<MountingOptions<Record<string, never>>['global']>['stubs']>,
  string[]
>

const QUASAR_STUBS = {
  'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
  'q-spinner': { props: ['color', 'size'], template: '<span class="q-spinner-stub" />' },
} satisfies TestStubs

function mountPanel(
  steps: StepState[],
  options?: {
    contextFacts?: OperationContextFact[]
    outputLines?: string[]
    showNavigateAwayMessage?: boolean
  },
) {
  return mount(UpdateProgressPanel, {
    props: {
      steps,
      contextFacts: options?.contextFacts,
      outputLines: options?.outputLines,
      showNavigateAwayMessage: options?.showNavigateAwayMessage,
    },
    global: { stubs: QUASAR_STUBS },
  })
}

describe('UpdateProgressPanel', () => {
  it('renders a timeline with an active-event summary for short operations', () => {
    const wrapper = mountPanel([
      {
        step: UpdateStep.DOWNLOADING,
        label: 'Download update package',
        status: StepStatus.COMPLETED,
      },
      {
        step: UpdateStep.INSTALLING,
        label: 'Apply updated files',
        status: StepStatus.IN_PROGRESS,
        message: 'The update is being written into place now.',
      },
      {
        step: UpdateStep.RESTARTING,
        label: 'Publish updated state',
        status: StepStatus.PENDING,
      },
    ])

    expect(wrapper.find('.operation-timeline-panel').exists()).toBe(true)
    expect(wrapper.text()).toContain('Active event')
    expect(wrapper.text()).toContain('Apply updated files')
    expect(wrapper.text()).toContain('Next: Publish updated state')
    expect(wrapper.findAll('.operation-timeline-entry')).toHaveLength(3)
    expect(wrapper.find('.q-spinner-stub').exists()).toBe(true)
    expect(wrapper.find('.operation-timeline-icon--complete').exists()).toBe(true)
  })

  it('omits the context rail when no facts are provided', () => {
    const wrapper = mountPanel([
      {
        step: UpdateStep.INSTALLING,
        label: 'Apply updated files',
        status: StepStatus.IN_PROGRESS,
      },
    ])

    expect(wrapper.find('.operation-context-rail').exists()).toBe(false)
    expect(wrapper.find('.operation-timeline-desc--placeholder').exists()).toBe(true)
  })

  it('renders the optional context rail when facts are provided', () => {
    const wrapper = mountPanel(
      [
        {
          step: 'variant-apply',
          label: 'Apply selected variant',
          status: StepStatus.IN_PROGRESS,
        },
      ],
      {
        contextFacts: [
          { label: 'Current', value: 'Vanilla 1.21.1' },
          { label: 'Target', value: 'Paper 1.21.5-28' },
        ],
      },
    )

    expect(wrapper.find('.operation-context-rail').exists()).toBe(true)
    expect(wrapper.text()).toContain('Current')
    expect(wrapper.text()).toContain('Paper 1.21.5-28')
  })

  it('keeps operation output collapsed until requested', async () => {
    const wrapper = mountPanel(
      [
        {
          step: UpdateStep.INSTALLING,
          label: 'Apply updated files',
          status: StepStatus.IN_PROGRESS,
        },
      ],
      {
        outputLines: ['[Xylona] Downloading update', '[Xylona] Applying update'],
      },
    )

    expect(wrapper.find('.operation-output-toggle').exists()).toBe(true)
    expect(wrapper.find('.operation-output-lines').exists()).toBe(false)
    expect(wrapper.text()).toContain('2 lines available')

    await wrapper.get('.operation-output-toggle').trigger('click')

    expect(wrapper.find('.operation-output-lines').exists()).toBe(true)
    expect(wrapper.text()).toContain('[Xylona] Applying update')
  })

  it('omits the output section when there is no operation output', () => {
    const wrapper = mountPanel([
      {
        step: UpdateStep.INSTALLING,
        label: 'Apply updated files',
        status: StepStatus.IN_PROGRESS,
      },
    ])

    expect(wrapper.find('.operation-output-toggle').exists()).toBe(false)
  })

  it('can reserve output space before any lines arrive', () => {
    const wrapper = mount(UpdateProgressPanel, {
      props: {
        steps: [
          {
            step: UpdateStep.INSTALLING,
            label: 'Apply updated files',
            status: StepStatus.IN_PROGRESS,
          },
        ],
        showOutputArea: true,
      },
      global: { stubs: QUASAR_STUBS },
    })

    expect(wrapper.find('.operation-output-toggle').exists()).toBe(true)
    expect(wrapper.text()).toContain('No operation output yet')
  })

  it('keeps the top status band visible after the operation completes', () => {
    const wrapper = mountPanel([
      {
        step: UpdateStep.DOWNLOADING,
        label: 'Download update package',
        status: StepStatus.COMPLETED,
      },
      {
        step: UpdateStep.INSTALLING,
        label: 'Apply updated files',
        status: StepStatus.COMPLETED,
        message: 'Update installed',
      },
    ])

    expect(wrapper.find('.operation-active-band').exists()).toBe(true)
    expect(wrapper.text()).toContain('Operation complete')
    expect(wrapper.text()).toContain('Apply updated files')
    expect(wrapper.text()).toContain('You can close this dialog whenever you are ready')
  })

  it('can hide the navigate-away helper copy', () => {
    const wrapper = mountPanel(
      [
        {
          step: UpdateStep.INSTALLING,
          label: 'Apply updated files',
          status: StepStatus.IN_PROGRESS,
        },
      ],
      { showNavigateAwayMessage: false },
    )

    expect(wrapper.text()).not.toContain('safely navigate away')
  })
})
