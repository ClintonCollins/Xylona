import { create } from '@bufbuild/protobuf'
import { mount, VueWrapper } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { InstalledModSchema, ModSearchResultSchema } from '@/proto/shared_pb'
import type { InstalledMod, ModSearchResult } from '@/proto/shared_pb'
import ModBrowse from './ModBrowse.vue'

function makeSearchResult(overrides: Partial<ModSearchResult> = {}): ModSearchResult {
  return create(ModSearchResultSchema, {
    source: 'modrinth',
    sourceId: 'abc123',
    name: 'Fabric API',
    author: 'FabricMC',
    description: 'Lightweight and modular API providing common hooks.',
    iconUrl: '',
    downloads: 5000000n,
    latestVersion: '0.90.0',
    isInstalled: false,
    ...overrides,
  })
}

function makeInstalledMod(overrides: Partial<InstalledMod> = {}): InstalledMod {
  return create(InstalledModSchema, {
    id: 'mod-1',
    gameServerId: 'gs-1',
    source: 'modrinth',
    sourceId: 'abc123',
    modName: 'Fabric API',
    modAuthor: 'FabricMC',
    installedVersion: '0.90.0',
    installedVersionId: 'v-1',
    autoUpdate: true,
    enabled: true,
    updateAvailable: false,
    latestVersion: '0.90.0',
    ...overrides,
  })
}

// Mock the RPC client
const mockSearchMods = vi.fn()
const mockGetModCategories = vi.fn()

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    searchMods: mockSearchMods,
    getModCategories: mockGetModCategories,
  }),
  ConnectErrorToString: (err: Error) => err.message,
}))

// Mock vue-router
const mockReplace = vi.fn()
vi.mock('vue-router', () => ({
  useRoute: () => ({ query: {} }),
  useRouter: () => ({ replace: mockReplace }),
}))

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
    props: ['label', 'icon', 'color', 'disable', 'outline', 'unelevated', 'dense', 'size'],
    emits: ['click'],
    template:
      '<button :disabled="disable" @click.stop="$emit(\'click\')">{{ label }}<slot /></button>',
  },
  'q-select': {
    props: ['modelValue', 'options', 'multiple', 'useChips'],
    emits: ['update:modelValue'],
    template: '<div class="q-select-stub" />',
  },
  'q-pagination': {
    props: ['modelValue', 'max', 'maxPages'],
    emits: ['update:modelValue'],
    template: '<div class="q-pagination-stub" />',
  },
} as Record<string, unknown>

const DEFAULT_SOURCES = [
  { id: 'modrinth', searchParams: {} },
  { id: 'hangar', searchParams: {} },
]

function mountBrowse(
  props: Partial<{
    gameServerId: string
    installedMods: InstalledMod[]
    sources: { id: string; searchParams: Record<string, unknown> }[]
  }> = {},
): VueWrapper {
  return mount(ModBrowse, {
    props: {
      gameServerId: 'gs-1',
      installedMods: [],
      sources: DEFAULT_SOURCES,
      ...props,
    },
    global: {
      stubs: QUASAR_STUBS,
    },
  })
}

// Provide a minimal localStorage mock for happy-dom
const localStorageStore: Record<string, string> = {}
const localStorageMock = {
  getItem: vi.fn((key: string) => localStorageStore[key] ?? null),
  setItem: vi.fn((key: string, value: string) => {
    localStorageStore[key] = value
  }),
  removeItem: vi.fn((key: string) => {
    delete localStorageStore[key]
  }),
  clear: vi.fn(() => {
    for (const key of Object.keys(localStorageStore)) {
      delete localStorageStore[key]
    }
  }),
  get length() {
    return Object.keys(localStorageStore).length
  },
  key: vi.fn((index: number) => Object.keys(localStorageStore)[index] ?? null),
}
Object.defineProperty(globalThis, 'localStorage', { value: localStorageMock, writable: true })

describe('ModBrowse', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    localStorageMock.clear()
    mockSearchMods.mockReset()
    mockGetModCategories.mockReset()
    mockReplace.mockReset()
    // Default: return empty results for initial load
    mockSearchMods.mockResolvedValue({ results: [], totalCount: 0 })
    mockGetModCategories.mockResolvedValue({ categories: [] })
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('renders search input and source filter chips', () => {
    const wrapper = mountBrowse()

    expect(wrapper.find('input').exists()).toBe(true)
    // "All" button + one per source
    const buttons = wrapper.findAll('.source-chips button')
    expect(buttons.length).toBe(3) // All + modrinth + hangar
    expect(wrapper.text()).toContain('All')
    expect(wrapper.text()).toContain('Modrinth')
    expect(wrapper.text()).toContain('Hangar')
  })

  it('triggers initial search on mount', async () => {
    mountBrowse()

    // onMounted fires performSearch immediately
    await vi.waitFor(() => {
      expect(mockSearchMods).toHaveBeenCalledTimes(1)
    })
  })

  it('triggers search after 300ms debounce when typing', async () => {
    const result = makeSearchResult()
    // First call is the auto-search on mount, second is the typed search
    mockSearchMods
      .mockResolvedValueOnce({ results: [], totalCount: 0 })
      .mockResolvedValueOnce({ results: [result], totalCount: 1 })

    const wrapper = mountBrowse()

    // Wait for initial search to complete
    await vi.waitFor(() => {
      expect(mockSearchMods).toHaveBeenCalledTimes(1)
    })

    const input = wrapper.find('input')
    await input.setValue('fabric')

    // Should not have searched again yet
    expect(mockSearchMods).toHaveBeenCalledTimes(1)

    // Advance past debounce
    await vi.advanceTimersByTimeAsync(350)

    expect(mockSearchMods).toHaveBeenCalledTimes(2)
  })

  it('renders results in a grid after search', async () => {
    const results = [
      makeSearchResult({ sourceId: 'abc', name: 'Fabric API' }),
      makeSearchResult({ sourceId: 'def', name: 'Sodium', author: 'CaffeineMC' }),
    ]
    mockSearchMods.mockResolvedValue({ results, totalCount: 2 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('Fabric API')
    })

    expect(wrapper.text()).toContain('Sodium')
    expect(wrapper.findAll('.mod-card').length).toBe(2)
  })

  it('shows "Installed" badge for mods that are already installed', async () => {
    const result = makeSearchResult({
      source: 'modrinth',
      sourceId: 'abc123',
      name: 'Fabric API',
    })
    mockSearchMods.mockResolvedValue({ results: [result], totalCount: 1 })

    const installed = makeInstalledMod({
      source: 'modrinth',
      sourceId: 'abc123',
    })

    const wrapper = mountBrowse({ installedMods: [installed] })

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('Fabric API')
    })

    expect(wrapper.find('.installed-badge').exists()).toBe(true)
    expect(wrapper.text()).toContain('Installed')
  })

  it('shows Install button for mods that are not installed', async () => {
    const result = makeSearchResult({
      source: 'modrinth',
      sourceId: 'xyz789',
      name: 'Sodium',
    })
    mockSearchMods.mockResolvedValue({ results: [result], totalCount: 1 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('Sodium')
    })

    expect(wrapper.find('.installed-badge').exists()).toBe(false)
    const buttons = wrapper.findAll('button')
    const installBtn = buttons.find((b) => b.text().includes('Install'))
    expect(installBtn).toBeDefined()
  })

  it('emits view-details when a card is clicked', async () => {
    const result = makeSearchResult({
      source: 'modrinth',
      sourceId: 'abc123',
      name: 'Fabric API',
    })
    mockSearchMods.mockResolvedValue({ results: [result], totalCount: 1 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.find('.mod-card').exists()).toBe(true)
    })

    await wrapper.find('.mod-card').trigger('click')

    const emitted = wrapper.emitted('view-details') ?? []
    expect(emitted).toHaveLength(1)
    expect(emitted[0]).toEqual(['modrinth', 'abc123'])
  })

  it('emits install when Install button is clicked', async () => {
    const result = makeSearchResult({
      source: 'modrinth',
      sourceId: 'xyz789',
      name: 'Sodium',
    })
    mockSearchMods.mockResolvedValue({ results: [result], totalCount: 1 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('Sodium')
    })

    // Find and click the Install button inside the card footer
    const footerButtons = wrapper.findAll('.mod-card-footer button')
    const installBtn = footerButtons.find((b) => b.text().includes('Install'))
    expect(installBtn).toBeDefined()
    if (!installBtn) return
    await installBtn.trigger('click')

    // The install event should have been emitted (may also emit view-details due to bubbling in stubs)
    const emitted = wrapper.emitted('install') ?? []
    expect(emitted.length).toBeGreaterThanOrEqual(1)
    expect(emitted[0]).toEqual(['modrinth', 'xyz789'])
  })

  it('shows "No mods found" when search returns empty results', async () => {
    mockSearchMods.mockResolvedValue({ results: [], totalCount: 0 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('No mods found')
    })
  })

  it('formats download counts correctly', async () => {
    const results = [
      makeSearchResult({ sourceId: 'a', name: 'Mod A', downloads: 5000000n }),
      makeSearchResult({ sourceId: 'b', name: 'Mod B', downloads: 1500n }),
      makeSearchResult({ sourceId: 'c', name: 'Mod C', downloads: 42n }),
    ]
    mockSearchMods.mockResolvedValue({ results, totalCount: 3 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('5.0M')
    })

    expect(wrapper.text()).toContain('1.5K')
    expect(wrapper.text()).toContain('42')
  })

  it('shows source badges with correct letters', async () => {
    const results = [
      makeSearchResult({ source: 'modrinth', sourceId: 'a', name: 'Mod M' }),
      makeSearchResult({ source: 'hangar', sourceId: 'b', name: 'Mod H' }),
    ]
    mockSearchMods.mockResolvedValue({ results, totalCount: 2 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.findAll('.mod-card').length).toBe(2)
    })

    const badges = wrapper.findAll('.source-badge')
    const badgeTexts = badges.map((b) => b.text())
    expect(badgeTexts).toContain('M')
    expect(badgeTexts).toContain('H')
  })

  it('shows pagination footer when there are results', async () => {
    const results = Array.from({ length: 20 }, (_, i) =>
      makeSearchResult({ sourceId: `id-${i}`, name: `Mod ${i}` }),
    )
    mockSearchMods.mockResolvedValue({ results, totalCount: 40 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.findAll('.mod-card').length).toBe(20)
    })

    expect(wrapper.find('.browse-pagination-footer').exists()).toBe(true)
    expect(wrapper.text()).toContain('40 mods found')
  })

  it('shows pagination footer even when all results fit on one page', async () => {
    const results = [makeSearchResult({ sourceId: 'a', name: 'Only Mod' })]
    mockSearchMods.mockResolvedValue({ results, totalCount: 1 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.findAll('.mod-card').length).toBe(1)
    })

    expect(wrapper.find('.browse-pagination-footer').exists()).toBe(true)
    expect(wrapper.text()).toContain('1 mod found')
  })

  it('shows a result summary without pagination when total count is unknown', async () => {
    const results = [makeSearchResult({ sourceId: 'a', name: 'Only Mod' })]
    mockSearchMods.mockResolvedValue({ results, totalCount: -1 })

    const wrapper = mountBrowse()

    await vi.waitFor(() => {
      expect(wrapper.findAll('.mod-card').length).toBe(1)
    })

    expect(wrapper.find('.browse-pagination-footer').exists()).toBe(true)
    expect(wrapper.find('.q-pagination-stub').exists()).toBe(false)
    expect(wrapper.text()).toContain('Showing 1 result')
  })
})
