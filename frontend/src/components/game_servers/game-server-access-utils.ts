export function formatProtoTimestamp(ts?: { seconds: bigint }): string {
  if (!ts || ts.seconds == null) {
    return 'Unknown time'
  }
  return new Date(Number(ts.seconds) * 1000).toLocaleString()
}
