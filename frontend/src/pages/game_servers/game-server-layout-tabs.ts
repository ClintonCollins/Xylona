export interface GameServerLayoutTab {
  name: string
  to: string
  icon: string
  exact: boolean
  requiredPermission?: string
}

export function buildGameServerTabs(
  serverID: string,
  permissions: string[],
  isOwnerOrSuper: boolean,
): GameServerLayoutTab[] {
  const basePath = `/game-servers/${serverID}`
  const has = (perm: string) => permissions.includes(perm)

  const tabs: GameServerLayoutTab[] = [
    { name: 'Console', to: `${basePath}/console`, icon: 'terminal', exact: true },
  ]

  if (has('game_server.files.view')) {
    tabs.push({
      name: 'Files',
      to: `${basePath}/files`,
      icon: 'folder',
      exact: true,
      requiredPermission: 'game_server.files.view',
    })
  }
  if (has('game_server.metrics')) {
    tabs.push({
      name: 'Metrics',
      to: `${basePath}/metrics`,
      icon: 'show_chart',
      exact: true,
      requiredPermission: 'game_server.metrics',
    })
  }
  if (has('game_server.settings')) {
    tabs.push({
      name: 'Configuration',
      to: `${basePath}/configuration`,
      icon: 'settings',
      exact: true,
      requiredPermission: 'game_server.settings',
    })
  }
  if (isOwnerOrSuper) {
    tabs.push({
      name: 'Access',
      to: `${basePath}/access`,
      icon: 'manage_accounts',
      exact: true,
    })
  }

  return tabs
}

export function getUnauthorizedRedirect(
  currentPath: string,
  serverID: string,
  permissions: string[],
  isOwnerOrSuper: boolean,
): string | null {
  const basePath = `/game-servers/${serverID}`
  const consolePath = `${basePath}/console`
  const has = (perm: string) => permissions.includes(perm)

  if (currentPath === `${basePath}/files` && !has('game_server.files.view')) {
    return consolePath
  }
  if (currentPath === `${basePath}/metrics` && !has('game_server.metrics')) {
    return consolePath
  }
  if (currentPath === `${basePath}/configuration` && !has('game_server.settings')) {
    return consolePath
  }
  if (currentPath === `${basePath}/access` && !isOwnerOrSuper) {
    return consolePath
  }

  return null
}
