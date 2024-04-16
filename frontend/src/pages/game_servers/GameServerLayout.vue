<template>
  <q-page padding>
    <q-tabs v-if="navQTabsStore.tabs.length > 0" class="bg-blue-grey-10 glossy">
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
import {useToolbarNavQTabsStore} from "stores/xylona";

const navQTabsStore = useToolbarNavQTabsStore()
</script>

<style scoped>

</style>
