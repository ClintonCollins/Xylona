export type ConsoleFeedKind = 'server' | 'chat' | 'player'
export type ConsoleFeedFilter = 'all' | ConsoleFeedKind

export interface ConsoleFeedClassifier {
  chat?: RegExp
  player?: RegExp
}

// Line classification is a declared per-game capability, not a heuristic: a
// game without an entry here simply exposes fewer feed filters instead of
// guessing (and being wrong) about what a chat line looks like.
const consoleFeedClassifiers: Record<string, ConsoleFeedClassifier> = {
  minecraft: {
    chat: /(?:\[Not Secure]\s*)?<[A-Za-z0-9_]{1,16}>\s/,
    player: /joined the game|left the game|lost connection|UUID of player|logged in with entity id/,
  },
  palworld: {
    player: /\sjoined the server\.|\sleft the server\./,
  },
}

export function getConsoleFeedClassifier(gameId: string): ConsoleFeedClassifier | null {
  return consoleFeedClassifiers[gameId.toLowerCase()] ?? null
}

// Console lines are stored as sanitized HTML (entities escaped by
// parseConsole, then highlight spans added). Classification runs against the
// reconstructed plain text so game patterns can match what the log actually
// said, e.g. Minecraft's `<name>` chat prefix.
export function consoleLinePlainText(html: string): string {
  return html
    .replace(/<[^>]*>/g, '')
    .replaceAll('&lt;', '<')
    .replaceAll('&gt;', '>')
    .replaceAll('&quot;', '"')
    .replaceAll('&#39;', "'")
    .replaceAll('&amp;', '&')
}

export function classifyConsoleLine(
  html: string,
  classifier: ConsoleFeedClassifier | null,
): ConsoleFeedKind {
  if (classifier === null) return 'server'
  const text = consoleLinePlainText(html)
  if (classifier.chat !== undefined && classifier.chat.test(text)) return 'chat'
  if (classifier.player !== undefined && classifier.player.test(text)) return 'player'
  return 'server'
}

export interface ConsoleFeedFilterOption {
  value: ConsoleFeedFilter
  label: string
}

export function getConsoleFeedFilterOptions(input: {
  classifier: ConsoleFeedClassifier | null
  playerEventsAvailable: boolean
}): ConsoleFeedFilterOption[] {
  const options: ConsoleFeedFilterOption[] = [{ value: 'all', label: 'All' }]
  if (input.classifier?.chat !== undefined) {
    options.push({ value: 'server', label: 'Server' }, { value: 'chat', label: 'Chat' })
  }
  if (input.playerEventsAvailable || input.classifier?.player !== undefined) {
    options.push({ value: 'player', label: 'Players' })
  }
  return options
}

export function consoleLineMatchesFilter(
  kind: ConsoleFeedKind | undefined,
  filter: ConsoleFeedFilter,
): boolean {
  if (filter === 'all') return true
  return (kind ?? 'server') === filter
}

export interface RosterDiff {
  joined: string[]
  left: string[]
}

export function diffRoster(previous: readonly string[], next: readonly string[]): RosterDiff {
  const previousSet = new Set(previous)
  const nextSet = new Set(next)
  return {
    joined: next.filter((name) => !previousSet.has(name)),
    left: previous.filter((name) => !nextSet.has(name)),
  }
}

function escapeHtml(value: string): string {
  return value
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;')
}

export interface PlayerFeedEvent {
  type: 'join' | 'leave'
  name: string
  playerCount: number
  playerCapacity: number
}

// Panel-generated marker line for roster changes detected by Xylona itself
// (query snapshot diffing), independent of any game log format.
export function buildPlayerEventHtml(event: PlayerFeedEvent): string {
  const glyph = event.type === 'join' ? '⇢' : '⇠'
  const verb = event.type === 'join' ? 'joined' : 'left'
  const capacity =
    event.playerCapacity > 0
      ? `${event.playerCount}/${event.playerCapacity}`
      : `${event.playerCount}`
  return (
    `<span class='console-player-event console-player-event--${event.type}'>` +
    `${glyph} <b>${escapeHtml(event.name)}</b> ${verb} · ${capacity} online</span>\n`
  )
}
