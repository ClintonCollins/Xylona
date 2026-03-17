<template>
  <div v-if="gameServer.status === Status.ONLINE" class="q-mt-md">
    <div class="text-subtitle2 q-mb-sm">Resource Usage</div>
    <q-list separator dense>
      <q-item>
        <q-item-section>
          <q-item-label>CPU <span class="text-caption text-grey">({{ cpuCores }} cores)</span></q-item-label>
          <q-linear-progress
            :value="cpuPercent / 100"
            :color="cpuColor"
            track-color="grey-3"
            rounded
            class="q-mt-xs"
          />
        </q-item-section>
        <q-item-section side>
          {{ cpuPercent.toFixed(1) }}%
        </q-item-section>
      </q-item>
      <q-item>
        <q-item-section>
          <q-item-label>Memory (Private)</q-item-label>
          <q-linear-progress
            v-if="maxMemoryBytes > 0"
            :value="memoryRatio"
            :color="memoryColor"
            track-color="grey-3"
            rounded
            class="q-mt-xs"
          />
        </q-item-section>
        <q-item-section side>
          <span>{{ bytesToSize(memoryBytes) }}<span v-if="maxMemoryBytes > 0"> / {{ bytesToSize(maxMemoryBytes) }}</span></span>
        </q-item-section>
      </q-item>
      <q-item>
        <q-item-section>Working Set</q-item-section>
        <q-item-section side>{{ bytesToSize(memoryWorkingSetBytes) }}</q-item-section>
      </q-item>
      <q-item v-if="memoryPercent > 0">
        <q-item-section>System RAM</q-item-section>
        <q-item-section side>{{ memoryPercent.toFixed(1) }}%</q-item-section>
      </q-item>
      <q-item>
        <q-item-section>Threads</q-item-section>
        <q-item-section side>{{ numThreads }}</q-item-section>
      </q-item>
      <q-item>
        <q-item-section>Disk Usage</q-item-section>
        <q-item-section side>{{ bytesToSize(diskUsageBytes) }}</q-item-section>
      </q-item>
      <q-item>
        <q-item-section>I/O Read</q-item-section>
        <q-item-section side>{{ formatRate(ioReadRate) }}</q-item-section>
      </q-item>
      <q-item>
        <q-item-section>I/O Write</q-item-section>
        <q-item-section side>{{ formatRate(ioWriteRate) }}</q-item-section>
      </q-item>
      <q-item>
        <q-item-section>Connections</q-item-section>
        <q-item-section side>{{ connectionCount }}</q-item-section>
      </q-item>
      <q-item>
        <q-item-section>Uptime</q-item-section>
        <q-item-section side>{{ formattedUptime }}</q-item-section>
      </q-item>
    </q-list>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { GameServer, Status } from 'src/proto/shared_pb'
import { AllServersMetrics } from 'src/proto/websocket_pb'
import { XylonaEventBus, bytesToSize } from '@/utils/shared'

const props = defineProps<{
  gameServerId: string
  gameServer: GameServer
}>()

const cpuPercent = ref<number>(Number(props.gameServer.cpuPercent))
const cpuCores = ref<number>(Number(props.gameServer.cpuCores))
const memoryBytes = ref<number>(Number(props.gameServer.memoryBytes))
const memoryWorkingSetBytes = ref<number>(Number(props.gameServer.memoryWorkingSetBytes))
const memoryPercent = ref<number>(props.gameServer.memoryPercent)
const numThreads = ref<number>(Number(props.gameServer.numberOfThreads))
const diskUsageBytes = ref<number>(Number(props.gameServer.diskUsageBytes))
const ioReadRate = ref<number>(props.gameServer.ioReadRate)
const ioWriteRate = ref<number>(props.gameServer.ioWriteRate)
const connectionCount = ref<number>(props.gameServer.connectionCount)
const uptimeSeconds = ref<number>(Number(props.gameServer.uptimeSeconds))

// Local timer to tick uptime every second.
let uptimeTicker: ReturnType<typeof setInterval> | null = null

const cpuColor = computed(() => {
  if (cpuPercent.value >= 80) return 'negative'
  if (cpuPercent.value >= 50) return 'warning'
  return 'positive'
})

const maxMemoryBytes = computed(() => Number(props.gameServer.maxMemoryMb) * 1024 * 1024)

const memoryRatio = computed(() => {
  if (maxMemoryBytes.value <= 0) return 0
  return Math.min(memoryBytes.value / maxMemoryBytes.value, 1)
})

const memoryColor = computed(() => {
  if (memoryRatio.value >= 0.8) return 'negative'
  if (memoryRatio.value >= 0.5) return 'warning'
  return 'positive'
})

const formattedUptime = computed(() => {
  const total = uptimeSeconds.value
  if (total <= 0) return '0s'
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0) parts.push(`${hours}h`)
  if (minutes > 0) parts.push(`${minutes}m`)
  if (parts.length === 0 || seconds > 0) parts.push(`${seconds}s`)
  return parts.join(' ')
})

function formatRate(bytesPerSec: number): string {
  if (bytesPerSec <= 0) return '0 B/s'
  if (bytesPerSec >= 1024 * 1024 * 1024) return `${(bytesPerSec / (1024 * 1024 * 1024)).toFixed(1)} GB/s`
  if (bytesPerSec >= 1024 * 1024) return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`
  if (bytesPerSec >= 1024) return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
  return `${bytesPerSec.toFixed(0)} B/s`
}

function onMetrics(metrics: AllServersMetrics) {
  const serverMetrics = metrics.servers[props.gameServerId]
  if (!serverMetrics) return
  cpuPercent.value = serverMetrics.cpuPercent
  cpuCores.value = serverMetrics.cpuCores
  memoryBytes.value = Number(serverMetrics.memoryBytes)
  memoryWorkingSetBytes.value = Number(serverMetrics.memoryWorkingSetBytes)
  memoryPercent.value = serverMetrics.memoryPercent
  numThreads.value = serverMetrics.numberOfThreads
  diskUsageBytes.value = Number(serverMetrics.diskUsageBytes)
  ioReadRate.value = serverMetrics.ioReadRate
  ioWriteRate.value = serverMetrics.ioWriteRate
  connectionCount.value = serverMetrics.connectionCount
  uptimeSeconds.value = Number(serverMetrics.uptimeSeconds)
}

onMounted(() => {
  XylonaEventBus.on('gameServerMetrics', onMetrics)
  uptimeTicker = setInterval(() => {
    if (props.gameServer.status === Status.ONLINE && uptimeSeconds.value > 0) {
      uptimeSeconds.value++
    }
  }, 1000)
})

onUnmounted(() => {
  XylonaEventBus.off('gameServerMetrics', onMetrics)
  if (uptimeTicker !== null) {
    clearInterval(uptimeTicker)
    uptimeTicker = null
  }
})
</script>
