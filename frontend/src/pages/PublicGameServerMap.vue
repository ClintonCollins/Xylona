<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import { GameServerMapKind, ResolvePublicGameServerMapRequestSchema } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'
import PublicMinecraftMap from './PublicMinecraftMap.vue'
import PublicPalworldMap from './PublicPalworldMap.vue'
import PublicSevenDaysToDieMap from './PublicSevenDaysToDieMap.vue'

const route = useRoute()
const kind = ref(GameServerMapKind.UNSPECIFIED)
const loading = ref(true)

const identifier = computed(() => String(route.params['identifier'] ?? ''))

async function resolveMap(): Promise<void> {
  try {
    const response = await GetXylonaClient().resolvePublicGameServerMap(
      create(ResolvePublicGameServerMapRequestSchema, {
        publicIdentifier: identifier.value,
      }),
    )
    kind.value = response.kind
  } catch {
    kind.value = GameServerMapKind.UNSPECIFIED
  } finally {
    loading.value = false
  }
}

onMounted(resolveMap)
</script>

<template>
  <public-palworld-map
    v-if="!loading && kind === GameServerMapKind.PALWORLD"
    :identifier="identifier" />
  <public-seven-days-to-die-map
    v-else-if="!loading && kind === GameServerMapKind.SEVEN_DAYS_TO_DIE"
    :identifier="identifier" />
  <public-minecraft-map
    v-else-if="!loading && kind === GameServerMapKind.MINECRAFT"
    :identifier="identifier" />
  <main v-else class="public-map-state">
    <q-spinner v-if="loading" color="primary" size="42px" />
    <template v-else>
      <q-icon name="link_off" size="48px" />
      <h1>This map link is not available</h1>
      <p>It may be unknown, replaced, or disabled by the server administrator.</p>
    </template>
  </main>
</template>

<style scoped>
.public-map-state {
  display: grid;
  min-width: 320px;
  min-height: 100dvh;
  place-content: center;
  justify-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-lg);
  color: var(--xy-text-primary);
  background: var(--xy-base);
  text-align: center;
}

.public-map-state h1 {
  margin: var(--xy-space-sm) 0 0;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-xl);
  font-weight: 500;
}

.public-map-state p {
  max-width: 48ch;
  margin: 0;
  color: var(--xy-text-secondary);
}
</style>
