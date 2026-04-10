<template>
  <q-card-section>
    <div class="row items-center">
      <q-icon class="q-mr-sm" color="primary" name="insights" size="sm" />
      <div class="text-h6">Metrics History</div>
    </div>
  </q-card-section>
  <q-card-section>
    <game-server-metrics-history :game-server-id="gameServerId" />
  </q-card-section>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import GameServerMetricsHistory from '@/components/game_servers/GameServerMetricsHistory.vue'

const route = useRoute()
const gameServerId = ref(route.params.id instanceof Array ? route.params.id[0] : route.params.id)
</script>
