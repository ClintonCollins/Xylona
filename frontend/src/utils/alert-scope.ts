import { AlertEventType } from '@/proto/shared_pb'

const nodeAlertEventTypes = new Set([
  AlertEventType.NODE_CPU_THRESHOLD,
  AlertEventType.NODE_MEMORY_THRESHOLD,
  AlertEventType.NODE_DISK_THRESHOLD,
])

/** Node event types watch a node's host; every other type watches game servers. */
export function isNodeAlertEventType(eventType: AlertEventType): boolean {
  return nodeAlertEventTypes.has(eventType)
}

interface Named {
  id: string
  name: string
}

/** Names what an alert rule or history entry watches: one node or server, or all of them. */
export function alertTargetName(
  entry: { eventType: AlertEventType; serverId?: string; nodeId?: string },
  servers: readonly Named[],
  nodes: readonly Named[],
): string {
  if (isNodeAlertEventType(entry.eventType)) {
    if (!entry.nodeId) return 'All Nodes'
    return nodes.find((node) => node.id === entry.nodeId)?.name || entry.nodeId
  }
  if (!entry.serverId) return 'All Servers'
  return servers.find((server) => server.id === entry.serverId)?.name || entry.serverId
}
