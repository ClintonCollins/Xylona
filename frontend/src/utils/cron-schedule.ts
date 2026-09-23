import cronstrue from 'cronstrue'

/** Describes a cron expression on the 24-hour clock, or returns null when it cannot be read. */
export function describeCron(expression: string): string | null {
  try {
    return cronstrue.toString(expression, { use24HourTimeFormat: true })
  } catch {
    return null
  }
}

interface FieldBounds {
  min: number
  max: number
  names?: string[]
}

interface CronField {
  values: Set<number>
  // True when the field is an unrestricted `*` or `?`, which decides how the
  // day-of-month and day-of-week fields combine.
  star: boolean
}

// Mirrors the controller's parser (robfig/cron ParseStandard): weekdays are
// 0-6 and three-letter month and weekday names are accepted.
const fieldBounds: FieldBounds[] = [
  { min: 0, max: 59 },
  { min: 0, max: 23 },
  { min: 1, max: 31 },
  {
    min: 1,
    max: 12,
    names: ['jan', 'feb', 'mar', 'apr', 'may', 'jun', 'jul', 'aug', 'sep', 'oct', 'nov', 'dec'],
  },
  { min: 0, max: 6, names: ['sun', 'mon', 'tue', 'wed', 'thu', 'fri', 'sat'] },
]

function parseValue(text: string, min: number, names?: string[]): number {
  const named = names?.indexOf(text.toLowerCase()) ?? -1
  if (named >= 0) return named + min
  return /^\d+$/.test(text) ? Number(text) : NaN
}

function parseField(text: string, { min, max, names }: FieldBounds): CronField | null {
  const values = new Set<number>()
  let star = false
  for (const part of text.split(',')) {
    const [range = '', stepText, extra] = part.split('/')
    if (extra !== undefined) return null
    const step = stepText === undefined ? 1 : parseValue(stepText, 0)
    const ends = range.split('-')
    let start = min
    let end = max
    if (range === '*' || range === '?') {
      star ||= step <= 1
    } else {
      if (ends.length > 2) return null
      start = parseValue(ends[0] ?? '', min, names)
      if (ends.length === 2) end = parseValue(ends[1] ?? '', min, names)
      else if (stepText === undefined) end = start
    }
    // Written so NaN fails every comparison.
    if (!(step > 0 && start >= min && end <= max && start <= end)) return null
    for (let value = start; value <= end; value += step) values.add(value)
  }
  return { values, star }
}

const minuteMs = 60_000
// The controller gives up after five years, so a schedule with no run in that
// window is treated as never running.
const searchLimitMs = 5 * 366 * 24 * 60 * minuteMs

/**
 * Returns the first run of a 5-field cron expression strictly after `from`,
 * evaluated on the wall clock of `timeZone`, or null when the expression or
 * zone is invalid or the schedule never fires.
 */
export function nextCronRun(
  expression: string,
  timeZone: string,
  from: Date = new Date(),
): Date | null {
  const parts = expression.trim().split(/\s+/)
  if (parts.length !== 5) return null
  const fields = fieldBounds.map((bounds, index) => parseField(parts[index] ?? '', bounds))
  if (fields.some((field) => field === null)) return null
  const [minute, hour, dom, month, dow] = fields as [
    CronField,
    CronField,
    CronField,
    CronField,
    CronField,
  ]

  let wallClock: Intl.DateTimeFormat
  try {
    wallClock = new Intl.DateTimeFormat('en-US', {
      timeZone,
      hourCycle: 'h23',
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      hour: 'numeric',
      minute: 'numeric',
    })
  } catch {
    return null
  }

  let time = (Math.floor(from.getTime() / minuteMs) + 1) * minuteMs
  const limit = time + searchLimitMs
  while (time < limit) {
    const wall = wallTime(wallClock, time)
    const weekday = new Date(Date.UTC(wall.year, wall.month - 1, wall.day)).getUTCDay()
    const domMatch = dom.values.has(wall.day)
    const dowMatch = dow.values.has(weekday)
    const dayMatch = dom.star || dow.star ? domMatch && dowMatch : domMatch || dowMatch

    if (!month.values.has(wall.month) || !dayMatch) {
      // Skip to about 23:00, then hour by hour past midnight, so a daylight
      // saving shift cannot carry the search past the next day's first hour.
      time += (wall.hour < 23 ? (23 - wall.hour) * 60 - wall.minute : 60 - wall.minute) * minuteMs
    } else if (!hour.values.has(wall.hour)) {
      time += (60 - wall.minute) * minuteMs
    } else if (!minute.values.has(wall.minute)) {
      time += minuteMs
    } else {
      return new Date(time)
    }
  }
  return null
}

function wallTime(format: Intl.DateTimeFormat, instant: number) {
  const parts = Object.fromEntries(
    format.formatToParts(instant).map((part) => [part.type, Number(part.value)]),
  )
  const read = (type: string): number => parts[type] ?? NaN
  return {
    year: read('year'),
    month: read('month'),
    day: read('day'),
    hour: read('hour'),
    minute: read('minute'),
  }
}
