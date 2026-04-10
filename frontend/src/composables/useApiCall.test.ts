import { afterEach, describe, expect, it, vi } from 'vitest'
import { useApiCall } from './useApiCall'

const mocks = vi.hoisted(() => ({
  buildXylonaErrorNotification: vi.fn(),
  connectErrorMessage: vi.fn(),
}))

vi.mock('@/api/connect-errors', () => ({
  buildXylonaErrorNotification: mocks.buildXylonaErrorNotification,
  connectErrorMessage: mocks.connectErrorMessage,
}))

describe('useApiCall', () => {
  afterEach(() => {
    mocks.buildXylonaErrorNotification.mockReset()
    mocks.connectErrorMessage.mockReset()
  })

  it('returns the result on success with error staying null', async () => {
    const apiFn = vi.fn().mockResolvedValue({ id: 'abc' })
    const { loading, error, execute } = useApiCall(apiFn)

    expect(loading.value).toBe(false)
    const result = await execute()

    expect(result).toEqual({ id: 'abc' })
    expect(error.value).toBeNull()
    expect(loading.value).toBe(false)
    expect(apiFn).toHaveBeenCalledOnce()
  })

  it('sets loading to true while the promise is pending', async () => {
    let resolve: ((v: string) => void) | undefined
    const pending = new Promise<string>((r) => {
      resolve = r
    })
    const apiFn = vi.fn().mockReturnValue(pending)
    const { loading, execute } = useApiCall(apiFn)

    const promise = execute()
    expect(loading.value).toBe(true)

    resolve?.('done')
    await promise
    expect(loading.value).toBe(false)
  })

  it('returns undefined and sets error on failure', async () => {
    const rawError = new Error('network failure')
    mocks.connectErrorMessage.mockReturnValue('network failure')

    const apiFn = vi.fn().mockRejectedValue(rawError)
    const { loading, error, execute } = useApiCall(apiFn)

    const result = await execute()

    expect(result).toBeUndefined()
    expect(error.value).toBe('network failure')
    expect(loading.value).toBe(false)
    expect(mocks.connectErrorMessage).toHaveBeenCalledWith(rawError, undefined)
  })

  it('calls notify with xylona-error format on error', async () => {
    const notification = {
      type: 'xylona-error',
      caption: 'Something broke',
      position: 'top',
      timeout: 5000,
    }
    mocks.connectErrorMessage.mockReturnValue('Something broke')
    mocks.buildXylonaErrorNotification.mockReturnValue(notification)

    const notify = vi.fn()
    const apiFn = vi.fn().mockRejectedValue(new Error('fail'))
    const { execute } = useApiCall(apiFn, { notify })

    await execute()

    expect(mocks.buildXylonaErrorNotification).toHaveBeenCalledWith('Something broke')
    expect(notify).toHaveBeenCalledWith(notification)
  })

  it('prepends errorPrefix to both error ref and notify caption', async () => {
    const notification = {
      type: 'xylona-error',
      caption: 'Failed to delete game: not found',
      position: 'top',
      timeout: 5000,
    }
    mocks.connectErrorMessage.mockReturnValue('Failed to delete game: not found')
    mocks.buildXylonaErrorNotification.mockReturnValue(notification)

    const notify = vi.fn()
    const apiFn = vi.fn().mockRejectedValue(new Error('fail'))
    const { error, execute } = useApiCall(apiFn, {
      notify,
      errorPrefix: 'Failed to delete game',
    })

    await execute()

    expect(error.value).toBe('Failed to delete game: not found')
    expect(mocks.connectErrorMessage).toHaveBeenCalledWith(
      expect.any(Error),
      'Failed to delete game',
    )
    expect(notify).toHaveBeenCalledWith(
      expect.objectContaining({
        caption: 'Failed to delete game: not found',
      }),
    )
  })

  it('resets error between successive calls', async () => {
    mocks.connectErrorMessage.mockReturnValue('bad request')

    const apiFn = vi.fn().mockRejectedValueOnce(new Error('fail')).mockResolvedValueOnce('ok')
    const { error, execute } = useApiCall(apiFn)

    await execute()
    expect(error.value).toBe('bad request')

    const result = await execute()
    expect(result).toBe('ok')
    expect(error.value).toBeNull()
  })

  it('forwards arguments to the API function', async () => {
    const apiFn = vi.fn().mockResolvedValue('result')
    const { execute } = useApiCall(apiFn)

    await execute('arg1' as never, 42 as never)

    expect(apiFn).toHaveBeenCalledWith('arg1', 42)
  })

  it('ignores stale failures from older in-flight executions', async () => {
    let rejectFirst: ((reason?: unknown) => void) | undefined
    let resolveSecond: ((value: string) => void) | undefined
    const first = new Promise<string>((_, reject) => {
      rejectFirst = reject
    })
    const second = new Promise<string>((resolve) => {
      resolveSecond = resolve
    })

    const apiFn = vi.fn().mockReturnValueOnce(first).mockReturnValueOnce(second)
    const notify = vi.fn()
    const { loading, error, execute } = useApiCall(apiFn, { notify })

    const firstPromise = execute()
    const secondPromise = execute()

    expect(loading.value).toBe(true)

    rejectFirst?.(new Error('stale failure'))
    await firstPromise

    expect(error.value).toBeNull()
    expect(loading.value).toBe(true)
    expect(notify).not.toHaveBeenCalled()

    resolveSecond?.('fresh success')
    await expect(secondPromise).resolves.toBe('fresh success')
    expect(error.value).toBeNull()
    expect(loading.value).toBe(false)
  })
})
