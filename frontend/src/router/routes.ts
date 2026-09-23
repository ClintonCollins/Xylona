import { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import { useUserAuthStore } from '@/stores/xylona'
import { CheckUserAuthenticatedResponse } from '@/proto/xylona_pb'
import { canViewAlerts } from '@/utils/alert-permissions'
import { loginPath } from '@/utils/login-redirect'
import { fetchSetupStatus, unauthenticatedRedirect } from '@/utils/setup-status'
import { legacyGameServerEditRedirect } from './game-server-route-helpers'

declare module 'vue-router' {
  interface RouteMeta {
    /** Page name for the browser tab; the router appends "· Xylona". */
    title?: string
  }
}

async function redirectForFirstRun(to: RouteLocationNormalized) {
  try {
    const status = await fetchSetupStatus()
    const token = typeof to.query['token'] === 'string' ? to.query['token'] : ''
    const redirect = unauthenticatedRedirect(status.needed, to.path, token)
    if (redirect === null) {
      return
    }
    return redirect
  } catch {
    return
  }
}

const requireSuperUser = async (to: RouteLocationNormalized) => {
  const store = useUserAuthStore()
  const resp: CheckUserAuthenticatedResponse | null = await store.checkUserAuthenticated()
  if (!resp || !resp.user || !resp.authenticated) {
    return loginPath(to.fullPath)
  }
  if (!resp.user.superUser) {
    return { path: '/' }
  }
}

const requireAlertAccess = async (to: RouteLocationNormalized) => {
  const store = useUserAuthStore()
  const resp: CheckUserAuthenticatedResponse | null = await store.checkUserAuthenticated()
  if (!resp || !resp.user || !resp.authenticated) {
    return loginPath(to.fullPath)
  }
  if (!canViewAlerts(resp.user, resp)) {
    return { path: '/' }
  }
}

const routes: RouteRecordRaw[] = [
  // Unauthenticated routes
  {
    path: '/maps/:identifier',
    component: () => import('pages/PublicGameServerMap.vue'),
    meta: { title: 'Live map' },
  },
  {
    path: '/status/:identifier',
    component: () => import('pages/PublicGameServerStatusPage.vue'),
    meta: { title: 'Status' },
  },
  {
    path: '/login',
    component: () => import('pages/Login.vue'),
    meta: { title: 'Sign in' },
    beforeEnter: async (to: RouteLocationNormalized, from: RouteLocationNormalized) => {
      const setupRedirect = await redirectForFirstRun(to)
      if (setupRedirect) {
        return setupRedirect
      }
      const resp: CheckUserAuthenticatedResponse | null =
        await useUserAuthStore().checkUserAuthenticated()
      if (resp && resp.user && resp.authenticated) {
        if (from.path !== to.path) {
          return { path: from.path }
        }
        return { path: '/' }
      }
    },
  },
  {
    path: '/setup',
    component: () => import('pages/Setup.vue'),
    meta: { title: 'Setup' },
    beforeEnter: async (to: RouteLocationNormalized) => {
      return redirectForFirstRun(to)
    },
  },
  // Regular routes
  {
    path: '/',
    beforeEnter: async (to: RouteLocationNormalized, _from: RouteLocationNormalized) => {
      const resp: CheckUserAuthenticatedResponse | null =
        await useUserAuthStore().checkUserAuthenticated()
      if (useUserAuthStore().user === null && (!resp || !resp.authenticated)) {
        const setupRedirect = await redirectForFirstRun(to)
        if (setupRedirect) {
          return setupRedirect
        }
        return loginPath(to.fullPath)
      }
    },
    component: () => import('layouts/MainLayout.vue'),
    children: [
      {
        path: '',
        redirect: '/game-servers',
      },
      {
        path: '/game-servers',
        component: () => import('pages/game_servers/GameServerList.vue'),
        meta: { title: 'Game servers' },
      },
      {
        path: 'games',
        component: () => import('pages/games/GameList.vue'),
        meta: { title: 'Games' },
      },
      {
        path: 'games/new',
        component: () => import('pages/games/GameCreateWizard.vue'),
        meta: { title: 'New game' },
      },
      {
        path: 'games/create',
        component: () => import('pages/games/GameFormPage.vue'),
        meta: { title: 'New game' },
      },
      {
        path: 'games/:id/edit',
        component: () => import('pages/games/GameFormPage.vue'),
        meta: { title: 'Edit game' },
        props: { mode: 'edit' },
      },
      {
        path: 'games/:id/copy',
        component: () => import('pages/games/GameFormPage.vue'),
        meta: { title: 'Copy game' },
        props: { mode: 'copy' },
      },
      {
        path: 'games/:id/config-schema/:fileIndex',
        component: () => import('pages/games/GameConfigSchema.vue'),
        meta: { title: 'Config schema' },
      },
      {
        path: 'game-servers/create',
        component: () => import('pages/game_servers/CreateGameServer.vue'),
        meta: { title: 'New game server' },
      },
      {
        path: '/game-servers/:id/edit',
        redirect: (to) => legacyGameServerEditRedirect(String(to.params['id'])),
      },
      {
        path: '/game-servers/:id',
        component: () => import('pages/game_servers/GameServerLayout.vue'),
        children: [
          {
            path: 'console',
            component: () => import('pages/game_servers/GameServerView.vue'),
            meta: { title: 'Console' },
          },
          {
            path: 'operations',
            component: () => import('pages/game_servers/GameServerOperations.vue'),
            meta: { title: 'Operations' },
          },
          {
            // Player management lives on the console page now; keep old
            // bookmarks working.
            path: 'players',
            redirect: (to) => `/game-servers/${String(to.params['id'])}/console`,
          },
          {
            path: 'map',
            component: () => import('pages/game_servers/GameServerMap.vue'),
            meta: { title: 'Map' },
          },
          {
            path: 'files',
            component: () => import('pages/game_servers/GameServerFiles.vue'),
            meta: { title: 'Files' },
          },
          {
            path: 'metrics',
            component: () => import('pages/game_servers/GameServerMetrics.vue'),
            meta: { title: 'Metrics' },
          },
          {
            path: 'configuration',
            component: () => import('pages/game_servers/GameServerConfig.vue'),
            meta: { title: 'Configuration' },
          },
          {
            path: 'settings',
            component: () => import('pages/game_servers/GameServerSettings.vue'),
            meta: { title: 'Settings' },
          },
          {
            path: 'start-command',
            component: () => import('pages/game_servers/GameServerStartArgs.vue'),
            meta: { title: 'Start Command' },
          },
          {
            path: 'mods',
            name: 'game-server-mods',
            component: () => import('pages/game_servers/GameServerMods.vue'),
            meta: { title: 'Mods' },
          },
          {
            path: 'alerts',
            component: () => import('pages/game_servers/GameServerAlerts.vue'),
            meta: { title: 'Alerts' },
          },
          {
            path: 'schedules',
            component: () => import('pages/game_servers/GameServerSchedules.vue'),
            meta: { title: 'Schedules' },
          },
          {
            path: 'backups',
            component: () => import('pages/game_servers/GameServerBackups.vue'),
            meta: { title: 'Backups' },
          },
          {
            path: 'access',
            component: () => import('components/game_servers/GameServerAccess.vue'),
            meta: { title: 'Access' },
          },
          {
            path: '',
            redirect: (to) => `/game-servers/${String(to.params['id'])}/console`,
          },
        ],
      },
      {
        path: '/nodes',
        component: () => import('pages/nodes/NodeList.vue'),
        meta: { title: 'Nodes' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/nodes/add',
        component: () => import('pages/nodes/NodeAdd.vue'),
        meta: { title: 'Add node' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/nodes/:id',
        component: () => import('pages/nodes/NodeList.vue'),
        meta: { title: 'Nodes' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/nodes/:id/edit',
        component: () => import('pages/nodes/NodeEdit.vue'),
        meta: { title: 'Edit node' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/notifications',
        component: () => import('pages/other/Notifications.vue'),
        meta: { title: 'Notifications' },
        beforeEnter: requireAlertAccess,
      },
      {
        path: '/admin/users',
        component: () => import('pages/admin/UserList.vue'),
        meta: { title: 'Users' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/users/create',
        component: () => import('pages/admin/UserCreate.vue'),
        meta: { title: 'Create user' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/users/:id/edit',
        component: () => import('pages/admin/UserEdit.vue'),
        meta: { title: 'Edit user' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/settings',
        component: () => import('pages/admin/ControllerSettings.vue'),
        meta: { title: 'Controller settings' },
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/updates',
        component: () => import('pages/admin/SystemUpdates.vue'),
        meta: { title: 'System updates' },
        beforeEnter: requireSuperUser,
      },
    ],
  },
  // Always leave this as last one,
  // but you can also remove it
  {
    path: '/:catchAll(.*)*',
    component: () => import('pages/ErrorNotFound.vue'),
    meta: { title: 'Page not found' },
  },
]

export default routes
