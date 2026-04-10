import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import StartArgsEditor from './StartArgsEditor.vue'
import type { StartArgBlock, StartArgPatch } from './start-args'

const template: StartArgBlock[] = [
  {
    id: 'system-arg',
    order: 0,
    ownership: 'system',
    label: 'Server executable',
    tokens: ['-jar', '{{SERVER_EXECUTABLE}}'],
  },
  {
    id: 'locked-arg',
    order: 1,
    ownership: 'locked',
    label: 'Security flag',
    tokens: ['-Dlog4j2.formatMsgNoLookups=true'],
  },
  {
    id: 'editable-arg',
    order: 2,
    ownership: 'editable',
    label: 'Max heap',
    tokens: ['-Xmx2G'],
  },
]

function createWrapper(patches: StartArgPatch[] = [], allowEditing = true) {
  return mount(StartArgsEditor, {
    props: {
      allowEditing,
      baseCommand: 'java',
      blocklist: [],
      patches,
      template,
    },
    global: {
      stubs: {
        SimilarArgDialog: {
          template: '<div data-testid="similar-dialog-stub"></div>',
        },
        'q-dialog': {
          props: ['modelValue'],
          template: '<div v-if="modelValue"><slot /></div>',
        },
        'q-card': { template: '<div><slot /></div>' },
        'q-card-section': { template: '<div><slot /></div>' },
        'q-card-actions': { template: '<div><slot /></div>' },
        'q-banner': { template: '<div><slot /></div>' },
        'q-icon': true,
        'q-badge': {
          props: ['label'],
          template: '<span><slot />{{ label }}</span>',
        },
        'q-btn': {
          props: ['label'],
          emits: ['click'],
          template: `<button v-bind="$attrs" @click="$emit('click')"><slot />{{ label }}</button>`,
        },
        'q-input': {
          props: ['modelValue', 'type', 'label'],
          emits: ['update:modelValue'],
          template: `
            <label>
              {{ label }}
              <textarea
                v-if="type === 'textarea'"
                :value="modelValue"
                @input="$emit('update:modelValue', $event.target.value)"
              />
              <input
                v-else
                :value="modelValue"
                @input="$emit('update:modelValue', $event.target.value)"
              />
            </label>
          `,
        },
      },
    },
  })
}

describe('StartArgsEditor', () => {
  it('renders provenance badges and base command header', () => {
    const wrapper = createWrapper()

    expect(wrapper.text()).toContain('Base command')
    expect(wrapper.text()).toContain('java')
    expect(wrapper.text()).toContain('System')
    expect(wrapper.text()).toContain('Locked')
    expect(wrapper.text()).toContain('Default')
  })

  it('shows edit and remove controls for editable defaults only', () => {
    const wrapper = createWrapper()

    expect(wrapper.find('[data-testid="edit-editable-arg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remove-editable-arg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="edit-system-arg"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="remove-system-arg"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="edit-locked-arg"]').exists()).toBe(false)
  })

  it('shows edited provenance, reset controls, and previous value note', () => {
    const wrapper = createWrapper([
      {
        id: 'editable-arg',
        op: 'edit',
        tokens: ['-Xmx4G'],
        afterId: null,
      },
    ])

    expect(wrapper.text()).toContain('Edited')
    expect(wrapper.text()).toContain('was -Xmx2G')
    expect(wrapper.find('[data-testid="reset-editable-arg"]').exists()).toBe(true)
  })

  it('shows move controls for user-added rows', () => {
    const wrapper = createWrapper([
      {
        id: 'added-arg',
        op: 'add',
        tokens: ['-Dcustom=true'],
        afterId: 'editable-arg',
      },
    ])

    expect(wrapper.find('[data-testid="edit-added-arg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="remove-added-arg"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="move-up-added-arg"]').exists()).toBe(true)
  })

  it('hides editing actions when allowEditing is false', () => {
    const wrapper = createWrapper([], false)

    expect(wrapper.find('[data-testid="add-arg-button"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="edit-editable-arg"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Start command editing is disabled')
  })

  it('emits updated patches when adding a new argument', async () => {
    const wrapper = createWrapper()

    await wrapper.get('[data-testid="add-arg-button"]').trigger('click')
    const textareas = wrapper.findAll('textarea')
    const tokensTextarea = textareas[0]
    if (!tokensTextarea) {
      throw new Error('expected tokens textarea to exist')
    }
    await tokensTextarea.setValue('-Dmy.flag=true')
    await wrapper.get('[data-testid="save-arg-button"]').trigger('click')

    const emitted = wrapper.emitted('update:patches')
    expect(emitted).toBeTruthy()
    expect(emitted?.at(-1)?.[0]).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          op: 'add',
          tokens: ['-Dmy.flag=true'],
        }),
      ]),
    )
  })
})
