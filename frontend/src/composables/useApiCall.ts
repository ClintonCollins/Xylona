import { ref, type Ref } from 'vue'
import { ConnectError } from '@connectrpc/connect'
import { ConnectErrorToString } from '@/utils/shared'

export interface UseApiCallOptions {
  notify?: (opts: { type: string; caption: string; position: string; timeout: number }) => void
  errorPrefix?: string
}

export function useApiCall<T, Args extends unknown[] = []>(
  apiFn: (...args: Args) => Promise<T>,
  options?: UseApiCallOptions,
): {
  loading: Ref<boolean>
  error: Ref<string | null>
  execute: (...args: Args) => Promise<T | undefined>
} {
  const loading = ref(false)
  const error = ref<string | null>(null)
  let latestExecutionID = 0

  async function execute(...args: Args): Promise<T | undefined> {
    latestExecutionID += 1
    const executionID = latestExecutionID
    loading.value = true
    error.value = null
    try {
      const result = await apiFn(...args)
      return result
    } catch (e: unknown) {
      if (executionID !== latestExecutionID) {
        return undefined
      }

      const connectErr = ConnectError.from(e)
      const message = options?.errorPrefix
        ? `${options.errorPrefix}: ${ConnectErrorToString(connectErr)}`
        : ConnectErrorToString(connectErr)
      error.value = message
      if (options?.notify) {
        options.notify({
          type: 'xylona-error',
          caption: message,
          position: 'top',
          timeout: 5000,
        })
      }
      return undefined
    } finally {
      if (executionID === latestExecutionID) {
        loading.value = false
      }
    }
  }

  return { loading, error, execute }
}
