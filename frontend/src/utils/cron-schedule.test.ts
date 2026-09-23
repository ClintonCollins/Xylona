import { describe, expect, it } from 'vitest'

import { describeCron, nextCronRun } from './cron-schedule'

describe('describeCron', () => {
  it('uses the 24-hour clock', () => {
    expect(describeCron('0 15 * * *')).toBe('At 15:00')
  })

  it('returns null for an unreadable expression', () => {
    expect(describeCron('99 3 * * *')).toBeNull()
  })
})

describe('nextCronRun', () => {
  // Tuesday, Sep 22 2026, 12:00 UTC (07:00 in Chicago, CDT).
  const from = new Date(Date.UTC(2026, 8, 22, 12, 0, 0))

  it.each([
    {
      name: 'daily in UTC',
      cron: '0 3 * * *',
      zone: 'UTC',
      want: '2026-09-23T03:00:00.000Z',
    },
    {
      name: 'daily on the schedule zone clock',
      cron: '0 3 * * *',
      zone: 'America/Chicago',
      want: '2026-09-23T08:00:00.000Z',
    },
    {
      name: 'later today in the schedule zone',
      cron: '30 15 * * *',
      zone: 'America/Chicago',
      want: '2026-09-22T20:30:00.000Z',
    },
    {
      name: 'every 15 minutes',
      cron: '*/15 * * * *',
      zone: 'UTC',
      want: '2026-09-22T12:15:00.000Z',
    },
    {
      name: 'weekday names',
      cron: '0 3 * * sat,sun',
      zone: 'UTC',
      want: '2026-09-26T03:00:00.000Z',
    },
    {
      name: 'day of month',
      cron: '0 3 1 * *',
      zone: 'UTC',
      want: '2026-10-01T03:00:00.000Z',
    },
    {
      name: 'restricted day of month and weekday match either',
      cron: '0 3 1 * 5',
      zone: 'UTC',
      want: '2026-09-25T03:00:00.000Z',
    },
    {
      name: 'the day after a daylight saving change keeps its first hour',
      cron: '30 0 * * 1',
      zone: 'America/Chicago',
      // Chicago springs forward on Sunday Mar 8 2026.
      from: new Date(Date.UTC(2026, 2, 7, 12, 0, 0)),
      want: '2026-03-09T05:30:00.000Z',
    },
  ])('$name', ({ cron, zone, from: start, want }) => {
    expect(nextCronRun(cron, zone, start ?? from)?.toISOString()).toBe(want)
  })

  it('runs strictly after the start time', () => {
    const atThree = new Date(Date.UTC(2026, 8, 23, 3, 0, 0))
    expect(nextCronRun('0 3 * * *', 'UTC', atThree)?.toISOString()).toBe('2026-09-24T03:00:00.000Z')
  })

  it.each([
    { name: 'minute out of range', cron: '99 3 * * *', zone: 'UTC' },
    { name: 'weekday 7', cron: '0 3 * * 7', zone: 'UTC' },
    { name: 'too few fields', cron: '0 3 * *', zone: 'UTC' },
    { name: 'zero step', cron: '*/0 * * * *', zone: 'UTC' },
    { name: 'a date that never exists', cron: '0 0 30 2 *', zone: 'UTC' },
    { name: 'unknown zone', cron: '0 3 * * *', zone: 'Mars/Olympus' },
  ])('returns null for $name', ({ cron, zone }) => {
    expect(nextCronRun(cron, zone, from)).toBeNull()
  })
})
