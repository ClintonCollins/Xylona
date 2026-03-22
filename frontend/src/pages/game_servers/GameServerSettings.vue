<template>
  <q-card-section>
    <game-server-form :existing-game-server-id="gameServerID"></game-server-form>
  </q-card-section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'

import GameServerForm from '@/components/game_servers/GameServerForm.vue'

const route = useRoute()
const gameServerID = ref(route.params.id as string)
</script>

<style scoped></style>
