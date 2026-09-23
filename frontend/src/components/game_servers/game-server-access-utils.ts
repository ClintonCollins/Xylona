import { formatTimestamp } from '@/utils/format-timestamp'

export function formatProtoTimestamp(ts?: { seconds: bigint }): string {
  if (!ts || ts.seconds == null) {
    return 'Unknown time'
  }
  return formatTimestamp(new Date(Number(ts.seconds) * 1000), 'Unknown time')
}
