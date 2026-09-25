import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'

import type { StartArgBlocklistEntry } from '@/components/game_servers/start-args'
import BlocklistEditor from './BlocklistEditor.vue'

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  inheritAttrs: false,
  props: {
    disable: {
      type: Boolean,
      default: false,
    },
    icon: {
      type: String,
      default: '',
    },
    label: {
      type: String,
      default: '',
    },
  },
  emits: ['click'],
  template:
    '<button v-bind="$attrs" :disabled="disable" :data-icon="icon" @click="$emit(\'click\')"><slot />{{ label }}</button>',
})

const QInputStub = defineComponent({
  name: 'QInputStub',
  props: {
    label: {
      type: String,
      default: '',
    },
    modelValue: {
      type: String,
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template:
    '<label><span>{{ label }}</span><input :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" /></label>',
})

describe('BlocklistEditor', () => {
  it('names each delete button after its pattern', () => {
    const wrapper = mount(BlocklistEditor, {
      props: {
        blocklist: [
          {
            pattern: '-javaagent:',
            reason: 'Disallow agents',
          },
        ],
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          'q-input': QInputStub,
          'q-icon': true,
        },
      },
    })

    const removeButton = wrapper.get('.blocklist-editor__row button')
    expect(removeButton.attributes('aria-label')).toBe('Remove -javaagent:')
  })

  // The parent applies each emitted list, as v-model does.
  function mountBound(blocklist: StartArgBlocklistEntry[]) {
    const wrapper = mount(BlocklistEditor, {
      attachTo: document.body,
      props: {
        blocklist,
        'onUpdate:blocklist': (next: StartArgBlocklistEntry[]) => {
          void wrapper.setProps({ blocklist: next })
        },
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          'q-input': QInputStub,
          'q-icon': true,
          'q-tooltip': true,
        },
      },
    })
    return wrapper
  }

  it('puts an undo row with focus where a removed rule was, and Undo restores it', async () => {
    const blocklist = [
      { pattern: '-javaagent:', reason: 'Disallow agents' },
      { pattern: '-Xdebug', reason: 'No debugging' },
    ]
    const wrapper = mountBound(blocklist)

    await wrapper.get('[aria-label="Remove -javaagent:"]').trigger('click')
    expect(wrapper.props('blocklist')).toEqual([blocklist[1]])

    const row = wrapper.get('.blocklist-editor__rows > [role="status"]')
    expect(row.text()).toContain('Removed -javaagent:.')
    expect(row.element.nextElementSibling?.getAttribute('aria-label')).toBe('Rule 1')
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Undo removing -javaagent:')

    await wrapper.get('[aria-label="Undo removing -javaagent:"]').trigger('click')
    expect(wrapper.props('blocklist')).toEqual(blocklist)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('restores a rule after its neighbour even when an earlier rule went meanwhile', async () => {
    const first = { pattern: '-a', reason: '' }
    const second = { pattern: '-b', reason: '' }
    const third = { pattern: '-c', reason: '' }
    const fourth = { pattern: '-d', reason: '' }
    const wrapper = mountBound([first, second, third, fourth])

    await wrapper.get('[aria-label="Remove -c"]').trigger('click')
    // The first rule goes some other way, so the old index now points past the neighbour.
    await wrapper.setProps({ blocklist: [second, fourth] })
    await wrapper.get('[aria-label="Undo removing -c"]').trigger('click')

    expect(wrapper.props('blocklist')).toEqual([second, third, fourth])
    wrapper.unmount()
  })

  it('drops the undo row on Escape or the next edit, without losing focus', async () => {
    const wrapper = mountBound([
      { pattern: '-a', reason: '' },
      { pattern: '-b', reason: '' },
      { pattern: '-c', reason: '' },
    ])

    await wrapper.get('[aria-label="Remove -a"]').trigger('click')
    await wrapper.get('[aria-label="Undo removing -a"]').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get('.blocklist-editor__action').element)

    await wrapper.get('[aria-label="Remove -b"]').trigger('click')
    expect(wrapper.find('[role="status"]').exists()).toBe(true)
    await wrapper.get('.blocklist-editor__row input').setValue('-cc')
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('keeps the row mounted while its pattern is edited, so focus stays in the field', async () => {
    const wrapper = mount(BlocklistEditor, {
      props: {
        blocklist: [{ pattern: '-agentlib:', reason: 'No agents' }],
      },
      global: {
        stubs: {
          'q-btn': QBtnStub,
          'q-input': QInputStub,
          'q-icon': true,
        },
      },
    })

    const rowBefore = wrapper.get('.blocklist-editor__row').element
    await wrapper.setProps({ blocklist: [{ pattern: '-agentlib:x', reason: 'No agents' }] })

    expect(wrapper.get('.blocklist-editor__row').element).toBe(rowBefore)
  })
})
