import { create } from '@bufbuild/protobuf'
import type { MountingOptions } from '@vue/test-utils'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ModDetails, ModVersion } from '@/proto/shared_pb'
import { ModDetailsSchema, ModVersionSchema } from '@/proto/shared_pb'
import ModDetailDialog from './ModDetailDialog.vue'

function makeModDetails(overrides: Partial<ModDetails> = {}): ModDetails {
  const details = create(ModDetailsSchema)

  Object.assign(details, {
    source: 'modrinth',
    sourceId: 'abc123',
    name: 'Sample Mod',
    author: 'Sample Author',
    description: 'Sample description',
    body: '<p>Safe body</p>',
    iconUrl: '',
    downloads: 42n,
    galleryImages: [],
    categories: [],
    license: '',
    sourceUrl: '',
    versions: [],
    ...overrides,
  })

  return details
}

function makeModVersion(overrides: Partial<ModVersion> = {}): ModVersion {
  const version = create(ModVersionSchema)

  Object.assign(version, {
    versionId: 'v1',
    versionString: '1.0.0',
    gameVersions: [],
    downloadUrl: '',
    fileSize: 0n,
    dependencies: [],
    changelog: '',
    ...overrides,
  })

  return version
}

const mockGetModDetails = vi.fn()
const mockGetModVersions = vi.fn()

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getModDetails: mockGetModDetails,
    getModVersions: mockGetModVersions,
  }),
  ConnectErrorToString: (err: Error) => err.message,
}))

type TestStubs = Exclude<
  NonNullable<NonNullable<MountingOptions<Record<string, never>>['global']>['stubs']>,
  string[]
>

const QUASAR_STUBS = {
  'q-dialog': {
    props: ['modelValue'],
    template: '<div class="q-dialog-stub"><slot v-if="modelValue" /></div>',
  },
  'q-card': { template: '<div class="q-card-stub"><slot /></div>' },
  'q-card-section': { template: '<section class="q-card-section-stub"><slot /></section>' },
  'q-card-actions': { template: '<footer class="q-card-actions-stub"><slot /></footer>' },
  'q-spinner': { template: '<div class="q-spinner-stub" />' },
  'q-icon': { template: '<i class="q-icon-stub" />' },
  'q-btn': {
    props: ['label', 'disable'],
    emits: ['click'],
    template: '<button :disabled="disable" @click="$emit(\'click\')"><slot />{{ label }}</button>',
  },
  'q-separator': { template: '<hr class="q-separator-stub" />' },
  'q-tabs': { template: '<div class="q-tabs-stub"><slot /></div>' },
  'q-tab': { props: ['label'], template: '<button class="q-tab-stub">{{ label }}</button>' },
  'q-tab-panels': { template: '<div class="q-tab-panels-stub"><slot /></div>' },
  'q-tab-panel': { template: '<div class="q-tab-panel-stub"><slot /></div>' },
  'q-badge': { props: ['label'], template: '<span class="q-badge-stub">{{ label }}</span>' },
  'q-select': { template: '<div class="q-select-stub" />' },
  'q-space': { template: '<span class="q-space-stub" />' },
} satisfies TestStubs

function mountDialog() {
  return mount(ModDetailDialog, {
    props: {
      show: false,
      gameServerId: 'gs-1',
      source: 'modrinth',
      sourceId: 'abc123',
      isInstalled: false,
    },
    global: {
      stubs: QUASAR_STUBS,
    },
  })
}

describe('ModDetailDialog', () => {
  beforeEach(() => {
    mockGetModDetails.mockReset()
    mockGetModVersions.mockReset()
  })

  it('sanitizes hostile mod description HTML while preserving safe formatting', async () => {
    mockGetModDetails.mockResolvedValue({
      details: makeModDetails({
        body: [
          '<p>Intro <strong>bold</strong> <em>italic</em> <a href="https://example.com">safe link</a></p>',
          '<ul><li><code>code</code> item</li></ul>',
          '<script>alert("xss")</script>',
          '<img src="x" onerror="alert(1)" />',
          '<a href="javascript:alert(2)" onclick="alert(3)">bad link</a>',
        ].join(''),
      }),
    })
    mockGetModVersions.mockResolvedValue({
      versions: [makeModVersion()],
    })

    const wrapper = mountDialog()

    await wrapper.setProps({ show: true })

    await vi.waitFor(() => {
      expect(mockGetModDetails).toHaveBeenCalledTimes(1)
      expect(mockGetModVersions).toHaveBeenCalledTimes(1)
    })

    await vi.waitFor(() => {
      expect(wrapper.find('.mod-detail-body').exists()).toBe(true)
    })

    const body = wrapper.find('.mod-detail-body')
    const bodyHtml = body.html()

    expect(bodyHtml).not.toContain('<script')
    expect(bodyHtml).not.toContain('<img')
    expect(bodyHtml).not.toContain('onerror=')
    expect(bodyHtml).not.toContain('onclick=')
    expect(bodyHtml).not.toContain('javascript:')
    expect(body.find('strong').exists()).toBe(true)
    expect(body.find('em').exists()).toBe(true)
    expect(body.find('code').exists()).toBe(true)
    expect(body.find('ul').exists()).toBe(true)
    expect(body.findAll('li')).toHaveLength(1)

    const safeLink = body.find('a[href="https://example.com"]')
    expect(safeLink.exists()).toBe(true)
    expect(safeLink.text()).toBe('safe link')
  })

  it('falls back to the summary and hides an empty author line', async () => {
    mockGetModDetails.mockResolvedValue({
      details: makeModDetails({ author: '', body: '', description: 'Keeps old clients online.' }),
    })
    mockGetModVersions.mockResolvedValue({ versions: [makeModVersion()] })

    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })

    await vi.waitFor(() => {
      expect(wrapper.find('.mod-detail-body').text()).toContain('Keeps old clients online.')
    })
    expect(wrapper.find('.mod-detail-author').exists()).toBe(false)
  })

  it('selects the newest version that supports the server and emits it on install', async () => {
    const snapshot = makeModVersion({
      versionId: 'v3',
      versionString: '3.0.0',
      gameVersions: ['1.21.5'],
    })
    const compatible = makeModVersion({
      versionId: 'v2',
      versionString: '2.0.0',
      gameVersions: ['1.20.4', '1.21.4'],
    })
    mockGetModDetails.mockResolvedValue({ details: makeModDetails() })
    mockGetModVersions.mockResolvedValue({
      versions: [snapshot, compatible, makeModVersion({ gameVersions: ['1.21.4'] })],
    })

    const wrapper = mountDialog()
    await wrapper.setProps({ gameVersion: '1.21.4', show: true })

    await vi.waitFor(() => {
      expect(wrapper.findAll('.mod-version-match')).toHaveLength(2)
    })
    expect(wrapper.find('.mod-version-game').text()).toBe('1.21.5')

    const installButton = wrapper.findAll('button').find((button) => button.text() === 'Install')
    await installButton?.trigger('click')

    expect(wrapper.emitted('install')?.[0]).toEqual([
      'modrinth',
      'abc123',
      compatible,
      'Sample Mod',
    ])
  })

  it('keeps the dialog heading on a failed load and retries', async () => {
    mockGetModDetails.mockRejectedValueOnce(new Error('provider down'))
    mockGetModVersions.mockResolvedValue({ versions: [makeModVersion()] })

    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })

    await vi.waitFor(() => {
      expect(wrapper.find('#mod-detail-dialog-title').text()).toBe(
        'Mod details could not be loaded',
      )
    })

    mockGetModDetails.mockResolvedValue({ details: makeModDetails() })
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Retry')
      ?.trigger('click')

    await vi.waitFor(() => {
      expect(wrapper.find('#mod-detail-dialog-title').text()).toBe('Sample Mod')
    })
    expect(mockGetModDetails).toHaveBeenCalledTimes(2)
  })

  it('offers Close while loading and ignores a late response for a closed mod', async () => {
    let resolveOldDetails!: (value: { details: ModDetails }) => void
    let resolveOldVersions!: (value: { versions: ModVersion[] }) => void
    mockGetModDetails.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOldDetails = resolve
      }),
    )
    mockGetModVersions.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveOldVersions = resolve
      }),
    )

    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Close')
      ?.trigger('click')
    expect(wrapper.emitted('update:show')?.[0]).toEqual([false])

    await wrapper.setProps({ show: false })
    mockGetModDetails.mockResolvedValueOnce({ details: makeModDetails({ name: 'New Mod' }) })
    mockGetModVersions.mockResolvedValueOnce({
      versions: [makeModVersion({ versionId: 'new-v1' })],
    })
    await wrapper.setProps({ sourceId: 'new456', show: true })
    await vi.waitFor(() => {
      expect(wrapper.find('#mod-detail-dialog-title').text()).toBe('New Mod')
    })

    resolveOldDetails({ details: makeModDetails({ name: 'Old Mod' }) })
    resolveOldVersions({ versions: [makeModVersion({ versionId: 'old-v1' })] })
    await flushPromises()

    expect(wrapper.find('#mod-detail-dialog-title').text()).toBe('New Mod')
    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Install')
      ?.trigger('click')
    expect(wrapper.emitted('install')?.[0]?.[2]).toMatchObject({ versionId: 'new-v1' })
  })

  it('renders the newest 20 versions until Show all is pressed', async () => {
    mockGetModDetails.mockResolvedValue({ details: makeModDetails() })
    mockGetModVersions.mockResolvedValue({
      versions: Array.from({ length: 25 }, (_, i) =>
        makeModVersion({ versionId: `v${i}`, versionString: `1.0.${i}` }),
      ),
    })

    const wrapper = mountDialog()
    await wrapper.setProps({ show: true })

    await vi.waitFor(() => {
      expect(wrapper.findAll('.mod-version-row')).toHaveLength(20)
    })

    await wrapper
      .findAll('button')
      .find((button) => button.text() === 'Show all 25 versions')
      ?.trigger('click')

    expect(wrapper.findAll('.mod-version-row')).toHaveLength(25)
  })
})
