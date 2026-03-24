import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import type { MountingOptions } from '@vue/test-utils'
import { describe, expect, it } from 'vitest'
import { InstalledModSchema } from '@/proto/shared_pb'
import type { InstalledMod } from '@/proto/shared_pb'
import InstalledModsTable from './InstalledModsTable.vue'

function makeMod(overrides: Partial<InstalledMod> = {}): InstalledMod {
  return create(InstalledModSchema, {
    id: 'mod-1',
    gameServerId: 'gs-1',
    source: 'modrinth',
    sourceId: 'abc',
    modName: 'Fabric API',
    modAuthor: 'FabricMC',
    installedVersion: '0.90.0',
    installedVersionId: 'v-1',
    autoUpdate: true,
    enabled: true,
    updateAvailable: false,
    latestVersion: '0.90.0',
    ...overrides,
  } as never)
}

type TestStubs = Exclude<
  NonNullable<NonNullable<MountingOptions<Record<string, never>>['global']>['stubs']>,
  string[]
>

const QUASAR_STUBS = {
  'q-input': {
    props: ['modelValue', 'placeholder'],
    emits: ['update:modelValue', 'clear'],
    template: `<div class="q-input-stub">
      <input
        :value="modelValue"
        :placeholder="placeholder"
        @input="$emit('update:modelValue', $event.target.value)"
      />
      <slot name="prepend" />
    </div>`,
  },
  'q-icon': { props: ['name', 'size', 'color'], template: '<i />' },
  'q-spinner': { template: '<div class="q-spinner-stub" />' },
  'q-btn': {
    props: ['label', 'icon', 'color', 'disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')">{{ label }}<slot /></button>',
  },
  'q-toggle': {
    props: ['modelValue', 'dense', 'color'],
    emits: ['update:modelValue'],
    template:
      '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', !modelValue)" />',
  },
  'q-menu': { template: '<div class="q-menu-stub"><slot /></div>' },
  'q-list': { template: '<div class="q-list-stub"><slot /></div>' },
  'q-item': {
    emits: ['click'],
    template: '<div class="q-item-stub" @click="$emit(\'click\')"><slot /></div>',
  },
  'q-item-section': { template: '<span><slot /></span>' },
  'q-separator': { template: '<hr />' },
} satisfies TestStubs

function mountTable(mods: InstalledMod[] = [], loading = false) {
  return mount(InstalledModsTable, {
    props: { installedMods: mods, loading },
    global: {
      stubs: QUASAR_STUBS,
      directives: {
        'close-popup': {},
      },
    },
  })
}

describe('InstalledModsTable', () => {
  it('renders mod rows with name, author, source, and version', () => {
    const mod = makeMod()
    const wrapper = mountTable([mod])

    expect(wrapper.text()).toContain('Fabric API')
    expect(wrapper.text()).toContain('FabricMC')
    expect(wrapper.text()).toContain('0.90.0')
    expect(wrapper.text()).toContain('Modrinth')
  })

  it('shows update badge when update_available is true', () => {
    const mod = makeMod({
      updateAvailable: true,
      latestVersion: '0.91.0',
    })
    const wrapper = mountTable([mod])

    const row = wrapper.find('.mod-row--update')
    expect(row.exists()).toBe(true)
    expect(wrapper.text()).toContain('Update to 0.91.0')
  })

  it('shows amber version text when update is available', () => {
    const mod = makeMod({
      updateAvailable: true,
      latestVersion: '0.91.0',
    })
    const wrapper = mountTable([mod])

    const versionEl = wrapper.find('.version-text--update')
    expect(versionEl.exists()).toBe(true)
    expect(versionEl.text()).toBe('0.90.0')
  })

  it('emits toggle-auto-update when toggle is changed', async () => {
    const mod = makeMod({ autoUpdate: true })
    const wrapper = mountTable([mod])

    const toggle = wrapper.find('input[type="checkbox"]')
    await toggle.trigger('change')

    const emitted = wrapper.emitted('toggle-auto-update') ?? []
    expect(emitted).toHaveLength(1)
    expect(emitted[0]).toEqual(['mod-1', false])
  })

  it('renders disabled mods with reduced opacity class', () => {
    const mod = makeMod({ enabled: false })
    const wrapper = mountTable([mod])

    const row = wrapper.find('.mod-row--disabled')
    expect(row.exists()).toBe(true)
    expect(wrapper.text()).toContain('Disabled')
  })

  it('filters mods by name when filter input is used', async () => {
    const mod1 = makeMod({ id: 'mod-1', modName: 'Fabric API', modAuthor: 'FabricMC' })
    const mod2 = makeMod({ id: 'mod-2', modName: 'Sodium', modAuthor: 'CaffeineMC' })
    const wrapper = mountTable([mod1, mod2])

    // Both mods visible initially
    expect(wrapper.findAll('.mod-row')).toHaveLength(2)

    // Filter to only Sodium
    const input = wrapper.find('input')
    await input.setValue('Sodium')

    expect(wrapper.findAll('.mod-row')).toHaveLength(1)
    expect(wrapper.text()).toContain('Sodium')
    expect(wrapper.text()).not.toContain('Fabric API')
  })

  it('filters mods by author', async () => {
    const mod1 = makeMod({ id: 'mod-1', modName: 'Fabric API', modAuthor: 'FabricMC' })
    const mod2 = makeMod({ id: 'mod-2', modName: 'Sodium', modAuthor: 'CaffeineMC' })
    const wrapper = mountTable([mod1, mod2])

    const input = wrapper.find('input')
    await input.setValue('CaffeineMC')

    expect(wrapper.findAll('.mod-row')).toHaveLength(1)
    expect(wrapper.text()).toContain('Sodium')
  })

  it('shows "Update All" button when updates are available', () => {
    const mod = makeMod({ updateAvailable: true, latestVersion: '0.91.0' })
    const wrapper = mountTable([mod])

    expect(wrapper.text()).toContain('Update All')
  })

  it('hides "Update All" button when no updates are available', () => {
    const mod = makeMod({ updateAvailable: false })
    const wrapper = mountTable([mod])

    expect(wrapper.text()).not.toContain('Update All')
  })

  it('does not count disabled mods with updates for "Update All" visibility', () => {
    const mod = makeMod({ updateAvailable: true, enabled: false, latestVersion: '0.91.0' })
    const wrapper = mountTable([mod])

    expect(wrapper.text()).not.toContain('Update All')
  })

  it('emits update-all when "Update All" button is clicked', async () => {
    const mod = makeMod({ updateAvailable: true, latestVersion: '0.91.0' })
    const wrapper = mountTable([mod])

    const buttons = wrapper.findAll('button')
    const updateAllBtn = buttons.find((b) => b.text().includes('Update All'))
    expect(updateAllBtn).toBeDefined()
    if (!updateAllBtn) return
    await updateAllBtn.trigger('click')

    expect(wrapper.emitted('update-all')).toBeTruthy()
  })

  it('shows empty state when no mods are installed', () => {
    const wrapper = mountTable([])

    expect(wrapper.text()).toContain('No mods installed')
  })

  it('shows loading spinner when loading is true', () => {
    const wrapper = mountTable([], true)

    expect(wrapper.find('.q-spinner-stub').exists()).toBe(true)
    expect(wrapper.text()).toContain('Loading mods')
  })

  it('shows correct source badges for different sources', () => {
    const modrinthMod = makeMod({ id: 'mod-m', source: 'modrinth' })
    const hangarMod = makeMod({ id: 'mod-h', source: 'hangar' })
    const wrapper = mountTable([modrinthMod, hangarMod])

    const badges = wrapper.findAll('.source-badge')
    expect(badges).toHaveLength(2)
    expect(badges[0].text()).toBe('M')
    expect(badges[1].text()).toBe('H')
  })

  it('emits update when single update button is clicked', async () => {
    const mod = makeMod({ updateAvailable: true, latestVersion: '0.91.0' })
    const wrapper = mountTable([mod])

    const buttons = wrapper.findAll('button')
    const updateBtn = buttons.find((b) => b.text().includes('Update to'))
    expect(updateBtn).toBeDefined()
    if (!updateBtn) return
    await updateBtn.trigger('click')

    const emitted = wrapper.emitted('update') ?? []
    expect(emitted).toHaveLength(1)
    expect(emitted[0]).toEqual(['mod-1'])
  })

  it('shows "Up to date" for enabled mods with no updates', () => {
    const mod = makeMod({ enabled: true, updateAvailable: false })
    const wrapper = mountTable([mod])

    expect(wrapper.text()).toContain('Up to date')
  })

  it('shows no-results message when filter matches nothing', async () => {
    const mod = makeMod()
    const wrapper = mountTable([mod])

    const input = wrapper.find('input')
    await input.setValue('nonexistentmod')

    expect(wrapper.text()).toContain('No mods match')
    expect(wrapper.findAll('.mod-row')).toHaveLength(0)
  })
})
