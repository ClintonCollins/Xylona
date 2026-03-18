<template>
  <q-list separator dense>
    <q-item>
      <q-item-section>
        <q-item-label>CPU</q-item-label>
        <q-linear-progress
          :value="cpuPercent / 100"
          :color="cpuColor"
          track-color="grey-3"
          rounded
          class="q-mt-xs" />
      </q-item-section>
      <q-item-section side> {{ cpuPercent.toFixed(1) }}% </q-item-section>
    </q-item>
    <q-item>
      <q-item-section>
        <q-item-label>Memory</q-item-label>
        <q-linear-progress
          :value="memoryPercent / 100"
          :color="memoryColor"
          track-color="grey-3"
          rounded
          class="q-mt-xs" />
      </q-item-section>
      <q-item-section side>
        {{ bytesToSize(memoryUsed) }} / {{ bytesToSize(memoryTotal) }}
      </q-item-section>
    </q-item>
    <q-item>
      <q-item-section>
        <q-item-label>Disk</q-item-label>
        <q-linear-progress
          :value="diskPercent / 100"
          :color="diskColor"
          track-color="grey-3"
          rounded
          class="q-mt-xs" />
      </q-item-section>
      <q-item-section side>
        {{ bytesToSize(diskUsed) }} / {{ bytesToSize(diskTotal) }}
      </q-item-section>
    </q-item>
  </q-list>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NodeResourceSnapshot } from 'src/proto/shared_pb'
import { bytesToSize } from '@/utils/shared'

const props = defineProps<{
  snapshot: NodeResourceSnapshot | undefined
}>()

const cpuPercent = computed(() => props.snapshot?.cpuPercent ?? 0)
const memoryPercent = computed(() => props.snapshot?.memoryPercent ?? 0)
const diskPercent = computed(() => props.snapshot?.diskPercent ?? 0)
const memoryUsed = computed(() => Number(props.snapshot?.memoryUsedBytes ?? 0))
const memoryTotal = computed(() => Number(props.snapshot?.memoryTotalBytes ?? 0))
const diskUsed = computed(() => Number(props.snapshot?.diskUsedBytes ?? 0))
const diskTotal = computed(() => Number(props.snapshot?.diskTotalBytes ?? 0))

function colorForPercent(value: number) {
  if (value >= 80) return 'negative'
  if (value >= 50) return 'warning'
  return 'positive'
}

const cpuColor = computed(() => colorForPercent(cpuPercent.value))
const memoryColor = computed(() => colorForPercent(memoryPercent.value))
const diskColor = computed(() => colorForPercent(diskPercent.value))
</script>
