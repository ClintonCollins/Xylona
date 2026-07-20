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
