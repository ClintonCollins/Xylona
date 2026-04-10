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

  it('returns the original message for other error codes', () => {
    expect(
      connectErrorToString({ code: Code.NotFound, message: 'missing resource' } as ConnectError),
    ).toBe('missing resource')
  })

  it('formats raw unknown errors through ConnectError.from and prefixes when requested', () => {
    const fromSpy = vi.spyOn(ConnectError, 'from').mockReturnValue({
      code: Code.InvalidArgument,
      message: 'invalid request',
    } as ConnectError)

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
