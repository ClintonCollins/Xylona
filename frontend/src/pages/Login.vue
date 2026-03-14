<template>
    <q-layout view="lHh Lpr fff">
        <q-page-container>
            <q-page class="flex flex-center row">
                <q-card class="q-pa-xl col-12 col-sm-10 col-md-6 col-xl-3" id="login-card">
                    <q-card-section class="text-center">
                        <div class="text-h5 text-weight-bold">Xylona Login</div>
                    </q-card-section>
                    <q-card-section>
                        <q-input filled v-model="username" label="Username" color="blue"></q-input>
                        <q-input filled class="q-mt-md" v-model="password" type="password" color="blue"
                                 label="Password"></q-input>
                    </q-card-section>
                    <q-card-section align="right">
                        <q-btn color="primary" size="md" label="Sign in" @click="login" no-caps></q-btn>
                    </q-card-section>
                </q-card>
            </q-page>
        </q-page-container>
    </q-layout>
</template>

<script setup lang="ts">
import { Code, ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'

// Login page layout
import { ref } from 'vue'
import { create } from "@bufbuild/protobuf";
import { LoginRequestSchema } from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import { useUserAuthStore } from '@/stores/xylona'
import { useRouter } from 'vue-router'

const router = useRouter()
const username = ref('')
const password = ref('')
const userAuthStore = useUserAuthStore()

const $q = useQuasar()

async function login() {
  const loginRequest = create(LoginRequestSchema, {
    userName: username.value,
    password: password.value
  })
  try {
    const response = await GetXylonaClient().login(loginRequest)
    if (response.user === undefined) {
      $q.notify({
        type: 'xylona-error',
        position: 'top',
        caption: 'Invalid username or password',
        icon: 'report_problem'
      })
      return
    }
    userAuthStore.setUser(response.user)
    await router.push({path: '/'})
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    let caption = ConnectErrorToString(err)
    if (err.code === Code.Unauthenticated) {
      caption = 'Invalid username or password'
    }
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: caption,
      icon: 'report_problem'
    })
    console.error(caption)
  }
}

</script>

<style scoped>
</style>
