import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { ScheduledTaskLogSchema, ScheduledTaskSchema } from '@/proto/shared_pb'
import {
  GetGameServerBackupOverviewResponseSchema,
  GetScheduledTaskLogsResponseSchema,
  ListScheduledTasksResponseSchema,
} from '@/proto/xylona_pb'
import GameServerSchedules from './GameServerSchedules.vue'

const mocks = vi.hoisted(() => ({
  getBackupOverview: vi.fn(),
  getScheduledTaskLogs: vi.fn(),
  listScheduledTasks: vi.fn(),
  mobile: true,
  notify: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      dialog: vi.fn(),
      notify: mocks.notify,
      screen: {
        lt: {
          get md() {
            return mocks.mobile
          },
        },
      },
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getGameServerBackupOverview: mocks.getBackupOverview,
    getScheduledTaskLogs: mocks.getScheduledTaskLogs,
    listScheduledTasks: mocks.listScheduledTasks,
  }),
}))

describe('GameServerSchedules', () => {
  afterEach(() => {
    mocks.mobile = true
    mocks.getBackupOverview.mockReset()
    mocks.getScheduledTaskLogs.mockReset()
    mocks.listScheduledTasks.mockReset()
    mocks.notify.mockReset()
  })

  it('loads latest results and fetches recent history only on demand', async () => {
    const restartTask = create(ScheduledTaskSchema, {
      id: 'restart-task',
      name: 'Nightly restart',
      taskType: 'restart',
      cronExpression: '0 3 * * *',
      timezone: 'UTC',
      enabled: true,
    })
    const consoleTask = create(ScheduledTaskSchema, {
      id: 'console-task',
      name: 'Save world',
      taskType: 'console_command',
      cronExpression: '*/15 * * * *',
      timezone: 'UTC',
      enabled: true,
    })
    const restartLog = create(ScheduledTaskLogSchema, {
      id: 'restart-log',
      scheduledTaskId: 'restart-task',
      status: 'timed_out',
      message: 'Server did not reach OFFLINE before the timeout.',
    })
    const consoleLog = create(ScheduledTaskLogSchema, {
      id: 'console-log',
      scheduledTaskId: 'console-task',
      taskType: 'console_command',
      status: 'success',
    })
    mocks.listScheduledTasks.mockResolvedValue(
      create(ListScheduledTasksResponseSchema, {
        tasks: [restartTask, consoleTask],
        latestLogs: [restartLog, consoleLog],
      }),
    )
    mocks.getBackupOverview.mockResolvedValue(create(GetGameServerBackupOverviewResponseSchema, {}))
    mocks.getScheduledTaskLogs.mockImplementation((request: { scheduledTaskId: string }) => {
      if (request.scheduledTaskId === 'restart-task') {
        return Promise.resolve(
          create(GetScheduledTaskLogsResponseSchema, {
            logs: [restartLog],
          }),
        )
      }
      return Promise.resolve(
        create(GetScheduledTaskLogsResponseSchema, {
          logs: [consoleLog],
        }),
      )
    })

    const wrapper = shallowMount(GameServerSchedules, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          QBadge: {
            props: ['label'],
            template: '<span>{{ label }}</span>',
          },
          QTable: {
            props: ['rows'],
            template:
              '<div><template v-for="row in rows" :key="row.id"><slot name="item" :row="row" /></template></div>',
          },
        },
      },
    })
    await flushPromises()

    expect(mocks.getScheduledTaskLogs).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('Server did not reach OFFLINE before the timeout.')
    expect(wrapper.text()).toContain('Execution details are hidden or unavailable.')
    expect(wrapper.text()).not.toContain('save-all')

    const viewModel = wrapper.vm as unknown as {
      loadTaskLogsIfNeeded: (taskID: string) => Promise<void>
    }
    await viewModel.loadTaskLogsIfNeeded('restart-task')

    expect(mocks.getScheduledTaskLogs).toHaveBeenCalledTimes(1)
    expect(mocks.getScheduledTaskLogs).toHaveBeenCalledWith(
      expect.objectContaining({ scheduledTaskId: 'restart-task', limit: 10 }),
    )
  })

  it('renders recent execution results in the desktop table body', async () => {
    mocks.mobile = false
    const task = create(ScheduledTaskSchema, {
      id: 'backup-task',
      name: 'Nightly backup',
      taskType: 'backup',
      cronExpression: '0 2 * * *',
      timezone: 'UTC',
      enabled: true,
    })
    const latestLog = create(ScheduledTaskLogSchema, {
      id: 'backup-log',
      scheduledTaskId: 'backup-task',
      status: 'failed',
      message: 'Backup destination is unavailable.',
    })
    mocks.listScheduledTasks.mockResolvedValue(
      create(ListScheduledTasksResponseSchema, { tasks: [task], latestLogs: [latestLog] }),
    )
    mocks.getBackupOverview.mockResolvedValue(create(GetGameServerBackupOverviewResponseSchema, {}))
    mocks.getScheduledTaskLogs.mockResolvedValue(
      create(GetScheduledTaskLogsResponseSchema, {
        logs: [latestLog],
      }),
    )

    const wrapper = shallowMount(GameServerSchedules, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          QBadge: {
            props: ['label'],
            template: '<span>{{ label }}</span>',
          },
          QTable: {
            props: ['rows'],
            template:
              '<div><template v-for="row in rows" :key="row.id"><slot name="body" :row="row" :expand="false" /></template></div>',
          },
        },
      },
    })
    await flushPromises()

    expect(wrapper.text()).toContain('Backup destination is unavailable.')
    expect(wrapper.text()).toContain('Failed')
    expect(mocks.getScheduledTaskLogs).not.toHaveBeenCalled()
  })
})
