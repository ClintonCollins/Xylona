import { Code, ConnectError } from '@connectrpc/connect'
import type { QNotifyCreateOptions } from 'quasar'

export type XylonaErrorNotification = QNotifyCreateOptions

export function connectErrorToString(err: ConnectError): string {
  switch (err.code) {
    case Code.Unavailable:
      return 'Unable to connect to Xylona backend.'
    default:
      return err.message
  }
}

export function connectErrorMessage(error: unknown, errorPrefix?: string): string {
  const message = connectErrorToString(ConnectError.from(error))
  return errorPrefix ? `${errorPrefix}: ${message}` : message
}

export function buildXylonaErrorNotification(
  caption: string,
  overrides: Partial<XylonaErrorNotification> = {},
): XylonaErrorNotification {
  return {
    type: 'xylona-error',
    caption,
    position: 'top',
    timeout: 5000,
    ...overrides,
  }
}
