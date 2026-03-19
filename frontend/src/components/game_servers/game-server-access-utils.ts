export function formatProtoTimestamp(ts?: { seconds: bigint }): string {
  if (!ts || ts.seconds == null) {
    return 'Unknown time'
  }
  return new Date(Number(ts.seconds) * 1000).toLocaleString()
}

export function hostFromBaseURL(raw: string): string {
  if (raw === '') {
    return ''
  }
  try {
    const parsed = new URL(raw)
    if (parsed.host !== '') {
      return parsed.host
    }
  } catch {
    // URL parsing failed, fall through to string manipulation below
  }
  return raw.replace('https://', '').replace('http://', '')
}
