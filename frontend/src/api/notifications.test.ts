import { afterEach, describe, expect, it, vi } from 'vitest'

import {
  notifyConnectError,
  notifyError,
  notifyInfo,
  notifySuccess,
  notifyWarning,
} from './notifications'

const mocks = vi.hoisted(() => ({
  create: vi.fn(),
  connectErrorMessage: vi.fn(),
}))

vi.mock('quasar', () => ({
  Notify: {
    create: mocks.create,
  },
}))

vi.mock('@/api/connect-errors', () => ({
  connectErrorMessage: mocks.connectErrorMessage,
}))

describe('notifications', () => {
  afterEach(() => {
    mocks.create.mockReset()
    mocks.connectErrorMessage.mockReset()
  })

  it('creates a shared success notification preset', () => {
    notifySuccess('Saved', { icon: 'check' })

    expect(mocks.create).toHaveBeenCalledWith({
      type: 'xylona-success',
      caption: 'Saved',
      position: 'top',
      timeout: 3000,
      icon: 'check',
    })
  })

  it('creates a shared warning notification preset', () => {
    notifyWarning('Careful', { timeout: 0 })

    expect(mocks.create).toHaveBeenCalledWith({
      type: 'xylona-alert',
      caption: 'Careful',
      position: 'top',
      timeout: 0,
    })
  })

  it('creates a shared error notification preset', () => {
    notifyError('Something broke', { closeBtn: 'Dismiss' })

    expect(mocks.create).toHaveBeenCalledWith({
      type: 'xylona-error',
      caption: 'Something broke',
      position: 'top',
      timeout: 5000,
      closeBtn: 'Dismiss',
    })
  })

  it('creates a shared info notification preset', () => {
    notifyInfo('FYI')

    expect(mocks.create).toHaveBeenCalledWith({
      type: 'xylona-info',
      caption: 'FYI',
      position: 'top',
      timeout: 3000,
    })
  })

  it('formats connect errors through the shared error helper', () => {
    mocks.connectErrorMessage.mockReturnValue('Failed to load: boom')

    notifyConnectError(new Error('boom'), 'Failed to load')

    expect(mocks.connectErrorMessage).toHaveBeenCalledWith(expect.any(Error), 'Failed to load')
    expect(mocks.create).toHaveBeenCalledWith({
      type: 'xylona-error',
      caption: 'Failed to load: boom',
      position: 'top',
      timeout: 5000,
    })
  })
})
