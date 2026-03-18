export interface GameServerLayoutTab {
  name: string
  to: string
  icon: string
  exact: boolean
}

export function buildGameServerTabs(
  serverID: string,
  canUseConfigurationTab: boolean,
  canUseAccessTab: boolean,
): GameServerLayoutTab[] {
  const basePath = `/game-servers/${serverID}`
  const tabs: GameServerLayoutTab[] = [
    { name: 'Console', to: `${basePath}/console`, icon: 'terminal', exact: true },
    { name: 'Files', to: `${basePath}/files`, icon: 'folder', exact: true },
    { name: 'Metrics', to: `${basePath}/metrics`, icon: 'show_chart', exact: true },
  ]

  if (canUseConfigurationTab) {
    tabs.push({
      name: 'Configuration',
      to: `${basePath}/configuration`,
      icon: 'settings',
      exact: true,
    })
  }
  if (canUseAccessTab) {
    tabs.push({ name: 'Access', to: `${basePath}/access`, icon: 'manage_accounts', exact: true })
  }

  return tabs
}

export function getUnauthorizedRedirect(
  currentPath: string,
  serverID: string,
  canUseConfigurationTab: boolean,
  canUseAccessTab: boolean,
): string | null {
  const basePath = `/game-servers/${serverID}`
  const consolePath = `${basePath}/console`

  if (!canUseAccessTab && currentPath === `${basePath}/access`) {
    return consolePath
  }

  if (!canUseConfigurationTab && currentPath === `${basePath}/configuration`) {
    return consolePath
  }

  return null
}
