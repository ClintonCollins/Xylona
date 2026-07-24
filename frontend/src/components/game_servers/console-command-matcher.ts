export interface ConsoleCommandSearchEntry {
  command: string
  syntax?: string
  summary?: string
  description?: string
  category?: string
  aliases?: readonly string[]
  keywords?: readonly string[]
  availability?: string
}

export type ConsoleCommandMatchField =
  | 'command'
  | 'syntax'
  | 'alias'
  | 'keyword'
  | 'category'
  | 'summary'
  | 'description'
  | 'availability'

export interface ConsoleCommandMatch<T extends ConsoleCommandSearchEntry> {
  entry: T
  field: ConsoleCommandMatchField
  score: number
}

interface SearchCandidate {
  field: ConsoleCommandMatchField
  value: string
  weight: number
}

interface InputToken {
  start: number
  value: string
}

const matchWeights: Record<ConsoleCommandMatchField, number> = {
  command: 0,
  syntax: 4,
  alias: 8,
  keyword: 20,
  category: 24,
  summary: 32,
  description: 40,
  availability: 48,
}

function normalizeSearchText(value: string): string {
  return value.trim().replace(/^\/+/, '').toLocaleLowerCase()
}

function tokenPrefixMatch(candidate: string, query: string): boolean {
  const candidateTokens = candidate.split(/\s+/).filter(Boolean)
  const queryTokens = query.split(/\s+/).filter(Boolean)

  if (queryTokens.length === 0 || queryTokens.length > candidateTokens.length) {
    return false
  }

  return queryTokens.every((queryToken, index) => candidateTokens[index]?.startsWith(queryToken))
}

function scoreCandidate(candidate: SearchCandidate, query: string): number | null {
  const normalizedCandidate = normalizeSearchText(candidate.value)
  if (normalizedCandidate === '') {
    return null
  }

  if (normalizedCandidate === query) {
    return candidate.weight
  }

  if (normalizedCandidate.startsWith(query)) {
    return candidate.weight + 10 + Math.min(normalizedCandidate.length - query.length, 20) / 100
  }

  if (tokenPrefixMatch(normalizedCandidate, query)) {
    return candidate.weight + 20
  }

  const substringIndex = normalizedCandidate.indexOf(query)
  if (substringIndex >= 0) {
    return candidate.weight + 30 + substringIndex / 100
  }

  return null
}

function searchCandidates(entry: ConsoleCommandSearchEntry): SearchCandidate[] {
  const candidates: SearchCandidate[] = [
    {
      field: 'command',
      value: entry.command,
      weight: matchWeights.command,
    },
  ]

  if (entry.syntax) {
    candidates.push({
      field: 'syntax',
      value: entry.syntax,
      weight: matchWeights.syntax,
    })
  }

  for (const alias of entry.aliases ?? []) {
    candidates.push({
      field: 'alias',
      value: alias,
      weight: matchWeights.alias,
    })
  }

  for (const keyword of entry.keywords ?? []) {
    candidates.push({
      field: 'keyword',
      value: keyword,
      weight: matchWeights.keyword,
    })
  }

  const metadata: Array<[ConsoleCommandMatchField, string | undefined]> = [
    ['category', entry.category],
    ['summary', entry.summary],
    ['description', entry.description],
    ['availability', entry.availability],
  ]
  for (const [field, value] of metadata) {
    if (!value) {
      continue
    }
    candidates.push({
      field,
      value,
      weight: matchWeights[field],
    })
  }

  return candidates
}

function compareMatches<T extends ConsoleCommandSearchEntry>(
  left: ConsoleCommandMatch<T>,
  right: ConsoleCommandMatch<T>,
): number {
  if (left.score !== right.score) {
    return left.score - right.score
  }

  const categoryOrder = (left.entry.category ?? '').localeCompare(right.entry.category ?? '')
  if (categoryOrder !== 0) {
    return categoryOrder
  }

  return left.entry.command.localeCompare(right.entry.command)
}

export function matchConsoleCommands<T extends ConsoleCommandSearchEntry>(
  entries: readonly T[],
  input: string,
): ConsoleCommandMatch<T>[] {
  const query = normalizeSearchText(input)
  if (query === '') {
    return entries
      .filter((entry) => entry.command.trim() !== '')
      .map((entry) => ({
        entry,
        field: 'command' as const,
        score: 0,
      }))
      .sort(compareMatches)
  }

  const matches: ConsoleCommandMatch<T>[] = []
  for (const entry of entries) {
    let bestMatch: ConsoleCommandMatch<T> | null = null
    for (const candidate of searchCandidates(entry)) {
      const score = scoreCandidate(candidate, query)
      if (score === null || (bestMatch && bestMatch.score <= score)) {
        continue
      }
      bestMatch = {
        entry,
        field: candidate.field,
        score,
      }
    }
    if (bestMatch) {
      matches.push(bestMatch)
    }
  }

  return matches.sort(compareMatches)
}

function inputTokens(input: string): InputToken[] {
  return Array.from(input.matchAll(/\S+/g), (match) => ({
    start: match.index,
    value: match[0],
  }))
}

function argumentSuffixForTerm(input: string, term: string): string {
  const tokens = inputTokens(input)
  const termTokens = normalizeSearchText(term).split(/\s+/).filter(Boolean)
  if (tokens.length === 0 || termTokens.length === 0) {
    return ''
  }

  let consumed = 0
  while (consumed < tokens.length && consumed < termTokens.length) {
    const inputToken = normalizeSearchText(tokens[consumed]?.value ?? '')
    const termToken = termTokens[consumed] ?? ''
    if (inputToken === '' || !termToken.startsWith(inputToken)) {
      break
    }
    consumed += 1
  }

  if (consumed !== termTokens.length || tokens.length <= termTokens.length) {
    return ''
  }

  const suffixStart = tokens[termTokens.length]?.start
  if (suffixStart === undefined) {
    return ''
  }
  return input.slice(suffixStart).trim()
}

export function completeConsoleCommandInput(
  input: string,
  entry: ConsoleCommandSearchEntry,
): string {
  let suffix = argumentSuffixForTerm(input, entry.command)
  if (suffix === '') {
    for (const alias of entry.aliases ?? []) {
      suffix = argumentSuffixForTerm(input, alias)
      if (suffix !== '') {
        break
      }
    }
  }

  if (suffix !== '') {
    return `${entry.command} ${suffix}`
  }
  return `${entry.command} `
}
