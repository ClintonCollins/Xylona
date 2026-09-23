import { useQuasar } from 'quasar'
import { onBeforeUnmount, onMounted, toValue, type MaybeRefOrGetter } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate } from 'vue-router'

const defaultDiscardMessage = 'You have unsaved changes. Are you sure you want to leave?'

/**
 * Protects a form's unsaved edits: leaving the route asks first, closing or
 * reloading the tab triggers the browser prompt, and confirmDiscard() lets the
 * form ask before any other action that would drop the edits. Moving to the
 * same page of another server reuses the route record, so a path change asks
 * too; a query- or hash-only change does not.
 */
export function useUnsavedChangesGuard(isDirty: MaybeRefOrGetter<boolean>) {
  const $q = useQuasar()

  function confirmDiscard(message = defaultDiscardMessage): Promise<boolean> {
    if (!toValue(isDirty)) {
      return Promise.resolve(true)
    }

    return new Promise<boolean>((resolve) => {
      $q.dialog({
        title: 'Unsaved Changes',
        message,
        cancel: { flat: true, label: 'Stay' },
        ok: { color: 'negative', label: 'Discard Changes' },
        persistent: true,
      })
        .onOk(() => resolve(true))
        .onCancel(() => resolve(false))
        .onDismiss(() => resolve(false))
    })
  }

  function onBeforeUnload(event: BeforeUnloadEvent) {
    if (toValue(isDirty)) {
      event.preventDefault()
    }
  }

  onMounted(() => {
    window.addEventListener('beforeunload', onBeforeUnload)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('beforeunload', onBeforeUnload)
  })

  onBeforeRouteLeave(() => confirmDiscard())
  onBeforeRouteUpdate((to, from) => to.path === from.path || confirmDiscard())

  return { confirmDiscard }
}
