import { mount } from '@vue/test-utils'
import { computed, defineComponent, nextTick } from 'vue'
import { afterEach, describe, expect, it, vi } from 'vitest'

import StartArgsTemplateEditor from './StartArgsTemplateEditor.vue'

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
  inheritAttrs: false,
  props: {
    label: {
      type: String,
      default: '',
    },
    modelValue: {
      type: String,
      default: '',
    },
    type: {
      type: String,
      default: 'text',
    },
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
    label: {
      type: String,
      default: '',
    },
    modelValue: {
      type: String,
      default: '',
    },
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

    return {
      normalizedOptions,
    }
  },
  template: `
    <label v-bind="$attrs">
      <span>{{ label }}</span>
      <select
        :value="modelValue"
        @change="$emit('update:model-value', $event.target.value)">
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

const quasarStubs = {
  'q-btn': QBtnStub,
  'q-icon': true,
  'q-input': QInputStub,
  'q-select': QSelectStub,
}

function mockMatchMedia(matches: boolean) {
  const listeners = new Set<(event: MediaQueryListEvent) => void>()

  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation(() => ({
      matches,
      media: '(max-width: 780px)',
      onchange: null,
      addEventListener: (_event: string, listener: (event: MediaQueryListEvent) => void) => {
        listeners.add(listener)
      },
      removeEventListener: (_event: string, listener: (event: MediaQueryListEvent) => void) => {
        listeners.delete(listener)
      },
      addListener: (listener: (event: MediaQueryListEvent) => void) => {
        listeners.add(listener)
      },
      removeListener: (listener: (event: MediaQueryListEvent) => void) => {
        listeners.delete(listener)
      },
      dispatchEvent: () => true,
    })),
  )
}

describe('StartArgsTemplateEditor', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('labels inventory drag handles and drawer actions for assistive technology', () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'editable',
            label: 'Min heap size',
            managedSource: '',
            tokens: ['-Xms512M'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const dragHandles = wrapper.findAll('.template-editor__drag-handle')
    expect(dragHandles).toHaveLength(2)
    expect(dragHandles[0].attributes('aria-label')).toBe('Drag to reorder argument')

    const selectButtons = wrapper.findAll('.template-editor__inventory-select')
    expect(selectButtons).toHaveLength(2)
    expect(selectButtons[0].attributes('aria-label')).toBe(
      'Select argument 1: Min heap size (Editable)',
    )
    expect(selectButtons[0].attributes('aria-pressed')).toBe('true')
    expect(selectButtons[1].attributes('aria-pressed')).toBe('false')

    const drawerActions = wrapper.findAll('.template-editor__drawer-actions button')
    expect(drawerActions).toHaveLength(4)
    expect(drawerActions[0].attributes('aria-label')).toBe('Move argument up')
    expect(drawerActions[1].attributes('aria-label')).toBe('Move argument down')
    expect(drawerActions[2].attributes('aria-label')).toBe('Remove argument')
  })

  it('selects an inventory row and updates the drawer context', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'editable',
            label: 'Min heap size',
            managedSource: '',
            tokens: ['-Xms512M'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const rows = wrapper.findAll('.template-editor__inventory-row')
    const selectButtons = wrapper.findAll('.template-editor__inventory-select')
    expect(rows).toHaveLength(2)
    expect(rows[0].attributes('role')).toBeUndefined()
    expect(rows[0].classes()).toContain('template-editor__inventory-row--selected')

    await selectButtons[1].trigger('click')

    expect(rows[1].classes()).toContain('template-editor__inventory-row--selected')
    expect(selectButtons[1].attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('.template-editor__selected-badge').text()).toContain('Max heap size')

    const labels = wrapper.findAll('.template-editor__drawer-fields label')
    const labelInput = labels.find((node) => node.text().includes('Label'))
    expect(labelInput?.find('input').element.value).toBe('Max heap size')
  })

  it('offers managed source as a placeholder dropdown for system arguments', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'system',
            label: '',
            managedSource: '',
            tokens: ['-jar', '{{SERVER_EXECUTABLE}}'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const labels = wrapper.findAll('.template-editor__drawer-fields label')
    const managedSourceField = labels.find((node) => node.text().includes('Managed Source'))
    const managedSourceSelect = managedSourceField?.find('select')

    expect(managedSourceSelect?.exists()).toBe(true)
    expect(managedSourceSelect?.findAll('option').map((option) => option.text())).toEqual([
      'Not set',
      'IP Address',
      'Server Port',
      'Query Port',
      'Game Server Memory (MB)',
      'Server Executable',
    ])

    await managedSourceSelect?.setValue('server_executable')

    const emissions = wrapper.emitted('update:linuxTemplate')
    expect(emissions).toBeTruthy()
    expect(emissions?.at(-1)?.[0]).toEqual([
      {
        id: 'first',
        order: 0,
        ownership: 'system',
        label: '',
        managedSource: 'server_executable',
        tokens: ['-jar', '{{SERVER_EXECUTABLE}}'],
      },
    ])
  })

  it('syncs shared block metadata updates across platforms for matching block ids', async () => {
    const sharedLinuxTemplate = [
      {
        id: 'shared',
        order: 0,
        ownership: 'system',
        label: 'Launch memory',
        managedSource: '',
        tokens: ['-Xmx{{MAX_MEMORY_MB}}M'],
      },
    ]
    const sharedWindowsTemplate = [
      {
        id: 'shared',
        order: 0,
        ownership: 'system',
        label: 'Launch memory',
        managedSource: '',
        tokens: ['-Xmx{{MAX_MEMORY_MB}}M'],
      },
    ]

    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: sharedLinuxTemplate,
        windowsBaseCommand: 'java',
        windowsEnabled: true,
        windowsTemplate: sharedWindowsTemplate,
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const labels = wrapper.findAll('.template-editor__drawer-fields label')
    const managedSourceField = labels.find((node) => node.text().includes('Managed Source'))
    const managedSourceSelect = managedSourceField?.find('select')

    await managedSourceSelect?.setValue('game_server.max_memory_mb')

    const windowsManagedSourceUpdate = wrapper.emitted('update:windowsTemplate')?.at(-1)?.[0]
    const linuxManagedSourceUpdate = wrapper.emitted('update:linuxTemplate')?.at(-1)?.[0]

    expect(windowsManagedSourceUpdate).toEqual([
      {
        id: 'shared',
        order: 0,
        ownership: 'system',
        label: 'Launch memory',
        managedSource: 'game_server.max_memory_mb',
        tokens: ['-Xmx{{MAX_MEMORY_MB}}M'],
      },
    ])
    expect(linuxManagedSourceUpdate).toEqual(windowsManagedSourceUpdate)

    await wrapper.setProps({
      linuxTemplate: linuxManagedSourceUpdate,
      windowsTemplate: windowsManagedSourceUpdate,
    })
    await nextTick()

    const mutabilityField = wrapper
      .findAll('.template-editor__drawer-fields label')
      .find((node) => node.text().includes('Mutability'))
    const mutabilitySelect = mutabilityField?.find('select')

    await mutabilitySelect?.setValue('locked')

    const windowsOwnershipUpdate = wrapper.emitted('update:windowsTemplate')?.at(-1)?.[0]
    const linuxOwnershipUpdate = wrapper.emitted('update:linuxTemplate')?.at(-1)?.[0]

    expect(windowsOwnershipUpdate).toEqual([
      {
        id: 'shared',
        order: 0,
        ownership: 'locked',
        label: 'Launch memory',
        managedSource: 'game_server.max_memory_mb',
        tokens: ['-Xmx{{MAX_MEMORY_MB}}M'],
      },
    ])
    expect(linuxOwnershipUpdate).toEqual(windowsOwnershipUpdate)
  })

  it('selects an argument when its launch preview segment is clicked', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'locked',
            label: 'Security flag',
            managedSource: '',
            tokens: ['-Dlog4j2.formatMsgNoLookups=true'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const previewButtons = wrapper.findAll('.template-editor__preview-segment-button')
    expect(previewButtons).toHaveLength(2)

    await previewButtons[1].trigger('click')

    const selectButtons = wrapper.findAll('.template-editor__inventory-select')
    expect(selectButtons[0].attributes('aria-pressed')).toBe('false')
    expect(selectButtons[1].attributes('aria-pressed')).toBe('true')
    expect(wrapper.find('.template-editor__selected-badge').text()).toContain('Max heap size')
  })

  it('scrolls the selected row and drawer into view when a preview segment is clicked', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'locked',
            label: 'Security flag',
            managedSource: '',
            tokens: ['-Dlog4j2.formatMsgNoLookups=true'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const inventoryRows = wrapper.findAll('.template-editor__inventory-row')
    const drawerHead = wrapper.get('.template-editor__drawer-head')
    const inventoryScrollIntoView = vi.fn()
    const drawerScrollIntoView = vi.fn()

    Object.defineProperty(inventoryRows[1].element, 'scrollIntoView', {
      configurable: true,
      value: inventoryScrollIntoView,
    })
    Object.defineProperty(drawerHead.element, 'scrollIntoView', {
      configurable: true,
      value: drawerScrollIntoView,
    })

    const previewButtons = wrapper.findAll('.template-editor__preview-segment-button')
    await previewButtons[1].trigger('click')
    await nextTick()

    expect(inventoryScrollIntoView).toHaveBeenCalledWith({
      behavior: 'smooth',
      block: 'nearest',
      inline: 'nearest',
    })
    expect(drawerScrollIntoView).toHaveBeenCalledWith({
      behavior: 'smooth',
      block: 'nearest',
      inline: 'nearest',
    })
  })

  it('reorders rows by drag and drop and emits the normalized template', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'editable',
            label: 'Min heap size',
            managedSource: '',
            tokens: ['-Xms512M'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    const handles = wrapper.findAll('.template-editor__drag-handle')
    const rows = wrapper.findAll('.template-editor__inventory-row')
    const firstRowElement = rows[0].element as HTMLElement
    firstRowElement.getBoundingClientRect = () =>
      ({
        bottom: 52,
        height: 40,
        left: 0,
        right: 200,
        top: 12,
        width: 200,
        x: 0,
        y: 12,
        toJSON: () => ({}),
      }) as DOMRect

    await handles[1].trigger('dragstart', {
      dataTransfer: {
        effectAllowed: 'move',
        setData: vi.fn(),
      },
    })
    await rows[0].trigger('drop', { clientY: 10 })

    const emissions = wrapper.emitted('update:linuxTemplate')
    expect(emissions).toBeTruthy()

    const reordered = emissions?.at(-1)?.[0] as Array<{ id: string; order: number }>
    expect(reordered.map((block) => block.id)).toEqual(['second', 'first'])
    expect(reordered.map((block) => block.order)).toEqual([0, 1])
  })

  it('resets only the sequence order against the loaded baseline', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        baselineLinuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'editable',
            label: 'Min heap size',
            managedSource: '',
            tokens: ['-Xms512M'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'second',
            order: 0,
            ownership: 'editable',
            label: 'Heap ceiling',
            managedSource: '',
            tokens: ['-Xmx4G'],
          },
          {
            id: 'first',
            order: 1,
            ownership: 'editable',
            label: 'Heap floor',
            managedSource: '',
            tokens: ['-Xms1G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    await wrapper.get('[data-testid="start-args-reset-order"]').trigger('click')

    const emissions = wrapper.emitted('update:linuxTemplate')
    expect(emissions).toBeTruthy()

    const reordered = emissions?.at(-1)?.[0] as Array<{
      id: string
      label: string
      order: number
      tokens: string[]
    }>
    expect(reordered.map((block) => block.id)).toEqual(['first', 'second'])
    expect(reordered.map((block) => block.label)).toEqual(['Heap floor', 'Heap ceiling'])
    expect(reordered.map((block) => block.tokens.join(' '))).toEqual(['-Xms1G', '-Xmx4G'])
    expect(reordered.map((block) => block.order)).toEqual([0, 1])
  })

  it('resets the active platform launch setup back to the loaded baseline', async () => {
    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        baselineLinuxBaseCommand: 'java',
        baselineLinuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'editable',
            label: 'Min heap size',
            managedSource: '',
            tokens: ['-Xms512M'],
          },
        ],
        linuxBaseCommand: 'java17',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'custom',
            order: 0,
            ownership: 'editable',
            label: 'Custom launch flag',
            managedSource: '',
            tokens: ['-Dcustom=true'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    await wrapper.get('[data-testid="start-args-reset-platform"]').trigger('click')

    expect(wrapper.emitted('update:linuxBaseCommand')?.at(-1)).toEqual(['java'])
    expect(wrapper.emitted('update:linuxTemplate')?.at(-1)?.[0]).toEqual([
      {
        id: 'first',
        order: 0,
        ownership: 'editable',
        label: 'Min heap size',
        managedSource: '',
        tokens: ['-Xms512M'],
      },
    ])
  })

  it('collapses preview and sequence by default on compact viewports', async () => {
    mockMatchMedia(true)

    const wrapper = mount(StartArgsTemplateEditor, {
      props: {
        linuxBaseCommand: 'java',
        linuxEnabled: true,
        linuxTemplate: [
          {
            id: 'first',
            order: 0,
            ownership: 'editable',
            label: 'Min heap size',
            managedSource: '',
            tokens: ['-Xms512M'],
          },
          {
            id: 'second',
            order: 1,
            ownership: 'editable',
            label: 'Max heap size',
            managedSource: '',
            tokens: ['-Xmx2G'],
          },
        ],
        windowsBaseCommand: '',
        windowsEnabled: false,
        windowsTemplate: [],
      },
      global: {
        stubs: quasarStubs,
      },
    })

    expect(wrapper.get('.template-editor__preview-toggle').attributes('aria-expanded')).toBe(
      'false',
    )
    expect(wrapper.find('.template-editor__terminal-shell').exists()).toBe(false)
    expect(
      wrapper.get('.template-editor__mobile-inventory-toggle').attributes('aria-expanded'),
    ).toBe('false')
    expect(wrapper.findAll('.template-editor__inventory-row')).toHaveLength(0)

    await wrapper.get('.template-editor__mobile-inventory-toggle').trigger('click')
    expect(wrapper.findAll('.template-editor__inventory-row')).toHaveLength(2)

    await wrapper.findAll('.template-editor__inventory-select')[1].trigger('click')
    expect(wrapper.find('.template-editor__selected-badge').text()).toContain('Max heap size')
    expect(wrapper.findAll('.template-editor__inventory-row')).toHaveLength(0)

    await wrapper.get('.template-editor__preview-toggle').trigger('click')
    expect(wrapper.get('.template-editor__preview-toggle').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('.template-editor__terminal-shell').exists()).toBe(true)
  })
})
