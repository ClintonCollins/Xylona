<template>
  <q-header>
    <q-toolbar class="bg-toolbar">
      <q-btn flat dense round icon="menu" aria-label="Menu" @click="toggleLeftDrawer" />

      <q-toolbar-title> Xylona </q-toolbar-title>

      <div>{{ user?.userName }}</div>
      <q-btn
        flat
        round
        dense
        icon="logout"
        class="q-ml-sm"
        aria-label="Logout"
        @click="logoutUser" />
    </q-toolbar>
  </q-header>

  <q-drawer v-model="leftDrawerOpen" show-if-above bordered>
    <q-list>
      <q-item-label header>Navigation</q-item-label>
      <div v-for="link in navLinks" :key="link.title">
        <q-item
          :class="
            overrideActiveLink(link.link)
              ? 'q-router-link--exact-active q-router-link--active'
              : null
          "
          v-if="link.groupItems.length === 0"
          clickable
          :to="link.link"
          :exact="link.exact">
          <q-item-section v-if="link.icon" avatar>
            <q-icon :name="link.icon" />
          </q-item-section>

          <q-item-section>
            <q-item-label>{{ link.title }}</q-item-label>
          </q-item-section>
        </q-item>
        <q-expansion-item v-else v-model="link.expanded" :icon="link.icon" :label="link.title">
          <q-item
            :inset-level="0.3"
            v-for="l in link.groupItems"
            :key="l.title"
            clickable
            :to="link.link"
            :exact="link.exact">
            <q-item-section v-if="l.icon" avatar>
              <q-icon :name="l.icon" />
            </q-item-section>

            <q-item-section>
              <q-item-label>{{ l.title }}</q-item-label>
            </q-item-section>
          </q-item>
        </q-expansion-item>
      </div>
    </q-list>
  </q-drawer>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import {
  ionGameController,
  ionHome,
  ionKey,
  ionPeople,
  ionPulse,
  ionServer,
} from '@quasar/extras/ionicons-v7'
import { laServerSolid } from '@quasar/extras/line-awesome'
import { useRoute, useRouter } from 'vue-router'
import { User } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'

const store = useUserAuthStore()
const user = computed(() => store.user as User | null)
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

  if (store.user?.superUser) {
    links.push({
      title: 'Dashboard',
      icon: ionPulse,
      link: '/dashboard',
      expanded: true,
      exact: false,
      groupItems: [],
    })
  }

  links.push(
    {
      title: 'Game Servers',
      icon: laServerSolid,
      link: '/game-servers',
      expanded: true,
      exact: false,
      groupItems: [],
    },
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
      title: 'Secret Keys',
      icon: ionKey,
      link: '/secret-keys',
      expanded: true,
      exact: false,
      groupItems: [],
    },
  )

  if (store.user?.superUser) {
    links.push({
      title: 'Users',
      icon: ionPeople,
      link: '/admin/users',
      expanded: true,
      exact: false,
      groupItems: [],
    })
  }

  return links
})

const leftDrawerOpen = ref(false)

function toggleLeftDrawer() {
  leftDrawerOpen.value = !leftDrawerOpen.value
}
</script>

<style scoped></style>
