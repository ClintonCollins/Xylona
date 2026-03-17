<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <q-card class="full-width">
        <q-card-section>
          <div class="row">
            <div class="text-h6">Edit User</div>
          </div>
        </q-card-section>
        <q-card-section>
          <q-form>
            <div class="column q-gutter-y-md">
              <div class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
                <q-input
                  class="col-12 col-xl-6"
                  outlined
                  type="text"
                  label="Username"
                  v-model="userName"></q-input>
                <q-input
                  class="col-12 col-xl-6"
                  outlined
                  type="email"
                  label="Email"
                  v-model="email"></q-input>
                <q-input
                  class="col-12 col-xl-6"
                  outlined
                  type="text"
                  label="First Name"
                  v-model="firstName"></q-input>
                <q-input
                  class="col-12 col-xl-6"
                  outlined
                  type="text"
                  label="Last Name"
                  v-model="lastName"></q-input>
              </div>

              <div class="row q-col-gutter-x-sm full-width">
                <q-toggle class="col-12 col-xl-2" v-model="superUser" label="Super User"></q-toggle>
              </div>

              <q-separator></q-separator>

              <div class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
                <q-input
                  class="col-12 col-xl-6"
                  outlined
                  type="password"
                  label="New Password (Optional)"
                  v-model="password"></q-input>
                <q-input
                  class="col-12 col-xl-6"
                  outlined
                  type="password"
                  label="Confirm Password"
                  v-model="confirmPassword"></q-input>
              </div>
            </div>
          </q-form>
        </q-card-section>
        <q-separator></q-separator>
        <q-card-actions class="q-pa-md" align="right">
          <q-btn flat label="Cancel" @click="router.push({ path: '/admin/users' })"></q-btn>
          <q-btn label="Save" color="primary" :loading="submitting" @click="submit"></q-btn>
        </q-card-actions>
      </q-card>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { GetXylonaClient } from '@/utils/shared'
import {
  GetUserDetailsRequest,
  GetUserDetailsRequestSchema,
  UpdateUserRequest,
  UpdateUserRequestSchema,
  User,
} from '@/proto/xylona_pb'

const $q = useQuasar()
const route = useRoute()
const router = useRouter()

const userID = ref('')
const userName = ref('')
const email = ref('')
const firstName = ref('')
const lastName = ref('')
const superUser = ref(false)
const password = ref('')
const confirmPassword = ref('')
const submitting = ref(false)

function showValidationError(message: string) {
  $q.notify({
    caption: message,
    type: 'xylona-error',
    position: 'top',
    timeout: 5000,
  })
}

onMounted(async () => {
  if (typeof route.params.id !== 'string' || route.params.id.trim() === '') {
    showValidationError('User ID is required')
    await router.push({ path: '/admin/users' })
    return
  }

  userID.value = route.params.id
  await getUser()
})

async function getUser() {
  const request: GetUserDetailsRequest = create(GetUserDetailsRequestSchema, {
    id: userID.value,
  })
  try {
    const response = await GetXylonaClient().getUser(request)
    if (!response.user) {
      showValidationError('User not found')
      await router.push({ path: '/admin/users' })
      return
    }
    hydrateForm(response.user)
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    $q.notify({
      caption: `Error loading user ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
    await router.push({ path: '/admin/users' })
  }
}

function hydrateForm(user: User) {
  userName.value = user.userName
  email.value = user.email
  firstName.value = user.firstName
  lastName.value = user.lastName
  superUser.value = user.superUser
}

async function submit() {
  if (submitting.value) {
    return
  }

  if (userName.value.trim() === '') {
    showValidationError('Username is required')
    return
  }
  if (email.value.trim() === '') {
    showValidationError('Email is required')
    return
  }
  if (
    (password.value !== '' || confirmPassword.value !== '') &&
    password.value !== confirmPassword.value
  ) {
    showValidationError('Passwords do not match')
    return
  }

  const request: UpdateUserRequest = create(UpdateUserRequestSchema, {
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

  submitting.value = true
  try {
    await GetXylonaClient().updateUser(request)
    $q.notify({
      caption: `${request.userName} updated successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    await router.push({ path: '/admin/users' })
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    $q.notify({
      caption: `Error updating user ${err.message}`,
      type: 'xylona-error',
      position: 'top',
      timeout: 5000,
    })
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped></style>
