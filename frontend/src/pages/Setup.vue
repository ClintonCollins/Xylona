<template>
  <auth-page-shell>
    <template v-if="mode === 'loading'">
      <div class="auth-header">
        <h1 class="auth-title">Checking setup</h1>
        <p class="auth-help">Looking up whether this controller still needs a first admin.</p>
      </div>
    </template>
    <template v-else-if="mode === 'error'">
      <div class="auth-header">
        <h1 class="auth-title">Unable to check setup</h1>
        <div class="auth-error rounded-borders q-mt-md" role="alert">
          <q-icon name="report_problem" size="sm" />
          <span>{{ setupError }}</span>
        </div>
      </div>
      <q-btn
        class="full-width auth-button"
        color="primary"
        label="Retry"
        no-caps
        size="lg"
        type="button"
        @click="loadSetupStatus" />
    </template>
    <template v-else-if="mode === 'blocked'">
      <div class="auth-header">
        <h1 class="auth-title">Xylona is not set up yet</h1>
        <p class="auth-help">
          Copy the setup URL from the host log, or SSH in and run
          <span class="setup-command">xylona setup</span>.
        </p>
      </div>
    </template>
    <q-form
      v-else-if="mode === 'form'"
      aria-describedby="setup-help"
      aria-labelledby="setup-title"
      greedy
      @submit.prevent="submitSetup">
      <div class="auth-header">
        <h1 id="setup-title" class="auth-title">Create the first admin</h1>
        <p id="setup-help" class="auth-help">
          This account is a superuser. You can add more users after you sign in.
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
        v-model="email"
        autocomplete="email"
        class="q-mt-md"
        color="primary"
        hint="Optional. Defaults to username@localhost."
        label="Email"
        name="email"
        outlined />
      <q-input
        v-model="password"
        :rules="[(val: string) => !!val || 'Password is required']"
        :type="showPassword ? 'text' : 'password'"
        autocomplete="new-password"
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
      <q-input
        v-model="confirmPassword"
        :rules="confirmRules"
        :type="showPassword ? 'text' : 'password'"
        autocomplete="new-password"
        class="q-mt-md"
        color="primary"
        label="Confirm password"
        lazy-rules
        name="confirm-password"
        outlined />
      <div v-if="setupError" class="auth-error rounded-borders q-mt-md" role="alert">
        <q-icon name="report_problem" size="sm" />
        <span>{{ setupError }}</span>
      </div>
      <q-btn
        :disable="submitting"
        :loading="submitting"
        class="full-width auth-button q-mt-lg"
        color="primary"
        label="Create admin"
        no-caps
        size="lg"
        type="submit" />
    </q-form>
  </auth-page-shell>
</template>

<script lang="ts" setup>
import { Code, ConnectError } from '@connectrpc/connect'
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AuthPageShell from '@/components/shared/AuthPageShell.vue'
import { CompleteSetupRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { fetchSetupStatus } from '@/utils/setup-status'
import { useUserAuthStore } from '@/stores/xylona'

const route = useRoute()
const router = useRouter()
const $q = useQuasar()
const userAuthStore = useUserAuthStore()

const mode = ref<'loading' | 'error' | 'blocked' | 'form'>('loading')
const username = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const showPassword = ref(false)
const submitting = ref(false)
const setupError = ref('')

const confirmRules = [
  (val: string) => !!val || 'Confirm the password',
  (val: string) => val === password.value || 'Passwords do not match',
]

function setupToken(): string {
  const token = route.query['token']
  return typeof token === 'string' ? token : ''
}

async function loadSetupStatus() {
  mode.value = 'loading'
  setupError.value = ''
  try {
    const status = await fetchSetupStatus()
    if (!status.needed) {
      await router.replace('/login')
      return
    }
    mode.value = setupToken().trim() === '' ? 'blocked' : 'form'
  } catch (unknownErr: unknown) {
    setupError.value = ConnectErrorToString(ConnectError.from(unknownErr))
    mode.value = 'error'
  }
}

onMounted(loadSetupStatus)

async function submitSetup() {
  if (submitting.value) {
    return
  }
  setupError.value = ''
  submitting.value = true
  try {
    const response = await GetXylonaClient().completeSetup(
      create(CompleteSetupRequestSchema, {
        userName: username.value,
        email: email.value,
        password: password.value,
        token: setupToken(),
      }),
    )
    if (response.user === undefined) {
      notifySetupError('Setup did not return a user')
      return
    }
    userAuthStore.setUser(response.user)
    await router.push({ path: '/game-servers' })
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    let caption = ConnectErrorToString(err)
    if (err.code === Code.PermissionDenied) {
      caption = 'Setup requires a valid token. Copy the URL from the host log.'
    }
    notifySetupError(caption)
  } finally {
    submitting.value = false
  }
}

function notifySetupError(message: string) {
  setupError.value = message
  $q.notify({
    type: 'xylona-error',
    position: 'top',
    caption: message,
    icon: 'report_problem',
  })
}
</script>

<style scoped>
.setup-command {
  font-family: var(--xy-font-mono);
  color: var(--xy-text-primary);
}
</style>
