<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <q-card class="full-width">
        <q-card-section>
          <div class="row">
            <div class="text-h6">Create User</div>
          </div>
        </q-card-section>
        <q-card-section>
          <q-form>
            <div class="column q-gutter-y-md">
              <div class="row q-col-gutter-md q-gutter-y-md justify-between full-width">
                <q-input
                  v-model="userName"
                  class="col-12 col-xl-6"
                  outlined
                  type="text"
                  label="Username"></q-input>
                <q-input
                  v-model="email"
                  class="col-12 col-xl-6"
                  outlined
                  type="email"
                  label="Email"></q-input>
                <q-input
                  v-model="password"
                  class="col-12 col-xl-6"
                  outlined
                  type="password"
                  label="Password"></q-input>
                <q-input
                  v-model="confirmPassword"
                  class="col-12 col-xl-6"
                  outlined
                  type="password"
                  label="Confirm Password"></q-input>
                <q-input
                  v-model="firstName"
                  class="col-12 col-xl-6"
                  outlined
                  type="text"
                  label="First Name"></q-input>
                <q-input
                  v-model="lastName"
                  class="col-12 col-xl-6"
                  outlined
                  type="text"
                  label="Last Name"></q-input>
              </div>

              <div class="row q-col-gutter-x-sm full-width">
                <q-toggle v-model="superUser" class="col-12 col-xl-2" label="Super User"></q-toggle>
              </div>
            </div>
          </q-form>
        </q-card-section>
        <q-separator></q-separator>
        <q-card-actions class="q-pa-md" align="right">
          <q-btn flat label="Cancel" @click="router.push({ path: '/admin/users' })"></q-btn>
          <q-btn label="Create User" color="primary" :loading="submitting" @click="submit"></q-btn>
        </q-card-actions>
      </q-card>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { GetXylonaClient } from '@/utils/shared'
import { CreateUserRequest, CreateUserRequestSchema } from '@/proto/xylona_pb'

const $q = useQuasar()
const router = useRouter()

const userName = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const firstName = ref('')
const lastName = ref('')
const superUser = ref(false)
const submitting = ref(false)

function showValidationError(message: string) {
  $q.notify({
    caption: message,
    type: 'xylona-error',
    position: 'top',
    timeout: 5000,
  })
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
  if (password.value.trim() === '') {
    showValidationError('Password is required')
    return
  }
  if (password.value !== confirmPassword.value) {
    showValidationError('Passwords do not match')
    return
  }

  const request: CreateUserRequest = create(CreateUserRequestSchema, {
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
    $q.notify({
      caption: `${request.userName} created successfully`,
      type: 'xylona-success',
      position: 'top',
      timeout: 5000,
    })
    await router.push({ path: '/admin/users' })
  } catch (unknownError: unknown) {
    const err = unknownError as Error
    $q.notify({
      caption: `Error creating user ${err.message}`,
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
