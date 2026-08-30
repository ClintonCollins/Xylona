import { GameServerPlayerAction } from '@/proto/xylona_pb'

export interface PlayerActionDefinition {
  action: GameServerPlayerAction
  operationId: string
  label: string
  description: string
  icon: string
  color: string
  reasonAllowed: boolean
}

const playerActionDefinitions: Record<number, PlayerActionDefinition> = {
  [GameServerPlayerAction.KICK]: {
    action: GameServerPlayerAction.KICK,
    operationId: 'player_moderation.kick',
    label: 'Kick',
    description: 'Disconnect the player from the server.',
    icon: 'logout',
    color: 'warning',
    reasonAllowed: true,
  },
  [GameServerPlayerAction.BAN]: {
    action: GameServerPlayerAction.BAN,
    operationId: 'player_moderation.ban',
    label: 'Ban',
    description: 'Block the player from joining the server.',
    icon: 'block',
    color: 'negative',
    reasonAllowed: true,
  },
  [GameServerPlayerAction.UNBAN]: {
    action: GameServerPlayerAction.UNBAN,
    operationId: 'player_moderation.unban',
    label: 'Unban',
    description: 'Remove the player ban.',
    icon: 'lock_open',
    color: 'positive',
    reasonAllowed: false,
  },
  [GameServerPlayerAction.ALLOWLIST_ADD]: {
    action: GameServerPlayerAction.ALLOWLIST_ADD,
    operationId: 'player_access.allowlist_add',
    label: 'Add to allowlist',
    description: 'Allow the player to join an allowlisted server.',
    icon: 'person_add',
    color: 'positive',
    reasonAllowed: false,
  },
  [GameServerPlayerAction.ALLOWLIST_REMOVE]: {
    action: GameServerPlayerAction.ALLOWLIST_REMOVE,
    operationId: 'player_access.allowlist_remove',
    label: 'Remove from allowlist',
    description: 'Remove the player from the server allowlist.',
    icon: 'person_remove',
    color: 'warning',
    reasonAllowed: false,
  },
}

export const playerActionOperationIDs = Object.freeze(
  Object.values(playerActionDefinitions).map((definition) => definition.operationId),
)

export function getPlayerActionDefinition(
  action: GameServerPlayerAction,
): PlayerActionDefinition | null {
  return playerActionDefinitions[action] ?? null
}

export function getSupportedPlayerActionDefinitions(
  supportedActions: GameServerPlayerAction[],
): PlayerActionDefinition[] {
  return supportedActions
    .map((action) => getPlayerActionDefinition(action))
    .filter((definition): definition is PlayerActionDefinition => definition !== null)
}

export function getQuickPlayerActionDefinitions(
  supportedActions: GameServerPlayerAction[],
): PlayerActionDefinition[] {
  return getSupportedPlayerActionDefinitions(supportedActions).filter(
    (definition) =>
      definition.action === GameServerPlayerAction.KICK ||
      definition.action === GameServerPlayerAction.BAN,
  )
}
