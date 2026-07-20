<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import { GetGameServerRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import GameServerPalworldMap from './GameServerPalworldMap.vue'
import GameServerMinecraftMap from './GameServerMinecraftMap.vue'
import GameServerSevenDaysToDieMap from './GameServerSevenDaysToDieMap.vue'

const route = useRoute()
const gameID = ref('')
const loadError = ref('')

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})

onMounted(async () => {
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerID.value }),
    )
    gameID.value = response.gameServer?.gameId ?? ''
  } catch (unknownError: unknown) {
    loadError.value = ConnectErrorToString(ConnectError.from(unknownError))
  }
})
</script>

<template>
  <game-server-palworld-map v-if="gameID === 'palworld'" />
  <game-server-minecraft-map v-else-if="gameID === 'minecraft'" />
  <game-server-seven-days-to-die-map v-else-if="gameID === '7_days_to_die'" />
  <div v-else-if="loadError" class="map-route-state">
    <q-icon name="error_outline" size="40px" />
    <strong>Could not open the live map</strong>
    <span>{{ loadError }}</span>
  </div>
  <div v-else-if="gameID" class="map-route-state">
    <q-icon name="map" size="40px" />
    <strong>This game does not provide a live map.</strong>
  </div>
  <div v-else class="map-route-state">
    <q-spinner color="primary" size="36px" />
    <span>Loading map...</span>
  </div>
</template>

<style scoped>
.map-route-state {
  display: grid;
  flex: 1;
  place-items: center;
  align-content: center;
  gap: var(--xy-space-sm);
  min-height: 420px;
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
}

.map-route-state strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-weight: 500;
}
</style>
