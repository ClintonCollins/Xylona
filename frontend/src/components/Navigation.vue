<template>
  <q-header>
    <q-toolbar class="bg-toolbar">
      <q-btn aria-label="Menu" dense flat icon="menu" round @click="toggleLeftDrawer" />

      <q-toolbar-title> Xylona </q-toolbar-title>

      <div>{{ user?.userName }}</div>
      <q-btn
        aria-label="Logout"
        class="q-ml-sm"
        dense
        flat
        icon="logout"
        round
        @click="logoutUser" />
    </q-toolbar>
  </q-header>

  <q-drawer v-model="leftDrawerOpen" bordered class="bg-xy-surface-2" show-if-above>
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
import { computed, ref } from 'vue'
import {
  ionGameController,
  ionHome,
  ionNotifications,
  ionPeople,
  ionServer,
  ionSettings,
} from '@quasar/extras/ionicons-v7'
import { laServerSolid } from '@quasar/extras/line-awesome'
import { useRoute, useRouter } from 'vue-router'
import type { User } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { canViewAlerts } from '@/utils/alert-permissions'

const store = useUserAuthStore()
const user = computed(() => store.user as User | null)
const canViewNotifications = computed(() => canViewAlerts(store.user, store.initialResponse))
const route = useRoute()
const router = useRouter()

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
      title: 'Home',
      icon: ionHome,
      link: '/',
      expanded: true,
      exact: true,
      groupItems: [],
    },
  ]

  const manageLinks: NavItem[] = [
    {
      title: 'Game Servers',
      icon: laServerSolid,
      link: '/game-servers',
      expanded: true,
      exact: false,
      section: 'Manage',
      groupItems: [],
    },
  ]

  if (canViewNotifications.value) {
    manageLinks.push({
      title: 'Notifications',
      icon: ionNotifications,
      link: '/notifications',
      expanded: true,
      exact: false,
      groupItems: [],
    })
  }

  if (store.user?.superUser) {
    manageLinks.push(
      {
        title: 'Games',
        icon: ionGameController,
        link: '/games',
        expanded: true,
        exact: false,
        groupItems: [],
      },
      {
        title: 'Nodes',
        icon: ionServer,
        link: '/nodes',
        expanded: true,
        exact: false,
        groupItems: [],
      },
      {
        title: 'Users',
        icon: ionPeople,
        link: '/admin/users',
        expanded: true,
        exact: false,
        groupItems: [],
      },
      {
        title: 'Node Settings',
        icon: ionSettings,
        link: '/admin/settings',
        expanded: true,
        exact: false,
        groupItems: [],
      },
    )
  }

  links.push(...manageLinks)

  return links
})

const leftDrawerOpen = ref(false)

function toggleLeftDrawer() {
  leftDrawerOpen.value = !leftDrawerOpen.value
}
</script>

<style scoped>
.drawer-brand {
  padding: var(--xy-space-lg) var(--xy-space-lg) var(--xy-space-md);
  display: flex;
  align-items: baseline;
  gap: var(--xy-space-sm);
}

.drawer-brand-text {
  font-family: var(--xy-font-brand);
  font-size: 1.4rem;
  color: var(--xy-accent);
  letter-spacing: 0.06em;
}

.drawer-brand-version {
  font-family: var(--xy-font-mono);
  font-size: 0.6rem;
  color: var(--xy-text-muted);
  letter-spacing: 0.04em;
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
</style>
