<template>
  <q-page class="q-pa-md">
    <game-server-create-form v-if="readyToRender" />
  </q-page>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useQuasar } from 'quasar'

import GameServerCreateForm from '@/components/game_servers/GameServerCreateForm.vue'
import { useUserAuthStore } from '@/stores/xylona'

const authStore = useUserAuthStore()
const router = useRouter()
const $q = useQuasar()
const readyToRender = ref(false)

onMounted(async () => {
  let user = authStore.user

  if (!user) {
    const response = await authStore.checkUserAuthenticated()
    user = response?.user ?? null
  }

  if (!user?.superUser) {
    $q.notify({
      type: 'warning',
      position: 'top',
      caption: 'Only superusers can create game servers.',
      icon: 'report_problem',
    })
    await router.replace('/game-servers')
    return
  }

  readyToRender.value = true
})
</script>

<style scoped></style>
