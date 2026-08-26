import { create } from '@bufbuild/protobuf'

import { GetSetupStatusRequestSchema } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

export function fetchSetupStatus() {
  return GetXylonaClient().getSetupStatus(create(GetSetupStatusRequestSchema, {}))
}

export function unauthenticatedRedirect(
  setupNeeded: boolean,
  currentPath: string,
  token = '',
): string | null {
  if (setupNeeded) {
    if (currentPath === '/setup') {
      return null
    }
    const trimmedToken = token.trim()
    return trimmedToken === '' ? '/setup' : `/setup?token=${encodeURIComponent(trimmedToken)}`
  }
  if (currentPath === '/setup') {
    return '/login'
  }
  return null
}
