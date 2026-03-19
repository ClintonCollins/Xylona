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
            <div class="login-form">
              <q-input v-model="username" outlined label="Username" color="primary" />
              <q-input
                v-model="password"
                outlined
                class="q-mt-md"
                type="password"
                color="primary"
                label="Password"
                @keyup.enter="login" />
              <q-btn
                color="primary"
                size="lg"
                label="Sign in"
                no-caps
                class="full-width login-btn q-mt-lg"
                @click="login" />
            </div>
          </div>
        </div>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { Code, ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'

import { ref } from 'vue'
import { create } from '@bufbuild/protobuf'
import { LoginRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { useUserAuthStore } from '@/stores/xylona'
import { useRouter } from 'vue-router'

const router = useRouter()
const username = ref('')
const password = ref('')
const userAuthStore = useUserAuthStore()

const $q = useQuasar()

async function login() {
  const loginRequest = create(LoginRequestSchema, {
    userName: username.value,
    password: password.value,
  })
  try {
    const response = await GetXylonaClient().login(loginRequest)
    if (response.user === undefined) {
      $q.notify({
        type: 'xylona-error',
        position: 'top',
        caption: 'Invalid username or password',
        icon: 'report_problem',
      })
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
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: caption,
      icon: 'report_problem',
    })
  }
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
}

.login-brand-side {
  flex: 0 0 55%;
  padding: var(--xy-space-3xl) var(--xy-space-3xl) var(--xy-space-3xl) clamp(2rem, 8vw, 6rem);
  display: flex;
  flex-direction: column;
  justify-content: center;
}

.login-brand-name {
  font-family: var(--xy-font-brand);
  font-size: clamp(4rem, 10vw, 8rem);
  color: var(--xy-accent);
  letter-spacing: 0.04em;
  line-height: 0.9;
}

.login-brand-tagline {
  font-family: var(--xy-font-display);
  font-size: clamp(1rem, 2vw, 1.4rem);
  color: var(--xy-text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
  margin-top: var(--xy-space-md);
  line-height: 1.5;
}

.login-form-side {
  flex: 0 0 45%;
  padding: var(--xy-space-3xl);
  display: flex;
  align-items: center;
  justify-content: center;
  border-left: 1px solid var(--xy-border);
}

.login-form {
  width: 100%;
  max-width: 360px;
}

.login-btn {
  font-family: var(--xy-font-display);
  font-weight: 600;
  letter-spacing: 0.04em;
}

@media (max-width: 768px) {
  .login-layout {
    flex-direction: column;
    justify-content: center;
    padding: var(--xy-space-xl);
    gap: var(--xy-space-xl);
  }

  .login-brand-side {
    flex: none;
    padding: 0;
    text-align: center;
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
}
</style>
