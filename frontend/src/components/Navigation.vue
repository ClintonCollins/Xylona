<template>
  <q-header>
    <q-toolbar class="bg-green-10 glossy">
      <q-btn
        flat
        dense
        round
        icon="menu"
        aria-label="Menu"
        @click="toggleLeftDrawer"
      />

      <q-toolbar-title>
        Xylona Control Panel
      </q-toolbar-title>

      <div>{{ user?.userName }}</div>
    </q-toolbar>
  </q-header>

  <q-drawer v-model="leftDrawerOpen" show-if-above bordered>
    <q-list>
      <q-item-label header>Navigation</q-item-label>
      <div v-for="link in navLinks" :key="link.title">
        <q-item v-if="link.groupItems.length === 0" clickable :to="link.link" exact>
          <q-item-section v-if="link.icon" avatar>
            <q-icon :name="link.icon"/>
          </q-item-section>

          <q-item-section>
            <q-item-label>{{ link.title }}</q-item-label>
          </q-item-section>
        </q-item>
        <q-expansion-item v-else v-model="link.expanded" :icon="link.icon" :label="link.title">
          <q-item inset-level="0.3" v-for="l in link.groupItems" :key="l.title" clickable :to="link.link" exact>
            <q-item-section v-if="l.icon" avatar>
              <q-icon :name="l.icon"/>
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
import {useToolbarNavQTabsStore, useUserAuthStore} from "stores/xylona";
import {User} from "src/proto/xylona_pb";
import {ref, Ref} from "vue";
import {ionGameController, ionPersonAdd, ionLogIn, ionHome} from "@quasar/extras/ionicons-v7";
import {mdiDns} from "@quasar/extras/mdi-v7";
import { laServerSolid} from "@quasar/extras/line-awesome";

const store = useUserAuthStore()
const user = store.user as User | null

interface NavItem {
  title: string;
  link: string;
  icon: string;
  expanded: boolean;
  groupItems: NavItem[];
}

const navLinks: Ref<NavItem[]> = ref([
  {
    title: 'Home',
    icon: ionHome,
    link: '/',
    expanded: true,
    groupItems: []
  },
  {
    title: 'Login',
    icon: ionLogIn,
    link: '/login',
    expanded: true,
    groupItems: []
  },
  {
    title: 'Game Servers',
    icon: laServerSolid,
    link: '/game-servers',
    expanded: true,
    groupItems: []
  },
  {
    title: 'Games',
    icon: ionGameController,
    link: '/games',
    expanded: true,
    groupItems: []
  },
  {
    title: 'Create User',
    icon: ionPersonAdd,
    link: '/admin/create-user',
    groupItems: [],
    expanded: true,
  },
  // {
  //   title: 'List Users',
  //   icon: 'chat',
  //   link: '/users',
  //   groupItems: [],
  //   expanded: true,
  // },
  // {
  //   title: 'Create User',
  //   icon: 'chat',
  //   link: '/users/create',
  //   groupItems: [],
  //   expanded: true,
  // },
])

// const navLinks: Ref<NavItem[]> = ref([
//   {
//     title: 'Home',
//     icon: 'school',
//     link: '/',
//     expanded: true,
//     groupItems: []
//   },
//   {
//     title: 'Login',
//     icon: 'code',
//     link: '/login',
//     expanded: true,
//
//     groupItems: []
//   },
//   {
//     title: 'Users',
//     icon: 'chat',
//     link: '',
//     expanded: true,
//     groupItems: [
//       {
//         title: 'List Users',
//         icon: 'chat',
//         link: '/users',
//         groupItems: [],
//         expanded: true,
//       },
//       {
//         title: 'Create User',
//         icon: 'chat',
//         link: '/users/create',
//         groupItems: [],
//         expanded: true,
//       }
//     ]
//   },
//   {
//     title: 'Game Servers',
//     icon: mdiDns,
//     link: '',
//     expanded: true,
//     groupItems: [
//       {
//         title: 'List Game Servers',
//         icon: mdiDns,
//         link: '/game_servers',
//         groupItems: [],
//         expanded: true,
//       },
//       {
//         title: 'Create Game Server',
//         icon: mdiDns,
//         link: '/game_servers/create',
//         groupItems: [],
//         expanded: true,
//       }
//     ]
//   },
//   {
//     title: 'Games',
//     icon: ionGameController,
//     link: '/games',
//     expanded: true,
//     groupItems: [
//       {
//         title: 'List Games',
//         icon: ionGameController,
//         link: '/games',
//         groupItems: [],
//         expanded: true,
//       },
//       {
//         title: 'Create Game',
//         icon: ionGameController,
//         link: '/games/create',
//         groupItems: [],
//         expanded: true,
//       }
//     ]
//   }
// ])

const leftDrawerOpen = ref(false)

function toggleLeftDrawer() {
  leftDrawerOpen.value = !leftDrawerOpen.value
}
</script>

<style scoped>

</style>
