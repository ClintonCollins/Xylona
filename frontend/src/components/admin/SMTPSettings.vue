<template>
  <div class="smtp-settings">
    <div class="xy-section-header q-mb-md">
      <div class="text-h6">SMTP Settings</div>
      <div class="text-caption text-xy-secondary">
        Configure system-wide SMTP settings for email notifications. These settings are used by all
        notification features that send email.
      </div>
    </div>

    <div v-if="!loading" class="q-mb-md">
      <q-badge :color="configured ? 'positive' : 'warning'" :label="statusLabel" />
    </div>

    <q-skeleton v-if="loading" type="rect" height="200px" />

    <template v-else>
      <div class="row q-col-gutter-md">
        <div class="col-12 col-md-6">
          <q-input
            v-model="host"
            outlined
            dense
            label="SMTP Host"
            placeholder="smtp.example.com"
            aria-label="SMTP Host" />
        </div>
        <div class="col-12 col-md-6">
          <q-input
            v-model.number="port"
            outlined
            dense
            label="Port"
            type="number"
            placeholder="587"
            :rules="[portRule]"
            aria-label="SMTP Port" />
        </div>
        <div class="col-12 col-md-6">
          <q-input
            v-model="user"
            outlined
            dense
            label="Username"
            autocomplete="off"
            aria-label="SMTP Username" />
        </div>
        <div class="col-12 col-md-6">
          <q-input
            v-model="password"
            outlined
            dense
            label="Password"
            type="password"
            autocomplete="new-password"
            :hint="
              hasExistingPassword
                ? 'Password is configured. Leave blank to keep current password.'
                : undefined
            "
            aria-label="SMTP Password" />
        </div>
        <div class="col-12 col-md-6">
          <q-input
            v-model="fromAddress"
            outlined
            dense
            label="From Address"
            type="email"
            placeholder="noreply@example.com"
            aria-label="From email address" />
        </div>
        <div class="col-12 col-md-6 self-center">
          <q-toggle v-model="tlsEnabled" label="TLS Enabled" aria-label="Enable TLS" />
        </div>
      </div>

      <div class="q-mt-md q-gutter-sm">
        <q-btn color="primary" label="Save" :loading="saving" @click="save" />
        <q-btn outline label="Send Test Email" :loading="testing" @click="testSMTP" />
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Notify, useQuasar } from 'quasar'
import { computed, onMounted, ref } from 'vue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { SystemSMTPConfigSchema } from '@/proto/shared_pb'
import {
  GetSystemSMTPConfigRequestSchema,
  SetSystemSMTPConfigRequestSchema,
  TestSystemSMTPRequestSchema,
} from '@/proto/xylona_pb'

const $q = useQuasar()

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const configured = ref(false)

const host = ref('')
const port = ref(587)
const user = ref('')
const password = ref('')
const fromAddress = ref('')
const tlsEnabled = ref(true)
const hasExistingPassword = ref(false)

const statusLabel = computed(() => (configured.value ? 'Configured' : 'Not Configured'))

function portRule(val: number | string | null | undefined): boolean | string {
  if (val === null || val === undefined || val === '') {
    return true
  }
  const num = Number(val)
  if (!Number.isInteger(num) || num < 1 || num > 65535) {
    return 'Port must be between 1 and 65535'
  }
  return true
}

onMounted(async () => {
  await loadConfig()
})

async function loadConfig(): Promise<void> {
  loading.value = true
  try {
    const response = await GetXylonaClient().getSystemSMTPConfig(
      create(GetSystemSMTPConfigRequestSchema, {}),
    )
    configured.value = response.configured
    if (response.config) {
      host.value = response.config.host
      port.value = response.config.port || 587
      user.value = response.config.user
      hasExistingPassword.value = response.passwordConfigured
      password.value = ''
      fromAddress.value = response.config.fromAddress
      tlsEnabled.value = response.config.tlsEnabled
    }
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  saving.value = true
  try {
    await GetXylonaClient().setSystemSMTPConfig(
      create(SetSystemSMTPConfigRequestSchema, {
        config: create(SystemSMTPConfigSchema, {
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
    Notify.create({
      type: 'xylona-success',
      position: 'top',
      caption: 'SMTP settings saved',
      timeout: 5000,
    })
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    Notify.create({
      type: 'xylona-error',
      position: 'top',
      caption: ConnectErrorToString(err),
      timeout: 0,
      closeBtn: 'Dismiss',
      icon: 'report_problem',
    })
  } finally {
    saving.value = false
  }
}

function testSMTP(): void {
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
      const err = ConnectError.from(unknownError)
      Notify.create({
        type: 'xylona-error',
        position: 'top',
        caption: ConnectErrorToString(err),
        timeout: 0,
        closeBtn: 'Dismiss',
        icon: 'report_problem',
      })
    } finally {
      testing.value = false
    }
  })
}
</script>

<style scoped></style>
