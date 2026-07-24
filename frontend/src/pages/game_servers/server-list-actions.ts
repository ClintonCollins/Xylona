import { Status } from '@/proto/shared_pb'
import type { DisplayRow } from './server-list-cache'

export function canStartServer(status: Status): boolean {
  return status === Status.OFFLINE
}

export function canStopServer(status: Status): boolean {
  return status === Status.ONLINE
}

export function canRestartServer(status: Status): boolean {
  return status === Status.ONLINE
}

export function canUpdateServer(server: DisplayRow): boolean {
  return (
    Boolean(server.canUpdate) &&
    (server.statusEnum === Status.ONLINE || server.statusEnum === Status.OFFLINE)
  )
}

export function getStartableServers(servers: DisplayRow[]): DisplayRow[] {
  return servers.filter((server) => canStartServer(server.statusEnum))
}

export function getStoppableServers(servers: DisplayRow[]): DisplayRow[] {
  return servers.filter((server) => canStopServer(server.statusEnum))
}

export function getRestartableServers(servers: DisplayRow[]): DisplayRow[] {
  return servers.filter((server) => canRestartServer(server.statusEnum))
}

export function getUpdateableServers(servers: DisplayRow[]): DisplayRow[] {
  return servers.filter(canUpdateServer)
}

export type LifecycleConfirmAction = 'stop' | 'restart'

export interface LifecycleImpact {
  displayName: string
  playerCount: number
}

export interface LifecycleConfirmation {
  title: string
  message: string
  confirmLabel: string
  confirmColor: 'negative' | 'warning'
}

// Risk-proportional confirmation: stop/restart run instantly when nobody is
// online, and require one confirm naming the affected players otherwise.
export function buildLifecycleConfirmation(
  action: LifecycleConfirmAction,
  servers: LifecycleImpact[],
): LifecycleConfirmation | null {
  const totalPlayers = servers.reduce((total, server) => total + Math.max(server.playerCount, 0), 0)
  if (totalPlayers === 0) {
    return null
  }

  const actionLabel = action === 'stop' ? 'Stop' : 'Restart'
  const confirmColor = action === 'stop' ? 'negative' : 'warning'
  const playerPhrase = totalPlayers === 1 ? '1 player is' : `${totalPlayers} players are`

  const singleServer = servers.length === 1 ? servers[0] : undefined
  if (singleServer !== undefined) {
    const consequence =
      action === 'stop'
        ? 'online and will be disconnected.'
        : 'online and will be disconnected while the server restarts.'
    return {
      title: `${actionLabel} ${singleServer.displayName}?`,
      message: `${playerPhrase} ${consequence}`,
      confirmLabel: `${actionLabel} server`,
      confirmColor,
    }
  }

  const consequence =
    action === 'stop'
      ? 'and will be disconnected.'
      : 'and will be disconnected while the servers restart.'
  return {
    title: `${actionLabel} ${servers.length} servers?`,
    message: `${playerPhrase} online across ${servers.length} servers ${consequence}`,
    confirmLabel: `${actionLabel} servers`,
    confirmColor,
  }
}
