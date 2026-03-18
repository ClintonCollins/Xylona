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
export default boot(async ({ app }) => {
  Notify.registerType('xylona-success', {
    message: '<span class="text-bold notification-title">Success</span>',
    icon: tabOutlineCheck,
    html: true,
    classes: 'xylona-notification notification-success',
  })
  Notify.registerType('xylona-error', {
    message: '<span class="text-bold notification-title">Error</span>',
    icon: tabOutlineX,
    html: true,
    classes: 'xylona-notification notification-error',
  })
  Notify.registerType('xylona-alert', {
    message: '<span class="text-bold notification-title">Alert</span>',
    icon: tabOutlineAlertTriangle,
    html: true,
    classes: 'xylona-notification notification-alert',
  })
  Notify.registerType('xylona-info', {
    message: '<span class="text-bold notification-title">Info</span>',
    icon: tabOutlineInfoSquareRounded,
    html: true,
    classes: 'xylona-notification notification-info',
  })
})
