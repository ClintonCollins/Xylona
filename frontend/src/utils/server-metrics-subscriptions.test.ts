import { fromJsonString } from '@bufbuild/protobuf'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { Request_Type, RequestSchema } from '@/proto/websocket_pb'
import { createServerMetricsSubscriptions } from './server-metrics-subscriptions'

const websocket = vi.hoisted(() => ({ isOpen: vi.fn(() => true), send: vi.fn() }))

vi.mock('@/utils/shared', () => ({ GetOrCreateXylonaWebsocketClient: () => websocket }))

function sentRequests(): [string, Request_Type][] {
  return websocket.send.mock.calls.map(([payload]) => {
    const request = fromJsonString(RequestSchema, payload as string)
    return [request.gameServerId, request.type]
  })
}

describe('createServerMetricsSubscriptions', () => {
  beforeEach(() => {
    websocket.isOpen.mockReturnValue(true)
    websocket.send.mockReset()
  })

  it('subscribes new servers, unsubscribes dropped ones and clears on the way out', () => {
    const subscriptions = createServerMetricsSubscriptions()
    subscriptions.sync(['a', 'b'])
    subscriptions.sync(['b', 'c'])
    subscriptions.clear()

    expect(sentRequests()).toEqual([
      ['a', Request_Type.SubscribeServerMetrics],
      ['b', Request_Type.SubscribeServerMetrics],
      ['a', Request_Type.UnsubscribeServerMetrics],
      ['c', Request_Type.SubscribeServerMetrics],
      ['b', Request_Type.UnsubscribeServerMetrics],
      ['c', Request_Type.UnsubscribeServerMetrics],
    ])
  })

  it('resubscribes after the websocket drops and the controller forgets them', () => {
    const subscriptions = createServerMetricsSubscriptions()
    subscriptions.sync(['a'])
    subscriptions.forget()
    subscriptions.sync(['a'])

    expect(sentRequests()).toEqual([
      ['a', Request_Type.SubscribeServerMetrics],
      ['a', Request_Type.SubscribeServerMetrics],
    ])
  })

  it('retries a subscription that could not be sent while the websocket was closed', () => {
    const subscriptions = createServerMetricsSubscriptions()
    websocket.isOpen.mockReturnValue(false)
    subscriptions.sync(['a'])
    websocket.isOpen.mockReturnValue(true)
    subscriptions.sync(['a'])

    expect(sentRequests()).toEqual([['a', Request_Type.SubscribeServerMetrics]])
  })
})
