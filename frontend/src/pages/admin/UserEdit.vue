<template>
  <q-page class="xy-page-content">
    <q-form greedy @submit="submit">
      <page-header title="Edit user">
        <template #actions>
          <q-btn :disable="submitting" flat label="Cancel" to="/admin/users" />
          <q-btn
            :disable="!loaded"
            :loading="submitting"
            color="primary"
            label="Save changes"
            type="submit" />
        </template>
      </page-header>
      <div class="user-form">
        <div class="user-form-grid">
          <q-input
            v-model="userName"
            :rules="[requiredRule('Username')]"
            aria-required="true"
            autocomplete="off"
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
          <q-input v-model="firstName" autocomplete="off" label="First Name" outlined type="text" />
          <q-input v-model="lastName" autocomplete="off" label="Last Name" outlined type="text" />
        </div>

        <q-toggle v-model="superUser" aria-describedby="super-user-hint" label="Super User" />
        <p id="super-user-hint" class="user-form-hint">
          Super users control every server, node, user and setting. Other users see only servers you
          grant them from each server's Access tab.
          <template v-if="loadedSuperUser">
            Turning this off signs {{ editingSelf ? 'you' : 'this user' }} out of all sessions.
          </template>
        </p>

        <q-separator class="q-my-lg" />

        <div class="user-form-grid">
          <q-input
            v-model="password"
            autocomplete="new-password"
            :hint="
              editingSelf
                ? 'Saving a new password signs you out of all sessions, including this one.'
                : 'Saving a new password signs this user out of all sessions.'
            "
            label="New Password (Optional)"
            outlined
            type="password" />
          <q-input
            v-model="confirmPassword"
            :rules="[matchesPasswordRule(() => password)]"
            autocomplete="new-password"
            label="Confirm Password"
            lazy-rules
            outlined
            type="password" />
        </div>
      </div>
    </q-form>
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { GetXylonaClient } from '@/utils/shared'
import { notifyConnectError, notifyError, notifySuccess } from '@/api/notifications'
import { GetUserDetailsRequestSchema, UpdateUserRequestSchema, User } from '@/proto/xylona_pb'
import PageHeader from '@/components/shared/PageHeader.vue'
import { useUserAuthStore } from '@/stores/xylona'
import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'
import { matchesPasswordRule, requiredRule } from './user-form-rules'

const route = useRoute()
const router = useRouter()
const authStore = useUserAuthStore()

const userID = ref('')
const userName = ref('')
const email = ref('')
const firstName = ref('')
const lastName = ref('')
const superUser = ref(false)
const password = ref('')
const confirmPassword = ref('')
const submitting = ref(false)
const loaded = ref(false)
const loadedSuperUser = ref(false)
const savedSnapshot = ref('')

const editingSelf = computed(() => userID.value !== '' && userID.value === authStore.user?.id)

function formSnapshot(): string {
  return JSON.stringify([
    userName.value,
    email.value,
    firstName.value,
    lastName.value,
    superUser.value,
  ])
}

useUnsavedChangesGuard(
  () =>
    loaded.value &&
    (formSnapshot() !== savedSnapshot.value ||
      password.value !== '' ||
      confirmPassword.value !== ''),
)

onMounted(async () => {
  if (typeof route.params.id !== 'string' || route.params.id.trim() === '') {
    notifyError('User ID is required')
    await router.push({ path: '/admin/users' })
    return
  }

  userID.value = route.params.id
  await getUser()
})

async function getUser() {
  try {
    const response = await GetXylonaClient().getUser(
      create(GetUserDetailsRequestSchema, { id: userID.value }),
    )
    if (!response.user) {
      notifyError('User not found')
      await router.push({ path: '/admin/users' })
      return
    }
    hydrateForm(response.user)
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError, 'Error loading user')
    await router.push({ path: '/admin/users' })
  }
}

function hydrateForm(user: User) {
  userName.value = user.userName
  email.value = user.email
  firstName.value = user.firstName
  lastName.value = user.lastName
  superUser.value = user.superUser
  loadedSuperUser.value = user.superUser
  savedSnapshot.value = formSnapshot()
  loaded.value = true
}

async function submit() {
  if (submitting.value || !loaded.value) {
    return
  }

  const request = create(UpdateUserRequestSchema, {
    id: userID.value,
    userName: userName.value.trim(),
    email: email.value.trim(),
    firstName: firstName.value.trim(),
    lastName: lastName.value.trim(),
    superUser: superUser.value,
  })
  if (password.value.trim() !== '') {
    request.password = password.value
  }
  // The server revokes every session of a user whose password changes or who
  // stops being a super user.
  const signsOutSelf =
    editingSelf.value && (request.password !== '' || (loadedSuperUser.value && !superUser.value))

  submitting.value = true
  try {
    await GetXylonaClient().updateUser(request)
    savedSnapshot.value = formSnapshot()
    password.value = ''
    confirmPassword.value = ''
    if (signsOutSelf) {
      await authStore.logout()
      await router.push({ path: '/login', query: { reason: 'account-changed' } })
      return
    }
    notifySuccess(`${request.userName} updated successfully`, { timeout: 5000 })
    await router.push({ path: '/admin/users' })
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError, 'Error updating user')
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
