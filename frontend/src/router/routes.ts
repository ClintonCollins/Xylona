import { RouteLocationNormalized, RouteRecordRaw } from 'vue-router'
import { useUserAuthStore } from 'src/stores/xylona'
import { CheckUserAuthenticatedResponse } from 'src/proto/xylona_pb'

const routes: RouteRecordRaw[] = [
  // Unauthenticated routes
  {
    path: '/login',
    component: () => import('pages/Login.vue'),
    beforeEnter: async (to: RouteLocationNormalized, from: RouteLocationNormalized) => {
      const resp: CheckUserAuthenticatedResponse | null = await useUserAuthStore().checkUserAuthenticated()
      if (resp && resp.user && resp.authenticated) {
        if (from.path !== to.path) {
          return {path: from.path}
        }
        return {path: '/'}
      }
    }
  },
  // Regular routes
  {
    path: '/',
    beforeEnter: async (to: RouteLocationNormalized, from: RouteLocationNormalized) => {
      const resp: CheckUserAuthenticatedResponse | null = await useUserAuthStore().checkUserAuthenticated()
      if (useUserAuthStore().user === null && (!resp || !resp.authenticated)) {
        return {path: '/login'}
      }
    },
    component: () => import('layouts/MainLayout.vue'),
    children: [
      {
        path: '',
        component: () => import('pages/IndexPage.vue')
      },
      {
        path: '/game-servers',
        component: () => import('pages/game_servers/GameServerList.vue')
      },
      {
        path: 'games',
        component: () => import('pages/games/GameList.vue')
      },
      {
        path: 'games/create',
        component: () => import('pages/games/GameCreate.vue')
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
        path: 'game-servers/create',
        component: () => import('pages/game_servers/CreateGameServer.vue')
      },
      {
        path: '/game-servers/:id/edit',
        component: () => import('pages/game_servers/GameServerEdit.vue')
      },
      {
        path: '/game-servers/:id',
        component: () => import('pages/game_servers/GameServerLayout.vue'),
        children: [
          {
            path: 'console',
            component: () => import('pages/game_servers/GameServerView.vue')
          },
          {
            path: 'files',
            component: () => import('pages/game_servers/GameServerFiles.vue')
          },
          {
            path: '',
            component: () => import('pages/game_servers/GameServerView.vue')
          },
        ]
      }
    ]
  },
  // Admin routes
  {
    path: '/admin',
    // beforeEnter: async (to: RouteLocationNormalized, from: RouteLocationNormalized) => {
    //   const resp: CheckUserAuthenticatedResponse| null = await useUserAuthStore().checkUserAuthenticated()
    //   if (!resp || !resp.user || !resp.authenticated) {
    //     return {path: '/login'}
    //   }
    //   if (!resp.user.superUser) {
    //     return from
    //   }
    // },
    component: () => import('layouts/MainLayout.vue'),
    children: [
      {
        path: 'create-user', component: () => import('pages/admin/CreateUser.vue')
      }
    ]
  },
  // Always leave this as last one,
  // but you can also remove it
  {
    path: '/:catchAll(.*)*',
    component: () => import('pages/ErrorNotFound.vue')
  }
]

export default routes
