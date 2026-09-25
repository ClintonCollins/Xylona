<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
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
// Only NotFound means the link is gone; any other failure is worth a retry.
const loadError = ref(false)

const identifier = computed(() => String(route.params['identifier'] ?? ''))

async function resolveMap(): Promise<void> {
  loading.value = true
  loadError.value = false
  try {
    const response = await GetXylonaClient().resolvePublicGameServerMap(
      create(ResolvePublicGameServerMapRequestSchema, {
        publicIdentifier: identifier.value,
      }),
    )
    kind.value = response.kind
  } catch (unknownError: unknown) {
    kind.value = GameServerMapKind.UNSPECIFIED
    loadError.value = ConnectError.from(unknownError).code !== Code.NotFound
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
    <div v-if="loading" role="status">
      <q-spinner aria-hidden="true" color="primary" size="42px" />
      <span class="xy-visually-hidden">Loading map</span>
    </div>
    <template v-else-if="loadError">
      <div class="public-map-state__brand">Xylona</div>
      <q-icon name="cloud_off" size="48px" />
      <h1>Map temporarily unavailable</h1>
      <p>The map could not be reached. Try again in a moment.</p>
      <q-btn color="primary" label="Retry" no-caps @click="resolveMap" />
    </template>
    <template v-else>
      <div class="public-map-state__brand">Xylona</div>
      <q-icon name="link_off" size="48px" />
      <h1>This map link is not available</h1>
      <p>The link may be incomplete, disabled, or no longer current.</p>
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

.public-map-state__brand {
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
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
