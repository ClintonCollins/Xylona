import {
  createCallbackClient,
  createClient,
  type CallbackClient,
  type Client,
} from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'

import { Xylona } from '@/proto/xylona_pb'

const transportCache = new Map<string, ReturnType<typeof createConnectTransport>>()
const unaryClientCache = new Map<string, Client<typeof Xylona>>()
const callbackClientCache = new Map<string, CallbackClient<typeof Xylona>>()
let sessionRedirecting = false

export function getXylonaApiBaseURL(nodeAddress: string = window.location.host): string {
  return nodeAddress === window.location.host
    ? `${window.location.protocol}//${window.location.host}`
    : `${window.location.protocol}//${nodeAddress}`
}

export function createXylonaTransport(nodeAddress: string = window.location.host) {
  return createConnectTransport({
    baseUrl: getXylonaApiBaseURL(nodeAddress),
    fetch: async (input, init) => {
      const response = await fetch(input, { ...init, credentials: 'include' })
      if (
        response.status === 401 &&
        !sessionRedirecting &&
        window.location.pathname !== '/login' &&
        window.location.pathname !== '/setup'
      ) {
        sessionRedirecting = true
        window.location.assign('/login?reason=session-expired')
      }
      return response
    },
  })
}

function getCachedXylonaTransport(nodeAddress: string = window.location.host) {
  const cachedTransport = transportCache.get(nodeAddress)
  if (cachedTransport) {
    return cachedTransport
  }

  const transport = createXylonaTransport(nodeAddress)
  transportCache.set(nodeAddress, transport)
  return transport
}

export function getXylonaClient(nodeAddress: string = window.location.host) {
  const cachedClient = unaryClientCache.get(nodeAddress)
  if (cachedClient) {
    return cachedClient
  }

  const client = createClient(Xylona, getCachedXylonaTransport(nodeAddress))
  unaryClientCache.set(nodeAddress, client)
  return client
}

export function getXylonaClientCallback(nodeAddress: string = window.location.host) {
  const cachedClient = callbackClientCache.get(nodeAddress)
  if (cachedClient) {
    return cachedClient
  }

  const client = createCallbackClient(Xylona, getCachedXylonaTransport(nodeAddress))
  callbackClientCache.set(nodeAddress, client)
  return client
}
