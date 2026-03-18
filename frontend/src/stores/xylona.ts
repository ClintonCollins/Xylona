import { ConnectError } from '@connectrpc/connect'
import { create } from '@bufbuild/protobuf'
import { defineStore } from 'pinia'
import {
  CheckUserAuthenticatedRequestSchema,
  CheckUserAuthenticatedResponse,
  LogoutRequestSchema,
  User,
} from 'src/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from 'src/utils/shared'
import { Notify } from 'quasar'

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
    },
    async checkUserAuthenticated(): Promise<CheckUserAuthenticatedResponse | null> {
      if (this.initialFetch) {
        return this.initialResponse as CheckUserAuthenticatedResponse
      }
      this.initialFetch = true
      try {
        const response: CheckUserAuthenticatedResponse =
          await GetXylonaClient().checkUserAuthenticated(
            create(CheckUserAuthenticatedRequestSchema),
          )
        this.initialResponse = response
        if (response.user) {
          this.user = response.user
          return response
        }
      } catch (unknownError: unknown) {
        this.initialFetch = false
        const err = ConnectError.from(unknownError)
        Notify.create({
          type: 'xylona-error',
          position: 'top',
          caption: ConnectErrorToString(err),
          timeout: 0,
          closeBtn: 'Dismiss',
          icon: 'report_problem',
        })
        console.error(err.message)
      }
      return null
    },
    async logout(): Promise<void> {
      try {
        await GetXylonaClient().logout(create(LogoutRequestSchema))
        this.user = null
        this.initialFetch = false
        this.initialResponse = null
      } catch (unknownError: unknown) {
        const err = ConnectError.from(unknownError)
        console.error('Logout error:', err.message)
      }
    },
  },
})

export const useToolbarNavQTabsStore = defineStore('toolbarNavQTabs', {
  state: (): {
    selectedTab: string
    tabs: { name: string; to: string; exact: boolean; icon: string }[]
  } => ({
    selectedTab: '',
    tabs: [],
  }),
  actions: {
    changeTabs(newTabs: { name: string; to: string; exact: boolean; icon: string }[]) {
      this.tabs = newTabs
    },
  },
})
