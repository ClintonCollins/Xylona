<template>
  <q-dialog v-model="showDialog" aria-labelledby="change-password-title" @hide="resetForm">
    <q-card class="change-password-card">
      <q-form greedy @submit.prevent="changePassword">
        <q-card-section>
          <h2 id="change-password-title" class="text-h6 q-my-none">Change password</h2>
          <p class="change-password-help q-mb-none">
            You stay signed in here. Other devices signed in to this account are signed out.
          </p>
        </q-card-section>
        <q-card-section class="change-password-fields q-pt-none">
          <q-input
            v-model="currentPassword"
            :error="currentPasswordError !== ''"
            :error-message="currentPasswordError"
            :rules="[(val: string) => val !== '' || 'Enter your current password.']"
            aria-required="true"
            autocomplete="current-password"
            autofocus
            label="Current password"
            lazy-rules
            name="current-password"
            outlined
            type="password"
            @update:model-value="currentPasswordError = ''" />
          <q-input
            v-model="newPassword"
            :rules="[(val: string) => val.trim() !== '' || 'Enter a new password.']"
            aria-required="true"
            autocomplete="new-password"
            label="New password"
            lazy-rules
            name="new-password"
            outlined
            type="password" />
          <q-input
            v-model="confirmPassword"
            :rules="[(val: string) => val === newPassword || 'Passwords do not match.']"
            aria-required="true"
            autocomplete="new-password"
            label="Confirm new password"
            lazy-rules
            name="confirm-new-password"
            outlined
            type="password" />
          <q-banner v-if="errorMessage" class="xy-banner-negative" dense role="alert">
            <template #avatar>
              <q-icon name="report_problem" />
            </template>
            {{ errorMessage }}
          </q-banner>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn v-close-popup :disable="saving" flat label="Cancel" no-caps />
          <q-btn :loading="saving" color="primary" label="Change password" no-caps type="submit" />
        </q-card-actions>
      </q-form>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { ref } from 'vue'

import { connectErrorMessage } from '@/api/connect-errors'
import { notifySuccess } from '@/api/notifications'
import { ChangePasswordRequestSchema } from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const showDialog = defineModel<boolean>('showDialog', { default: false })

const currentPassword = ref('')
const newPassword = ref('')
const confirmPassword = ref('')
const currentPasswordError = ref('')
const errorMessage = ref('')
const saving = ref(false)

async function changePassword() {
  if (saving.value) return
  saving.value = true
  currentPasswordError.value = ''
  errorMessage.value = ''
  try {
    await GetXylonaClient().changePassword(
      create(ChangePasswordRequestSchema, {
        currentPassword: currentPassword.value,
        newPassword: newPassword.value,
      }),
    )
    showDialog.value = false
    notifySuccess('Password changed. Other devices were signed out.')
  } catch (unknownError: unknown) {
    const err = ConnectError.from(unknownError)
    if (err.code === Code.InvalidArgument && err.rawMessage.includes('current password')) {
      currentPasswordError.value = 'Current password is incorrect.'
    } else {
      errorMessage.value = connectErrorMessage(err, 'Password not changed')
    }
  } finally {
    saving.value = false
  }
}

function resetForm() {
  currentPassword.value = ''
  newPassword.value = ''
  confirmPassword.value = ''
  currentPasswordError.value = ''
  errorMessage.value = ''
}
</script>

<style scoped>
.change-password-card {
  width: min(28rem, 100%);
}

.change-password-help {
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.change-password-fields {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-2xs);
}
</style>
