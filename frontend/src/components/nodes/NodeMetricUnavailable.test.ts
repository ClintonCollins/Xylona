import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import NodeMetricUnavailable from './NodeMetricUnavailable.vue'

describe('NodeMetricUnavailable', () => {
  it('reads as muted "Unavailable" and tells assistive tech why', () => {
    const wrapper = mount(NodeMetricUnavailable, {
      props: { resource: 'memory' },
      global: { stubs: { 'q-tooltip': { template: '<span class="tooltip"><slot /></span>' } } },
    })

    expect(wrapper.classes()).toContain('text-xy-muted')
    expect(wrapper.text()).toContain('Unavailable')
    expect(wrapper.text()).not.toContain('%')
    expect(wrapper.find('.xy-visually-hidden').text()).toBe(
      ": The node couldn't read its memory usage.",
    )
    expect(wrapper.find('.tooltip').text()).toBe("The node couldn't read its memory usage.")
  })
})
