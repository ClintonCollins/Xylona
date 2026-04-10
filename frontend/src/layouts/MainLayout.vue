<template>
  <q-layout view="hHh Lpr lFf">
    <navigation></navigation>
    <q-page-container>
      <router-view />
    </q-page-container>
  </q-layout>
</template>

<script lang="ts" setup>
import Navigation from '@/components/Navigation.vue'
import { onMounted } from 'vue'

import { initGameServerNotificationService } from '@/utils/game-server-notifications'

onMounted(() => {
  initGameServerNotificationService()
})
</script>
