import { createCallbackClient, createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'

import { Xylona } from '@/proto/xylona_pb'

export function getXylonaApiBaseURL(nodeAddress: string = window.location.host): string {
  return nodeAddress === window.location.host
    ? `${window.location.protocol}//${window.location.host}`
    : `${window.location.protocol}//${nodeAddress}`
}

export function createXylonaTransport(nodeAddress: string = window.location.host) {
  return createConnectTransport({
    baseUrl: getXylonaApiBaseURL(nodeAddress),
    fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
  })
}

export function getXylonaClient(nodeAddress: string = window.location.host) {
  return createClient(Xylona, createXylonaTransport(nodeAddress))
}

export function getXylonaClientCallback(nodeAddress: string = window.location.host) {
  return createCallbackClient(Xylona, createXylonaTransport(nodeAddress))
}
