<template>
  <q-page ref="pageRef" class="game-server-page">
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
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()
const pageRef = ref<{ $el: HTMLElement } | null>(null)

let currentPermissions: string[] = []
let currentIsOwnerOrSuper = false

// Sync page height from Quasar's min-height for scroll containment.
// Quasar sets min-height inline on q-page; we copy it to height so
// flex children get a bounded container.
function syncPageHeight() {
  const el = pageRef.value?.$el
  if (!el) return
  const minH = el.style.minHeight
  if (minH) {
    el.style.height = minH
  }
}

let resizeObserverCleanup: (() => void) | null = null

onMounted(async () => {
  await configureTabs()
  await enforceRouteAccess()

  nextTick(() => {
    syncPageHeight()
    // Re-sync on window resize (Quasar updates min-height)
    const ro = new ResizeObserver(() => syncPageHeight())
    const el = pageRef.value?.$el
    if (el) {
      ro.observe(el)
      resizeObserverCleanup = () => ro.disconnect()
    }
  })
})

onBeforeUnmount(() => {
  if (resizeObserverCleanup) {
    resizeObserverCleanup()
  }
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
/* q-page height is set via JS (syncPageHeight) to match Quasar's min-height.
   This establishes a bounded container for flex scroll containment. */
.game-server-page {
  display: flex;
  flex-direction: column;
  padding: 0 !important;
  overflow: hidden;
}

.game-server-card {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
.game-server-tabs {
  background-color: var(--xy-surface-2);
}
</style>
