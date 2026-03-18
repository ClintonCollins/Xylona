<template>
  <q-card flat bordered class="cursor-pointer" @click="emit('select')">
    <q-card-section>
      <div class="row items-center q-mb-sm">
        <div class="text-h6 q-mr-sm">{{ node.name || 'Unnamed Node' }}</div>
        <q-badge :color="healthColor" :label="healthLabel" />
        <q-space />
        <q-badge v-if="node.local" color="blue-grey" label="Local" />
      </div>
      <div class="text-caption text-grey q-mb-sm">
        <span v-if="systemInfo"
          >{{ systemInfo.os }} {{ systemInfo.osVersion }} &middot;
          {{ systemInfo.architecture }}</span
        >
        <span v-if="systemInfo?.xylonaVersion"> &middot; v{{ systemInfo.xylonaVersion }}</span>
        <span v-if="!systemInfo">No system info available</span>
      </div>
      <NodeResourceGauges :snapshot="snapshot" />
      <div class="row q-mt-sm q-gutter-md text-caption">
        <div>
          <q-icon name="dns" size="xs" class="q-mr-xs" />
          {{ snapshot?.gameServerCount ?? 0 }} servers ({{
            snapshot?.runningGameServerCount ?? 0
          }}
          running)
        </div>
        <div>
          <q-icon name="people" size="xs" class="q-mr-xs" />
          {{ snapshot?.userCount ?? 0 }} users
        </div>
      </div>
    </q-card-section>
  </q-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Node, NodeSystemInfo, NodeResourceSnapshot } from 'src/proto/shared_pb'
import NodeResourceGauges from './NodeResourceGauges.vue'

const props = defineProps<{
  node: Node
  systemInfo: NodeSystemInfo | undefined
  snapshot: NodeResourceSnapshot | undefined
}>()

const emit = defineEmits<{
  select: []
}>()

const healthLabel = computed(() => {
  const s = props.node.healthStatus
  if (s === 'healthy') return 'Healthy'
  if (s === 'degraded') return 'Degraded'
  if (s === 'unreachable') return 'Unreachable'
  return s || 'Unknown'
})

const healthColor = computed(() => {
  const s = props.node.healthStatus
  if (s === 'healthy') return 'positive'
  if (s === 'degraded') return 'warning'
  if (s === 'unreachable') return 'negative'
  return 'grey'
})
</script>
