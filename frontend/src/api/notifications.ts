import { Notify, type QNotifyCreateOptions } from 'quasar'

import { connectErrorMessage } from '@/api/connect-errors'

export type XylonaNotificationOptions = Partial<QNotifyCreateOptions>

const xylonaNotificationPosition = 'top'

function createXylonaNotification(
  type: 'xylona-success' | 'xylona-error' | 'xylona-alert' | 'xylona-info',
  caption: string,
  overrides: XylonaNotificationOptions = {},
): void {
  Notify.create({
    type,
    caption,
    position: xylonaNotificationPosition,
    ...overrides,
  })
}

export function notifySuccess(caption: string, overrides: XylonaNotificationOptions = {}): void {
  createXylonaNotification('xylona-success', caption, {
    timeout: 3000,
    ...overrides,
  })
}

export function notifyError(caption: string, overrides: XylonaNotificationOptions = {}): void {
  createXylonaNotification('xylona-error', caption, {
    timeout: 5000,
    ...overrides,
  })
}

export function notifyWarning(caption: string, overrides: XylonaNotificationOptions = {}): void {
  createXylonaNotification('xylona-alert', caption, {
    timeout: 5000,
    ...overrides,
  })
}

export function notifyInfo(caption: string, overrides: XylonaNotificationOptions = {}): void {
  createXylonaNotification('xylona-info', caption, {
    timeout: 3000,
    ...overrides,
  })
}

export function notifyConnectError(
  error: unknown,
  errorPrefix?: string,
  overrides: XylonaNotificationOptions = {},
): void {
  notifyError(connectErrorMessage(error, errorPrefix), overrides)
}
