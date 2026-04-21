import { create, toJsonString } from '@bufbuild/protobuf'
import { computed, onMounted, onUnmounted, ref, type Ref } from 'vue'
import { type GameServer, Status } from '@/proto/shared_pb'
import { Request, Request_Type, RequestSchema, type AllServersMetrics } from '@/proto/websocket_pb'
import { GetOrCreateXylonaWebsocketClient, XylonaEventBus } from '@/utils/shared'

interface UseGameServerMetricsPreviewOptions {
  gameServer: Ref<GameServer>
  gameServerId: Ref<string>
}

export function formatMetricsRate(bytesPerSec: number): string {
  if (bytesPerSec <= 0) return '0 B/s'
  if (bytesPerSec >= 1024 * 1024 * 1024) {
    return `${(bytesPerSec / (1024 * 1024 * 1024)).toFixed(1)} GB/s`
  }
  if (bytesPerSec >= 1024 * 1024) {
    return `${(bytesPerSec / (1024 * 1024)).toFixed(1)} MB/s`
  }
  if (bytesPerSec >= 1024) {
    return `${(bytesPerSec / 1024).toFixed(1)} KB/s`
  }
  return `${bytesPerSec.toFixed(0)} B/s`
}

export function formatMetricsUptime(total: number): string {
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
}

export function useGameServerMetricsPreview({
  gameServer,
  gameServerId,
}: UseGameServerMetricsPreviewOptions) {
  const metricsCpu = ref(0)
  const metricsCpuCores = ref(0)
  const metricsMemory = ref(0)
  const metricsMemoryPercent = ref(0)
  const metricsThreads = ref(0)
  const metricsDisk = ref(0)
  const metricsIoReadRate = ref(0)
  const metricsIoWriteRate = ref(0)
  const metricsConnections = ref(0)
  const metricsUptimeSeconds = ref(0)
  let uptimeTicker: ReturnType<typeof setInterval> | null = null
  let metricsSubscriptionActive = false
  let metricsPreviewUnmounted = false

  const metricsMaxMemory = computed(() => Number(gameServer.value.maxMemoryMb) * 1024 * 1024)
  const metricsMemoryRatio = computed(() => {
    if (metricsMaxMemory.value <= 0) return 0
    return Math.min(metricsMemory.value / metricsMaxMemory.value, 1)
  })

  const cpuBarClass = computed(() => {
    if (metricsCpu.value >= 80) return 'fill-high'
    if (metricsCpu.value >= 50) return 'fill-mid'
    return 'fill-low'
  })

  const memoryBarClass = computed(() => {
    if (metricsMemoryRatio.value >= 0.8) return 'fill-high'
    if (metricsMemoryRatio.value >= 0.5) return 'fill-mid'
    return 'fill-low'
  })

  const formattedUptime = computed(() => formatMetricsUptime(metricsUptimeSeconds.value))

  function onMetrics(metrics: AllServersMetrics) {
    const serverMetrics = metrics.servers[gameServerId.value]
    if (!serverMetrics) return

    metricsCpu.value = serverMetrics.cpuPercent
    metricsCpuCores.value = serverMetrics.cpuCores
    const workingSetBytes = Number(serverMetrics.memoryWorkingSetBytes)
    metricsMemory.value = workingSetBytes > 0 ? workingSetBytes : Number(serverMetrics.memoryBytes)
    metricsMemoryPercent.value = serverMetrics.memoryPercent
    metricsThreads.value = serverMetrics.numberOfThreads
    metricsDisk.value = Number(serverMetrics.diskUsageBytes)
    metricsIoReadRate.value = serverMetrics.ioReadRate
    metricsIoWriteRate.value = serverMetrics.ioWriteRate
    metricsConnections.value = serverMetrics.connectionCount
    metricsUptimeSeconds.value = Number(serverMetrics.uptimeSeconds)
  }

  function sendMetricsSubscriptionRequest(type: Request_Type): boolean {
    const ws = GetOrCreateXylonaWebsocketClient()
    if (!ws.isOpen()) {
      return false
    }

    const request: Request = create(RequestSchema, {})
    request.type = type
    request.gameServerId = gameServerId.value
    ws.send(toJsonString(RequestSchema, request))
    return true
  }

  function subscribeServerMetrics(): boolean {
    return sendMetricsSubscriptionRequest(Request_Type.SubscribeServerMetrics)
  }

  function unsubscribeServerMetrics(): boolean {
    return sendMetricsSubscriptionRequest(Request_Type.UnsubscribeServerMetrics)
  }

  function onMetricsWebsocketConnected() {
    if (metricsPreviewUnmounted || !metricsSubscriptionActive) {
      return
    }

    try {
      subscribeServerMetrics()
    } catch (error) {
      console.error('Failed to resubscribe to server metrics', error)
    }
  }

  function startMetricsPreviewLifecycle() {
    if (metricsPreviewUnmounted || metricsSubscriptionActive) {
      return
    }

    metricsSubscriptionActive = true
    XylonaEventBus.on('websocketConnected', onMetricsWebsocketConnected)
    try {
      subscribeServerMetrics()
    } catch (error) {
      console.error('Failed to subscribe to server metrics', error)
    }
  }

  function stopMetricsPreviewLifecycle() {
    if (!metricsSubscriptionActive) {
      return
    }

    metricsSubscriptionActive = false
    XylonaEventBus.off('websocketConnected', onMetricsWebsocketConnected)
    try {
      unsubscribeServerMetrics()
    } catch (error) {
      console.error('Failed to unsubscribe from server metrics', error)
    }
  }

  onMounted(() => {
    XylonaEventBus.on('gameServerMetrics', onMetrics)
    uptimeTicker = setInterval(() => {
      if (gameServer.value.status === Status.ONLINE && metricsUptimeSeconds.value > 0) {
        metricsUptimeSeconds.value++
      }
    }, 1000)
  })

  onUnmounted(() => {
    metricsPreviewUnmounted = true
    XylonaEventBus.off('gameServerMetrics', onMetrics)
    stopMetricsPreviewLifecycle()
    if (uptimeTicker !== null) {
      clearInterval(uptimeTicker)
      uptimeTicker = null
    }
  })

  return {
    cpuBarClass,
    formatRate: formatMetricsRate,
    formattedUptime,
    memoryBarClass,
    metricsConnections,
    metricsCpu,
    metricsCpuCores,
    metricsDisk,
    metricsIoReadRate,
    metricsIoWriteRate,
    metricsMaxMemory,
    metricsMemory,
    metricsMemoryPercent,
    metricsMemoryRatio,
    metricsThreads,
    metricsUptimeSeconds,
    startMetricsPreviewLifecycle,
    stopMetricsPreviewLifecycle,
  }
}
