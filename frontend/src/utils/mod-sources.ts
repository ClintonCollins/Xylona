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
