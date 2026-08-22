import { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import { useUserAuthStore } from '@/stores/xylona'
import { CheckUserAuthenticatedResponse } from '@/proto/xylona_pb'
import { canViewAlerts } from '@/utils/alert-permissions'
import { legacyGameServerEditRedirect } from './game-server-route-helpers'

const requireSuperUser = async () => {
  const store = useUserAuthStore()
  const resp: CheckUserAuthenticatedResponse | null = await store.checkUserAuthenticated()
  if (!resp || !resp.user || !resp.authenticated) {
    return { path: '/login' }
  }
  if (!resp.user.superUser) {
    return { path: '/' }
  }
}

const requireAlertAccess = async () => {
  const store = useUserAuthStore()
  const resp: CheckUserAuthenticatedResponse | null = await store.checkUserAuthenticated()
  if (!resp || !resp.user || !resp.authenticated) {
    return { path: '/login' }
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
  },
  {
    path: '/status/:identifier',
    component: () => import('pages/PublicGameServerStatusPage.vue'),
  },
  {
    path: '/login',
    component: () => import('pages/Login.vue'),
    beforeEnter: async (to: RouteLocationNormalized, from: RouteLocationNormalized) => {
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
  // Regular routes
  {
    path: '/',
    beforeEnter: async (_to: RouteLocationNormalized, _from: RouteLocationNormalized) => {
      const resp: CheckUserAuthenticatedResponse | null =
        await useUserAuthStore().checkUserAuthenticated()
      if (useUserAuthStore().user === null && (!resp || !resp.authenticated)) {
        return { path: '/login' }
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
      },
      {
        path: 'games',
        component: () => import('pages/games/GameList.vue'),
      },
      {
        path: 'games/new',
        component: () => import('pages/games/GameCreateWizard.vue'),
      },
      {
        path: 'games/create',
        component: () => import('pages/games/GameFormPage.vue'),
      },
      {
        path: 'games/:id/edit',
        component: () => import('pages/games/GameFormPage.vue'),
        props: { mode: 'edit' },
      },
      {
        path: 'games/:id/copy',
        component: () => import('pages/games/GameFormPage.vue'),
        props: { mode: 'copy' },
      },
      {
        path: 'games/:id/config-schema/:fileIndex',
        component: () => import('pages/games/GameConfigSchema.vue'),
      },
      {
        path: 'game-servers/create',
        component: () => import('pages/game_servers/CreateGameServer.vue'),
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
          },
          {
            path: 'files',
            component: () => import('pages/game_servers/GameServerFiles.vue'),
          },
          {
            path: 'metrics',
            component: () => import('pages/game_servers/GameServerMetrics.vue'),
          },
          {
            path: 'configuration',
            component: () => import('pages/game_servers/GameServerConfig.vue'),
          },
          {
            path: 'settings',
            component: () => import('pages/game_servers/GameServerSettings.vue'),
          },
          {
            path: 'start-command',
            component: () => import('pages/game_servers/GameServerStartArgs.vue'),
          },
          {
            path: 'mods',
            name: 'game-server-mods',
            component: () => import('pages/game_servers/GameServerMods.vue'),
          },
          {
            path: 'alerts',
            component: () => import('pages/game_servers/GameServerAlerts.vue'),
          },
          {
            path: 'schedules',
            component: () => import('pages/game_servers/GameServerSchedules.vue'),
          },
          {
            path: 'backups',
            component: () => import('pages/game_servers/GameServerBackups.vue'),
          },
          {
            path: 'access',
            component: () => import('components/game_servers/GameServerAccess.vue'),
          },
          {
            path: '',
            component: () => import('pages/game_servers/GameServerView.vue'),
          },
        ],
      },
      {
        path: '/nodes',
        component: () => import('pages/nodes/NodeList.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/nodes/add',
        component: () => import('pages/nodes/NodeAdd.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/nodes/:id/edit',
        component: () => import('pages/nodes/NodeEdit.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/notifications',
        component: () => import('pages/other/Notifications.vue'),
        beforeEnter: requireAlertAccess,
      },
      {
        path: '/admin/users',
        component: () => import('pages/admin/UserList.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/users/create',
        component: () => import('pages/admin/UserCreate.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/users/:id/edit',
        component: () => import('pages/admin/UserEdit.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/settings',
        component: () => import('pages/admin/ControllerSettings.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/admin/updates',
        component: () => import('pages/admin/SystemUpdates.vue'),
        beforeEnter: requireSuperUser,
      },
    ],
  },
  // Always leave this as last one,
  // but you can also remove it
  {
    path: '/:catchAll(.*)*',
    component: () => import('pages/ErrorNotFound.vue'),
  },
]

export default routes
