<template>
  <q-card-section>
    <game-server-settings-form
      v-if="canEditProvisioning !== undefined"
      :can-edit-provisioning="canEditProvisioning"
      :game-server-id="gameServerID"></game-server-settings-form>
  </q-card-section>
</template>

<script lang="ts" setup>
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import GameServerSettingsForm from '@/components/game_servers/GameServerSettingsForm.vue'
import { useUserAuthStore } from '@/stores/xylona'

const route = useRoute()
const gameServerID = ref(route.params.id as string)
const authStore = useUserAuthStore()
const canEditProvisioning = ref<boolean>()

onMounted(async () => {
  let user = authStore.user

  if (!user) {
    const response = await authStore.checkUserAuthenticated()
    user = response?.user ?? null
  }

  canEditProvisioning.value = user?.superUser ?? false
})
</script>

<style scoped></style>
