import { create } from '@bufbuild/protobuf'
import { onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import type { Status } from '@/proto/shared_pb'
import { GetGameServerDiagnosisRequestSchema, type GameServerDiagnosis } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

export function useGameServerDiagnosis(serverId: Ref<string>, status: Ref<Status>) {
  const diagnosis = ref<GameServerDiagnosis>()
  const loading = ref(true)
  const stale = ref(false)
  const loaded = ref(false)
  let request: AbortController | undefined
  let timer: ReturnType<typeof setInterval> | undefined
  let mounted = false

  async function refresh() {
    if (!mounted || document.visibilityState === 'hidden') return
    request?.abort()
    const current = new AbortController()
    request = current
    loading.value = true
    try {
      const response = await GetXylonaClient().getGameServerDiagnosis(
        create(GetGameServerDiagnosisRequestSchema, { serverId: serverId.value }),
        { signal: current.signal, timeoutMs: 10_000 },
      )
      if (current.signal.aborted) return
      diagnosis.value = response.diagnosis
      loaded.value = true
      stale.value = false
    } catch {
      if (!current.signal.aborted) stale.value = true
    } finally {
      if (!current.signal.aborted) loading.value = false
    }
  }

  function onVisibilityChange() {
    if (document.visibilityState === 'hidden') {
      request?.abort()
      loading.value = false
    } else {
      void refresh()
    }
  }

  watch(
    serverId,
    () => {
      request?.abort()
      diagnosis.value = undefined
      loaded.value = false
      stale.value = false
      void refresh()
    },
    { flush: 'sync' },
  )
  watch(status, () => void refresh())
  onMounted(() => {
    mounted = true
    void refresh()
    document.addEventListener('visibilitychange', onVisibilityChange)
    timer = setInterval(() => {
      if (!loading.value) void refresh()
    }, 5_000)
  })
  onBeforeUnmount(() => {
    mounted = false
    request?.abort()
    clearInterval(timer)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })

  return { diagnosis, loading, stale, loaded, refresh }
}
