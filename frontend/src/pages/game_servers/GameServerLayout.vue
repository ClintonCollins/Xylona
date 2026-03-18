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
import { Code, ConnectError } from '@connectrpc/connect'
import { ListDirectoryFilesRequestSchema } from '@/proto/gameserver_files_operations_pb'
import {
  GetGameServerRequestSchema,
  ListGameServerAccessGrantsRequestSchema,
} from '@/proto/xylona_pb'
import { useToolbarNavQTabsStore, useUserAuthStore } from '@/stores/xylona'
import { GetXylonaClient, WindowWidth } from '@/utils/shared'
import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()
const canUseConfigurationTab = ref(false)
const canUseAccessTab = ref(false)

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

  const { isSuperUser, isOwner } = await resolveUserContext(serverID)
  const canManageAccessByRequest = await hasAccessGrantsPermission(serverID)
  canUseAccessTab.value = isSuperUser || isOwner || canManageAccessByRequest

  const canManageConfigurationByRequest = await hasFilesViewPermission(serverID)
  canUseConfigurationTab.value = canUseAccessTab.value || canManageConfigurationByRequest

  navQTabsStore.changeTabs(
    buildGameServerTabs(serverID, canUseConfigurationTab.value, canUseAccessTab.value),
  )
}

async function resolveUserContext(
  serverID: string,
): Promise<{ isSuperUser: boolean; isOwner: boolean }> {
  const authStore = useUserAuthStore()
  const authResponse = await authStore.checkUserAuthenticated()
  const currentUser = authResponse?.user ?? authStore.user
  if (!currentUser) {
    return { isSuperUser: false, isOwner: false }
  }

  let isOwner = false
  try {
    const gameServerResp = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, {
        id: serverID,
      }),
    )
    isOwner = gameServerResp.gameServer?.userId === currentUser.id
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    if (err.code !== Code.PermissionDenied) {
      console.error(err)
    }
  }

  return {
    isSuperUser: currentUser.superUser,
    isOwner,
  }
}

async function hasAccessGrantsPermission(serverID: string): Promise<boolean> {
  try {
    await GetXylonaClient().listGameServerAccessGrants(
      create(ListGameServerAccessGrantsRequestSchema, {
        gameServerId: serverID,
      }),
    )
    return true
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    if (err.code !== Code.PermissionDenied && err.code !== Code.NotFound) {
      console.error(err)
    }
    return false
  }
}

async function hasFilesViewPermission(serverID: string): Promise<boolean> {
  try {
    await GetXylonaClient().listDirectoryFiles(
      create(ListDirectoryFilesRequestSchema, {
        gameServerId: serverID,
        path: '',
      }),
    )
    return true
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    if (
      err.code !== Code.PermissionDenied &&
      err.code !== Code.NotFound &&
      err.code !== Code.InvalidArgument
    ) {
      console.error(err)
    }
    return false
  }
}

async function enforceRouteAccess() {
  const serverID = getServerID()
  if (serverID === '') {
    return
  }

  const redirectPath = getUnauthorizedRedirect(
    route.path,
    serverID,
    canUseConfigurationTab.value,
    canUseAccessTab.value,
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
