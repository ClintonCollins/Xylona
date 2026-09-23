<template>
  <q-page class="xy-page-content">
    <q-form greedy @submit="submit">
      <page-header title="Create user">
        <template #actions>
          <q-btn :disable="submitting" flat label="Cancel" to="/admin/users" />
          <q-btn :loading="submitting" color="primary" label="Create user" type="submit" />
        </template>
      </page-header>
      <div class="user-form">
        <div class="user-form-grid">
          <q-input
            v-model="userName"
            :rules="[requiredRule('Username')]"
            aria-required="true"
            autocomplete="off"
            autofocus
            label="Username *"
            lazy-rules
            outlined
            type="text" />
          <q-input
            v-model="email"
            :rules="[requiredRule('Email')]"
            aria-required="true"
            autocomplete="off"
            label="Email *"
            lazy-rules
            outlined
            type="email" />
          <q-input
            v-model="password"
            :rules="[requiredRule('Password')]"
            aria-required="true"
            autocomplete="new-password"
            label="Password *"
            lazy-rules
            outlined
            type="password" />
          <q-input
            v-model="confirmPassword"
            :rules="[matchesPasswordRule(() => password)]"
            aria-required="true"
            autocomplete="new-password"
            label="Confirm Password *"
            lazy-rules
            outlined
            type="password" />
          <q-input v-model="firstName" autocomplete="off" label="First Name" outlined type="text" />
          <q-input v-model="lastName" autocomplete="off" label="Last Name" outlined type="text" />
        </div>

        <q-toggle v-model="superUser" aria-describedby="super-user-hint" label="Super User" />
        <p id="super-user-hint" class="user-form-hint">
          Super users control every server, node, user and setting. Other users see only servers you
          grant them from each server's Access tab.
        </p>
      </div>
    </q-form>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { GetXylonaClient } from '@/utils/shared'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import { CreateUserRequestSchema } from '@/proto/xylona_pb'
import PageHeader from '@/components/shared/PageHeader.vue'
import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'
import { matchesPasswordRule, requiredRule } from './user-form-rules'

const router = useRouter()

const userName = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const firstName = ref('')
const lastName = ref('')
const superUser = ref(false)
const submitting = ref(false)
const created = ref(false)

useUnsavedChangesGuard(
  () =>
    !created.value &&
    (superUser.value ||
      [userName, email, password, confirmPassword, firstName, lastName].some(
        (field) => field.value !== '',
      )),
)

async function submit() {
  if (submitting.value) {
    return
  }

  const request = create(CreateUserRequestSchema, {
    userName: userName.value.trim(),
    email: email.value.trim(),
    password: password.value,
    firstName: firstName.value.trim(),
    lastName: lastName.value.trim(),
    superUser: superUser.value,
  })

  submitting.value = true
  try {
    await GetXylonaClient().createUser(request)
    created.value = true
    notifySuccess(`${request.userName} created successfully`, { timeout: 5000 })
    await router.push({ path: '/admin/users' })
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError, 'Error creating user')
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.user-form {
  max-width: 720px;
}

.user-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  column-gap: var(--xy-space-md);
  row-gap: var(--xy-space-xs);
}

.user-form-hint {
  max-width: 70ch;
  margin: var(--xy-space-2xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

@media (max-width: 599px) {
  .user-form-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
