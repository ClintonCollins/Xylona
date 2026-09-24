import { describe, expect, it } from 'vitest'

import { AlertEventType } from '@/proto/shared_pb'

import { alertEventTypeLabel, alertTargetName, isNodeAlertEventType } from './alert-scope'

describe('alertEventTypeLabel', () => {
  it.each([
    { type: AlertEventType.CRASH, want: 'Server Crash' },
    { type: AlertEventType.PLAYER_COUNT_THRESHOLD, want: 'Player Count Threshold' },
    { type: AlertEventType.NODE_DISK_THRESHOLD, want: 'Node Disk Threshold' },
    { type: AlertEventType.UNSPECIFIED, want: 'Unknown' },
  ])('$type → $want', ({ type, want }) => {
    expect(alertEventTypeLabel(type)).toBe(want)
  })
})

describe('isNodeAlertEventType', () => {
  it.each([
    { type: AlertEventType.NODE_CPU_THRESHOLD, want: true },
    { type: AlertEventType.NODE_MEMORY_THRESHOLD, want: true },
    { type: AlertEventType.NODE_DISK_THRESHOLD, want: true },
    { type: AlertEventType.CPU_THRESHOLD, want: false },
    { type: AlertEventType.CRASH, want: false },
  ])('$type → $want', ({ type, want }) => {
    expect(isNodeAlertEventType(type)).toBe(want)
  })
})

describe('alertTargetName', () => {
  const servers = [{ id: 'server-1', name: 'Survival' }]
  const nodes = [
    { id: 'node-1', name: 'Rack A' },
    { id: 'node-2', name: '' },
  ]

  it.each([
    {
      name: 'node rule names its node',
      entry: { eventType: AlertEventType.NODE_CPU_THRESHOLD, nodeId: 'node-1' },
      want: 'Rack A',
    },
    {
      name: 'unnamed or unknown node falls back to its id',
      entry: { eventType: AlertEventType.NODE_DISK_THRESHOLD, nodeId: 'node-2' },
      want: 'node-2',
    },
    {
      name: 'node rule without a node watches every node',
      entry: { eventType: AlertEventType.NODE_MEMORY_THRESHOLD },
      want: 'All Nodes',
    },
    {
      name: 'server rule names its server',
      entry: { eventType: AlertEventType.CRASH, serverId: 'server-1' },
      want: 'Survival',
    },
    {
      name: 'unknown server falls back to its id',
      entry: { eventType: AlertEventType.CPU_THRESHOLD, serverId: 'server-9' },
      want: 'server-9',
    },
    {
      name: 'server rule without a server watches every server',
      entry: { eventType: AlertEventType.STATUS_CHANGE },
      want: 'All Servers',
    },
  ])('$name', ({ entry, want }) => {
    expect(alertTargetName(entry, servers, nodes)).toBe(want)
  })
})
