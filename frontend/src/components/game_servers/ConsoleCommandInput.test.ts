import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick } from 'vue'
import { describe, expect, it } from 'vitest'

import { GameConsoleCommandRisk, GameConsoleCommandSchema } from '@/proto/shared_pb'
import ConsoleCommandInput from './ConsoleCommandInput.vue'

const QBtnStub = defineComponent({
  inheritAttrs: false,
  props: {
    ariaLabel: { type: String, default: '' },
    disable: Boolean,
    icon: { type: String, default: '' },
    loading: Boolean,
  },
  emits: ['click', 'mousedown'],
  template: `
    <button
      :aria-label="ariaLabel"
      :data-icon="icon"
      :disabled="disable"
      type="button"
      @mousedown="$emit('mousedown', $event)"
      @click="$emit('click', $event)">
      <slot />
    </button>
  `,
})

const commands = [
  create(GameConsoleCommandSchema, {
    command: 'save-all',
    syntax: 'save-all [flush]',
    summary: 'Saves server state to disk.',
    category: 'World',
  }),
  create(GameConsoleCommandSchema, {
    command: 'whitelist add',
    syntax: 'whitelist add <targets>',
    summary: 'Adds players to the allowlist.',
    category: 'Players',
    aliases: ['allowlist add'],
    availability: 'Available when the allowlist is enabled.',
    risk: GameConsoleCommandRisk.CAUTION,
  }),
]

function mountInput(modelValue = '', catalog = commands) {
  return mount(ConsoleCommandInput, {
    props: {
      commands: catalog,
      gameName: 'Minecraft',
      modelValue,
    },
    global: {
      stubs: {
        'q-btn': QBtnStub,
        'q-icon': true,
        'q-tooltip': true,
      },
    },
  })
}

describe('ConsoleCommandInput', () => {
  it('opens the known-command list and exposes combobox state on focus', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')

    await input.trigger('focus')

    expect(input.attributes('role')).toBe('combobox')
    expect(input.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('[role="listbox"]').attributes('aria-label')).toBe('Known console commands')
    expect(wrapper.findAll('[role="option"]')).toHaveLength(2)
  })

  it('uses Tab to complete the active match without submitting', async () => {
    const wrapper = mountInput('white')
    const input = wrapper.get('input')

    await input.trigger('focus')
    await input.trigger('keydown', { key: 'Tab' })

    expect(wrapper.emitted('update:modelValue')).toEqual([['whitelist add ']])
    expect(wrapper.emitted('submit')).toBeUndefined()
  })

  it('navigates matches with arrows and completes the highlighted command', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')

    await input.trigger('focus')
    await input.trigger('keydown', { key: 'ArrowDown' })
    await input.trigger('keydown', { key: 'Tab' })

    expect(wrapper.emitted('update:modelValue')).toEqual([['save-all ']])
  })

  it('always emits submit for Enter even when input is not a known command', async () => {
    const wrapper = mountInput('custom-plugin-command value')
    const input = wrapper.get('input')

    await input.trigger('focus')
    await input.trigger('keydown', { key: 'Enter' })

    expect(wrapper.emitted('submit')).toHaveLength(1)
    expect(wrapper.emitted('update:modelValue')).toBeUndefined()
  })

  it('does not submit while Enter is confirming an IME composition', async () => {
    const wrapper = mountInput('say こんにちは')
    const input = wrapper.get('input')

    await input.trigger('focus')
    await input.trigger('keydown', { isComposing: true, key: 'Enter' })

    expect(wrapper.emitted('submit')).toBeUndefined()
  })

  it('restores command-history arrows after Escape closes suggestions', async () => {
    const wrapper = mountInput()
    const input = wrapper.get('input')

    await input.trigger('focus')
    await input.trigger('keydown', { key: 'Escape' })
    await input.trigger('keydown', { key: 'ArrowUp' })

    expect(wrapper.emitted('history')).toEqual([['up']])
  })

  it('shows unrestricted-input guidance when there are no matches', async () => {
    const wrapper = mountInput('unknown')

    await wrapper.get('input').trigger('focus')

    expect(wrapper.text()).toContain('No known command matches')
    expect(wrapper.text()).toContain('Press Enter to send your input exactly as typed.')
  })

  it('keeps the existing plain input behavior when a game has no catalog', async () => {
    const wrapper = mountInput('status', [])
    const input = wrapper.get('input')

    await input.trigger('focus')
    await nextTick()

    expect(input.attributes('aria-expanded')).toBe('false')
    expect(wrapper.find('[role="listbox"]').exists()).toBe(false)
    await input.trigger('keydown', { key: 'ArrowDown' })
    expect(wrapper.emitted('history')).toEqual([['down']])
  })
})
