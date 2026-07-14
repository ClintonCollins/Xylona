<template>
  <q-layout view="hHh Lpr lFf">
    <a class="xy-skip-link" href="#main-content">Skip to main content</a>
    <navigation></navigation>
    <q-page-container id="main-content" role="main" tabindex="-1">
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
