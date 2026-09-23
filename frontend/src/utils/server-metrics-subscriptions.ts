import { create, toJsonString } from '@bufbuild/protobuf'
import { Request_Type, RequestSchema } from '@/proto/websocket_pb'
import { GetOrCreateXylonaWebsocketClient } from '@/utils/shared'

// Tracks which game servers' live metrics a page has subscribed to over the shared
// websocket, so it can follow a changing server list and unsubscribe on the way out.
export function createServerMetricsSubscriptions() {
  const subscribed = new Set<string>()

  function send(serverID: string, type: Request_Type): boolean {
    const websocket = GetOrCreateXylonaWebsocketClient()
    if (!websocket.isOpen()) {
      return false
    }
    websocket.send(
      toJsonString(RequestSchema, create(RequestSchema, { gameServerId: serverID, type })),
    )
    return true
  }

  function sync(serverIDs: string[]) {
    const desired = new Set(serverIDs)
    for (const serverID of subscribed) {
      if (desired.has(serverID)) {
        continue
      }
      try {
        send(serverID, Request_Type.UnsubscribeServerMetrics)
      } catch (error) {
        console.error('Failed to unsubscribe from server metrics', error)
      }
      subscribed.delete(serverID)
    }

    for (const serverID of desired) {
      if (subscribed.has(serverID)) {
        continue
      }
      try {
        if (send(serverID, Request_Type.SubscribeServerMetrics)) {
          subscribed.add(serverID)
        }
      } catch (error) {
        console.error('Failed to subscribe to server metrics', error)
      }
    }
  }

  function clear() {
    sync([])
  }

  // The websocket dropped, so the controller already forgot these subscriptions.
  function forget() {
    subscribed.clear()
  }

  return { sync, clear, forget }
}
