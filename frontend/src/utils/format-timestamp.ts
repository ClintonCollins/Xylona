import type { Timestamp } from '@bufbuild/protobuf/wkt'
import { timestampDate } from '@bufbuild/protobuf/wkt'

/** Canonical 24-hour timestamp format used across the UI. */
export const TIMESTAMP_FORMAT = 'MMM D, YYYY HH:mm:ss'

const timestampFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
  hour12: false,
})

const dateFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

function formatWith(
  formatter: Intl.DateTimeFormat,
  input: Timestamp | Date | undefined,
  fallback: string,
  includeTime: boolean,
): string {
  if (input === undefined) {
    return fallback
  }
  const date = input instanceof Date ? input : timestampDate(input)
  if (Number.isNaN(date.getTime())) {
    return fallback
  }

  const parts = Object.fromEntries(
    formatter.formatToParts(date).map((part) => [part.type, part.value]),
  )
  const dateText = `${parts['month']} ${parts['day']}, ${parts['year']}`
  if (!includeTime) {
    return dateText
  }
  return `${dateText} ${parts['hour']}:${parts['minute']}:${parts['second']}`
}

/**
 * Formats a protobuf Timestamp or JS Date with the canonical 24-hour format.
 * Returns `fallback` for missing or invalid input.
 */
export function formatTimestamp(
  input: Timestamp | Date | undefined,
  fallback: string = '',
): string {
  return formatWith(timestampFormatter, input, fallback, true)
}

/** Formats a protobuf Timestamp or JS Date as a date-only label. */
export function formatDate(input: Timestamp | Date | undefined, fallback: string = ''): string {
  return formatWith(dateFormatter, input, fallback, false)
}
