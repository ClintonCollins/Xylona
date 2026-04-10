import { describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import VersionBadge from './VersionBadge.vue'

describe('VersionBadge', () => {
  it('renders amber badge when update is available', () => {
    const wrapper = mount(VersionBadge, {
      props: { updateAvailable: true },
    })
    expect(wrapper.find('.update-badge').exists()).toBe(true)
    expect(wrapper.text()).toContain('Update')
  })

  it('does not render when no update available', () => {
    const wrapper = mount(VersionBadge, {
      props: { updateAvailable: false },
    })
    expect(wrapper.find('.update-badge').exists()).toBe(false)
  })
})
