import { Code, ConnectError } from '@connectrpc/connect'
import { describe, expect, it, vi } from 'vitest'

import {
  buildXylonaErrorNotification,
  connectErrorMessage,
  connectErrorToString,
} from './connect-errors'

describe('connect-errors', () => {
  it('maps unavailable errors to the shared backend message', () => {
    expect(
      connectErrorToString({ code: Code.Unavailable, message: 'no route' } as ConnectError),
    ).toBe('Unable to connect to Xylona backend.')
  })

  it('returns the message without the RPC code prefix for other error codes', () => {
    const err = new ConnectError('missing resource', Code.NotFound)
    expect(err.message).toBe('[not_found] missing resource')
    expect(connectErrorToString(err)).toBe('missing resource')
  })

  it('formats raw unknown errors through ConnectError.from and prefixes when requested', () => {
    const fromSpy = vi
      .spyOn(ConnectError, 'from')
      .mockReturnValue(new ConnectError('invalid request', Code.InvalidArgument))

    expect(connectErrorMessage(new Error('boom'), 'Failed to save')).toBe(
      'Failed to save: invalid request',
    )

    fromSpy.mockRestore()
  })

  it('builds the standard Xylona error notification shape with overrides', () => {
    expect(
      buildXylonaErrorNotification('Failed to load nodes', {
        timeout: 0,
        closeBtn: 'Dismiss',
        icon: 'report_problem',
      }),
    ).toEqual({
      type: 'xylona-error',
      caption: 'Failed to load nodes',
      position: 'top',
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
  })
})
