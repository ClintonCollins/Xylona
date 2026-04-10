<template>
  <router-view />
</template>

<script lang="ts" setup>
import { onMounted } from 'vue'
import { GetOrCreateXylonaWebsocketClient } from './utils/shared'

onMounted(async () => {
  GetOrCreateXylonaWebsocketClient()
})
</script>
