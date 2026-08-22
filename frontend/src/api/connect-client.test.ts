import { beforeEach, describe, expect, it, vi } from 'vitest'

const mocks = vi.hoisted(() => ({
  assign: vi.fn(),
  createCallbackClient: vi.fn(),
  createClient: vi.fn(),
  createConnectTransport: vi.fn(),
  fetch: vi.fn(),
}))

vi.mock('@connectrpc/connect', () => ({
  createCallbackClient: mocks.createCallbackClient,
  createClient: mocks.createClient,
}))

vi.mock('@connectrpc/connect-web', () => ({
  createConnectTransport: mocks.createConnectTransport,
}))

describe('connect-client', () => {
  beforeEach(() => {
    vi.resetModules()
    mocks.createCallbackClient.mockReset()
    mocks.createClient.mockReset()
    mocks.createConnectTransport.mockReset()
    mocks.fetch.mockReset()
    mocks.assign.mockReset()
    vi.stubGlobal('fetch', mocks.fetch)
    vi.spyOn(window.location, 'assign').mockImplementation(mocks.assign)
    window.history.replaceState({}, '', '/')
  })

  it('builds the local API base URL from the browser location', async () => {
    const { getXylonaApiBaseURL } = await import('./connect-client')

    expect(getXylonaApiBaseURL()).toBe(`${window.location.protocol}//${window.location.host}`)
  })

  it('builds a remote API base URL for a peer node', async () => {
    const { getXylonaApiBaseURL } = await import('./connect-client')

    expect(getXylonaApiBaseURL('peer.example.test:8443')).toBe(
      `${window.location.protocol}//peer.example.test:8443`,
    )
  })

  it('creates a transport that always includes browser credentials', async () => {
    const { createXylonaTransport } = await import('./connect-client')

    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' })
    mocks.fetch.mockResolvedValue(new Response(null, { status: 200 }))

    const transport = createXylonaTransport('peer.example.test:8443')

    expect(transport).toEqual({ kind: 'transport' })
    expect(mocks.createConnectTransport).toHaveBeenCalledTimes(1)

    const transportCall = mocks.createConnectTransport.mock.calls[0]
    if (!transportCall) {
      throw new Error('expected createConnectTransport to be called')
    }
    const options = transportCall[0]
    await options.fetch('/rpc', { method: 'POST' })

    expect(mocks.fetch).toHaveBeenCalledWith('/rpc', {
      method: 'POST',
      credentials: 'include',
    })
  })

  it('redirects once when concurrent API requests report an expired session', async () => {
    const { createXylonaTransport } = await import('./connect-client')

    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' })
    mocks.fetch.mockResolvedValue(new Response(null, { status: 401 }))
    createXylonaTransport()

    const transportCall = mocks.createConnectTransport.mock.calls[0]
    if (!transportCall) {
      throw new Error('expected createConnectTransport to be called')
    }
    const options = transportCall[0]
    await Promise.all([
      options.fetch('/rpc/jobs'),
      options.fetch('/rpc/availability'),
      options.fetch('/rpc/context'),
    ])

    expect(mocks.assign).toHaveBeenCalledOnce()
    expect(mocks.assign).toHaveBeenCalledWith('/login?reason=session-expired')
  })

  it('does not redirect an unauthenticated request from the login page', async () => {
    const { createXylonaTransport } = await import('./connect-client')

    window.history.replaceState({}, '', '/login')
    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' })
    mocks.fetch.mockResolvedValue(new Response(null, { status: 401 }))
    createXylonaTransport()

    const transportCall = mocks.createConnectTransport.mock.calls[0]
    if (!transportCall) {
      throw new Error('expected createConnectTransport to be called')
    }
    await transportCall[0].fetch('/rpc/login')

    expect(mocks.assign).not.toHaveBeenCalled()
  })

  it('creates the unary client from the shared transport', async () => {
    const { getXylonaClient } = await import('./connect-client')

    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' })
    const client = { kind: 'client' }
    mocks.createClient.mockReturnValue(client)

    expect(getXylonaClient()).toBe(client)
    expect(mocks.createClient).toHaveBeenCalledWith(expect.anything(), { kind: 'transport' })
  })

  it('creates the callback client from the shared transport', async () => {
    const { getXylonaClientCallback } = await import('./connect-client')

    mocks.createConnectTransport.mockReturnValue({ kind: 'transport' })
    const client = { kind: 'callback-client' }
    mocks.createCallbackClient.mockReturnValue(client)

    expect(getXylonaClientCallback()).toBe(client)
    expect(mocks.createCallbackClient).toHaveBeenCalledWith(expect.anything(), {
      kind: 'transport',
    })
  })

  it('reuses the unary client for repeated calls to the same node', async () => {
    const { getXylonaClient } = await import('./connect-client')

    const transport = { kind: 'transport' }
    const client = { kind: 'client' }
    mocks.createConnectTransport.mockReturnValue(transport)
    mocks.createClient.mockReturnValue(client)

    expect(getXylonaClient('peer.example.test:8443')).toBe(client)
    expect(getXylonaClient('peer.example.test:8443')).toBe(client)
    expect(mocks.createConnectTransport).toHaveBeenCalledTimes(1)
    expect(mocks.createClient).toHaveBeenCalledTimes(1)
  })

  it('reuses the callback client for repeated calls to the same node', async () => {
    const { getXylonaClientCallback } = await import('./connect-client')

    const transport = { kind: 'transport' }
    const client = { kind: 'callback-client' }
    mocks.createConnectTransport.mockReturnValue(transport)
    mocks.createCallbackClient.mockReturnValue(client)

    expect(getXylonaClientCallback('peer.example.test:8443')).toBe(client)
    expect(getXylonaClientCallback('peer.example.test:8443')).toBe(client)
    expect(mocks.createConnectTransport).toHaveBeenCalledTimes(1)
    expect(mocks.createCallbackClient).toHaveBeenCalledTimes(1)
  })
})
