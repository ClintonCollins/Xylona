<template>
  <q-page :padding="windowWidth > 1024" class="game-server-page">
    <q-card class="full-width game-server-card">
      <q-tabs
        v-if="navQTabsStore.tabs.length > 0"
        active-color="primary"
        align="left"
        class="game-server-tabs"
        indicator-color="primary"
        mobile-arrows
        narrow-indicator
        no-caps>
        <q-route-tab
          v-for="tab in navQTabsStore.tabs"
          :key="tab.name"
          :exact="tab.exact"
          :icon="tab.icon"
          :label="tab.name"
          :to="tab.to" />
      </q-tabs>
      <q-separator v-if="navQTabsStore.tabs.length > 0" />
      <div class="game-server-content">
        <router-view :key="gameServerRouteKey"></router-view>
      </div>
    </q-card>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { GetGameServerRequestSchema } from '@/proto/xylona_pb'
import { useToolbarNavQTabsStore, useUserAuthStore } from '@/stores/xylona'
import { GetXylonaClient, WindowWidth } from '@/utils/shared'
import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'
import { useServerSoftwareInstall } from '@/composables/useServerSoftwareInstall'
import { computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()
const gameServerRouteKey = computed(() => getServerID())

let currentPermissions: string[] = []
let currentIsOwnerOrSuper = false
let currentHasModSupport = false
let currentAllowStartArgEditing = true
let currentIsSuperUser = false
let currentHasLiveMap = false
let tabConfigurationSequence = 0

onMounted(async () => {
  const configured = await configureTabs()
  if (configured) {
    await enforceRouteAccess()
  }
})

watch(
  () => ({ path: route.path, serverID: getServerID() }),
  (nextRoute, previousRoute) => {
    if (nextRoute.serverID === previousRoute.serverID) {
      void enforceRouteAccess()
      return
    }
    void configureTabs().then((configured) => {
      if (configured) {
        return enforceRouteAccess()
      }
    })
  },
)

useServerSoftwareInstall((gameServerId, status) => {
  const currentId = getServerID()
  if (gameServerId !== currentId) return
  if (status === 'complete' || status === 'failed') {
    void configureTabs().then((configured) => {
      if (configured) {
        return enforceRouteAccess()
      }
    })
  }
})

function getServerID(): string {
  return route.params.id instanceof Array ? route.params.id[0] : route.params.id
}

async function configureTabs() {
  const configurationSequence = ++tabConfigurationSequence
  const serverID = getServerID()
  if (serverID === '') {
    if (configurationSequence === tabConfigurationSequence) {
      navQTabsStore.changeTabs([])
    }
    return configurationSequence === tabConfigurationSequence
  }

  const authStore = useUserAuthStore()
  const authResponse = await authStore.checkUserAuthenticated()
  const currentUser = authResponse?.user ?? authStore.user

  let permissions: string[] = []
  let isOwnerOrSuper = false
  let hasModSupport = false
  let allowStartArgEditing = true
  let isSuperUser = false
  let hasLiveMap = false

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
      hasLiveMap = ['palworld', '7_days_to_die'].includes(gameServerResp.gameServer?.gameId ?? '')
    } catch (unknownError: unknown) {
      const err = ConnectError.from(unknownError)
      console.error(err)
    }
  }

  if (configurationSequence !== tabConfigurationSequence || serverID !== getServerID()) {
    return false
  }

  currentPermissions = permissions
  currentIsOwnerOrSuper = isOwnerOrSuper
  currentHasModSupport = hasModSupport
  currentAllowStartArgEditing = allowStartArgEditing
  currentIsSuperUser = isSuperUser
  currentHasLiveMap = hasLiveMap

  navQTabsStore.changeTabs(
    buildGameServerTabs(
      serverID,
      permissions,
      isOwnerOrSuper,
      hasModSupport,
      allowStartArgEditing,
      isSuperUser,
      hasLiveMap,
    ),
  )
  return true
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
    currentHasLiveMap,
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
  max-height: calc(100dvh - var(--xy-header-stack-height, 50px));
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
