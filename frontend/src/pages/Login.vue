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
            route.query.reason === 'session-expired'
              ? 'Your session expired. Sign in again to continue.'
              : 'Use your panel credentials to access the servers and tools assigned to you.'
          }}
        </p>
      </div>
      <q-input
        v-model="username"
        :rules="[(val: string) => !!val || 'Username is required']"
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
            :aria-pressed="showPassword"
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
      <div v-if="loginError" class="auth-error rounded-borders q-mt-md" role="alert">
        <q-icon name="report_problem" size="sm" />
        <span>{{ loginError }}</span>
      </div>
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
import { useQuasar } from 'quasar'

import { ref } from 'vue'
import { create } from '@bufbuild/protobuf'
import AuthPageShell from '@/components/shared/AuthPageShell.vue'
import { LoginRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { useUserAuthStore } from '@/stores/xylona'
import { useRoute, useRouter } from 'vue-router'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const showPassword = ref(false)
const loggingIn = ref(false)
const loginError = ref('')
const userAuthStore = useUserAuthStore()

const $q = useQuasar()

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
      notifyLoginError('Invalid username or password')
      return
    }
    userAuthStore.setUser(response.user)
    await router.push({ path: '/' })
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    let caption = ConnectErrorToString(err)
    if (err.code === Code.Unauthenticated) {
      caption = 'Invalid username or password'
    }
    notifyLoginError(caption)
  } finally {
    loggingIn.value = false
  }
}

function notifyLoginError(message: string) {
  loginError.value = message
  $q.notify({
    type: 'xylona-error',
    position: 'top',
    caption: message,
    icon: 'report_problem',
  })
}
</script>
