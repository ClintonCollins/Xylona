<template>
  <router-view />
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { GetOrCreateXylonaWebsocketClient } from './utils/shared'

onMounted(async () => {
  GetOrCreateXylonaWebsocketClient()
})
</script>
