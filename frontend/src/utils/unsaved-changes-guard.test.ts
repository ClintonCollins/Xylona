import { enableAutoUnmount, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { useUnsavedChangesGuard } from './unsaved-changes-guard'

const mocks = vi.hoisted(() => ({
  dialog: vi.fn(),
  leaveGuard: undefined as undefined | (() => boolean | Promise<boolean>),
  updateGuard: undefined as
    undefined | ((to: { path: string }, from: { path: string }) => boolean | Promise<boolean>),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return { ...actual, useQuasar: () => ({ dialog: mocks.dialog }) }
})

vi.mock('vue-router', () => ({
  onBeforeRouteLeave: (guard: () => boolean | Promise<boolean>) => {
    mocks.leaveGuard = guard
  },
  onBeforeRouteUpdate: (
    guard: (to: { path: string }, from: { path: string }) => boolean | Promise<boolean>,
  ) => {
    mocks.updateGuard = guard
  },
}))

enableAutoUnmount(afterEach)

function answerDialog(answer: 'ok' | 'cancel') {
  mocks.dialog.mockImplementation(() => {
    const chain = {
      onOk: (handler: () => void) => {
        if (answer === 'ok') handler()
        return chain
      },
      onCancel: (handler: () => void) => {
        if (answer === 'cancel') handler()
        return chain
      },
      onDismiss: () => chain,
    }
    return chain
  })
}

function mountGuard(dirty: boolean) {
  const isDirty = ref(dirty)
  let confirmDiscard: ReturnType<typeof useUnsavedChangesGuard>['confirmDiscard'] = () =>
    Promise.resolve(true)
  const wrapper = mount(
    defineComponent({
      setup() {
        confirmDiscard = useUnsavedChangesGuard(isDirty).confirmDiscard
        return {}
      },
      template: '<div />',
    }),
  )
  return { wrapper, isDirty, confirmDiscard: (message?: string) => confirmDiscard(message) }
}

describe('useUnsavedChangesGuard', () => {
  beforeEach(() => {
    mocks.dialog.mockReset()
    mocks.leaveGuard = undefined
    mocks.updateGuard = undefined
  })

  it('lets a clean form leave without asking', async () => {
    mountGuard(false)

    await expect(Promise.resolve(mocks.leaveGuard?.())).resolves.toBe(true)
    expect(mocks.dialog).not.toHaveBeenCalled()
  })

  it.each([
    { answer: 'ok' as const, leaves: true },
    { answer: 'cancel' as const, leaves: false },
  ])('asks before a dirty form leaves ($answer)', async ({ answer, leaves }) => {
    answerDialog(answer)
    mountGuard(true)

    await expect(Promise.resolve(mocks.leaveGuard?.())).resolves.toBe(leaves)
    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Unsaved Changes', persistent: true }),
    )
  })

  it('asks when the same page opens for another server, not on a query-only change', async () => {
    answerDialog('cancel')
    mountGuard(true)

    await expect(
      Promise.resolve(
        mocks.updateGuard?.(
          { path: '/game-servers/a/settings' },
          { path: '/game-servers/a/settings' },
        ),
      ),
    ).resolves.toBe(true)
    expect(mocks.dialog).not.toHaveBeenCalled()

    await expect(
      Promise.resolve(
        mocks.updateGuard?.(
          { path: '/game-servers/b/settings' },
          { path: '/game-servers/a/settings' },
        ),
      ),
    ).resolves.toBe(false)
    expect(mocks.dialog).toHaveBeenCalledOnce()
  })

  it('uses a custom message for in-page discards', async () => {
    answerDialog('ok')
    const { confirmDiscard } = mountGuard(true)

    await expect(confirmDiscard('Discard them and switch files?')).resolves.toBe(true)
    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({ message: 'Discard them and switch files?' }),
    )
  })

  it('blocks tab close only while dirty and stops listening after unmount', () => {
    const { wrapper, isDirty } = mountGuard(true)

    const dirtyEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(dirtyEvent)
    expect(dirtyEvent.defaultPrevented).toBe(true)

    isDirty.value = false
    const cleanEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(cleanEvent)
    expect(cleanEvent.defaultPrevented).toBe(false)

    isDirty.value = true
    wrapper.unmount()
    const unmountedEvent = new Event('beforeunload', { cancelable: true })
    window.dispatchEvent(unmountedEvent)
    expect(unmountedEvent.defaultPrevented).toBe(false)
  })
})
