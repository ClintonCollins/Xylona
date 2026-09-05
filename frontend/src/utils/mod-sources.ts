/** Mod source badge configuration shared across mod components. */
export const SOURCE_BADGES: Record<
  string,
  { bg: string; fg: string; letter: string; name: string }
> = {
  modrinth: { bg: '#1BD96A', fg: 'var(--xy-base)', letter: 'M', name: 'Modrinth' },
  hangar: { bg: '#2196F3', fg: 'var(--xy-base)', letter: 'H', name: 'Hangar' },
  thunderstore: {
    bg: '#0066FF',
    fg: 'var(--xy-text-on-color)',
    letter: 'T',
    name: 'Thunderstore',
  },
  steam_workshop: {
    bg: '#1B2838',
    fg: 'var(--xy-text-on-color)',
    letter: 'S',
    name: 'Steam Workshop',
  },
  papermc: { bg: '#2196F3', fg: 'var(--xy-base)', letter: 'P', name: 'PaperMC' },
}

export function sourceBadgeStyle(source: string): Record<string, string> {
  const config = SOURCE_BADGES[source]
  if (!config) return { backgroundColor: 'var(--xy-surface-3)', color: 'var(--xy-text-primary)' }
  return { backgroundColor: config.bg, color: config.fg }
}

export function sourceLabel(source: string): string {
  return SOURCE_BADGES[source]?.letter ?? source.charAt(0).toUpperCase()
}

export function sourceDisplayName(source: string): string {
  return SOURCE_BADGES[source]?.name ?? source
}

export function iconGradient(name: string): string {
  let hash = 0
  for (let i = 0; i < name.length; i++) {
    hash = name.charCodeAt(i) + ((hash << 5) - hash)
  }
  const hue1 = Math.abs(hash) % 360
  const hue2 = (hue1 + 40) % 360
  return `linear-gradient(135deg, hsl(${hue1}, 60%, 40%), hsl(${hue2}, 60%, 30%))`
}

export function formatDownloads(downloads: bigint): string {
  const num = Number(downloads)
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`
  return num.toString()
}
