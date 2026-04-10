import { mount } from '@vue/test-utils'
import { computed, defineComponent } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import type { StartArgBlock } from '@/components/game_servers/start-args'

import StartArgsTemplateEditor from './StartArgsTemplateEditor.vue'

const QInputStub = defineComponent({
  name: 'QInputStub',
  inheritAttrs: false,
  props: {
    label: { type: String, default: '' },
    modelValue: { type: String, default: '' },
    type: { type: String, default: 'text' },
  },
  emits: ['update:model-value'],
  template: `
    <label v-bind="$attrs">
      <span>{{ label }}</span>
      <textarea
        v-if="type === 'textarea'"
        :value="modelValue"
        @input="$emit('update:model-value', $event.target.value)" />
      <input
        v-else
        :value="modelValue"
        @input="$emit('update:model-value', $event.target.value)" />
    </label>
  `,
})

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  inheritAttrs: false,
  props: {
    label: { type: String, default: '' },
    modelValue: { type: String, default: '' },
    options: {
      type: Array as () => Array<string | { label: string; value: string }>,
      default: () => [],
    },
  },
  emits: ['update:model-value'],
  setup(props) {
    const normalizedOptions = computed(() =>
      props.options.map((option) =>
        typeof option === 'string' ? { label: option, value: option } : option,
      ),
    )

    return { normalizedOptions }
  },
  template: `
    <label v-bind="$attrs">
      <span>{{ label }}</span>
      <select :value="modelValue" @change="$emit('update:model-value', $event.target.value)">
        <option
          v-for="option in normalizedOptions"
          :key="option.value"
          :value="option.value">
          {{ option.label }}
        </option>
      </select>
    </label>
  `,
})

const QDialogStub = defineComponent({
  name: 'QDialogStub',
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:model-value'],
  template: '<div v-if="modelValue"><slot /></div>',
})

const QCardStub = defineComponent({ name: 'QCardStub', template: '<div><slot /></div>' })
const QCardSectionStub = defineComponent({
  name: 'QCardSectionStub',
  template: '<section><slot /></section>',
})
const QCardActionsStub = defineComponent({
  name: 'QCardActionsStub',
  template: '<div><slot /></div>',
})
const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: { label: { type: String, default: '' } },
  emits: ['click'],
  template: '<button v-bind="$attrs" @click="$emit(\'click\')"><slot />{{ label }}</button>',
})

const quasarStubs = {
  'q-btn': QBtnStub,
  'q-card': QCardStub,
  'q-card-actions': QCardActionsStub,
  'q-card-section': QCardSectionStub,
  'q-dialog': QDialogStub,
  'q-icon': true,
  'q-input': QInputStub,
  'q-select': QSelectStub,
}

function buildBlock(overrides: Partial<StartArgBlock> = {}): StartArgBlock {
  return {
    id: overrides.id ?? 'block',
    order: overrides.order ?? 0,
    ownership: overrides.ownership ?? 'editable',
    label: overrides.label ?? '',
    managedSource: overrides.managedSource ?? '',
    tokens: overrides.tokens ?? [],
  }
}

function mountEditor(
  overrides: Partial<InstanceType<typeof StartArgsTemplateEditor>['$props']> = {},
) {
  return mount(StartArgsTemplateEditor, {
    props: {
      linuxBaseCommand: 'java',
      linuxEnabled: true,
      linuxTemplate: [
        buildBlock({
          id: 'first',
          order: 0,
          ownership: 'locked',
          label: 'Security flag',
          tokens: ['-Dlog4j2.formatMsgNoLookups=true'],
        }),
        buildBlock({
          id: 'second',
          order: 1,
          ownership: 'editable',
          label: 'Max heap size',
          tokens: ['-Xmx2G'],
        }),
      ],
      windowsBaseCommand: 'javaw',
      windowsEnabled: true,
      windowsTemplate: [
        buildBlock({
          id: 'win-first',
          order: 0,
          ownership: 'system',
          label: 'Jar file',
          managedSource: 'game.server_executable',
          tokens: ['-jar', '{{SERVER_EXECUTABLE}}'],
        }),
      ],
      ...overrides,
    },
    global: {
      stubs: quasarStubs,
    },
  })
}

describe('StartArgsTemplateEditor', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('renders the familiar platform toggle with a terminal-like preview shell', () => {
    const wrapper = mountEditor()

    expect(wrapper.findAll('.platform-tab')).toHaveLength(2)
    expect(
      (wrapper.get('[data-testid="start-args-base-command"]').element as HTMLInputElement).value,
    ).toBe('javaw')
    expect(wrapper.findAll('.template-editor__arg-chip')).toHaveLength(1)
  })

  it('opens an edit dialog from a preview chip', async () => {
    const wrapper = mountEditor({ windowsEnabled: false })

    await wrapper.get('[data-testid="preview-chip-second"]').trigger('click')

    expect(wrapper.get('[data-testid="start-args-dialog"]').text()).toContain('Max heap size')
    expect(
      (wrapper.get('[data-testid="tokens-input"] textarea').element as HTMLTextAreaElement).value,
    ).toBe('-Xmx2G')
  })

  it('adds a new argument from the static add button and appends it to the template', async () => {
    const wrapper = mountEditor({ windowsEnabled: false })

    await wrapper.get('[data-testid="start-args-add-block"]').trigger('click')
    await wrapper.get('[data-testid="start-args-dialog-label"] input').setValue('No GUI')
    await wrapper.get('[data-testid="tokens-input"] textarea').setValue('-nogui')
    await wrapper.get('[data-testid="save-arg-button"]').trigger('click')

    const emissions = wrapper.emitted('update:linuxTemplate')
    expect(emissions).toBeTruthy()

    const updatedTemplate = emissions?.at(-1)?.[0] as StartArgBlock[]
    const addedBlock = updatedTemplate[2]
    expect(updatedTemplate).toHaveLength(3)
    expect(addedBlock).toMatchObject({
      label: 'No GUI',
      ownership: 'editable',
      tokens: ['-nogui'],
    })
  })

  it('reorders preview arguments by dragging between chips', async () => {
    const wrapper = mountEditor({ windowsEnabled: false })
    const chips = wrapper.findAll('.template-editor__arg-chip')
    const dataTransfer = { effectAllowed: '', setData: vi.fn() }
    const firstChip = chips[0]
    const secondChip = chips[1]
    if (!firstChip || !secondChip) {
      throw new Error('expected template chips to exist')
    }

    vi.spyOn(firstChip.element, 'getBoundingClientRect').mockReturnValue({
      bottom: 40,
      height: 40,
      left: 0,
      right: 100,
      top: 0,
      width: 100,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    })

    await secondChip.trigger('dragstart', { dataTransfer })
    await firstChip.trigger('dragover', { clientX: 0, clientY: 20 })
    await firstChip.trigger('drop', { clientX: 0, clientY: 20 })

    const emissions = wrapper.emitted('update:linuxTemplate')
    expect(emissions).toBeTruthy()
    expect((emissions?.at(-1)?.[0] as StartArgBlock[]).map((block) => block.id)).toEqual([
      'second',
      'first',
    ])
  })

  it('keeps the advanced sequence collapsed by default and expands on demand', async () => {
    const wrapper = mountEditor({ windowsEnabled: false })

    expect(
      wrapper.get('[data-testid="start-args-advanced-panel-toggle"]').attributes('aria-expanded'),
    ).toBe('false')
    expect(wrapper.findAll('.template-editor__sequence-row')).toHaveLength(0)

    await wrapper.get('[data-testid="start-args-advanced-panel-toggle"]').trigger('click')

    expect(
      wrapper.get('[data-testid="start-args-advanced-panel-toggle"]').attributes('aria-expanded'),
    ).toBe('true')
    expect(wrapper.findAll('.template-editor__sequence-row')).toHaveLength(2)
  })

  it('resets order and the full platform launch setup from the preview toolbar', async () => {
    const wrapper = mountEditor({
      baselineLinuxBaseCommand: 'java',
      baselineLinuxTemplate: [
        buildBlock({
          id: 'first',
          order: 0,
          ownership: 'editable',
          label: 'Min heap',
          tokens: ['-Xms512M'],
        }),
        buildBlock({
          id: 'second',
          order: 1,
          ownership: 'editable',
          label: 'Max heap',
          tokens: ['-Xmx2G'],
        }),
      ],
      linuxBaseCommand: 'java17',
      linuxTemplate: [
        buildBlock({
          id: 'second',
          order: 0,
          ownership: 'editable',
          label: 'Heap ceiling',
          tokens: ['-Xmx4G'],
        }),
        buildBlock({
          id: 'first',
          order: 1,
          ownership: 'editable',
          label: 'Heap floor',
          tokens: ['-Xms1G'],
        }),
      ],
      windowsEnabled: false,
    })

    await wrapper.get('[data-testid="start-args-reset-order"]').trigger('click')
    const orderReset = wrapper.emitted('update:linuxTemplate')?.at(-1)?.[0] as StartArgBlock[]
    expect(orderReset.map((block) => block.id)).toEqual(['first', 'second'])

    await wrapper.setProps({ linuxBaseCommand: 'java17', linuxTemplate: orderReset })
    await wrapper.get('[data-testid="start-args-reset-platform"]').trigger('click')

    expect(wrapper.emitted('update:linuxBaseCommand')?.at(-1)).toEqual(['java'])
    const resetTemplate = wrapper.emitted('update:linuxTemplate')?.at(-1)?.[0] as StartArgBlock[]
    expect(resetTemplate[0]?.label).toBe('Min heap')
  })

  it('syncs shared block metadata across both platforms when edited in the dialog', async () => {
    const wrapper = mountEditor({
      linuxBaseCommand: 'java',
      linuxTemplate: [
        buildBlock({
          id: 'shared',
          order: 0,
          ownership: 'system',
          label: 'Launch memory',
          managedSource: '',
          tokens: ['-Xmx{{MAX_MEMORY_MB}}M'],
        }),
      ],
      windowsBaseCommand: 'java',
      windowsTemplate: [
        buildBlock({
          id: 'shared',
          order: 0,
          ownership: 'system',
          label: 'Launch memory',
          managedSource: '',
          tokens: ['-Xmx{{MAX_MEMORY_MB}}M'],
        }),
      ],
    })

    await wrapper.get('[data-testid="preview-chip-shared"]').trigger('click')
    await wrapper.get('[data-testid="start-args-dialog-label"] input').setValue('Game memory')
    await wrapper
      .get('[data-testid="start-args-dialog-managed-source"] select')
      .setValue('game_server.max_memory_mb')
    await wrapper.get('[data-testid="save-arg-button"]').trigger('click')

    expect(
      (wrapper.emitted('update:linuxTemplate')?.at(-1)?.[0] as StartArgBlock[])[0],
    ).toMatchObject({
      label: 'Game memory',
      managedSource: 'game_server.max_memory_mb',
    })
    expect(
      (wrapper.emitted('update:windowsTemplate')?.at(-1)?.[0] as StartArgBlock[])[0],
    ).toMatchObject({
      label: 'Game memory',
      managedSource: 'game_server.max_memory_mb',
    })
  })
})
