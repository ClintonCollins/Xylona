import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { describe, expect, it } from 'vitest'

import {
  GameConsoleCommandArgumentSchema,
  GameConsoleCommandExampleSchema,
  GameConsoleCommandRisk,
  GameConsoleCommandSchema,
  GameSchema,
  type Game,
} from '@/proto/shared_pb'
import GameFormConsoleCommandsTab from './GameFormConsoleCommandsTab.vue'
import { gameFormContextKey, type GameFormContext, type GameFormTabID } from './GameFormTypes'

const QInputStub = defineComponent({
  name: 'QInputStub',
  inheritAttrs: false,
  props: {
    error: { type: Boolean, default: false },
    errorMessage: { type: String, default: '' },
    label: { type: String, default: '' },
    modelValue: { type: [String, Number], default: '' },
  },
  emits: ['update:modelValue'],
  methods: {
    focus() {},
  },
  template: `
    <label v-bind="$attrs">
      <span>{{ label }}</span>
      <input
        :aria-label="label"
        :value="modelValue"
        @input="$emit('update:modelValue', $event.target.value)"
      />
      <span v-if="error">{{ errorMessage }}</span>
      <slot name="prepend" />
    </label>
  `,
})

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  inheritAttrs: false,
  props: {
    label: { type: String, default: '' },
    modelValue: { type: [String, Number, Array], default: '' },
  },
  emits: ['update:modelValue'],
  template: '<div v-bind="$attrs">{{ label }}</div>',
})

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  inheritAttrs: false,
  props: {
    disable: { type: Boolean, default: false },
    label: { type: String, default: '' },
  },
  emits: ['click'],
  template: `
    <button
      v-bind="$attrs"
      :disabled="disable"
      type="button"
      @click="$emit('click')"
    >
      {{ label }}<slot />
    </button>
  `,
})

function mountTab(gameValue: Game) {
  const game = ref(gameValue)
  const activeFormTab = ref<GameFormTabID>('overview')
  const context = { activeFormTab, game } as unknown as GameFormContext

  return {
    activeFormTab,
    game,
    wrapper: mount(GameFormConsoleCommandsTab, {
      global: {
        provide: {
          [gameFormContextKey as symbol]: context,
        },
        stubs: {
          'q-btn': QBtnStub,
          'q-icon': true,
          'q-input': QInputStub,
          'q-select': QSelectStub,
          'q-toggle': true,
          'q-tooltip': true,
        },
      },
    }),
  }
}

describe('GameFormConsoleCommandsTab', () => {
  it('edits an ordered command catalog and its ordered nested collections', async () => {
    const first = create(GameConsoleCommandSchema, {
      command: 'save',
      syntax: 'save',
      summary: 'Save the world.',
    })
    const second = create(GameConsoleCommandSchema, {
      command: 'stop',
      syntax: 'stop',
      summary: 'Stop the server.',
      arguments: [
        create(GameConsoleCommandArgumentSchema, { name: 'seconds' }),
        create(GameConsoleCommandArgumentSchema, { name: 'message' }),
      ],
      examples: [
        create(GameConsoleCommandExampleSchema, { command: 'stop 10' }),
        create(GameConsoleCommandExampleSchema, { command: 'stop 30 Maintenance' }),
      ],
      notes: ['Saves first.', 'Disconnects players.'],
      risk: GameConsoleCommandRisk.DESTRUCTIVE,
    })
    const { game, wrapper } = mountTab(
      create(GameSchema, {
        consoleCommands: [first, second],
      }),
    )

    await wrapper.get('[data-testid="console-command-list-item-1"]').trigger('click')

    expect(wrapper.get('[data-testid="console-command-preview"]').text()).toContain('stop')
    expect(wrapper.get('[data-testid="console-command-preview"]').text()).toContain('Destructive')

    await wrapper.get('[data-testid="move-console-command-up"]').trigger('click')
    expect(game.value.consoleCommands.map((command) => command.command)).toEqual(['stop', 'save'])

    await wrapper.get('[aria-label="Move argument 2 up"]').trigger('click')
    expect(second.arguments.map((argument) => argument.name)).toEqual(['message', 'seconds'])

    await wrapper.get('[aria-label="Move example 2 up"]').trigger('click')
    expect(second.examples.map((example) => example.command)).toEqual([
      'stop 30 Maintenance',
      'stop 10',
    ])

    await wrapper.get('[aria-label="Move note 2 up"]').trigger('click')
    expect(second.notes).toEqual(['Disconnects players.', 'Saves first.'])

    await wrapper.get('[data-testid="add-console-command-argument"]').trigger('click')
    await wrapper.get('[data-testid="add-console-command-example"]').trigger('click')
    await wrapper.get('[data-testid="add-console-command-note"]').trigger('click')
    expect(second.arguments).toHaveLength(3)
    expect(second.examples).toHaveLength(3)
    expect(second.notes).toHaveLength(3)

    await wrapper.get('[data-testid="add-console-command"]').trigger('click')
    expect(game.value.consoleCommands).toHaveLength(3)
    expect(game.value.consoleCommands[2]?.risk).toBe(GameConsoleCommandRisk.NONE)

    await wrapper.get('[data-testid="remove-console-command"]').trigger('click')
    expect(game.value.consoleCommands).toHaveLength(2)
  })

  it('filters the catalog by command metadata without changing the selected command', async () => {
    const save = create(GameConsoleCommandSchema, {
      command: 'save',
      summary: 'Persist the world.',
      category: 'World',
    })
    const stop = create(GameConsoleCommandSchema, {
      command: 'stop',
      summary: 'Shut down safely.',
      category: 'Lifecycle',
    })
    const { wrapper } = mountTab(
      create(GameSchema, {
        consoleCommands: [save, stop],
      }),
    )

    await wrapper.get('[aria-label="Filter console commands"] input').setValue('lifecycle')

    expect(wrapper.find('[data-testid="console-command-list-item-0"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="console-command-list-item-1"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="console-command-preview"]').text()).toContain('save')
  })

  it.each([
    {
      name: 'missing canonical command',
      commands: [create(GameConsoleCommandSchema)],
      message: 'needs a canonical command',
    },
    {
      name: 'duplicate canonical command',
      commands: [
        create(GameConsoleCommandSchema, { command: 'Save' }),
        create(GameConsoleCommandSchema, { command: ' save ' }),
      ],
      message: 'used by more than one command',
    },
    {
      name: 'missing argument name',
      commands: [
        create(GameConsoleCommandSchema, {
          command: 'ban',
          arguments: [create(GameConsoleCommandArgumentSchema)],
        }),
      ],
      message: 'needs a name',
    },
    {
      name: 'missing example command',
      commands: [
        create(GameConsoleCommandSchema, {
          command: 'ban',
          examples: [create(GameConsoleCommandExampleSchema)],
        }),
      ],
      message: 'needs a command',
    },
    {
      name: 'invalid documentation URL',
      commands: [
        create(GameConsoleCommandSchema, {
          command: 'ban',
          documentationUrl: 'server manual',
        }),
      ],
      message: 'must be a valid HTTP or HTTPS URL',
    },
  ])('rejects $name through its QForm validation contract', async ({ commands, message }) => {
    const { activeFormTab, wrapper } = mountTab(create(GameSchema, { consoleCommands: commands }))
    const component = wrapper.vm as unknown as { validate: () => boolean }

    expect(component.validate()).toBe(false)
    await nextTick()

    expect(activeFormTab.value).toBe('console-commands')
    expect(wrapper.get('[data-testid="console-command-validation"]').text()).toContain(message)
  })

  it('accepts a complete catalog while preserving custom descriptive values', () => {
    const command = create(GameConsoleCommandSchema, {
      command: 'custom-action',
      syntax: 'custom-action <target>',
      category: 'Community Operations',
      aliases: ['ca'],
      keywords: ['custom workflow'],
      arguments: [
        create(GameConsoleCommandArgumentSchema, {
          name: 'target',
          valueType: 'community-member-handle',
          suggestedValues: ['@moderators'],
          defaultValue: '@everyone',
        }),
      ],
      examples: [
        create(GameConsoleCommandExampleSchema, {
          command: 'custom-action @moderators',
        }),
      ],
      documentationUrl: 'https://example.com/custom-action',
      availability: 'Available when the community extension is installed.',
      notes: ['Custom values remain author-controlled.'],
    })
    const { game, wrapper } = mountTab(create(GameSchema, { consoleCommands: [command] }))
    const component = wrapper.vm as unknown as { validate: () => boolean }

    expect(component.validate()).toBe(true)
    expect(game.value.consoleCommands[0]).toMatchObject({
      category: 'Community Operations',
      aliases: ['ca'],
      keywords: ['custom workflow'],
      availability: 'Available when the community extension is installed.',
    })
    expect(game.value.consoleCommands[0]?.arguments[0]).toMatchObject({
      valueType: 'community-member-handle',
      suggestedValues: ['@moderators'],
      defaultValue: '@everyone',
    })
  })
})
