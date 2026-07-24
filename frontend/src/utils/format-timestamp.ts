import type { Timestamp } from '@bufbuild/protobuf/wkt'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import dayjs from 'dayjs'

/** Canonical 24-hour timestamp format used across the UI. */
export const TIMESTAMP_FORMAT = 'MMM D, YYYY HH:mm:ss'

/**
 * Formats a protobuf Timestamp or JS Date with the canonical 24-hour format.
 * Returns `fallback` for missing or invalid input.
 */
export function formatTimestamp(
  input: Timestamp | Date | undefined,
  fallback: string = '',
): string {
  if (input === undefined) {
    return fallback
  }
  const date = input instanceof Date ? input : timestampDate(input)
  const parsed = dayjs(date)
  if (!parsed.isValid()) {
    return fallback
  }
  return parsed.format(TIMESTAMP_FORMAT)
}
