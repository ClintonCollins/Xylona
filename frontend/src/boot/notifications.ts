import {
  tabOutlineAlertTriangle,
  tabOutlineCheck,
  tabOutlineInfoSquareRounded,
  tabOutlineX,
} from 'quasar-extras-svg-icons/tabler-icons-v3'
import { boot } from 'quasar/wrappers'
import { Dialog, Notify, type QDialogOptions } from 'quasar'

// "async" is optional;
// more info on params: https://v2.quasar.dev/quasar-cli/boot-files
export default boot(async ({ app }) => {
  // One toast position for the whole app; app.css keeps it below the header.
  Notify.setDefaults({ position: 'top' })

  Notify.registerType('xylona-success', {
    message: 'Success',
    icon: tabOutlineCheck,
    classes: 'xylona-notification notification-success',
  })
  Notify.registerType('xylona-error', {
    message: 'Error',
    icon: tabOutlineX,
    classes: 'xylona-notification notification-error',
  })
  Notify.registerType('xylona-alert', {
    message: 'Alert',
    icon: tabOutlineAlertTriangle,
    classes: 'xylona-notification notification-alert',
  })
  Notify.registerType('xylona-info', {
    message: 'Info',
    icon: tabOutlineInfoSquareRounded,
    classes: 'xylona-notification notification-info',
  })

  // Quasar colours plugin dialogs (Cancel, prompt inputs) caution amber in dark
  // mode. Default them to primary so a Cancel never reads as a warning.
  const $q = app.config.globalProperties.$q
  const createDialog = $q.dialog
  $q.dialog = Dialog.create = (opts: QDialogOptions) => createDialog({ color: 'primary', ...opts })
})
