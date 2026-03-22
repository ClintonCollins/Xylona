import { describe, it, expect } from 'vitest'
import { mount } from '@vue/test-utils'
import { UpdateStep, StepStatus } from '@/proto/xylona_pb'
import type { StepState } from './UpdateProgressPanel.types'
import UpdateProgressPanel from './UpdateProgressPanel.vue'

const QUASAR_STUBS = {
  'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
  'q-spinner': { props: ['color', 'size'], template: '<span class="q-spinner-stub" />' },
} as Record<string, unknown>

function makeSteps(overrides: Partial<Record<UpdateStep, StepStatus>> = {}): StepState[] {
  const defaults: Partial<Record<UpdateStep, StepStatus>> = {
    [UpdateStep.STOPPING]: StepStatus.PENDING,
    [UpdateStep.BACKING_UP]: StepStatus.PENDING,
    [UpdateStep.DOWNLOADING]: StepStatus.PENDING,
    [UpdateStep.INSTALLING]: StepStatus.PENDING,
    [UpdateStep.RESTARTING]: StepStatus.PENDING,
  }
  const merged = { ...defaults, ...overrides }
  return [
    { step: UpdateStep.STOPPING, status: merged[UpdateStep.STOPPING] ?? StepStatus.PENDING },
    { step: UpdateStep.BACKING_UP, status: merged[UpdateStep.BACKING_UP] ?? StepStatus.PENDING },
    { step: UpdateStep.DOWNLOADING, status: merged[UpdateStep.DOWNLOADING] ?? StepStatus.PENDING },
    { step: UpdateStep.INSTALLING, status: merged[UpdateStep.INSTALLING] ?? StepStatus.PENDING },
    { step: UpdateStep.RESTARTING, status: merged[UpdateStep.RESTARTING] ?? StepStatus.PENDING },
  ]
}

function mountPanel(steps: StepState[]) {
  return mount(UpdateProgressPanel, {
    props: { steps },
    global: { stubs: QUASAR_STUBS },
  })
}

describe('UpdateProgressPanel', () => {
  it('renders a step for each of the 5 main update steps', () => {
    const wrapper = mountPanel(makeSteps())
    expect(wrapper.find('.update-progress-panel').exists()).toBe(true)
    expect(wrapper.findAll('.update-step')).toHaveLength(5)
    expect(wrapper.text()).toContain('Stopping')
    expect(wrapper.text()).toContain('Backing Up')
    expect(wrapper.text()).toContain('Downloading')
    expect(wrapper.text()).toContain('Installing')
    expect(wrapper.text()).toContain('Restarting')
  })

  it('shows "safely navigate away" text', () => {
    const wrapper = mountPanel(makeSteps())
    expect(wrapper.text()).toContain('safely navigate away')
  })

  it('renders a spinner for in-progress steps', () => {
    const wrapper = mountPanel(makeSteps({ [UpdateStep.DOWNLOADING]: StepStatus.IN_PROGRESS }))
    const inProgressStep = wrapper
      .findAll('.update-step')
      .find((el) => el.text().includes('Downloading'))
    expect(inProgressStep).toBeDefined()
    if (inProgressStep) {
      expect(inProgressStep.find('.step-icon--in-progress').exists()).toBe(true)
    }
  })

  it('renders a checkmark for completed steps', () => {
    const wrapper = mountPanel(
      makeSteps({
        [UpdateStep.STOPPING]: StepStatus.COMPLETED,
        [UpdateStep.BACKING_UP]: StepStatus.COMPLETED,
      }),
    )
    const stoppingStep = wrapper
      .findAll('.update-step')
      .find((el) => el.text().includes('Stopping'))
    expect(stoppingStep).toBeDefined()
    if (stoppingStep) {
      expect(stoppingStep.find('.step-icon--completed').exists()).toBe(true)
    }

    const backingUpStep = wrapper
      .findAll('.update-step')
      .find((el) => el.text().includes('Backing Up'))
    expect(backingUpStep).toBeDefined()
    if (backingUpStep) {
      expect(backingUpStep.find('.step-icon--completed').exists()).toBe(true)
    }
  })

  it('renders an error icon for failed steps', () => {
    const wrapper = mountPanel(makeSteps({ [UpdateStep.INSTALLING]: StepStatus.FAILED }))
    const installingStep = wrapper
      .findAll('.update-step')
      .find((el) => el.text().includes('Installing'))
    expect(installingStep).toBeDefined()
    if (installingStep) {
      expect(installingStep.find('.step-icon--failed').exists()).toBe(true)
    }
  })

  it('renders pending steps with dimmed styling', () => {
    const wrapper = mountPanel(makeSteps())
    const allSteps = wrapper.findAll('.update-step')
    allSteps.forEach((step) => {
      expect(step.find('.step-icon--pending').exists()).toBe(true)
    })
  })
})
