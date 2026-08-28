import {
  tabOutlineAlertTriangle,
  tabOutlineCheck,
  tabOutlineInfoSquareRounded,
  tabOutlineX,
} from 'quasar-extras-svg-icons/tabler-icons-v3'
import { boot } from 'quasar/wrappers'
import { Notify } from 'quasar'

// "async" is optional;
// more info on params: https://v2.quasar.dev/quasar-cli/boot-files
export default boot(async ({ app: _app }) => {
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
})
