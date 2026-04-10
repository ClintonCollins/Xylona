<template>
  <q-page :padding="windowWidth > 1024" class="game-server-page">
    <q-card class="full-width game-server-card">
      <q-tabs
        v-if="navQTabsStore.tabs.length > 0"
        class="game-server-tabs"
        active-color="primary"
        indicator-color="primary"
        align="left"
        narrow-indicator
        no-caps
        mobile-arrows>
        <q-route-tab
          v-for="tab in navQTabsStore.tabs"
          :key="tab.name"
          :to="tab.to"
          :label="tab.name"
          :exact="tab.exact"
          :icon="tab.icon" />
      </q-tabs>
      <q-separator v-if="navQTabsStore.tabs.length > 0" />
      <div class="game-server-content">
        <router-view></router-view>
      </div>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { GetGameServerRequestSchema } from '@/proto/xylona_pb'
import { useToolbarNavQTabsStore, useUserAuthStore } from '@/stores/xylona'
import { GetXylonaClient, WindowWidth } from '@/utils/shared'
import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'
import { useServerSoftwareInstall } from '@/composables/useServerSoftwareInstall'
import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()

let currentPermissions: string[] = []
let currentIsOwnerOrSuper = false
let currentHasModSupport = false
let currentAllowStartArgEditing = true
let currentIsSuperUser = false

onMounted(async () => {
  await configureTabs()
  await enforceRouteAccess()
})

watch(
  () => route.path,
  () => {
    void enforceRouteAccess()
  },
)

watch(
  () => route.params.id,
  () => {
    void configureTabs().then(enforceRouteAccess)
  },
)

useServerSoftwareInstall((gameServerId, status) => {
  const currentId = route.params.id as string
  if (gameServerId !== currentId) return
  if (status === 'complete' || status === 'failed') {
    void configureTabs().then(enforceRouteAccess)
  }
})

function getServerID(): string {
  return route.params.id instanceof Array ? route.params.id[0] : route.params.id
}

async function configureTabs() {
  const serverID = getServerID()
  if (serverID === '') {
    navQTabsStore.changeTabs([])
    return
  }

  const authStore = useUserAuthStore()
  const authResponse = await authStore.checkUserAuthenticated()
  const currentUser = authResponse?.user ?? authStore.user

  let permissions: string[] = []
  let isOwnerOrSuper = false
  let hasModSupport = false
  let allowStartArgEditing = true
  let isSuperUser = false

  if (currentUser) {
    try {
      const gameServerResp = await GetXylonaClient().getGameServer(
        create(GetGameServerRequestSchema, {
          id: serverID,
        }),
      )
      permissions = gameServerResp.gameServer?.effectivePermissions ?? []
      // Merge global alert permissions into the server-scoped permissions
      // so the Alerts tab and redirect logic can see them.
      const globalPerms = authResponse?.permissionIds ?? []
      const alertPerms = globalPerms.filter(
        (p) => p === 'alerts.manage' || p === 'alerts.view_history',
      )
      for (const p of alertPerms) {
        if (!permissions.includes(p)) {
          permissions.push(p)
        }
      }
      const isOwner = gameServerResp.gameServer?.userId === currentUser.id
      isSuperUser = currentUser.superUser
      isOwnerOrSuper = currentUser.superUser || isOwner
      hasModSupport = Boolean(gameServerResp.gameServer?.resolvedHasModSupport)
      allowStartArgEditing = gameServerResp.gameServer?.game?.allowStartArgEditing ?? true
    } catch (unknownError: unknown) {
      const err = ConnectError.from(unknownError)
      console.error(err)
    }
  }

  currentPermissions = permissions
  currentIsOwnerOrSuper = isOwnerOrSuper
  currentHasModSupport = hasModSupport
  currentAllowStartArgEditing = allowStartArgEditing
  currentIsSuperUser = isSuperUser

  navQTabsStore.changeTabs(
    buildGameServerTabs(
      serverID,
      permissions,
      isOwnerOrSuper,
      hasModSupport,
      allowStartArgEditing,
      isSuperUser,
    ),
  )
}

async function enforceRouteAccess() {
  const serverID = getServerID()
  if (serverID === '') {
    return
  }

  const redirectPath = getUnauthorizedRedirect(
    route.path,
    serverID,
    currentPermissions,
    currentIsOwnerOrSuper,
    currentHasModSupport,
    currentAllowStartArgEditing,
    currentIsSuperUser,
  )
  if (redirectPath !== null && route.path !== redirectPath) {
    await router.replace(redirectPath)
  }
}
</script>

<style scoped>
.game-server-page {
  display: flex;
  flex-direction: column;
  /* Override Quasar's inline min-height (full viewport) so max-height
     can constrain the page to the available space below the header. */
  min-height: 0 !important;
  max-height: calc(100dvh - 50px);
  overflow: hidden;
}

.game-server-card {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.game-server-content {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}
.game-server-tabs {
  background-color: var(--xy-surface-2);
}
</style>
