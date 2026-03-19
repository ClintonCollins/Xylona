<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <game-form :existing-game-id="gameID"></game-form>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import GameForm from '@/components/games/GameForm.vue'
import { ref } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()
const gameID = ref(route.params.id)
</script>

<style scoped></style>
