import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'
import { ExportGameResponseSchema, type ExportGameRequest } from '@/proto/xylona_pb'
import GameList from './GameList.vue'

const mocks = vi.hoisted(() => ({
  exportGame: vi.fn(),
  listGames: vi.fn(),
  notify: vi.fn(),
  push: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      exportGame: mocks.exportGame,
      listGames: mocks.listGames,
    }),
  }
})

vi.mock('@/utils/persisted-ref', async () => {
  const vue = await vi.importActual<typeof import('vue')>('vue')
  return {
    usePersistedRef: (_key: string, initialValue: unknown) => vue.ref(initialValue),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
      screen: {
        lt: {
          md: false,
        },
      },
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      push: mocks.push,
    }),
  }
})

const QTableStub = defineComponent({
  name: 'QTableStub',
  props: {
    rowKey: {
      type: String,
      default: '',
    },
    rows: {
      type: Array,
      default: () => [],
    },
  },
  template:
    '<div data-testid="games-table" :data-row-key="rowKey"><div v-for="row in rows" :key="row[rowKey]" class="game-row"><slot name="body-cell-name" :row="row" /><slot name="body-cell-actions" :row="row" /></div><slot v-if="rows.length === 0" name="no-data" /></div>',
})

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: {
    label: {
      type: String,
      default: '',
    },
    ariaLabel: {
      type: String,
      default: '',
    },
  },
  emits: ['click'],
  template:
    '<button :aria-label="ariaLabel" @click="$emit(\'click\')"><slot />{{ label }}</button>',
})

const GameImportDialogStub = defineComponent({
  name: 'GameImportDialogStub',
  emits: ['imported'],
  template:
    '<button data-testid="finish-import" type="button" @click="$emit(\'imported\', \'imported-game\')">Finish import</button>',
})

function mountGameList() {
  return mount(GameList, {
    global: {
      stubs: {
        'q-page': { template: '<main><slot /></main>' },
        'q-input': { template: '<label><slot name="append" /></label>' },
        'q-icon': true,
        'q-btn': QBtnStub,
        'q-tooltip': true,
        'q-table': QTableStub,
        'q-td': { template: '<span><slot /></span>' },
        'q-badge': { template: '<span>{{ label }}</span>', props: ['label'] },
        'router-link': { template: '<a><slot /></a>' },
        GameDeleteDialog: { template: '<div />' },
        GameImportDialog: GameImportDialogStub,
      },
    },
  })
}

describe('GameList', () => {
  beforeEach(() => {
    mocks.exportGame.mockReset()
    mocks.listGames.mockReset()
    mocks.notify.mockReset()
    mocks.push.mockReset()
    window.URL.createObjectURL = vi.fn(() => 'blob:game-definition')
    window.URL.revokeObjectURL = vi.fn()

    mocks.listGames.mockResolvedValue({
      games: [create(GameSchema, { id: 'minecraft', name: 'Minecraft' })],
    })
    mocks.exportGame.mockResolvedValue(
      create(ExportGameResponseSchema, {
        fileName: 'minecraft.game.json',
        gameDefinitionJson: '{"document_type":"xylona.game_definition"}',
      }),
    )
  })

  it('exports a game definition JSON file from the row action', async () => {
    const wrapper = mountGameList()
    await flushPromises()

    await wrapper.get('[aria-label="Export game JSON"]').trigger('click')
    await flushPromises()

    expect((mocks.exportGame.mock.calls[0][0] as ExportGameRequest).gameId).toBe('minecraft')
    expect(window.URL.createObjectURL).toHaveBeenCalled()
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ caption: 'Exported minecraft.game.json.' }),
    )
  })

  it('refreshes games and routes to the imported definition after import', async () => {
    const wrapper = mountGameList()
    await flushPromises()

    await wrapper.get('[data-testid="finish-import"]').trigger('click')
    await flushPromises()

    expect(mocks.listGames).toHaveBeenCalledTimes(2)
    expect(mocks.push).toHaveBeenCalledWith('/games/imported-game/edit')
  })

  it('uses game IDs as row keys when imported copies share a name', async () => {
    mocks.listGames.mockResolvedValue({
      games: [
        create(GameSchema, { id: 'minecraft', name: 'Minecraft' }),
        create(GameSchema, { id: 'minecraft_import', name: 'Minecraft' }),
      ],
    })

    const wrapper = mountGameList()
    await flushPromises()

    expect(wrapper.get('[data-testid="games-table"]').attributes('data-row-key')).toBe('id')
    expect(wrapper.findAll('.game-row')).toHaveLength(2)
  })
})
