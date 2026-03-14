<template>
  <q-card-section>
    <GameServerForm :existing-game-server-id="gameServerID"></GameServerForm>
  </q-card-section>
</template>

<script setup lang="ts">
import GameServerForm from 'src/components/game_servers/GameServerForm.vue'
import {Ref, ref} from 'vue'
import {useRoute} from 'vue-router'

const route = useRoute()
const gameServerID = ref(route.params.id) as Ref<string>
</script>

<style scoped>

</style>
