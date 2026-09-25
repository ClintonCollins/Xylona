import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { describe, expect, it } from 'vitest'

import { GameSchema, ModProfileSchema, ModSourceSchema } from '@/proto/shared_pb'
import GameFormModsTab from './GameFormModsTab.vue'
import { gameFormContextKey, type GameFormContext } from './GameFormTypes'
import { useGameFormModProfile } from './useGameFormModProfile'

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  inheritAttrs: false,
  props: { label: { type: String, default: '' } },
  emits: ['click'],
  template:
    '<button v-bind="$attrs" type="button" @click="$emit(\'click\')">{{ label }}<slot /></button>',
})

const QInputStub = defineComponent({
  name: 'QInputStub',
  props: { label: { type: String, default: '' } },
  methods: {
    focus() {
      ;(this.$el as HTMLElement).querySelector('input')?.focus()
    },
  },
  template: '<label>{{ label }}<input /></label>',
})

function mountTab() {
  const game = ref(
    create(GameSchema, {
      modProfile: create(ModProfileSchema, {
        installPath: 'plugins/',
        sources: [create(ModSourceSchema, { id: 'modrinth' })],
      }),
    }),
  )
  const context = {
    game,
    isDirty: ref(false),
    ...useGameFormModProfile(game),
  } as unknown as GameFormContext

  const wrapper = mount(GameFormModsTab, {
    attachTo: document.body,
    global: {
      provide: { [gameFormContextKey as symbol]: context },
      stubs: { 'q-btn': QBtnStub, 'q-input': QInputStub, 'q-select': true, 'q-tooltip': true },
    },
  })
  const button = (label: string) => wrapper.findAll('button').find((btn) => btn.text() === label)
  return { button, game, wrapper }
}

describe('GameFormModsTab', () => {
  it('puts a focused undo row where mod support was, and Undo brings the profile back', async () => {
    const { button, game, wrapper } = mountTab()
    const profile = game.value.modProfile

    await button('Remove')?.trigger('click')
    expect(game.value.modProfile).toBeUndefined()
    expect(wrapper.get('[role="status"]').text()).toContain('Removed mod support.')
    expect(document.activeElement?.getAttribute('aria-label')).toBe('Undo removing mod support')

    await wrapper.get('[aria-label="Undo removing mod support"]').trigger('click')
    expect(game.value.modProfile).toBe(profile)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(document.activeElement).toBe(wrapper.get('input').element)
    wrapper.unmount()
  })

  it('withdraws the undo once a newer profile exists, so it cannot overwrite it', async () => {
    const { button, game, wrapper } = mountTab()
    const removed = game.value.modProfile

    await button('Remove')?.trigger('click')
    await button('Enable Mod Support')?.trigger('click')
    const newer = game.value.modProfile

    expect(newer).toBeDefined()
    expect(newer).not.toBe(removed)
    expect(wrapper.find('[role="status"]').exists()).toBe(false)
    expect(game.value.modProfile).toBe(newer)
    wrapper.unmount()
  })
})
