import { type GameServer, UpdateProviderKind } from '@/proto/shared_pb'

export function canShowUpdateButton(gameServer: GameServer): boolean {
  if (gameServer.resolvedHasUpdate) {
    return true
  }
  const kind = gameServer.resolvedUpdateProvider?.kind ?? UpdateProviderKind.NONE
  return kind !== UpdateProviderKind.NONE
}
