<template>
  <q-layout view="lHh Lpr fff">
    <q-page-container>
      <q-page class="login-page flex flex-center row">
        <div class="login-grid-bg"></div>
        <q-card class="q-pa-xl col-12 col-sm-10 col-md-5 col-xl-3 login-card" id="login-card">
          <q-card-section class="text-center">
            <div class="login-brand">Xylona</div>
            <div class="login-brand-line"></div>
            <div class="login-tagline">Game Server Control Panel</div>
          </q-card-section>
          <q-card-section>
            <q-input outlined v-model="username" label="Username" color="primary"></q-input>
            <q-input
              outlined
              class="q-mt-md"
              v-model="password"
              type="password"
              color="primary"
              label="Password"
              @keyup.enter="login"></q-input>
          </q-card-section>
          <q-card-section>
            <q-btn
              color="primary"
              size="lg"
              label="Sign in"
              @click="login"
              no-caps
              class="full-width login-btn"></q-btn>
          </q-card-section>
        </q-card>
      </q-page>
    </q-page-container>
  </q-layout>
</template>

<script setup lang="ts">
import { Code, ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'

// Login page layout
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
    console.error(caption)
  }
}
</script>

<style scoped>
.login-page {
  position: relative;
  overflow: hidden;
  background: var(--xy-base);
}

/* Subtle perspective grid background */
.login-grid-bg {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(34, 211, 238, 0.03) 1px, transparent 1px),
    linear-gradient(90deg, rgba(34, 211, 238, 0.03) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(ellipse 60% 50% at 50% 50%, black 20%, transparent 70%);
  pointer-events: none;
}

.login-card {
  position: relative;
  z-index: 1;
  border: 1px solid var(--xy-border);
  box-shadow: var(--xy-shadow-lg), var(--xy-glow-accent);
}

.login-brand {
  font-family: var(--xy-font-brand);
  font-size: 2.5rem;
  color: var(--xy-accent);
  letter-spacing: 0.06em;
  line-height: 1;
}

.login-brand-line {
  width: 48px;
  height: 2px;
  background: var(--xy-accent);
  margin: 12px auto 10px;
  border-radius: 1px;
}

.login-tagline {
  font-family: var(--xy-font-display);
  font-size: 0.8rem;
  color: var(--xy-text-muted);
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.login-btn {
  font-family: var(--xy-font-display);
  font-weight: 600;
  letter-spacing: 0.04em;
}
</style>
