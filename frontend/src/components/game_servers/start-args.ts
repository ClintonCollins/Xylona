import type { GameServer } from '@/proto/shared_pb'

export type StartArgOwnership = 'system' | 'locked' | 'editable'
export type StartArgPatchOp = 'edit' | 'remove' | 'add'
export type StartArgProvenance = 'system' | 'locked' | 'default' | 'edited' | 'added'

export interface StartArgBlock {
  id: string
  order: number
  ownership: StartArgOwnership
  tokens: string[]
  label?: string
  managedSource?: string
}

export interface StartArgPatch {
  id: string
  op: StartArgPatchOp
  tokens?: string[]
  label?: string
  afterId?: string | null
}

export interface StartArgBlocklistEntry {
  pattern: string
  reason: string
}

export interface ResolvedStartArgBlock {
  id: string
  ownership: StartArgOwnership
  tokens: string[]
  resolvedTokens: string[]
  label?: string
  provenance: StartArgProvenance
  originalTokens?: string[]
  managedSource?: string
}

interface PendingSimilarAction {
  mode: 'add'
  id: string
  afterId: string | null
  label: string
  tokens: string[]
}

const legacyPlaceholderKeys: Record<string, string> = {
  '%GAMESERVER_DIRECTORY%': 'INSTALL_DIR',
  '%GAMESERVER_ID%': 'SERVER_ID',
  '%GAMESERVER_BACKUP_DIRECTORY%': 'BACKUP_DIR',
  '%GAMESERVER_NAME%': 'SERVER_NAME',
  '%GAMESERVER_IP%': 'IP',
  '%GAMESERVER_PORT%': 'PORT',
  '%GAMESERVER_QUERY_PORT%': 'QUERY_PORT',
  '%GAMESERVER_MAX_MEMORY_MB%': 'MAX_MEMORY_MB',
  '%GAMESERVER_MAX_PLAYERS%': 'MAX_PLAYERS',
  '%GAMESERVER_SET_PLAYERS%': 'SET_PLAYERS',
}

export function parseStartArgsTemplate(jsonStr: string): StartArgBlock[] {
  if (!jsonStr) {
    return []
  }

  try {
    const parsed = JSON.parse(jsonStr)
    if (!Array.isArray(parsed)) {
      return []
    }

    return parsed
      .map((entry, index) => normalizeTemplateBlock(entry, index))
      .filter((entry): entry is StartArgBlock => entry !== null)
  } catch {
    return []
  }
}

export function parseStartArgsPatches(jsonStr: string): StartArgPatch[] {
  if (!jsonStr) {
    return []
  }

  try {
    const parsed = JSON.parse(jsonStr)
    if (!Array.isArray(parsed)) {
      return []
    }

    return parsed
      .map((entry) => normalizePatch(entry))
      .filter((entry): entry is StartArgPatch => entry !== null)
  } catch {
    return []
  }
}

export function parseStartArgBlocklist(jsonStr: string): StartArgBlocklistEntry[] {
  if (!jsonStr) {
    return []
  }

  try {
    const parsed = JSON.parse(jsonStr)
    if (!Array.isArray(parsed)) {
      return []
    }

    return parsed
      .map((entry) => normalizeBlocklistEntry(entry))
      .filter((entry): entry is StartArgBlocklistEntry => entry !== null)
  } catch {
    return []
  }
}

export function serializeStartArgsPatches(patches: StartArgPatch[]): string {
  if (patches.length === 0) {
    return ''
  }

  return JSON.stringify(patches)
}

export function serializeStartArgsTemplate(template: StartArgBlock[]): string {
  if (template.length === 0) {
    return ''
  }

  return JSON.stringify(
    template.map((block) => ({
      id: block.id,
      order: block.order,
      ownership: block.ownership,
      tokens: block.tokens,
      label: block.label ?? '',
      managed_source: block.managedSource ?? '',
    })),
  )
}

export function serializeStartArgBlocklist(blocklist: StartArgBlocklistEntry[]): string {
  if (blocklist.length === 0) {
    return ''
  }

  return JSON.stringify(blocklist)
}

export function buildPlaceholderVars(gameServer: Partial<GameServer> | null | undefined) {
  const port = gameServer?.port ?? 0n
  const queryPort = gameServer?.queryPort ?? 0n

  return {
    SERVER_ID: gameServer?.id ?? '',
    IP: gameServer?.ip?.address ?? '',
    PORT: String(port),
    PORT_PLUS_1: String(port + 1n),
    PORT_PLUS_2: String(port + 2n),
    QUERY_PORT: String(queryPort),
    QUERY_PORT_PLUS_1: String(queryPort + 1n),
    MAX_MEMORY_MB: String(gameServer?.maxMemoryMb ?? 0n),
    MAX_PLAYERS: String(gameServer?.maxPlayers ?? 0n),
    SERVER_NAME: gameServer?.name ?? '',
    INSTALL_DIR: gameServer?.directory ?? '',
    BACKUP_DIR: gameServer?.backupDirectory ?? '',
    SET_PLAYERS: String(gameServer?.setMaxPlayers ?? 0n),
    SERVER_EXECUTABLE: gameServer?.serverExecutable ?? '',
  }
}

export function resolveStartCommandBase(baseCommand: string, vars: Record<string, string>) {
  return resolveToken(baseCommand.trim(), vars)
}

export function resolveStartArgs(
  template: StartArgBlock[],
  patches: StartArgPatch[],
  vars: Record<string, string>,
) {
  if (template.length === 0) {
    return {
      args: [] as string[],
      resolvedBlocks: [] as ResolvedStartArgBlock[],
    }
  }

  const orderedTemplate = cloneTemplate(template).sort((left, right) => left.order - right.order)
  const templateById = new Map(orderedTemplate.map((block) => [block.id, block]))
  const editedIds = new Set<string>()
  const removedIds = new Set<string>()
  const originalTokens = new Map<string, string[]>()
  const addsByAnchor = new Map<string, StartArgPatch[]>()

  for (const patch of patches) {
    if (patch.op === 'edit') {
      const block = templateById.get(patch.id)
      if (!block) {
        continue
      }
      if (!originalTokens.has(patch.id)) {
        originalTokens.set(patch.id, [...block.tokens])
      }
      block.tokens = [...(patch.tokens ?? [])]
      if (patch.label) {
        block.label = patch.label
      }
      editedIds.add(patch.id)
      continue
    }

    if (patch.op === 'remove') {
      if (templateById.has(patch.id)) {
        removedIds.add(patch.id)
      }
      continue
    }

    if (patch.op === 'add') {
      const anchorId = patch.afterId ?? ''
      const anchoredPatches = addsByAnchor.get(anchorId) ?? []
      anchoredPatches.push({
        ...patch,
        tokens: [...(patch.tokens ?? [])],
      })
      addsByAnchor.set(anchorId, anchoredPatches)
    }
  }

  const resolvedBlocks: ResolvedStartArgBlock[] = []
  const visitedAddIds = new Set<string>()

  const emitAnchoredAdds = (anchorId: string) => {
    const anchoredAdds = addsByAnchor.get(anchorId) ?? []
    for (const patch of anchoredAdds) {
      if (visitedAddIds.has(patch.id)) {
        continue
      }
      visitedAddIds.add(patch.id)

      const tokens = [...(patch.tokens ?? [])]
      resolvedBlocks.push({
        id: patch.id,
        ownership: 'editable',
        tokens,
        resolvedTokens: resolveTokens(tokens, vars),
        label: patch.label,
        provenance: 'added',
      })

      emitAnchoredAdds(patch.id)
    }
  }

  emitAnchoredAdds('')

  for (const block of orderedTemplate) {
    if (removedIds.has(block.id)) {
      continue
    }

    const tokens = [...block.tokens]
    resolvedBlocks.push({
      id: block.id,
      ownership: block.ownership,
      tokens,
      resolvedTokens: resolveTokens(tokens, vars),
      label: block.label,
      managedSource: block.managedSource,
      provenance: editedIds.has(block.id) ? 'edited' : templateBlockProvenance(block),
      originalTokens: originalTokens.get(block.id),
    })

    emitAnchoredAdds(block.id)
  }

  return {
    args: resolvedBlocks.flatMap((block) => block.resolvedTokens),
    resolvedBlocks,
  }
}

export function validateTokensAgainstBlocklist(
  tokens: string[],
  blocklist: StartArgBlocklistEntry[],
): StartArgBlocklistEntry | null {
  if (tokens.length === 0 || blocklist.length === 0) {
    return null
  }

  for (const entry of blocklist) {
    try {
      const pattern = new RegExp(entry.pattern)
      if (tokens.some((token) => pattern.test(token))) {
        return entry
      }
    } catch {
      return {
        pattern: entry.pattern,
        reason: `Invalid regex: ${entry.pattern}`,
      }
    }
  }

  return null
}

export function findSimilarArg<
  T extends { id: string; ownership: StartArgOwnership; tokens: string[] },
>(newTokens: string[], existingBlocks: T[]): T | null {
  if (newTokens.length === 0 || existingBlocks.length === 0) {
    return null
  }

  const firstNewToken = newTokens[0]
  if (firstNewToken === undefined) {
    return null
  }

  const newPrefix = flagPrefix(firstNewToken)
  if (!newPrefix) {
    return existingBlocks.find((block) => equalTokens(newTokens, block.tokens)) ?? null
  }

  return (
    existingBlocks.find((block) => {
      const firstExistingToken = block.tokens[0]
      if (firstExistingToken === undefined) {
        return false
      }

      const existingPrefix = flagPrefix(firstExistingToken)
      return existingPrefix !== '' && existingPrefix === newPrefix
    }) ?? null
  )
}

export function splitTokensInput(value: string): string[] {
  return value
    .split(/\r?\n/u)
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
}

export function joinTokensInput(tokens: string[]): string {
  return tokens.join('\n')
}

export function formatTokensInline(tokens: string[]): string {
  return tokens.join(' ')
}

export function clonePatches(patches: StartArgPatch[]): StartArgPatch[] {
  return patches.map((patch) => ({
    ...patch,
    tokens: patch.tokens ? [...patch.tokens] : undefined,
    afterId: patch.afterId ?? null,
  }))
}

export function createPendingSimilarAction(
  id: string,
  afterId: string | null,
  label: string,
  tokens: string[],
): PendingSimilarAction {
  return {
    mode: 'add',
    id,
    afterId,
    label,
    tokens: [...tokens],
  }
}

export function applyAddAction(
  patches: StartArgPatch[],
  pendingAction: PendingSimilarAction,
): StartArgPatch[] {
  return [
    ...patches,
    {
      id: pendingAction.id,
      op: 'add',
      afterId: pendingAction.afterId,
      label: pendingAction.label,
      tokens: [...pendingAction.tokens],
    },
  ]
}

function normalizeTemplateBlock(entry: unknown, index: number): StartArgBlock | null {
  if (!isRecord(entry)) {
    return null
  }

  if (typeof entry['id'] !== 'string' || entry['id'].trim() === '') {
    return null
  }

  const tokens = normalizeTokens(entry['tokens'])
  if (tokens.length === 0) {
    return null
  }

  return {
    id: entry['id'],
    order: typeof entry['order'] === 'number' ? entry['order'] : index,
    ownership: normalizeOwnership(entry['ownership']),
    tokens,
    label: typeof entry['label'] === 'string' ? entry['label'] : '',
    managedSource:
      typeof entry['managed_source'] === 'string'
        ? entry['managed_source']
        : typeof entry['managedSource'] === 'string'
          ? entry['managedSource']
          : '',
  }
}

function normalizePatch(entry: unknown): StartArgPatch | null {
  if (!isRecord(entry)) {
    return null
  }

  if (typeof entry['id'] !== 'string' || entry['id'].trim() === '') {
    return null
  }

  if (!isPatchOp(entry['op'])) {
    return null
  }

  return {
    id: entry['id'],
    op: entry['op'],
    tokens: normalizeTokens(entry['tokens']),
    label: typeof entry['label'] === 'string' ? entry['label'] : '',
    afterId:
      typeof entry['afterId'] === 'string'
        ? entry['afterId']
        : entry['afterId'] === null
          ? null
          : undefined,
  }
}

function normalizeBlocklistEntry(entry: unknown): StartArgBlocklistEntry | null {
  if (!isRecord(entry)) {
    return null
  }

  if (typeof entry['pattern'] !== 'string' || entry['pattern'].trim() === '') {
    return null
  }

  return {
    pattern: entry['pattern'],
    reason: typeof entry['reason'] === 'string' ? entry['reason'] : 'Blocked by game definition',
  }
}

function normalizeTokens(value: unknown): string[] {
  if (!Array.isArray(value)) {
    return []
  }

  return value.filter((token): token is string => typeof token === 'string')
}

function normalizeOwnership(value: unknown): StartArgOwnership {
  if (value === 'system' || value === 'locked' || value === 'editable') {
    return value
  }

  return 'editable'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null
}

function isPatchOp(value: unknown): value is StartArgPatchOp {
  return value === 'edit' || value === 'remove' || value === 'add'
}

function resolveTokens(tokens: string[], vars: Record<string, string>): string[] {
  return tokens.map((token) => resolveToken(token, vars))
}

function resolveToken(token: string, vars: Record<string, string>): string {
  if (!token) {
    return ''
  }

  let resolved = token
  for (const [placeholder, key] of Object.entries(legacyPlaceholderKeys)) {
    resolved = resolved.replaceAll(placeholder, vars[key] ?? '')
  }

  return resolved.replace(/\{\{([A-Z_]+)\}\}/gu, (_match, key: string) => vars[key] ?? '')
}

function cloneTemplate(template: StartArgBlock[]): StartArgBlock[] {
  return template.map((block) => ({
    ...block,
    tokens: [...block.tokens],
  }))
}

function templateBlockProvenance(block: StartArgBlock): StartArgProvenance {
  if (block.ownership === 'system') {
    return 'system'
  }

  if (block.ownership === 'locked') {
    return 'locked'
  }

  return 'default'
}

function flagPrefix(token: string): string {
  if (!token.startsWith('-')) {
    return ''
  }

  for (let i = 0; i < token.length; i += 1) {
    const current = token.charAt(i)
    if (/[0-9=]/u.test(current)) {
      return token.slice(0, i)
    }
  }

  return token
}

function equalTokens(left: string[], right: string[]) {
  if (left.length !== right.length) {
    return false
  }

  return left.every((token, index) => token === right[index])
}
