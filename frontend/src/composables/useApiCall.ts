import { ref, type Ref } from 'vue'

import {
  buildXylonaErrorNotification,
  connectErrorMessage,
  type XylonaErrorNotification,
} from '@/api/connect-errors'

export interface UseApiCallOptions {
  notify?: (opts: XylonaErrorNotification) => void
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

      const message = connectErrorMessage(e, options?.errorPrefix)
      error.value = message
      if (options?.notify) {
        options.notify(buildXylonaErrorNotification(message))
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
