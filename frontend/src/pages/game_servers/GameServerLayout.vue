<template>
  <div class="q-page game-server-page" :class="{ 'q-layout-padding': windowWidth > 1024 }">
    <q-card class="full-width game-server-card">
      <q-tabs
        v-if="layoutTabs.length > 0"
        :dense="windowWidth <= 767"
        :inline-label="windowWidth <= 767"
        :mobile-arrows="windowWidth <= 767"
        :outside-arrows="windowWidth <= 767"
        active-color="primary"
        align="left"
        aria-label="Game server sections"
        class="game-server-tabs"
        indicator-color="primary"
        narrow-indicator
        no-caps>
        <q-route-tab
          v-for="(tab, index) in layoutTabs"
          :key="tab.name"
          :class="{ 'game-server-tab--group-start': isGroupStart(index) }"
          :exact="tab.exact"
          :icon="tab.icon"
          :label="tab.name"
          :to="tab.to" />
      </q-tabs>
      <q-separator v-if="layoutTabs.length > 0" />
      <div class="game-server-content">
        <router-view :key="gameServerRouteKey"></router-view>
      </div>
    </q-card>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { GetGameServerRequestSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { useQuasar } from 'quasar'
import { GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import { buildGameServerTabs, getUnauthorizedRedirect } from './game-server-layout-tabs'
import type { GameServerLayoutTab } from './game-server-layout-tabs'
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const $q = useQuasar()
const windowWidth = computed(() => $q.screen.width)
const gameServerRouteKey = computed(() => getServerID())
const layoutTabs = ref<GameServerLayoutTab[]>([])

function isGroupStart(index: number): boolean {
  if (index === 0) {
    return false
  }
  return layoutTabs.value[index]?.group !== layoutTabs.value[index - 1]?.group
}

let currentPermissions: string[] = []
let currentIsOwnerOrSuper = false
let currentHasModSupport = false
let currentAllowStartArgEditing = true
let currentIsSuperUser = false
let currentHasLiveMap = false
let currentHasOperations = false
let tabConfigurationSequence = 0

onMounted(async () => {
  XylonaEventBus.on('serverSoftwareInstall', handleServerSoftwareInstall)
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

function handleServerSoftwareInstall(
  gameServerId: string,
  _gameServerName: string,
  status: string,
) {
  if (gameServerId !== getServerID()) {
    return
  }
  if (status === 'complete' || status === 'failed') {
    void configureTabs().then((configured) => {
      if (configured) {
        return enforceRouteAccess()
      }
    })
  }
}

onUnmounted(() => {
  XylonaEventBus.off('serverSoftwareInstall', handleServerSoftwareInstall)
})

function getServerID(): string {
  return route.params.id instanceof Array ? route.params.id[0] : route.params.id
}

async function configureTabs() {
  const configurationSequence = ++tabConfigurationSequence
  const serverID = getServerID()
  if (serverID === '') {
    if (configurationSequence === tabConfigurationSequence) {
      layoutTabs.value = []
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
  let hasOperations = false

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
      hasModSupport =
        Boolean(gameServerResp.gameServer?.resolvedHasModSupport) ||
        gameServerResp.gameServer?.gameId === '7_days_to_die'
      allowStartArgEditing = gameServerResp.gameServer?.game?.allowStartArgEditing ?? true
      hasLiveMap = ['minecraft', 'palworld', '7_days_to_die'].includes(
        gameServerResp.gameServer?.gameId ?? '',
      )
      hasOperations = gameServerResp.gameServer?.gameId === '7_days_to_die'
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
  currentHasOperations = hasOperations

  layoutTabs.value = buildGameServerTabs(
    serverID,
    permissions,
    isOwnerOrSuper,
    hasModSupport,
    allowStartArgEditing,
    isSuperUser,
    hasLiveMap,
    hasOperations,
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
    currentHasOperations,
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
  /* Override Quasar's inline min-height and explicitly occupy the space
     below the header so flex children can size the console correctly. */
  min-height: 0 !important;
  height: calc(100dvh - var(--xy-header-stack-height, 50px));
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

/* Desktop-only visual clustering: a subtle vertical rule + breathing room
   before the first tab of each group (Operate | Configure | Automate | Access).
   Presentation only — routing and tab behavior are untouched. */
@media (min-width: 768px) {
  .game-server-tabs :deep(.q-tab.game-server-tab--group-start) {
    margin-left: var(--xy-space-md);
  }

  .game-server-tabs :deep(.q-tab.game-server-tab--group-start)::before {
    content: '';
    position: absolute;
    left: calc(-1 * var(--xy-space-sm));
    top: 50%;
    transform: translateY(-50%);
    width: 1px;
    height: 1.25rem;
    background-color: var(--xy-border);
  }
}

@media (max-width: 767px) {
  .game-server-card {
    border-radius: 0;
  }

  .game-server-tabs {
    min-height: 3rem;
    border-bottom: 1px solid var(--xy-border);
  }

  .game-server-tabs :deep(.q-tabs__content) {
    justify-content: flex-start;
    scroll-padding-inline: var(--xy-space-sm);
  }

  .game-server-tabs :deep(.q-tab) {
    flex: 0 0 auto;
    min-width: 5.75rem;
    min-height: 3rem;
    padding-inline: var(--xy-space-base);
  }

  .game-server-tabs :deep(.q-tab.q-tab--active) {
    background: var(--xy-primary-muted);
  }

  .game-server-tabs :deep(.q-tab__content) {
    flex-direction: row;
    flex-wrap: nowrap;
    gap: var(--xy-space-xs);
    min-width: max-content;
  }

  .game-server-tabs :deep(.q-tab__icon) {
    font-size: var(--xy-font-size-lg);
  }

  .game-server-tabs :deep(.q-tab__label) {
    font-size: var(--xy-font-size-xs);
    white-space: nowrap;
  }
}
</style>
