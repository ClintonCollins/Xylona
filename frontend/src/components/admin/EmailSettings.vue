<template>
  <section aria-labelledby="email-delivery-heading" class="email-settings">
    <div class="xy-section-header q-mb-md">
      <h2 id="email-delivery-heading" class="text-h6 q-my-none">Email Delivery</h2>
      <p class="email-settings-description">
        Send controller notifications through your own SMTP server or a connected Google account.
      </p>
    </div>

    <q-skeleton v-if="loading" height="18rem" type="rect" />

    <template v-else>
      <div class="email-status-strip" role="status">
        <q-icon
          :color="configured ? 'positive' : 'warning'"
          :name="configured ? 'check_circle' : 'warning'"
          size="1.4rem" />
        <div>
          <div class="email-status-title">{{ activeStatusTitle }}</div>
          <div class="email-status-detail">{{ activeStatusDetail }}</div>
        </div>
      </div>

      <q-btn-toggle
        v-model="selectedProvider"
        :options="providerOptions"
        aria-label="Email delivery provider"
        class="provider-toggle q-mt-lg"
        no-caps
        outline
        spread
        toggle-color="primary"
        unelevated />

      <div v-if="selectedProvider === 'smtp'" class="email-provider-panel q-mt-md">
        <div class="provider-heading">
          <div>
            <h3 class="text-subtitle1 q-my-none">Manual SMTP</h3>
            <p>Use credentials from any SMTP provider. Saving this form makes SMTP active.</p>
          </div>
          <q-badge
            :color="manualConfigured ? 'positive' : 'warning'"
            :label="manualConfigured ? 'Ready' : 'Setup required'" />
        </div>

        <div class="row q-col-gutter-md q-mt-xs">
          <div class="col-12 col-md-8">
            <q-input
              v-model="host"
              aria-label="SMTP host"
              dense
              label="SMTP Host"
              outlined
              placeholder="smtp.example.com" />
          </div>
          <div class="col-12 col-md-4">
            <q-input
              v-model.number="port"
              :rules="[portRule]"
              aria-label="SMTP port"
              dense
              label="Port"
              outlined
              placeholder="587"
              type="number" />
          </div>
          <div class="col-12 col-md-6">
            <q-input
              v-model="user"
              aria-label="SMTP username"
              autocomplete="off"
              dense
              label="Username"
              outlined />
          </div>
          <div class="col-12 col-md-6">
            <q-input
              v-model="password"
              :hint="
                hasExistingPassword
                  ? 'A password is stored. Leave blank to keep it.'
                  : 'Stored encrypted by the controller.'
              "
              aria-label="SMTP password"
              autocomplete="new-password"
              dense
              label="Password"
              outlined
              type="password" />
          </div>
          <div class="col-12 col-md-8">
            <q-input
              v-model="fromAddress"
              aria-label="From email address"
              dense
              label="From Address"
              outlined
              placeholder="noreply@example.com"
              type="email" />
          </div>
          <div class="col-12 col-md-4 self-center">
            <q-toggle v-model="tlsEnabled" aria-label="Enable TLS" label="TLS Enabled" />
          </div>
        </div>

        <div class="provider-actions q-mt-lg">
          <q-btn
            :disable="!manualConfigured"
            :loading="saving"
            color="primary"
            icon="save"
            label="Save and Use SMTP"
            no-caps
            @click="saveManualSMTP" />
          <q-btn
            :disable="activeProvider !== SystemEmailProvider.SMTP"
            :loading="testing"
            label="Send Test Email"
            no-caps
            outline
            @click="testEmail" />
        </div>
      </div>

      <div v-else class="email-provider-panel q-mt-md">
        <div class="provider-heading">
          <div>
            <h3 class="text-subtitle1 q-my-none">Google account</h3>
            <p>
              Xylona requests send-only Gmail access plus basic account identity. Your Google
              password is never shared with the controller.
            </p>
          </div>
          <q-badge
            :color="googleConnected ? 'positive' : 'warning'"
            :label="googleConnected ? 'Connected' : 'Not connected'" />
        </div>

        <div v-if="googleConnected" class="google-account-row q-mt-md">
          <q-icon aria-hidden="true" color="positive" name="mark_email_read" size="1.5rem" />
          <div>
            <div class="google-account-label">Connected account</div>
            <div class="google-account-email">{{ googleEmail }}</div>
          </div>
        </div>

        <div class="google-setup q-mt-lg">
          <h4 class="text-subtitle2 q-my-none">Google Cloud setup</h4>
          <ol>
            <li>
              Create or select a Google Cloud project and
              <a
                href="https://console.cloud.google.com/apis/library/gmail.googleapis.com"
                rel="noopener noreferrer"
                target="_blank"
                >enable the Gmail API</a
              >.
            </li>
            <li>
              Configure the OAuth consent screen. Add the account as a test user while testing; for
              durable unattended delivery, publish the app because Testing refresh tokens expire
              after seven days. External apps may also need Google OAuth verification before
              production use.
            </li>
            <li>
              Create an OAuth client with application type <strong>Web application</strong> and add
              the redirect URI below exactly.
            </li>
          </ol>

          <q-input
            :model-value="googleRedirectURI"
            aria-label="Google OAuth redirect URI"
            class="redirect-uri-field"
            dense
            label="Authorized Redirect URI"
            outlined
            readonly>
            <template #append>
              <q-btn
                aria-label="Copy Google OAuth redirect URI"
                dense
                flat
                icon="content_copy"
                round
                @click="copyRedirectURI" />
            </template>
          </q-input>
          <div v-if="!googleRedirectSecure" class="redirect-warning q-mt-sm" role="alert">
            <q-icon aria-hidden="true" name="warning" />
            Google requires HTTPS redirect URIs except on localhost.
          </div>
        </div>

        <div class="row q-col-gutter-md q-mt-sm">
          <div class="col-12">
            <q-input
              v-model="googleClientID"
              aria-label="Google OAuth client ID"
              autocomplete="off"
              dense
              label="OAuth Client ID"
              outlined
              placeholder="000000000000-example.apps.googleusercontent.com" />
          </div>
          <div class="col-12">
            <q-input
              v-model="googleClientSecret"
              :hint="
                googleClientSecretConfigured
                  ? 'A client secret is stored. Leave blank to keep it.'
                  : 'Stored encrypted after Google authorization succeeds.'
              "
              aria-label="Google OAuth client secret"
              autocomplete="new-password"
              dense
              label="OAuth Client Secret"
              outlined
              type="password" />
          </div>
        </div>

        <div class="provider-actions q-mt-lg">
          <q-btn
            :disable="!googleRedirectSecure || !googleCredentialsReady"
            :loading="connectingGoogle"
            color="primary"
            icon="login"
            :label="googleConnected ? 'Reconnect Google' : 'Connect Google'"
            no-caps
            @click="beginGoogleConnection" />
          <q-btn
            v-if="googleConnected"
            :disable="activeProvider !== SystemEmailProvider.GOOGLE"
            :loading="testing"
            label="Send Test Email"
            no-caps
            outline
            @click="testEmail" />
          <q-btn
            v-if="googleConnected"
            :loading="disconnectingGoogle"
            color="negative"
            flat
            label="Disconnect"
            no-caps
            @click="confirmGoogleDisconnect" />
        </div>
      </div>
    </template>
  </section>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Notify, copyToClipboard, useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { SystemEmailProvider, SystemSMTPConfigSchema } from '@/proto/shared_pb'
import {
  BeginGoogleMailOAuthRequestSchema,
  DisconnectGoogleMailRequestSchema,
  GetSystemSMTPConfigRequestSchema,
  SetSystemSMTPConfigRequestSchema,
  TestSystemSMTPRequestSchema,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const googleCallbackPath = '/api/oauth/google/mail/callback'
const $q = useQuasar()
const route = useRoute()
const router = useRouter()

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const connectingGoogle = ref(false)
const disconnectingGoogle = ref(false)
const configured = ref(false)
const activeProvider = ref(SystemEmailProvider.UNSPECIFIED)
const selectedProvider = ref<'smtp' | 'google'>('smtp')

const host = ref('')
const port = ref(587)
const user = ref('')
const password = ref('')
const fromAddress = ref('')
const tlsEnabled = ref(true)
const hasExistingPassword = ref(false)

const googleClientID = ref('')
const storedGoogleClientID = ref('')
const googleClientSecret = ref('')
const googleClientSecretConfigured = ref(false)
const googleConnected = ref(false)
const googleEmail = ref('')

const providerOptions = [
  { label: 'Manual SMTP', value: 'smtp', icon: 'dns' },
  { label: 'Google Account', value: 'google', icon: 'alternate_email' },
]

const manualConfigured = computed(
  () =>
    host.value.trim() !== '' &&
    Number.isInteger(Number(port.value)) &&
    Number(port.value) >= 1 &&
    Number(port.value) <= 65535 &&
    user.value.trim() !== '' &&
    (hasExistingPassword.value || password.value.trim() !== '') &&
    fromAddress.value.trim() !== '',
)

const googleRedirectURI = computed(() => {
  if (typeof window === 'undefined') return ''
  return `${window.location.origin}${googleCallbackPath}`
})

const googleRedirectSecure = computed(() => {
  if (typeof window === 'undefined') return false
  if (window.location.protocol === 'https:') return true
  return ['localhost', '127.0.0.1', '::1'].includes(window.location.hostname)
})

const googleCredentialsReady = computed(
  () =>
    googleClientID.value.trim() !== '' &&
    (googleClientSecret.value.trim() !== '' ||
      (googleClientSecretConfigured.value &&
        googleClientID.value.trim() === storedGoogleClientID.value)),
)

const activeStatusTitle = computed(() => {
  if (!configured.value) return 'Email delivery is not configured'
  if (activeProvider.value === SystemEmailProvider.GOOGLE) return 'Google delivery is active'
  return 'Manual SMTP is active'
})

const activeStatusDetail = computed(() => {
  if (!configured.value) return 'Choose a provider below to enable email notifications.'
  if (activeProvider.value === SystemEmailProvider.GOOGLE) {
    return googleEmail.value
      ? `Sending as ${googleEmail.value}`
      : 'Connected through the Gmail API.'
  }
  return host.value ? `Sending through ${host.value}:${port.value}` : 'SMTP credentials are ready.'
})

function portRule(val: number | string | null | undefined): boolean | string {
  if (val === null || val === undefined || val === '') return true
  const numberValue = Number(val)
  if (!Number.isInteger(numberValue) || numberValue < 1 || numberValue > 65535) {
    return 'Port must be between 1 and 65535'
  }
  return true
}

onMounted(async () => {
  await handleGoogleRedirectResult()
  await loadConfig()
})

async function handleGoogleRedirectResult(): Promise<void> {
  const queryValue = route.query['google']
  const result = Array.isArray(queryValue) ? queryValue[0] : queryValue
  if (!result) return

  const nextQuery = { ...route.query }
  delete nextQuery['google']
  await router.replace({ query: nextQuery })

  if (result === 'connected') {
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: 'Google account connected for email delivery',
      timeout: 5000,
    })
    return
  }

  const captions: Record<string, string> = {
    denied: 'Google authorization was cancelled',
    invalid_state: 'Google authorization expired or was already used. Please try again.',
  }
  Notify.create({
    type: result === 'denied' ? 'xylona-warning' : 'xylona-error',
    position: 'top',
    caption: captions[result] || 'Google authorization could not be completed',
    timeout: 0,
    closeBtn: 'Dismiss',
    icon: result === 'denied' ? 'info' : 'report_problem',
  })
}

async function loadConfig(): Promise<void> {
  loading.value = true
  try {
    const response = await GetXylonaClient().getSystemSMTPConfig(
      create(GetSystemSMTPConfigRequestSchema, {}),
    )
    configured.value = response.configured
    hasExistingPassword.value = response.passwordConfigured
    googleClientSecretConfigured.value = response.googleClientSecretConfigured
    googleConnected.value = response.googleConnected

    const config = response.config
    if (!config) return

    activeProvider.value = config.provider
    selectedProvider.value = config.provider === SystemEmailProvider.GOOGLE ? 'google' : 'smtp'
    host.value = config.host
    port.value = config.port || 587
    user.value = config.user
    password.value = ''
    fromAddress.value = config.fromAddress
    tlsEnabled.value = config.tlsEnabled
    googleClientID.value = config.googleClientId
    storedGoogleClientID.value = config.googleClientId
    googleClientSecret.value = ''
    googleEmail.value = config.googleEmail
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError)
  } finally {
    loading.value = false
  }
}

async function saveManualSMTP(): Promise<void> {
  saving.value = true
  try {
    await GetXylonaClient().setSystemSMTPConfig(
      create(SetSystemSMTPConfigRequestSchema, {
        config: create(SystemSMTPConfigSchema, {
          provider: SystemEmailProvider.SMTP,
          host: host.value,
          port: port.value,
          user: user.value,
          password: password.value,
          fromAddress: fromAddress.value,
          tlsEnabled: tlsEnabled.value,
        }),
      }),
    )
    configured.value = true
    activeProvider.value = SystemEmailProvider.SMTP
    hasExistingPassword.value = true
    password.value = ''
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: 'Manual SMTP saved and activated',
      timeout: 5000,
    })
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError)
  } finally {
    saving.value = false
  }
}

async function beginGoogleConnection(): Promise<void> {
  connectingGoogle.value = true
  try {
    const response = await GetXylonaClient().beginGoogleMailOAuth(
      create(BeginGoogleMailOAuthRequestSchema, {
        clientId: googleClientID.value,
        clientSecret: googleClientSecret.value,
        redirectUri: googleRedirectURI.value,
      }),
    )
    window.location.assign(response.authorizationUrl)
  } catch (unknownError: unknown) {
    connectingGoogle.value = false
    notifyConnectError(unknownError)
  }
}

function confirmGoogleDisconnect(): void {
  $q.dialog({
    title: 'Disconnect Google account?',
    message:
      'Xylona will revoke its Google access and remove the stored refresh token. Email delivery may switch back to saved SMTP settings.',
    cancel: true,
    persistent: false,
    ok: {
      label: 'Disconnect',
      color: 'negative',
      noCaps: true,
    },
  }).onOk(disconnectGoogle)
}

async function disconnectGoogle(): Promise<void> {
  disconnectingGoogle.value = true
  try {
    await GetXylonaClient().disconnectGoogleMail(create(DisconnectGoogleMailRequestSchema, {}))
    await loadConfig()
    selectedProvider.value = 'google'
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: 'Google account disconnected',
      timeout: 5000,
    })
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError)
  } finally {
    disconnectingGoogle.value = false
  }
}

function testEmail(): void {
  $q.dialog({
    title: 'Send Test Email',
    message: 'Enter the email address to send a test message to:',
    prompt: {
      model: '',
      type: 'email',
      outlined: true,
    },
    cancel: true,
    persistent: false,
  }).onOk(async (email: string) => {
    testing.value = true
    try {
      const response = await GetXylonaClient().testSystemSMTP(
        create(TestSystemSMTPRequestSchema, {
          toAddress: email,
        }),
      )
      if (response.success) {
        Notify.create({
          type: 'xylona-success',
          position: 'top',
          caption: 'Test email sent successfully',
          timeout: 5000,
        })
      } else {
        Notify.create({
          type: 'xylona-error',
          position: 'top',
          caption: response.error || 'Unknown error',
          timeout: 0,
          closeBtn: 'Dismiss',
          icon: 'report_problem',
        })
      }
    } catch (unknownError: unknown) {
      notifyConnectError(unknownError)
    } finally {
      testing.value = false
    }
  })
}

async function copyRedirectURI(): Promise<void> {
  try {
    await copyToClipboard(googleRedirectURI.value)
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: 'Redirect URI copied',
      timeout: 3000,
    })
  } catch {
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: 'Could not copy the redirect URI',
      timeout: 5000,
    })
  }
}

function notifyConnectError(unknownError: unknown): void {
  const error = ConnectError.from(unknownError)
  Notify.create({
    type: 'xylona-error',
    position: 'top',
    caption: ConnectErrorToString(error),
    timeout: 0,
    closeBtn: 'Dismiss',
    icon: 'report_problem',
  })
}
</script>

<style scoped>
.email-settings {
  max-width: 68rem;
}

.email-settings-description,
.provider-heading p {
  max-width: 70ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
}

.email-status-strip,
.google-account-row,
.redirect-warning {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.email-status-strip {
  padding: var(--xy-space-base) var(--xy-space-md);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.email-status-title {
  color: var(--xy-text-primary);
  font-weight: 600;
}

.email-status-detail,
.google-account-label {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.provider-toggle {
  width: min(100%, 34rem);
}

.email-provider-panel {
  padding: var(--xy-space-lg);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.provider-heading {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.provider-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
}

.google-account-row {
  padding: var(--xy-space-base) 0;
  border-top: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
}

.google-account-email {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
}

.google-setup {
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.google-setup ol {
  margin: var(--xy-space-sm) 0 var(--xy-space-md);
  padding-left: var(--xy-space-lg);
  color: var(--xy-text-secondary);
}

.google-setup li + li {
  margin-top: var(--xy-space-sm);
}

.google-setup a {
  color: var(--xy-accent);
}

.redirect-uri-field {
  font-family: var(--xy-font-mono);
}

.redirect-warning {
  color: var(--xy-warning);
  font-size: var(--xy-font-size-sm);
}

@media (max-width: 599px) {
  .email-provider-panel {
    padding: var(--xy-space-md);
  }

  .provider-heading {
    flex-direction: column;
  }

  .provider-actions :deep(.q-btn) {
    width: 100%;
  }
}
</style>
