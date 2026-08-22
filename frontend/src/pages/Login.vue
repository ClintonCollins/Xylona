<template>
  <q-layout view="lHh Lpr fff">
    <q-page-container>
      <q-page class="login-page">
        <div class="login-layout">
          <div class="login-brand-side">
            <div class="login-brand-name">Xylona</div>
            <div class="login-brand-tagline">
              Game Server<br />
              Control Panel
            </div>
          </div>
          <div class="login-form-side">
            <q-form
              aria-describedby="login-help"
              aria-labelledby="login-title"
              class="login-form"
              greedy
              @submit.prevent="login">
              <div class="login-form-header">
                <h1 id="login-title" class="login-form-title">Sign in to Xylona</h1>
                <p id="login-help" class="login-form-help">
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
              <div v-if="loginError" class="login-error rounded-borders q-mt-md" role="alert">
                <q-icon name="report_problem" size="sm" />
                <span>{{ loginError }}</span>
              </div>
              <q-btn
                :disable="loggingIn"
                :loading="loggingIn"
                class="full-width login-btn q-mt-lg"
                color="primary"
                label="Sign in"
                no-caps
                size="lg"
                type="submit" />
            </q-form>
          </div>
        </div>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script lang="ts" setup>
import { Code, ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'

import { ref } from 'vue'
import { create } from '@bufbuild/protobuf'
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

<style scoped>
.login-page {
  background: var(--xy-base);
}

.login-layout {
  display: flex;
  min-height: 100vh;
  align-items: center;
  background: var(--xy-base);
}

.login-brand-side {
  flex: 0 0 44%;
  align-self: stretch;
  padding: var(--xy-space-3xl) var(--xy-space-3xl) var(--xy-space-3xl) clamp(2rem, 8vw, 6rem);
  display: flex;
  flex-direction: column;
  justify-content: center;
  background: var(--xy-surface-1);
  border-right: 1px solid var(--xy-border);
}

.login-brand-name {
  font-family: var(--xy-font-brand);
  font-size: clamp(3.5rem, 6vw, 5.5rem);
  color: var(--xy-accent);
  letter-spacing: 0.04em;
  line-height: 0.9;
}

.login-brand-tagline {
  font-family: var(--xy-font-body);
  font-size: clamp(1rem, 2vw, 1.4rem);
  color: var(--xy-text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  margin-top: var(--xy-space-md);
  line-height: 1.5;
}

.login-form-side {
  flex: 1;
  padding: var(--xy-space-3xl);
  display: flex;
  align-items: center;
  justify-content: center;
}

.login-form {
  width: 100%;
  max-width: 420px;
  padding: clamp(var(--xy-space-lg), 4vw, var(--xy-space-2xl));
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
  box-shadow: var(--xy-shadow-lg);
}

.login-form-header {
  margin-bottom: var(--xy-space-xl);
}

.login-form-title {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: clamp(1.6rem, 3vw, 2rem);
  line-height: 1.2;
}

.login-form-help {
  margin: var(--xy-space-sm) 0 0;
  color: var(--xy-text-secondary);
  line-height: 1.5;
}

.login-error {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
}

.login-btn {
  font-family: var(--xy-font-control);
  font-weight: 600;
  letter-spacing: 0.04em;
}

@media (max-width: 768px) {
  .login-layout {
    flex-direction: column;
    justify-content: center;
    padding: var(--xy-space-lg);
    gap: var(--xy-space-xl);
  }

  .login-brand-side {
    flex: none;
    padding: 0;
    text-align: center;
    background: transparent;
    border-right: 0;
  }

  .login-brand-name {
    font-size: clamp(3rem, 12vw, 5rem);
  }

  .login-form-side {
    flex: none;
    padding: 0;
    width: 100%;
    border-left: none;
  }

  .login-form {
    padding: var(--xy-space-lg);
  }
}
</style>
