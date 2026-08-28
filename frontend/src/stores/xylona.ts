import { create } from '@bufbuild/protobuf'
import { defineStore } from 'pinia'
import { Notify } from 'quasar'

import { getXylonaClient } from '@/api/connect-client'
import { buildXylonaErrorNotification, connectErrorMessage } from '@/api/connect-errors'
import { DisposeXylonaWebsocketClients } from '@/utils/shared'
import {
  CheckUserAuthenticatedRequestSchema,
  CheckUserAuthenticatedResponse,
  LogoutRequestSchema,
  User,
} from '@/proto/xylona_pb'

interface userAuthState {
  user: User | null
  initialFetch: boolean
  initialResponse: CheckUserAuthenticatedResponse | null
}

const legacyUserScopedCacheKeys = [
  'game-server-display-rows-cache',
  'game-server-remote-node-ids-cache',
]

function clearLegacyUserScopedCaches(): void {
  try {
    for (const key of legacyUserScopedCacheKeys) {
      localStorage.removeItem(key)
    }
  } catch {
    // Storage may be unavailable in restricted browser contexts.
  }
}

export const useUserAuthStore = defineStore('userAuth', {
  state: (): userAuthState => {
    clearLegacyUserScopedCaches()
    return {
      user: null,
      initialFetch: false,
      initialResponse: null,
    }
  },
  actions: {
    setUser(user: User) {
      this.user = user
      this.initialFetch = false
      this.initialResponse = null
    },
    async checkUserAuthenticated(): Promise<CheckUserAuthenticatedResponse | null> {
      if (this.initialFetch) {
        return this.initialResponse as CheckUserAuthenticatedResponse
      }
      this.initialFetch = true
      try {
        const response: CheckUserAuthenticatedResponse =
          await getXylonaClient().checkUserAuthenticated(
            create(CheckUserAuthenticatedRequestSchema, {}),
          )
        this.initialResponse = response
        if (response.user) {
          this.user = response.user
          return response
        }
      } catch (unknownError: unknown) {
        this.initialFetch = false
        const message = connectErrorMessage(unknownError)
        Notify.create(
          buildXylonaErrorNotification(message, {
            timeout: 0,
            closeBtn: 'Dismiss',
            icon: 'report_problem',
          }),
        )
        console.error(message)
      }
      return null
    },
    async logout(): Promise<void> {
      try {
        await getXylonaClient().logout(create(LogoutRequestSchema, {}))
        DisposeXylonaWebsocketClients()
        clearLegacyUserScopedCaches()
        this.user = null
        this.initialFetch = false
        this.initialResponse = null
      } catch (unknownError: unknown) {
        console.error('Logout error:', connectErrorMessage(unknownError))
      }
    },
  },
})
