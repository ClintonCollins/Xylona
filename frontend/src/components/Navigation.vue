<template>
  <q-header ref="headerRef">
    <q-toolbar class="bg-toolbar">
      <q-btn aria-label="Menu" dense flat icon="menu" round @click="toggleLeftDrawer" />

      <q-toolbar-title>
        <router-link aria-label="Xylona game servers" class="toolbar-brand" to="/game-servers">
          Xylona
        </router-link>
      </q-toolbar-title>

      <div class="toolbar-user">
        <q-icon aria-hidden="true" name="account_circle" size="sm" />
        <span>{{ user?.userName }}</span>
      </div>
      <q-btn
        aria-label="Logout"
        class="q-ml-sm"
        dense
        flat
        icon="logout"
        round
        @click="logoutUser" />
    </q-toolbar>
    <div
      v-if="connectionNotice"
      :class="`live-connection-banner--${websocketConnectionStatus}`"
      class="live-connection-banner"
      role="status"
      aria-live="assertive">
      <q-icon :name="connectionNotice.icon" size="sm" />
      <div class="live-connection-banner__copy">
        <strong>{{ connectionNotice.title }}</strong>
        <span>{{ connectionNotice.detail }}</span>
      </div>
      <q-spinner
        v-if="websocketConnectionStatus !== 'disconnected'"
        aria-label="Reconnecting"
        size="1.1rem" />
    </div>
  </q-header>

  <q-drawer v-model="leftDrawerOpen" :width="248" bordered class="bg-xy-surface-2" show-if-above>
    <nav aria-label="Main navigation">
      <q-list class="nav-list q-mt-md">
        <template v-for="(link, index) in navLinks" :key="link.title">
          <div v-if="link.section && index > 0" class="xy-nav-divider"></div>
          <q-item-label v-if="link.section" class="nav-section-label" header>{{
            link.section
          }}</q-item-label>
          <q-item
            v-if="link.groupItems.length === 0"
            :class="
              overrideActiveLink(link.link)
                ? 'q-router-link--exact-active q-router-link--active'
                : null
            "
            :exact="link.exact"
            :to="link.link"
            clickable>
            <q-item-section v-if="link.icon" avatar>
              <q-icon :name="link.icon" />
            </q-item-section>

            <q-item-section>
              <q-item-label>{{ link.title }}</q-item-label>
            </q-item-section>
          </q-item>
          <q-expansion-item v-else v-model="link.expanded" :icon="link.icon" :label="link.title">
            <q-item
              v-for="l in link.groupItems"
              :key="l.title"
              :exact="l.exact"
              :inset-level="0.3"
              :to="l.link"
              clickable>
              <q-item-section v-if="l.icon" avatar>
                <q-icon :name="l.icon" />
              </q-item-section>

              <q-item-section>
                <q-item-label>{{ l.title }}</q-item-label>
              </q-item-section>
            </q-item>
          </q-expansion-item>
        </template>
      </q-list>
    </nav>
  </q-drawer>
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import type { User } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { canViewAlerts } from '@/utils/alert-permissions'
import { websocketConnectionStatus } from '@/utils/websocket-connection'

const store = useUserAuthStore()
const user = computed(() => store.user as User | null)
const canViewNotifications = computed(() => canViewAlerts(store.user, store.initialResponse))
const route = useRoute()
const router = useRouter()
const headerRef = ref<{ $el: HTMLElement } | null>(null)

let headerResizeObserver: ResizeObserver | null = null

async function logoutUser() {
  await store.logout()
  await router.push('/login')
}

interface NavItem {
  title: string
  link: string
  icon: string
  expanded: boolean
  exact: boolean
  section?: string
  groupItems: NavItem[]
}

function overrideActiveLink(link: string) {
  const pathSplit = route.path.split('/')
  if (pathSplit.length === 0) {
    return false
  }
  return pathSplit[1] === 'game-servers' && link === '/game-servers'
}

const navLinks = computed((): NavItem[] => {
  const links: NavItem[] = [
    {
      title: 'Game Servers',
      icon: 'dns',
      link: '/game-servers',
      expanded: true,
      exact: false,
      section: 'Operations',
      groupItems: [],
    },
  ]

  if (canViewNotifications.value) {
    links.push({
      title: 'Notifications',
      icon: 'notifications',
      link: '/notifications',
      expanded: true,
      exact: false,
      groupItems: [],
    })
  }

  if (store.user?.superUser) {
    links.push(
      {
        title: 'Games',
        icon: 'sports_esports',
        link: '/games',
        expanded: true,
        exact: false,
        section: 'Administration',
        groupItems: [],
      },
      {
        title: 'Nodes',
        icon: 'device_hub',
        link: '/nodes',
        expanded: true,
        exact: false,
        groupItems: [],
      },
      {
        title: 'Users',
        icon: 'group',
        link: '/admin/users',
        expanded: true,
        exact: false,
        groupItems: [],
      },
      {
        title: 'System Updates',
        icon: 'system_update_alt',
        link: '/admin/updates',
        expanded: true,
        exact: false,
        section: 'System',
        groupItems: [],
      },
      {
        title: 'Node Settings',
        icon: 'settings',
        link: '/admin/settings',
        expanded: true,
        exact: false,
        groupItems: [],
      },
    )
  }

  return links
})

const leftDrawerOpen = ref(false)

const connectionNotice = computed(() => {
  switch (websocketConnectionStatus.value) {
    case 'connecting':
      return {
        icon: 'sync',
        title: 'Connecting to live updates',
        detail: 'Server status and lifecycle controls are temporarily unavailable.',
      }
    case 'reconnecting':
      return {
        icon: 'sync_problem',
        title: 'Live updates interrupted',
        detail: 'Displayed server status may be stale. Reconnecting…',
      }
    case 'disconnected':
      return {
        icon: 'cloud_off',
        title: 'Live updates unavailable',
        detail: 'Check your connection. Displayed server status may be stale.',
      }
    default:
      return null
  }
})

function toggleLeftDrawer() {
  leftDrawerOpen.value = !leftDrawerOpen.value
}

function getHeaderElement(): HTMLElement | null {
  const element = headerRef.value?.$el
  return element instanceof HTMLElement ? element : null
}

function updateHeaderStackHeight() {
  const headerElement = getHeaderElement()
  if (headerElement === null) return

  const height = Math.ceil(headerElement.getBoundingClientRect().height)
  if (height <= 0) return

  document.documentElement.style.setProperty('--xy-header-stack-height', `${height}px`)
}

onMounted(() => {
  const headerElement = getHeaderElement()
  if (headerElement === null) return

  updateHeaderStackHeight()
  if (typeof ResizeObserver === 'undefined') return

  headerResizeObserver = new ResizeObserver(updateHeaderStackHeight)
  headerResizeObserver.observe(headerElement)
})

onBeforeUnmount(() => {
  headerResizeObserver?.disconnect()
  document.documentElement.style.removeProperty('--xy-header-stack-height')
})
</script>

<style scoped>
.toolbar-brand {
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: 1.25rem;
  letter-spacing: 0.05em;
  text-decoration: none;
}

.toolbar-user {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-width: 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.toolbar-user span {
  overflow: hidden;
  max-width: 12rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-section-label {
  font-size: 0.65rem;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  padding-top: var(--xy-space-sm);
  padding-bottom: var(--xy-space-xs);
  min-height: auto;
}

.nav-list {
  padding-top: 0;
}

.live-connection-banner {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-warning) 14%, var(--xy-surface-1));
  border-top: 1px solid var(--xy-warning-border);
  border-bottom: 1px solid var(--xy-warning-border);
  font-family: var(--xy-font-body);
}

.live-connection-banner--disconnected {
  background: color-mix(in srgb, var(--xy-danger) 14%, var(--xy-surface-1));
  border-color: var(--xy-danger-border);
}

.live-connection-banner__copy {
  display: flex;
  flex: 1;
  flex-wrap: wrap;
  gap: var(--xy-space-xs) var(--xy-space-sm);
  min-width: 0;
}

.live-connection-banner__copy span {
  color: var(--xy-text-secondary);
}

@media (max-width: 599px) {
  .toolbar-user span {
    display: none;
  }

  .live-connection-banner__copy {
    flex-direction: column;
    gap: 0;
  }
}
</style>
