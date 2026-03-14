<template>
  <q-page :padding="windowWidth > 1024">
    <q-tabs v-if="navQTabsStore.tabs.length > 0" class="bg-sub-toolbar">
      <q-route-tab
        v-for="tab in navQTabsStore.tabs"
        :key="tab.name"
        :to="tab.to"
        :label="tab.name"
        :exact="tab.exact"
        :icon="tab.icon"
      />
    </q-tabs>
    <q-card class="full-width">
      <router-view></router-view>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import {useToolbarNavQTabsStore} from "src/stores/xylona"
import {WindowWidth} from "src/utils/shared"
import {useRoute} from "vue-router";


const route = useRoute()
const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()

useToolbarNavQTabsStore().changeTabs([
  {name: "Console", to: "/game-servers/" + route.params.id + "/console", icon: "terminal", exact: true},
  {name: "Files", to: "/game-servers/" + route.params.id + "/files", icon: "folder", exact: true},
  {name: "Configuration", to: "/game-servers/" + route.params.id + "/configuration", icon: "settings", exact: true},
])

</script>

<style scoped>

</style>
