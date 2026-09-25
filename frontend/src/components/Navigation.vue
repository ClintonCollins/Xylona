<template>
  <q-header ref="headerRef">
    <q-toolbar class="bg-toolbar">
      <q-btn
        :aria-expanded="leftDrawerOpen ? 'true' : 'false'"
        aria-label="Menu"
        dense
        flat
        icon="menu"
        round
        @click="toggleLeftDrawer" />

      <q-toolbar-title>
        <router-link aria-label="Xylona game servers" class="toolbar-brand" to="/game-servers">
          Xylona
        </router-link>
      </q-toolbar-title>

      <q-btn
        :aria-label="`Account menu for ${user?.userName ?? 'this user'}`"
        class="toolbar-account"
        flat
        no-caps>
        <q-icon aria-hidden="true" name="account_circle" size="sm" />
        <span class="toolbar-account__name">{{ user?.userName }}</span>
        <q-icon aria-hidden="true" name="arrow_drop_down" size="xs" />
        <q-menu anchor="bottom right" self="top right" @hide="openRequestedDialog">
          <q-list class="account-menu">
            <q-item class="account-menu__identity">
              <q-item-section>
                <q-item-label caption>Signed in as</q-item-label>
                <q-item-label class="account-menu__user">{{ user?.userName }}</q-item-label>
              </q-item-section>
            </q-item>
            <q-separator />
            <q-item v-close-popup clickable @click="changePasswordRequested = true">
              <q-item-section avatar>
                <q-icon name="lock_reset" />
              </q-item-section>
              <q-item-section>Change password</q-item-section>
            </q-item>
            <q-item v-close-popup clickable @click="logoutUser">
              <q-item-section avatar>
                <q-icon name="logout" />
              </q-item-section>
              <q-item-section>Sign out</q-item-section>
            </q-item>
          </q-list>
        </q-menu>
      </q-btn>
    </q-toolbar>
    <div
      v-if="connectionNotice"
      :class="`live-connection-banner--${websocketConnectionStatus}`"
      class="live-connection-banner"
      role="status"
      aria-atomic="true"
      aria-live="polite">
      <q-icon :name="connectionNotice.icon" aria-hidden="true" size="sm" />
      <div class="live-connection-banner__copy">
        <strong>{{ connectionNotice.title }}</strong>
        <span>{{ connectionNotice.detail }}</span>
      </div>
      <q-spinner
        v-if="websocketBrowserOnline && websocketConnectionStatus !== 'disconnected'"
        aria-label="Reconnecting"
        size="1.1rem" />
      <q-btn
        v-if="websocketBrowserOnline"
        class="live-connection-banner__retry"
        dense
        flat
        icon="refresh"
        label="Retry now"
        @click="reconnectControllerWebsocket" />
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
            :aria-current="overrideActiveLink(link.link) ? 'page' : undefined"
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
              :aria-current="overrideActiveLink(l.link) ? 'page' : undefined"
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

  <change-password-dialog v-model:show-dialog="showChangePassword" />
</template>

<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ChangePasswordDialog from '@/components/ChangePasswordDialog.vue'
import type { User } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { canViewAlerts } from '@/utils/alert-permissions'
import { reconnectControllerWebsocket } from '@/utils/shared'
import {
  websocketBrowserOnline,
  websocketConnectingQuietly,
  websocketConnectionStatus,
} from '@/utils/websocket-connection'

const store = useUserAuthStore()
const user = computed(() => store.user as User | null)
const canViewNotifications = computed(() => canViewAlerts(store.user, store.initialResponse))
const route = useRoute()
const router = useRouter()
const headerRef = ref<{ $el: HTMLElement } | null>(null)
const showChangePassword = ref(false)
const changePasswordRequested = ref(false)

// The menu hands focus back to its button as it closes, so the dialog opens
// only once the menu is gone; otherwise its first field would lose focus.
function openRequestedDialog() {
  if (!changePasswordRequested.value) return
  changePasswordRequested.value = false
  showChangePassword.value = true
}

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

// Sub-pages such as /games/new or /admin/users/create are sibling routes, so
// the router does not mark their section's link active on its own.
function overrideActiveLink(link: string) {
  return route.path === link || route.path.startsWith(`${link}/`)
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
        title: 'Controller Settings',
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
  if (!websocketBrowserOnline.value) {
    return {
      icon: 'cloud_off',
      title: "You're offline",
      detail: 'Live server state is paused and may be stale. Reconnection resumes when online.',
    }
  }

  switch (websocketConnectionStatus.value) {
    case 'connecting':
      if (websocketConnectingQuietly.value) {
        return null
      }
      return {
        icon: 'sync',
        title: 'Connecting to live updates',
        detail: 'Live server state remains unavailable until the controller responds.',
      }
    case 'reconnecting':
      return {
        icon: 'sync_problem',
        title: 'Live updates interrupted',
        detail:
          'Displayed server state may be stale. Saved data remains available while reconnecting.',
      }
    case 'disconnected':
      return {
        icon: 'cloud_off',
        title: 'Controller connection unavailable',
        detail: 'Displayed server state may be stale. Retry the live connection.',
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
  font-size: var(--xy-font-size-xl);
  letter-spacing: 0.05em;
  text-decoration: none;
}

.toolbar-account {
  min-width: 0;
  padding: 0 var(--xy-space-xs) 0 var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.toolbar-account :deep(.q-btn__content) {
  flex-wrap: nowrap;
  gap: var(--xy-space-xs);
  min-width: 0;
}

.toolbar-account__name {
  overflow: hidden;
  max-width: 12rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-menu {
  min-width: 13rem;
}

.account-menu__user {
  overflow: hidden;
  max-width: 16rem;
  color: var(--xy-text-primary);
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav-section-label {
  font-size: var(--xy-font-size-2xs);
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

.live-connection-banner__retry {
  flex: 0 0 auto;
  color: var(--xy-text-primary);
  background: var(--xy-surface-3);
}

@media (max-width: 599px) {
  .toolbar-account__name {
    max-width: 6rem;
  }

  .live-connection-banner__copy {
    flex-direction: column;
    gap: 0;
  }

  .live-connection-banner {
    align-items: flex-start;
  }

  .live-connection-banner__retry {
    align-self: center;
  }
}
</style>
