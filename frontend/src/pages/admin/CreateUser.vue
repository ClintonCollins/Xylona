<template>
      <q-page class="flex flex-center row">
        <q-card class="q-pa-xl col col-sm-10 col-md-5 col-lg-4 col-xl-3" id="login-card">
          <q-card-section class="text-center">
            <div class="text-h5 text-weight-bold">Xylona Create User</div>
          </q-card-section>
          <q-card-section>
            <q-input outlined v-model="user.email" class="rounded-input" label="Email Address" color="blue"></q-input>
            <q-input outlined class="q-mt-md rounded-input" v-model="user.userName" type="text" color="blue" label="Username"></q-input>
            <q-input outlined class="q-mt-md rounded-input" v-model="user.password" type="password" color="blue" label="Password"></q-input>
            <q-input outlined class="q-mt-md rounded-input" v-model="user.firstName" type="text" color="blue" label="First Name"></q-input>
            <q-input outlined class="q-mt-md rounded-input" v-model="user.lastName" type="text" color="blue" label="Last Name"></q-input>
            <q-toggle v-model="user.superUser" label="Super User" color="blue"></q-toggle>
          </q-card-section>
          <q-card-section align="right">
            <q-btn color="primary" size="md" label="Create user" @click="createUser" no-caps></q-btn>
          </q-card-section>
        </q-card>
      </q-page>
</template>

<script setup lang="ts">
import {CreateUserRequest} from "src/proto/xylona_pb";
import {Ref} from "vue";
import {ref} from "vue";
import {GetXylonaClient} from "src/utils/shared";

const user: Ref<CreateUserRequest> = ref({}) as Ref<CreateUserRequest>
user.value.superUser = false

async function createUser() {
  try {
    await GetXylonaClient().createUser(user.value)
  } catch (e) {
    alert(e)
    console.error(e)
  }
}

</script>

<style scoped>

</style>
