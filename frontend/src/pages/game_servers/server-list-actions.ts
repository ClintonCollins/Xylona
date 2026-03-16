import { Status } from '@/proto/shared_pb'
import type { DisplayRow } from './server-list-cache'

export function canStartServer(status: Status): boolean {
  return status === Status.OFFLINE || status === Status.UNKNOWN
}

export function canStopServer(status: Status): boolean {
  return status === Status.ONLINE
}

export function getStartableServers(servers: DisplayRow[]): DisplayRow[] {
  return servers.filter((server) => canStartServer(server.statusEnum))
}

export function getStoppableServers(servers: DisplayRow[]): DisplayRow[] {
  return servers.filter((server) => canStopServer(server.statusEnum))
}
