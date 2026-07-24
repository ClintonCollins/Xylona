import { mount, type VueWrapper } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'

import PageHeader from './PageHeader.vue'

interface MountOptions {
  props: { title: string; subtitle?: string; icon?: string }
  slots?: Record<string, string>
}

function mountHeader(options: MountOptions): VueWrapper {
  return mount(PageHeader, {
    props: options.props,
    slots: options.slots,
    global: {
      stubs: {
        'q-icon': {
          props: ['name'],
          template: '<i class="q-icon-stub" :data-name="name" />',
        },
      },
    },
  })
}

describe('PageHeader', () => {
  const cases: Array<{
    name: string
    options: MountOptions
    assert: (wrapper: VueWrapper) => void
  }> = [
    {
      name: 'renders the title as an h1 with the shared page-title class',
      options: { props: { title: 'Backups' } },
      assert: (wrapper) => {
        const heading = wrapper.find('h1.xy-page-title')
        expect(heading.exists()).toBe(true)
        expect(heading.text()).toBe('Backups')
      },
    },
    {
      name: 'renders the subtitle prop when provided',
      options: { props: { title: 'Players', subtitle: 'Inspect the live roster.' } },
      assert: (wrapper) => {
        const subtitle = wrapper.find('.xy-page-subtitle')
        expect(subtitle.exists()).toBe(true)
        expect(subtitle.text()).toBe('Inspect the live roster.')
      },
    },
    {
      name: 'omits the subtitle area when no subtitle prop or default slot is given',
      options: { props: { title: 'Files' } },
      assert: (wrapper) => {
        expect(wrapper.find('.xy-page-subtitle').exists()).toBe(false)
      },
    },
    {
      name: 'renders default slot content in the subtitle area instead of the subtitle prop',
      options: {
        props: { title: 'Game Servers', subtitle: 'ignored fallback' },
        slots: { default: '<span class="summary-slot">3 servers online</span>' },
      },
      assert: (wrapper) => {
        const subtitle = wrapper.find('.xy-page-subtitle')
        expect(subtitle.find('.summary-slot').exists()).toBe(true)
        expect(subtitle.text()).toBe('3 servers online')
      },
    },
    {
      name: 'renders the actions slot right-aligned in the page-actions container',
      options: {
        props: { title: 'Scheduled Tasks' },
        slots: { actions: '<button class="action-slot">Add Schedule</button>' },
      },
      assert: (wrapper) => {
        const actions = wrapper.find('.xy-page-actions')
        expect(actions.exists()).toBe(true)
        expect(actions.find('.action-slot').exists()).toBe(true)
      },
    },
    {
      name: 'omits the actions container when no actions slot is given',
      options: { props: { title: 'Settings' } },
      assert: (wrapper) => {
        expect(wrapper.find('.xy-page-actions').exists()).toBe(false)
      },
    },
    {
      name: 'renders an icon before the title when the icon prop is given',
      options: { props: { title: 'Performance & Health', icon: 'insights' } },
      assert: (wrapper) => {
        const icon = wrapper.find('h1.xy-page-title .q-icon-stub')
        expect(icon.exists()).toBe(true)
        expect(icon.attributes('data-name')).toBe('insights')
      },
    },
    {
      name: 'omits the icon when the icon prop is not given',
      options: { props: { title: 'Configuration' } },
      assert: (wrapper) => {
        expect(wrapper.find('.q-icon-stub').exists()).toBe(false)
      },
    },
  ]

  it.each(cases)('$name', ({ options, assert }) => {
    const wrapper = mountHeader(options)
    expect(wrapper.find('header.xy-page-header').exists()).toBe(true)
    assert(wrapper)
  })
})
