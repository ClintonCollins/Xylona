/** Mod source badge configuration shared across mod components. */
export const SOURCE_BADGES: Record<
  string,
  { bg: string; fg: string; letter: string; name: string }
> = {
  modrinth: { bg: 'var(--xy-category-2)', fg: 'var(--xy-base)', letter: 'M', name: 'Modrinth' },
  hangar: { bg: 'var(--xy-category-1)', fg: 'var(--xy-base)', letter: 'H', name: 'Hangar' },
  thunderstore: {
    bg: 'var(--xy-category-6)',
    fg: 'var(--xy-base)',
    letter: 'T',
    name: 'Thunderstore',
  },
  steam_workshop: {
    bg: 'var(--xy-surface-4)',
    fg: 'var(--xy-text-primary)',
    letter: 'S',
    name: 'Steam Workshop',
  },
  papermc: { bg: 'var(--xy-category-1)', fg: 'var(--xy-base)', letter: 'P', name: 'PaperMC' },
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
  // Mixed toward the base so the white initial stays above 5:1 on every category.
  const category = `var(--xy-category-${(Math.abs(hash) % 8) + 1})`
  return `linear-gradient(135deg, color-mix(in srgb, ${category} 60%, var(--xy-base)), color-mix(in srgb, ${category} 40%, var(--xy-base)))`
}

export function formatDownloads(downloads: bigint): string {
  const num = Number(downloads)
  if (num >= 1_000_000) return `${(num / 1_000_000).toFixed(1)}M`
  if (num >= 1_000) return `${(num / 1_000).toFixed(1)}K`
  return num.toString()
}
