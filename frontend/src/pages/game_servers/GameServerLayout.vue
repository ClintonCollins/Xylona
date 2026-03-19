<template>
  <q-page :padding="windowWidth > 1024">
    <q-card class="full-width game-server-card">
      <q-tabs
        v-if="navQTabsStore.tabs.length > 0"
        class="game-server-tabs"
        active-color="primary"
        indicator-color="primary"
        align="left"
        narrow-indicator>
        <q-route-tab
          v-for="tab in navQTabsStore.tabs"
          :key="tab.name"
          :to="tab.to"
          :label="tab.name"
          :exact="tab.exact"
          :icon="tab.icon" />
      </q-tabs>
      <q-separator v-if="navQTabsStore.tabs.length > 0" />
      <router-view></router-view>
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
import { onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()

let currentPermissions: string[] = []
let currentIsOwnerOrSuper = false

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

  if (currentUser) {
    try {
      const gameServerResp = await GetXylonaClient().getGameServer(
        create(GetGameServerRequestSchema, {
          id: serverID,
        }),
      )
      permissions = gameServerResp.gameServer?.effectivePermissions ?? []
      const isOwner = gameServerResp.gameServer?.userId === currentUser.id
      isOwnerOrSuper = currentUser.superUser || isOwner
    } catch (unknownError: unknown) {
      const err = ConnectError.from(unknownError)
      console.error(err)
    }
  }

  currentPermissions = permissions
  currentIsOwnerOrSuper = isOwnerOrSuper

  navQTabsStore.changeTabs(buildGameServerTabs(serverID, permissions, isOwnerOrSuper))
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
  )
  if (redirectPath !== null && route.path !== redirectPath) {
    await router.replace(redirectPath)
  }
}
</script>

<style scoped>
.game-server-card {
  overflow: hidden;
}
.game-server-tabs {
  background-color: var(--xy-surface-2);
}
</style>
