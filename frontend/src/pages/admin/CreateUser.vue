<template>
  <q-page class="flex flex-center row">
    <q-card id="login-card" class="q-pa-xl col col-sm-10 col-md-5 col-lg-4 col-xl-3">
      <q-card-section class="text-center">
        <div class="text-h5 text-weight-bold">Xylona Create User</div>
      </q-card-section>
      <q-card-section>
        <q-input
          v-model="user.email"
          outlined
          class="rounded-input"
          label="Email Address"
          color="primary"></q-input>
        <q-input
          v-model="user.userName"
          outlined
          class="q-mt-md rounded-input"
          type="text"
          color="primary"
          label="Username"></q-input>
        <q-input
          v-model="user.password"
          outlined
          class="q-mt-md rounded-input"
          type="password"
          color="primary"
          label="Password"></q-input>
        <q-input
          v-model="user.firstName"
          outlined
          class="q-mt-md rounded-input"
          type="text"
          color="primary"
          label="First Name"></q-input>
        <q-input
          v-model="user.lastName"
          outlined
          class="q-mt-md rounded-input"
          type="text"
          color="primary"
          label="Last Name"></q-input>
        <q-toggle v-model="user.superUser" label="Super User" color="primary"></q-toggle>
      </q-card-section>
      <q-card-section align="right">
        <q-btn color="primary" size="md" label="Create user" no-caps @click="createUser"></q-btn>
      </q-card-section>
    </q-card>
  </q-page>
</template>

<script setup lang="ts">
import { CreateUserRequest } from '@/proto/xylona_pb'
import { Ref } from 'vue'
import { ref } from 'vue'
import { GetXylonaClient } from '@/utils/shared'

const user: Ref<CreateUserRequest> = ref({}) as Ref<CreateUserRequest>
user.value.superUser = false

async function createUser() {
  try {
    await GetXylonaClient().createUser(user.value)
  } catch (e) {
    console.error(e)
  }
}
</script>

<style scoped></style>
