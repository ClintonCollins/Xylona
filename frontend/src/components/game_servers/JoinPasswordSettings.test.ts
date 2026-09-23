import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import JoinPasswordSettings from './JoinPasswordSettings.vue'

const calls = vi.hoisted(() => ({
  getJoinPasswordState: vi.fn(),
  setJoinPassword: vi.fn(),
  clearJoinPassword: vi.fn(),
  dialog: vi.fn(),
}))
vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => calls,
  ConnectErrorToString: () => 'Password was rejected',
}))
vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return { ...actual, useQuasar: () => ({ dialog: calls.dialog }) }
})
beforeEach(() => {
  vi.resetAllMocks()
  calls.getJoinPasswordState.mockResolvedValue({
    state: { supported: true, configured: true, validationIssues: [] },
  })
})

it('stages a replacement for the settings Save, keeps it after failure, and confirms disabling', async () => {
  const confirmations: Array<() => void> = []
  calls.dialog.mockImplementation(() => ({
    onOk(handler: () => void) {
      confirmations.push(handler)
      return this
    },
  }))
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
        'q-badge': { props: ['label'], template: '<span>{{ label }}</span>' },
        'q-banner': { template: '<div><slot /><slot name="action" /></div>' },
        'q-icon': true,
      },
    },
  })
  await flushPromises()
  const settings = wrapper.vm as unknown as { dirty: boolean; save: () => Promise<void> }
  expect(wrapper.text()).toContain('Protected')
  expect(wrapper.get('input').element.value).toBe('')
  expect(settings.dirty).toBe(false)

  await wrapper.get('input').setValue('synthetic-replacement')
  expect(settings.dirty).toBe(true)
  expect(calls.setJoinPassword).not.toHaveBeenCalled()

  calls.setJoinPassword.mockRejectedValueOnce(new Error('rejected'))
  await expect(settings.save()).rejects.toThrow('rejected')
  await flushPromises()
  expect(wrapper.get('input').element.value).toBe('synthetic-replacement')
  expect(wrapper.text()).toContain('Password was rejected')
  expect(settings.dirty).toBe(true)

  calls.setJoinPassword.mockResolvedValueOnce({
    state: { supported: true, configured: true, validationIssues: [] },
  })
  await settings.save()
  await flushPromises()
  expect(wrapper.get('input').element.value).toBe('')
  expect(settings.dirty).toBe(false)
  expect(wrapper.text()).toContain('Applies on next start')
  expect(wrapper.text()).not.toContain('synthetic-replacement')
  expect(calls.setJoinPassword.mock.calls[0]?.[0]).toMatchObject({
    serverId: 'server-1',
    password: 'synthetic-replacement',
  })

  calls.clearJoinPassword.mockResolvedValueOnce({
    state: { supported: true, configured: false, validationIssues: [] },
  })
  const disable = wrapper.findAll('button').find((b) => b.text() === 'Disable password protection')
  if (!disable) throw new Error('Missing Disable password protection button')
  await disable.trigger('click')
  expect(calls.dialog).toHaveBeenCalledWith(
    expect.objectContaining({ title: 'Disable password protection?' }),
  )
  expect(calls.clearJoinPassword).not.toHaveBeenCalled()
  confirmations[0]?.()
  await flushPromises()
  expect(calls.clearJoinPassword).toHaveBeenCalledOnce()
  expect(calls.clearJoinPassword.mock.calls[0]?.[0]).toMatchObject({ serverId: 'server-1' })
  expect(wrapper.text()).toContain('Password protection disabled. Applies on next start.')
  expect(wrapper.text()).toContain('Open')
  wrapper.unmount()
})
