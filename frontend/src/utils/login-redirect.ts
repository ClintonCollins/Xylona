/**
 * The in-app page to open after signing in. Anything that is not a
 * same-origin path, or that would lead back to sign-in or setup, becomes '/'.
 */
export function safeReturnPath(value: unknown): string {
  if (typeof value !== 'string' || !value.startsWith('/') || value.startsWith('//')) {
    return '/'
  }

  // The URL parser also catches tricks such as '/\evil.example'.
  const url = new URL(value, window.location.origin)
  if (url.origin !== window.location.origin || ['/login', '/setup'].includes(url.pathname)) {
    return '/'
  }
  return `${url.pathname}${url.search}${url.hash}`
}

/** The sign-in URL that returns the user to `returnTo` afterwards. */
export function loginPath(returnTo: string, reason?: 'session-expired'): string {
  const params = new URLSearchParams()
  if (reason !== undefined) {
    params.set('reason', reason)
  }
  const target = safeReturnPath(returnTo)
  if (target !== '/') {
    params.set('redirect', target)
  }
  const query = params.toString()
  return query === '' ? '/login' : `/login?${query}`
}

/** The page the browser is showing, as a path the router can return to. */
export function currentPagePath(): string {
  return `${window.location.pathname}${window.location.search}${window.location.hash}`
}
