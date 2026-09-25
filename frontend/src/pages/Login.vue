<template>
  <auth-page-shell>
    <q-form
      aria-describedby="login-help"
      aria-labelledby="login-title"
      greedy
      @submit.prevent="login">
      <div class="auth-header">
        <h1 id="login-title" class="auth-title">Sign in to Xylona</h1>
        <p id="login-help" class="auth-help">
          {{
            signInReasons[String(route.query.reason)] ??
            'Use your panel credentials to access the servers and tools assigned to you.'
          }}
        </p>
      </div>
      <q-input
        v-model="username"
        :rules="[(val: string) => !!val || 'Username is required']"
        aria-required="true"
        autofocus
        autocomplete="username"
        color="primary"
        label="Username"
        lazy-rules
        name="username"
        outlined />
      <q-input
        v-model="password"
        :rules="[(val: string) => !!val || 'Password is required']"
        aria-required="true"
        :type="showPassword ? 'text' : 'password'"
        autocomplete="current-password"
        class="q-mt-md"
        color="primary"
        label="Password"
        lazy-rules
        name="password"
        outlined>
        <template #append>
          <q-btn
            :aria-label="showPassword ? 'Hide password' : 'Show password'"
            :icon="showPassword ? 'visibility_off' : 'visibility'"
            dense
            flat
            round
            type="button"
            @click="showPassword = !showPassword">
            <q-tooltip>{{ showPassword ? 'Hide password' : 'Show password' }}</q-tooltip>
          </q-btn>
        </template>
      </q-input>
      <q-banner v-if="loginError" class="xy-banner-negative q-mt-md" dense role="alert">
        <template #avatar>
          <q-icon name="report_problem" />
        </template>
        {{ loginError }}
      </q-banner>
      <q-btn
        :disable="loggingIn"
        :loading="loggingIn"
        class="full-width auth-button q-mt-lg"
        color="primary"
        label="Sign in"
        no-caps
        size="lg"
        type="submit" />
    </q-form>
  </auth-page-shell>
</template>

<script lang="ts" setup>
import { Code, ConnectError } from '@connectrpc/connect'

import { ref } from 'vue'
import { create } from '@bufbuild/protobuf'
import AuthPageShell from '@/components/shared/AuthPageShell.vue'
import { LoginRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { useUserAuthStore } from '@/stores/xylona'
import { safeReturnPath } from '@/utils/login-redirect'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const showPassword = ref(false)
const loggingIn = ref(false)
const loginError = ref('')
const userAuthStore = useUserAuthStore()

const signInReasons: Record<string, string> = {
  'session-expired': 'Your session expired. Sign in again to continue.',
  'account-changed': 'Your account changed, so every session was signed out. Sign in again.',
}

async function login() {
  if (loggingIn.value) return
  loginError.value = ''
  loggingIn.value = true
  try {
    const loginRequest = create(LoginRequestSchema, {
      userName: username.value,
      password: password.value,
    })
    const response = await GetXylonaClient().login(loginRequest)
    if (response.user === undefined) {
      loginError.value = 'Invalid username or password'
      return
    }
    userAuthStore.setUser(response.user)
    await router.push(safeReturnPath(route.query['redirect']))
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    // The inline alert announces the failure, so there is no toast as well.
    loginError.value =
      err.code === Code.Unauthenticated ? 'Invalid username or password' : ConnectErrorToString(err)
  } finally {
    loggingIn.value = false
  }
}
</script>
