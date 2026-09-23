<template>
  <div class="q-page game-server-page" :class="{ 'q-layout-padding': windowWidth > 1024 }">
    <div class="full-width game-server-card">
      <empty-state
        v-if="serverMissing"
        class="game-server-missing"
        description="It may have been deleted, or you may not have access to it."
        icon="dns"
        title="Game server not found"
        title-tag="h1">
        <template #actions>
          <q-btn
            color="primary"
            label="Back to Game Servers"
            no-caps
            to="/game-servers"
            unelevated />
        </template>
      </empty-state>
      <template v-else>
        <game-server-identity-bar
          v-if="gameServer !== null"
          :key="gameServer.id"
          :game-server="gameServer" />
        <nav
          v-if="layoutTabs.length > 0"
          ref="tabNav"
          aria-label="Game server sections"
          class="game-server-nav">
          <q-select
            v-if="isPhone"
            :model-value="activeTab"
            :options="layoutTabs"
            class="game-server-section-select"
            dense
            label="Section"
            option-label="name"
            option-value="to"
            options-dense
            outlined
            @update:model-value="onSectionSelected">
            <template #prepend>
              <q-icon :name="activeTab?.icon ?? 'terminal'" />
            </template>
            <template #option="scope">
              <q-item v-bind="scope.itemProps">
                <q-item-section avatar>
                  <q-icon :name="scope.opt.icon" />
                </q-item-section>
                <q-item-section>{{ scope.opt.name }}</q-item-section>
              </q-item>
            </template>
          </q-select>
          <template v-else>
            <q-resize-observer @resize="fitTabs" />
            <q-tabs
              active-color="primary"
              align="left"
              class="game-server-tabs"
              dense
              indicator-color="primary"
              inline-label
              mobile-arrows
              narrow-indicator
              no-caps
              outside-arrows>
              <q-route-tab
                v-for="(tab, index) in layoutTabs"
                :key="tab.name"
                :class="{
                  'game-server-tab--group-start': isGroupStart(index),
                  'game-server-tab--overflow': index >= visibleTabCount,
                }"
                :icon="tab.icon"
                :label="tab.name"
                :to="tab.to" />
            </q-tabs>
            <q-btn-dropdown
              v-if="overflowTabs.length > 0"
              :aria-current="activeTabInOverflow ? 'page' : undefined"
              :class="{ 'game-server-more--active': activeTabInOverflow }"
              class="game-server-more"
              dense
              flat
              label="More"
              no-caps>
              <q-list dense>
                <q-item
                  v-for="tab in overflowTabs"
                  :key="tab.name"
                  v-close-popup
                  :active="tab === activeTab"
                  clickable
                  :to="tab.to">
                  <q-item-section avatar>
                    <q-icon :name="tab.icon" />
                  </q-item-section>
                  <q-item-section>{{ tab.name }}</q-item-section>
                </q-item>
              </q-list>
            </q-btn-dropdown>
          </template>
        </nav>
        <div class="game-server-content">
          <router-view v-if="serverReady" :key="gameServerRouteKey"></router-view>
        </div>
      </template>
    </div>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import GameServerIdentityBar from '@/components/game_servers/GameServerIdentityBar.vue'
import EmptyState from '@/components/shared/EmptyState.vue'
import type { GameServer } from '@/proto/shared_pb'
import { GetGameServerRequestSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { setPageTitle } from '@/utils/page-title'
import { useQuasar } from 'quasar'
import { GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import {
  buildGameServerTabs,
  countTabsThatFit,
  getUnauthorizedRedirect,
} from './game-server-layout-tabs'
import type { GameServerLayoutTab } from './game-server-layout-tabs'
import { gameServerNameKey } from './game-server-context'
import { createGameServerReadiness, gameServerReadinessKey } from './game-server-readiness'
import { computed, nextTick, onMounted, onUnmounted, provide, ref, watch, watchEffect } from 'vue'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const $q = useQuasar()
const windowWidth = computed(() => $q.screen.width)
const isPhone = computed(() => windowWidth.value < 600)
const gameServerRouteKey = computed(() => getServerID())
const layoutTabs = ref<GameServerLayoutTab[]>([])
const gameServer = ref<GameServer | null>(null)
// Unknown or forbidden ids are final: the section pages never mount for them.
const serverMissing = ref(false)
const loadedServerID = ref('')
provide(
  gameServerNameKey,
  computed(() => gameServer.value?.name ?? ''),
)
const serverReady = computed(
  () => !serverMissing.value && loadedServerID.value === gameServerRouteKey.value,
)
// Start in the identity bar and the console's setup list read the same checks.
const readiness = createGameServerReadiness(
  computed(() => (serverMissing.value ? '' : loadedServerID.value)),
)
provide(gameServerReadinessKey, readiness)

// Room for the trailing "More" menu when not every tab fits.
const moreMenuWidth = 96
const tabNav = ref<HTMLElement | null>(null)
const visibleTabCount = ref(Number.POSITIVE_INFINITY)
const overflowTabs = computed(() => layoutTabs.value.slice(visibleTabCount.value))
const activeTab = computed(() =>
  layoutTabs.value.find((tab) => route.path === tab.to || route.path.startsWith(`${tab.to}/`)),
)
const activeTabInOverflow = computed(
  () => activeTab.value !== undefined && overflowTabs.value.includes(activeTab.value),
)

function isGroupStart(index: number): boolean {
  if (index === 0) {
    return false
  }
  return layoutTabs.value[index]?.group !== layoutTabs.value[index - 1]?.group
}

function fitTabs(): void {
  const nav = tabNav.value
  if (nav === null) return
  // Measure every tab, including the ones currently folded into More.
  nav.classList.add('game-server-nav--measuring')
  const tabs = Array.from(nav.querySelectorAll<HTMLElement>('.game-server-tabs .q-tab'))
  const start = tabs[0]?.getBoundingClientRect().left ?? 0
  const rightEdges = tabs.map((tab) => tab.getBoundingClientRect().right - start)
  nav.classList.remove('game-server-nav--measuring')
  visibleTabCount.value = countTabsThatFit(rightEdges, nav.clientWidth, moreMenuWidth)
}

function onSectionSelected(tab: GameServerLayoutTab): void {
  void router.push(tab.to)
}

watch(layoutTabs, () => void nextTick(fitTabs), { flush: 'post' })

watchEffect(() => {
  if (serverMissing.value) {
    setPageTitle('Game server not found')
    return
  }
  const current = gameServer.value
  // The id check skips the run that happens while navigating out of the server.
  if (current !== null && current.name !== '' && current.id === gameServerRouteKey.value) {
    setPageTitle(current.name, route.meta.title)
  }
})

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
  XylonaEventBus.on('websocketConnected', refreshServer)
  XylonaEventBus.on('gameServerEdited', handleGameServerEdited)
  void document.fonts?.ready.then(fitTabs)
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
      // Setup is often finished on another section, such as Configuration.
      void readiness.reload()
      return
    }
    refreshServer()
  },
)

// Status after a reconnect, and permissions after an install, can only be
// trusted once the server is read again.
function refreshServer() {
  void configureTabs().then((configured) => {
    if (configured) {
      return enforceRouteAccess()
    }
  })
}

function handleServerSoftwareInstall(
  gameServerId: string,
  _gameServerName: string,
  status: string,
) {
  if (gameServerId !== getServerID()) {
    return
  }
  if (status === 'complete' || status === 'failed') {
    refreshServer()
  }
}

// A rename or software change on a section page must reach the bar and title.
function handleGameServerEdited(gameServerId: string) {
  if (gameServerId === getServerID()) {
    refreshServer()
  }
}

onUnmounted(() => {
  XylonaEventBus.off('serverSoftwareInstall', handleServerSoftwareInstall)
  XylonaEventBus.off('websocketConnected', refreshServer)
  XylonaEventBus.off('gameServerEdited', handleGameServerEdited)
})

function getServerID(): string {
  const id = route.params['id']
  // Empty while navigating away, so no request goes out for an undefined id.
  return (Array.isArray(id) ? id[0] : id) ?? ''
}

async function configureTabs() {
  const configurationSequence = ++tabConfigurationSequence
  const serverID = getServerID()
  if (gameServer.value !== null && gameServer.value.id !== serverID) {
    gameServer.value = null
  }
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
  let loadedServer: GameServer | null = gameServer.value
  let missing = false
  let transientError = false

  if (currentUser) {
    try {
      const gameServerResp = await GetXylonaClient().getGameServer(
        create(GetGameServerRequestSchema, {
          id: serverID,
        }),
      )
      loadedServer = gameServerResp.gameServer ?? null
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
        ['7_days_to_die', 'valheim'].includes(gameServerResp.gameServer?.gameId ?? '')
      allowStartArgEditing = gameServerResp.gameServer?.game?.allowStartArgEditing ?? true
      hasLiveMap = ['minecraft', 'palworld', '7_days_to_die'].includes(
        gameServerResp.gameServer?.gameId ?? '',
      )
      hasOperations = ['7_days_to_die', 'valheim'].includes(gameServerResp.gameServer?.gameId ?? '')
    } catch (unknownError: unknown) {
      const err = ConnectError.from(unknownError)
      missing = err.code === Code.NotFound || err.code === Code.PermissionDenied
      transientError = !missing
      console.error(err)
    }
  }

  if (configurationSequence !== tabConfigurationSequence || serverID !== getServerID()) {
    return false
  }
  // A failed re-read (a blip after a reconnect) keeps the last good tabs and
  // permissions instead of bouncing the user to Console.
  if (transientError && loadedServerID.value === serverID) {
    return false
  }

  serverMissing.value = missing
  gameServer.value = missing ? null : loadedServer
  loadedServerID.value = serverID
  if (missing) {
    layoutTabs.value = []
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
  if (serverID === '' || serverMissing.value) {
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
  background: var(--xy-surface-1);
}
.game-server-content {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
}

.game-server-missing {
  margin: auto;
}

.game-server-nav {
  position: relative;
  display: flex;
  align-items: stretch;
  flex-shrink: 0;
  background-color: var(--xy-surface-2);
  border-bottom: 1px solid var(--xy-border);
}

.game-server-tabs {
  flex: 0 1 auto;
  min-width: 0;
}

.game-server-tabs :deep(.q-tab) {
  padding-inline: var(--xy-space-base);
}

.game-server-tabs :deep(.q-tab__icon) {
  font-size: var(--xy-font-size-lg);
}

.game-server-tabs :deep(.q-tab--inline .q-tab__label) {
  padding-left: var(--xy-space-xs);
}

.game-server-tabs :deep(.q-tab.game-server-tab--overflow) {
  display: none;
}

.game-server-nav--measuring .game-server-tabs :deep(.q-tab.game-server-tab--overflow) {
  display: inline-flex;
}

.game-server-more {
  flex-shrink: 0;
  color: var(--xy-text-secondary);
}

.game-server-more--active {
  color: var(--xy-primary);
  box-shadow: inset 0 -2px 0 var(--xy-primary);
}

.game-server-section-select {
  flex: 1;
  margin: var(--xy-space-sm) var(--xy-space-base);
}

/* Desktop-only visual clustering: a subtle vertical rule + breathing room
   before the first tab of each group (Operate | Configure | Automate | Access).
   Presentation only — routing and tab behavior are untouched. */
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
</style>
