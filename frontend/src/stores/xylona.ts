import { defineStore } from 'pinia';
import {CheckUserAuthenticatedRequest, CheckUserAuthenticatedResponse, User} from "src/proto/xylona_pb";
import {GetXylonaClient} from "src/utils/shared";

interface userAuthState {
  user: User | null
  initialFetch: boolean
  initialResponse: CheckUserAuthenticatedResponse|null
}

export const useUserAuthStore = defineStore('userAuth', {
  state: (): userAuthState => ({
    user: null,
    initialFetch: false,
    initialResponse: null,
  }),
  actions: {
    setUser(user: User) {
      this.user = user;
    },
    async checkUserAuthenticated(): Promise<CheckUserAuthenticatedResponse|null> {
      if (this.initialFetch) {
        return this.initialResponse as CheckUserAuthenticatedResponse
      }
      this.initialFetch = true
      try {
        const response: CheckUserAuthenticatedResponse = await GetXylonaClient().checkUserAuthenticated(new CheckUserAuthenticatedRequest())
        this.initialResponse = response
        if (response.user) {
          this.user = response.user
          return response
        }
      } catch (e) {
        console.error(e)
      }
      return null
    }
  },
});

export const useToolbarNavQTabsStore = defineStore('toolbarNavQTabs', {
  state: (): {selectedTab: string, tabs: {name: string, to: string, exact: boolean, icon: string}[]} => ({
    selectedTab: '',
    tabs: [],
  }),
  actions: {
    changeTabs(newTabs: {name: string, to: string, exact: boolean, icon: string}[]) {
      this.tabs = newTabs
    },
  },
})
