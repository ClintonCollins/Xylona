import { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import { useUserAuthStore } from 'src/stores/xylona'
import { CheckUserAuthenticatedResponse } from 'src/proto/xylona_pb'

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

const routes: RouteRecordRaw[] = [
  // Unauthenticated routes
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
        component: () => import('pages/IndexPage.vue'),
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
        component: () => import('pages/games/GameCreate.vue'),
      },
      {
        path: 'games/:id/edit',
        component: () => import('pages/games/GameEdit.vue'),
      },
      {
        path: 'games/:id/copy',
        component: () => import('pages/games/GameCopy.vue'),
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
        component: () => import('pages/game_servers/GameServerEdit.vue'),
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
        path: '/nodes/activity',
        component: () => import('pages/nodes/NodeActivity.vue'),
        beforeEnter: requireSuperUser,
      },
      {
        path: '/secret-keys',
        component: () => import('pages/other/LocalSecretKeyList.vue'),
        beforeEnter: requireSuperUser,
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
