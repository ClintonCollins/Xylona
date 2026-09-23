import { reactive } from 'vue'

import { Status } from '@/proto/shared_pb'
import { XylonaEventBus } from '@/utils/shared'

// Servers the controller is stopping right now, whoever asked: this tab,
// another tab, a schedule or an update.
const stoppingServerIds = reactive(new Set<string>())

export function isServerStopping(serverId: string): boolean {
  return stoppingServerIds.has(serverId)
}

XylonaEventBus.on('gameServerStopping', (serverId, stopping) => {
  if (stopping) {
    stoppingServerIds.add(serverId)
  } else {
    stoppingServerIds.delete(serverId)
  }
})

// The process leaving ONLINE/PRE_START ends the stop.
XylonaEventBus.on('gameServerStatus', (serverId, _serverName, status) => {
  if (status !== Status.ONLINE && status !== Status.PRE_START) {
    stoppingServerIds.delete(serverId)
  }
})

// Missed events while disconnected: the reloaded status is authoritative.
XylonaEventBus.on('websocketConnected', () => {
  stoppingServerIds.clear()
})
