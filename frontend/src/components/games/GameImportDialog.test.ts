import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'
import {
  GameImportMode,
  ImportGameResponseSchema,
  type ImportGameRequest,
  type ImportGameResponse,
} from '@/proto/xylona_pb'
import GameImportDialog from './GameImportDialog.vue'

const mocks = vi.hoisted(() => ({
  importGame: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      importGame: mocks.importGame,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

const QDialogStub = defineComponent({
  name: 'QDialogStub',
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  template: '<div v-if="modelValue"><slot /></div>',
})

const QFileStub = defineComponent({
  name: 'QFileStub',
  emits: ['update:modelValue'],
  template: '<div data-testid="file-input"><slot name="prepend" /></div>',
})

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: {
    label: {
      type: String,
      default: '',
    },
    disable: {
      type: Boolean,
      default: false,
    },
  },
  emits: ['click'],
  template: '<button :disabled="disable" @click="$emit(\'click\')"><slot />{{ label }}</button>',
})

const QBtnToggleStub = defineComponent({
  name: 'QBtnToggleStub',
  props: {
    modelValue: {
      type: Number,
      required: true,
    },
    options: {
      type: Array,
      required: true,
    },
  },
  emits: ['update:modelValue'],
  template:
    '<div><button v-for="option in options" :key="option.value" type="button" @click="$emit(\'update:modelValue\', option.value)">{{ option.label }}</button></div>',
})

function mountImportDialog() {
  return mount(GameImportDialog, {
    props: {
      showDialog: true,
    },
    global: {
      stubs: {
        'q-dialog': QDialogStub,
        'q-card': { template: '<section><slot /></section>' },
        'q-card-section': { template: '<div><slot /></div>' },
        'q-card-actions': { template: '<div><slot /></div>' },
        'q-separator': true,
        'q-file': QFileStub,
        'q-icon': true,
        'q-btn': QBtnStub,
        'q-btn-toggle': QBtnToggleStub,
        'q-badge': { template: '<span>{{ label }}</span>', props: ['label'] },
        'q-banner': { template: '<div><slot /></div>' },
        'q-chip': { template: '<span><slot /></span>' },
      },
    },
  })
}

function makeFile(contents: string): File {
  return new File([contents], 'game.json', { type: 'application/json' })
}

describe('GameImportDialog', () => {
  beforeEach(() => {
    mocks.importGame.mockReset()
    mocks.notify.mockReset()
  })

  it('previews a JSON file and applies an existing-game conflict', async () => {
    mocks.importGame
      .mockResolvedValueOnce(
        create(ImportGameResponseSchema, {
          game: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
          idConflict: true,
          updatesExisting: true,
          affectedGameServerCount: 2n,
          affectedGameServerNames: ['Alpha', 'Beta'],
          changes: [
            {
              section: 'General',
              label: 'Name',
              path: 'game.name',
              previousValue: 'Minecraft',
              importedValue: 'Minecraft Imported',
            },
          ],
        }),
      )
      .mockResolvedValueOnce(
        create(ImportGameResponseSchema, {
          game: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
          success: true,
          importedGameId: 'minecraft',
        }),
      )

    const wrapper = mountImportDialog()
    wrapper.getComponent(QFileStub).vm.$emit('update:modelValue', makeFile('{"game":{}}'))
    await flushPromises()

    expect(wrapper.text()).toContain('Conflict')
    expect(wrapper.text()).toContain('Alpha')
    expect(wrapper.text()).toContain('Changed fields')
    expect(wrapper.text()).toContain('Minecraft Imported')
    expect(wrapper.text()).toContain('Update existing')
    expect((mocks.importGame.mock.calls[0][0] as ImportGameRequest).mode).toBe(
      GameImportMode.PREVIEW,
    )

    const importButton = wrapper.findAll('button').find((button) => button.text() === 'Import')
    await importButton?.trigger('click')
    await flushPromises()

    expect((mocks.importGame.mock.calls[1][0] as ImportGameRequest).mode).toBe(GameImportMode.APPLY)
    expect(wrapper.emitted('imported')?.[0]).toEqual(['minecraft'])
  })

  it('renders validation errors from preview and blocks import', async () => {
    mocks.importGame.mockResolvedValueOnce(
      create(ImportGameResponseSchema, {
        validationErrors: ['game.id is required'],
      }),
    )

    const wrapper = mountImportDialog()
    wrapper.getComponent(QFileStub).vm.$emit('update:modelValue', makeFile('{bad json'))
    await flushPromises()

    expect(wrapper.text()).toContain('game.id is required')
    const importButton = wrapper.findAll('button').find((button) => button.text() === 'Import')
    expect(importButton?.attributes('disabled')).toBeDefined()
  })

  it('renders a no-change state for unchanged same-id conflicts', async () => {
    mocks.importGame.mockResolvedValueOnce(
      create(ImportGameResponseSchema, {
        game: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
        idConflict: true,
        affectedGameServerCount: 1n,
        affectedGameServerNames: ['Alpha'],
        changes: [],
      }),
    )

    const wrapper = mountImportDialog()
    wrapper.getComponent(QFileStub).vm.$emit('update:modelValue', makeFile('{"game":{}}'))
    await flushPromises()

    expect(wrapper.text()).toContain('No field changes detected.')
    expect(wrapper.text()).toContain('Update existing')
    const importButton = wrapper.findAll('button').find((button) => button.text() === 'Import')
    expect(importButton?.attributes('disabled')).toBeUndefined()
  })

  it('describes copy-mode conflict changes as differences from existing', async () => {
    mocks.importGame.mockResolvedValueOnce(
      create(ImportGameResponseSchema, {
        game: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
        idConflict: true,
        changes: [
          {
            section: 'General',
            label: 'Name',
            path: 'game.name',
            previousValue: 'Minecraft',
            importedValue: 'Minecraft Copy',
          },
        ],
      }),
    )

    const wrapper = mountImportDialog()
    wrapper.getComponent(QFileStub).vm.$emit('update:modelValue', makeFile('{"game":{}}'))
    await flushPromises()

    expect(wrapper.text()).toContain('1 field will change.')

    const importCopyButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'Import copy')
    await importCopyButton?.trigger('click')

    expect(wrapper.text()).toContain('1 field differs from existing.')
    expect(wrapper.text()).not.toContain('1 field will change.')
  })

  it('keeps the current preview visible while preview refreshes', async () => {
    let resolveSecondPreview: (value: ImportGameResponse) => void = () => undefined
    mocks.importGame
      .mockResolvedValueOnce(
        create(ImportGameResponseSchema, {
          game: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
          idConflict: true,
          changes: [
            {
              section: 'General',
              label: 'Name',
              path: 'game.name',
              previousValue: 'Minecraft',
              importedValue: 'Minecraft One',
            },
          ],
        }),
      )
      .mockReturnValueOnce(
        new Promise((resolve) => {
          resolveSecondPreview = resolve
        }),
      )

    const wrapper = mountImportDialog()
    wrapper.getComponent(QFileStub).vm.$emit('update:modelValue', makeFile('{"game":{}}'))
    await flushPromises()
    expect(wrapper.text()).toContain('Minecraft One')

    const previewButton = wrapper.findAll('button').find((button) => button.text() === 'Preview')
    await previewButton?.trigger('click')

    expect(wrapper.text()).toContain('Minecraft One')

    resolveSecondPreview(
      create(ImportGameResponseSchema, {
        game: create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
        idConflict: true,
        changes: [
          {
            section: 'General',
            label: 'Name',
            path: 'game.name',
            previousValue: 'Minecraft',
            importedValue: 'Minecraft Two',
          },
        ],
      }),
    )
    await flushPromises()

    expect(wrapper.text()).toContain('Minecraft Two')
    expect(wrapper.text()).not.toContain('Minecraft One')
  })
})
