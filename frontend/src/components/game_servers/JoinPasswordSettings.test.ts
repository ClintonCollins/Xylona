import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import JoinPasswordSettings from './JoinPasswordSettings.vue'

const calls = vi.hoisted(() => ({
  getJoinPasswordState: vi.fn(),
  setJoinPassword: vi.fn(),
  clearJoinPassword: vi.fn(),
}))
vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => calls,
  ConnectErrorToString: () => 'Password was rejected',
}))
beforeEach(() => {
  vi.resetAllMocks()
  calls.getJoinPasswordState.mockResolvedValue({
    state: { supported: true, configured: true, validationIssues: [] },
  })
})

it('keeps replacement input local after failure and clears it after successful save', async () => {
  const wrapper = mount(JoinPasswordSettings, {
    props: { serverId: 'server-1', canEdit: true },
    global: {
      stubs: {
        'q-input': {
          props: ['modelValue'],
          emits: ['update:modelValue'],
          template:
            '<input type="password" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
        },
        'q-btn': { props: ['label'], template: '<button>{{ label }}</button>' },
      },
    },
  })
  await flushPromises()
  expect(wrapper.get('input').element.value).toBe('')
  await wrapper.get('input').setValue('synthetic-replacement')
  const click = async (label: string) => {
    const button = wrapper.findAll('button').find((button) => button.text() === label)
    if (!button) throw new Error(`Missing button: ${label}`)
    await button.trigger('click')
    await flushPromises()
  }
  calls.setJoinPassword.mockRejectedValueOnce(new Error('rejected'))
  await click('Replace password')
  expect(wrapper.get('input').element.value).toBe('synthetic-replacement')
  expect(wrapper.text()).toContain('Password was rejected')
  calls.setJoinPassword.mockResolvedValueOnce({
    state: { supported: true, configured: true, validationIssues: [] },
  })
  await click('Replace password')
  expect(wrapper.get('input').element.value).toBe('')
  expect(wrapper.text()).toContain('Applies on next start')
  expect(wrapper.text()).not.toContain('synthetic-replacement')
  expect(calls.setJoinPassword.mock.calls[0]?.[0]).toMatchObject({
    serverId: 'server-1',
    password: 'synthetic-replacement',
  })
  calls.clearJoinPassword.mockResolvedValueOnce({
    state: { supported: true, configured: false, validationIssues: [] },
  })
  await click('Disable password protection')
  expect(calls.clearJoinPassword).toHaveBeenCalledOnce()
  expect(calls.clearJoinPassword.mock.calls[0]?.[0]).toMatchObject({ serverId: 'server-1' })
  expect(wrapper.text()).toContain('Password protection disabled. Applies on next start.')
  expect(wrapper.text()).not.toContain('blocked')
  wrapper.unmount()
})
