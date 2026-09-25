import { route } from 'quasar/wrappers'
import { nextTick } from 'vue'
import {
  createMemoryHistory,
  createRouter,
  createWebHashHistory,
  createWebHistory,
} from 'vue-router'

import { setPageTitle } from '@/utils/page-title'
import routes from './routes'

/*
 * If not building with SSR mode, you can
 * directly export the Router instantiation;
 *
 * The function below can be async too; either use
 * async/await or return a Promise which resolves
 * with the Router instance.
 */

export default route(function (/* { store, ssrContext } */) {
  const createHistory = process.env['SERVER']
    ? createMemoryHistory
    : process.env.VUE_ROUTER_MODE === 'history'
      ? createWebHistory
      : createWebHashHistory

  const Router = createRouter({
    scrollBehavior: () => ({ left: 0, top: 0 }),
    routes,

    // Leave this as is and make changes in quasar.conf.js instead!
    // quasar.conf.js -> build -> vueRouterMode
    // quasar.conf.js -> build -> publicPath
    history: createHistory(process.env.VUE_ROUTER_BASE),
  })

  // Pages that know a more specific name (a server, a public map) refine this
  // after it runs. A failed navigation (the active tab clicked again, a guard
  // abort) never reached `to`, so it keeps the refined title.
  Router.afterEach((to, from, failure) => {
    if (!failure) {
      setPageTitle(to.meta.title)
    }
    // Start keyboard and screen-reader users on the new page, not on the link
    // they left. Skipped on first load and on query-only changes (?range=, ?q=),
    // and when focus is already inside the page: an in-page tab bar or a field
    // the new page autofocused.
    if (!failure && from.matched.length > 0 && to.path !== from.path) {
      void nextTick(() => {
        const main = document.getElementById('main-content')
        if (main === null || main.contains(document.activeElement)) return
        main.focus({ preventScroll: true })
      })
    }
  })

  return Router
})
