import type { CheckUserAuthenticatedResponse, User } from '@/proto/xylona_pb'

export const permissionAlertsManage = 'alerts.manage'
export const permissionAlertsViewHistory = 'alerts.view_history'

function hasAlertPermission(
  authResponse: CheckUserAuthenticatedResponse | null | undefined,
  permissionID: string,
): boolean {
  return authResponse?.permissionIds.includes(permissionID) ?? false
}

export function canManageAlerts(
  user: User | null | undefined,
  authResponse: CheckUserAuthenticatedResponse | null | undefined,
): boolean {
  if (user?.superUser) {
    return true
  }

  return hasAlertPermission(authResponse, permissionAlertsManage)
}

export function canViewAlerts(
  user: User | null | undefined,
  authResponse: CheckUserAuthenticatedResponse | null | undefined,
): boolean {
  if (canManageAlerts(user, authResponse)) {
    return true
  }

  return hasAlertPermission(authResponse, permissionAlertsViewHistory)
}
