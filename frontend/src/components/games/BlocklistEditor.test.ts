import { mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it } from 'vitest'

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
  template: '<label><span>{{ label }}</span><input :value="modelValue" /></label>',
})

describe('BlocklistEditor', () => {
  it('labels delete buttons for assistive technology', () => {
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
    expect(removeButton.attributes('aria-label')).toBe('Remove blocklist pattern')
  })
})
