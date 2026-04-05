import { mount } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import DownstreamImpactPanel from './DownstreamImpactPanel.vue'

function makeServers(count: number) {
  return Array.from({ length: count }, (_, index) => ({
    name: `Server ${index + 1}`,
    patchCount: index % 2,
  }))
}

describe('DownstreamImpactPanel', () => {
  it('starts as a quiet summary and reveals the server list on demand', async () => {
    const wrapper = mount(DownstreamImpactPanel, {
      props: {
        servers: makeServers(8),
        showHeader: false,
      },
    })

    expect(wrapper.findAll('.downstream-impact__row')).toHaveLength(0)
    expect(wrapper.text()).toContain('Review 8 servers')

    await wrapper.get('.downstream-impact__toggle').trigger('click')

    expect(wrapper.findAll('.downstream-impact__row')).toHaveLength(8)
    expect(wrapper.text()).toContain('Hide server list')
  })
})
