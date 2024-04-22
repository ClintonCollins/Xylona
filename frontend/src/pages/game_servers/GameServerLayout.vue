<template>
  <q-page :padding="windowWidth > 1024">
    <q-tabs v-if="navQTabsStore.tabs.length > 0" class="bg-success-darker">
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
import {useToolbarNavQTabsStore} from "stores/xylona"
import {WindowWidth} from "src/utils/shared"

const navQTabsStore = useToolbarNavQTabsStore()
const windowWidth = WindowWidth()

</script>

<style scoped>

</style>
