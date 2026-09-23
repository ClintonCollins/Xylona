import { describe, expect, it } from 'vitest'

import { loginPath, safeReturnPath } from './login-redirect'

describe('safeReturnPath', () => {
  it.each([
    { value: '/nodes', want: '/nodes' },
    {
      value: '/game-servers/abc/files?path=%2Fconfig#top',
      want: '/game-servers/abc/files?path=%2Fconfig#top',
    },
    { value: undefined, want: '/' },
    { value: ['/nodes'], want: '/' },
    { value: '', want: '/' },
    { value: 'nodes', want: '/' },
    { value: '//evil.example/path', want: '/' },
    { value: '/\\evil.example/path', want: '/' },
    { value: 'https://evil.example/', want: '/' },
    { value: 'javascript:alert(1)', want: '/' },
    { value: '/login', want: '/' },
    { value: '/login?redirect=%2Fnodes', want: '/' },
    { value: '/setup', want: '/' },
  ])('returns $want for $value', ({ value, want }) => {
    expect(safeReturnPath(value)).toBe(want)
  })
})

describe('loginPath', () => {
  it.each([
    { returnTo: '/', reason: undefined, want: '/login' },
    { returnTo: '/', reason: 'session-expired' as const, want: '/login?reason=session-expired' },
    {
      returnTo: '/admin/users?page=2',
      reason: undefined,
      want: '/login?redirect=%2Fadmin%2Fusers%3Fpage%3D2',
    },
    {
      returnTo: '/nodes',
      reason: 'session-expired' as const,
      want: '/login?reason=session-expired&redirect=%2Fnodes',
    },
    { returnTo: '//evil.example', reason: undefined, want: '/login' },
  ])('builds $want', ({ returnTo, reason, want }) => {
    expect(loginPath(returnTo, reason)).toBe(want)
  })
})
