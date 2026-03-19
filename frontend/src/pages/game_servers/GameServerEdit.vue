<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <game-server-form :existing-game-server-id="gameServerID"></game-server-form>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import GameServerForm from '@/components/game_servers/GameServerForm.vue'
import { Ref, ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const gameServerID = ref(route.params.id) as Ref<string>
</script>

<style scoped></style>
