import type { GameServer } from '@/proto/shared_pb'

export function canShowUpdateButton(gameServer: GameServer): boolean {
  return Boolean(gameServer.resolvedHasUpdate)
}
