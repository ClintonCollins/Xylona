<template>
  <q-page>
    <div class="row justify-center q-pa-md">
      <node-form :existing-node-id="nodeID"></node-form>
    </div>
  </q-page>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import NodeForm from '@/components/nodes/NodeForm.vue'

const route = useRoute()
const nodeID = ref(route.params.id)
</script>

<style scoped></style>
