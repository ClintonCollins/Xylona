import { Status } from '@/proto/shared_pb'
import type { DisplayRow } from './server-list-cache'

/** Online, or started and not yet ready for players: the process is up either way. */
export function isServerRunning(status: Status): boolean {
  return status === Status.ONLINE || status === Status.PRE_START
}

export function canStartServer(status: Status): boolean {
  return status === Status.OFFLINE
}

export function canStopServer(status: Status): boolean {
  return isServerRunning(status)
}

export function canRestartServer(status: Status): boolean {
  return isServerRunning(status)
}

export function canUpdateServer(server: DisplayRow): boolean {
  return (
    Boolean(server.canUpdate) &&
    (isServerRunning(server.statusEnum) || server.statusEnum === Status.OFFLINE)
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
  /** Null when the server is up but has not answered a player query. */
  playerCount: number | null
}

export interface LifecycleConfirmation {
  title: string
  message: string
  confirmLabel: string
  confirmColor: 'negative' | 'warning'
}

// Risk-proportional confirmation: stop/restart run instantly when nobody is
// online, and require one confirm naming the affected players otherwise. An
// unknown player count is never treated as nobody.
export function buildLifecycleConfirmation(
  action: LifecycleConfirmAction,
  servers: LifecycleImpact[],
): LifecycleConfirmation | null {
  const unknownCount = servers.filter((server) => server.playerCount === null).length
  const totalPlayers = servers.reduce(
    (total, server) => total + Math.max(server.playerCount ?? 0, 0),
    0,
  )
  if (totalPlayers === 0 && unknownCount === 0) {
    return null
  }

  const actionLabel = action === 'stop' ? 'Stop' : 'Restart'
  const confirmColor = action === 'stop' ? 'negative' : 'warning'
  const playerPhrase = totalPlayers === 1 ? '1 player is' : `${totalPlayers} players are`

  const singleServer = servers.length === 1 ? servers[0] : undefined
  if (singleServer !== undefined) {
    const restartSuffix = action === 'stop' ? '' : ' while the server restarts'
    return {
      title: `${actionLabel} ${singleServer.displayName}?`,
      message:
        unknownCount > 0
          ? `Player count unknown — anyone connected will be disconnected${restartSuffix}.`
          : `${playerPhrase} online and will be disconnected${restartSuffix}.`,
      confirmLabel: `${actionLabel} server`,
      confirmColor,
    }
  }

  const restartSuffix = action === 'stop' ? '' : ' while the servers restart'
  let message = `${playerPhrase} online across ${servers.length} servers and will be disconnected${restartSuffix}.`
  if (unknownCount > 0) {
    const knownPlayers = totalPlayers > 0 ? `, and ${playerPhrase} online on the others` : ''
    message = `Player count unknown on ${unknownCount} of ${servers.length} servers${knownPlayers} — anyone connected will be disconnected${restartSuffix}.`
  }
  return {
    title: `${actionLabel} ${servers.length} servers?`,
    message,
    confirmLabel: `${actionLabel} servers`,
    confirmColor,
  }
}
