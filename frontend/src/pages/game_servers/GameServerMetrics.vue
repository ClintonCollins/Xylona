<template>
  <q-card-section>
    <div class="row items-center">
      <q-icon name="insights" size="sm" color="primary" class="q-mr-sm" />
      <div class="text-h6">Metrics History</div>
    </div>
  </q-card-section>
  <q-card-section>
    <GameServerMetricsHistory :game-server-id="gameServerId" />
  </q-card-section>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import GameServerMetricsHistory from '@/components/game_servers/GameServerMetricsHistory.vue'

const route = useRoute()
const gameServerId = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
</script>
