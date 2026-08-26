import { create } from '@bufbuild/protobuf'
import { defineStore } from 'pinia'
import { Notify } from 'quasar'

import { getXylonaClient } from '@/api/connect-client'
import { buildXylonaErrorNotification, connectErrorMessage } from '@/api/connect-errors'
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

export const useUserAuthStore = defineStore('userAuth', {
  state: (): userAuthState => ({
    user: null,
    initialFetch: false,
    initialResponse: null,
  }),
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
        this.user = null
        this.initialFetch = false
        this.initialResponse = null
      } catch (unknownError: unknown) {
        console.error('Logout error:', connectErrorMessage(unknownError))
      }
    },
  },
})
