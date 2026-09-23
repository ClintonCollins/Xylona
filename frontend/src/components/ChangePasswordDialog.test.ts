import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import ChangePasswordDialog from './ChangePasswordDialog.vue'

const mocks = vi.hoisted(() => ({
  changePassword: vi.fn(),
  notifySuccess: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ changePassword: mocks.changePassword }),
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
}))

const passthrough = { template: '<div><slot /></div>' }

function mountDialog() {
  return mount(ChangePasswordDialog, {
    props: { showDialog: true },
    global: {
      stubs: {
        'q-dialog': passthrough,
        'q-card': passthrough,
        'q-card-section': passthrough,
        'q-card-actions': passthrough,
        'q-banner': { template: '<div role="alert"><slot /></div>' },
        'q-icon': true,
        'q-form': {
          emits: ['submit'],
          template: '<form @submit.prevent="$emit(\'submit\', $event)"><slot /></form>',
        },
        'q-input': {
          props: ['modelValue', 'label', 'error', 'errorMessage'],
          emits: ['update:modelValue'],
          template:
            '<label>{{ label }}<input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" /><span v-if="error" class="field-error">{{ errorMessage }}</span></label>',
        },
        'q-btn': { props: ['label'], template: '<button>{{ label }}</button>' },
      },
    },
  })
}

async function submit(current: string, next: string) {
  const wrapper = mountDialog()
  const [currentInput, newInput, confirmInput] = wrapper.findAll('input')
  await currentInput?.setValue(current)
  await newInput?.setValue(next)
  await confirmInput?.setValue(next)
  await wrapper.find('form').trigger('submit')
  await flushPromises()
  return wrapper
}

describe('ChangePasswordDialog', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('changes the password, closes and confirms', async () => {
    mocks.changePassword.mockResolvedValueOnce({})

    const wrapper = await submit('old-secret', 'new-secret')

    expect(mocks.changePassword).toHaveBeenCalledWith(
      expect.objectContaining({ currentPassword: 'old-secret', newPassword: 'new-secret' }),
    )
    expect(wrapper.emitted('update:showDialog')).toEqual([[false]])
    expect(mocks.notifySuccess).toHaveBeenCalledOnce()
  })

  it('marks the current password field when it is wrong and stays open', async () => {
    mocks.changePassword.mockRejectedValueOnce(
      new ConnectError('current password is incorrect', Code.InvalidArgument),
    )

    const wrapper = await submit('wrong', 'new-secret')

    expect(wrapper.find('.field-error').text()).toBe('Current password is incorrect.')
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.emitted('update:showDialog')).toBeUndefined()
  })

  it('shows any other failure inline', async () => {
    mocks.changePassword.mockRejectedValueOnce(new ConnectError('boom', Code.Internal))

    const wrapper = await submit('old-secret', 'new-secret')

    expect(wrapper.find('[role="alert"]').text()).toBe('Password not changed: boom')
    expect(wrapper.emitted('update:showDialog')).toBeUndefined()
  })
})
