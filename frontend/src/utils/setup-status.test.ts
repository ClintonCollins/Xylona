import { describe, expect, it } from 'vitest'

import { unauthenticatedRedirect } from './setup-status'

describe('unauthenticatedRedirect', () => {
  it.each([
    {
      name: 'sends login to tokenized setup',
      setupNeeded: true,
      path: '/login',
      token: 'setup token',
      want: '/setup?token=setup%20token',
    },
    {
      name: 'sends login to blocked setup without a token',
      setupNeeded: true,
      path: '/login',
      token: '',
      want: '/setup',
    },
    {
      name: 'leaves active setup alone',
      setupNeeded: true,
      path: '/setup',
      token: 'token',
      want: null,
    },
    {
      name: 'returns completed setup to login',
      setupNeeded: false,
      path: '/setup',
      token: '',
      want: '/login',
    },
  ])('$name', ({ setupNeeded, path, token, want }) => {
    expect(unauthenticatedRedirect(setupNeeded, path, token)).toBe(want)
  })
})
