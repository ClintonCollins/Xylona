import { describe, expect, it } from 'vitest'

import { AlertEventType } from '@/proto/shared_pb'

import { formatAlertEventData, formatCondition } from './alert-conditions'

describe('formatCondition', () => {
  it.each([
    {
      name: 'crash without a condition',
      type: AlertEventType.CRASH,
      condition: '',
      want: 'Any crash',
    },
    {
      name: 'status change without a condition',
      type: AlertEventType.STATUS_CHANGE,
      condition: '',
      want: 'Any status',
    },
    {
      name: 'status change with an empty list',
      type: AlertEventType.STATUS_CHANGE,
      condition: '{"statuses":[]}',
      want: 'Any status',
    },
    {
      name: 'status change with statuses',
      type: AlertEventType.STATUS_CHANGE,
      condition: '{"statuses":["ONLINE","OFFLINE"]}',
      want: 'Online, Offline',
    },
    {
      name: 'percent threshold with behavior',
      type: AlertEventType.NODE_CPU_THRESHOLD,
      condition: '{"operator":">=","value":90,"for_seconds":300,"recovery_value":80}',
      want: '>= 90% · for 5m · recover at 80%',
    },
    {
      name: 'player threshold has no unit',
      type: AlertEventType.PLAYER_COUNT_THRESHOLD,
      condition: '{"operator":">","value":10}',
      want: '> 10',
    },
    { name: 'text that is not JSON', type: AlertEventType.CRASH, condition: 'raw', want: 'raw' },
  ])('$name', ({ type, condition, want }) => {
    expect(formatCondition(type, condition)).toBe(want)
  })
})

describe('formatAlertEventData', () => {
  it.each([
    {
      name: 'threshold entered',
      type: AlertEventType.CPU_THRESHOLD,
      data: '{"current_value":92.345,"threshold":90,"direction":"entered"}',
      want: '92.3% (threshold 90%)',
    },
    {
      name: 'threshold resolved',
      type: AlertEventType.MEMORY_THRESHOLD,
      data: '{"current_value":41,"threshold":90,"direction":"resolved"}',
      want: 'Recovered: 41% (threshold 90%)',
    },
    {
      name: 'player count threshold',
      type: AlertEventType.PLAYER_COUNT_THRESHOLD,
      data: '{"current_value":12,"threshold":10}',
      want: '12 (threshold 10)',
    },
    {
      name: 'crash exit code',
      type: AlertEventType.CRASH,
      data: '{"exit_code":1}',
      want: 'Exit code 1',
    },
    {
      name: 'status transition',
      type: AlertEventType.STATUS_CHANGE,
      data: '{"new_status":"OFFLINE","old_status":"ONLINE"}',
      want: 'Online → Offline',
    },
    {
      name: 'status without a previous status',
      type: AlertEventType.STATUS_CHANGE,
      data: '{"new_status":"PRE_START"}',
      want: 'Starting',
    },
    {
      name: 'status the app has no label for',
      type: AlertEventType.STATUS_CHANGE,
      data: '{"new_status":"SHUTTING_DOWN"}',
      want: 'Shutting Down',
    },
    { name: 'empty data', type: AlertEventType.CRASH, data: '', want: '-' },
    { name: 'text that is not JSON', type: AlertEventType.CRASH, data: 'boom', want: 'boom' },
    {
      name: 'unexpected shape falls back to the raw data',
      type: AlertEventType.DISK_THRESHOLD,
      data: '{"foo":1}',
      want: '{"foo":1}',
    },
  ])('$name', ({ type, data, want }) => {
    expect(formatAlertEventData(type, data)).toBe(want)
  })
})
