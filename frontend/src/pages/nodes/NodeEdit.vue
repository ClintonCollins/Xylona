<template>
  <q-page class="xy-page-content">
    <node-form :existing-node-id="nodeID"></node-form>
  </q-page>
</template>

<script lang="ts" setup>
import { ref } from 'vue'
import { useRoute } from 'vue-router'
import NodeForm from '@/components/nodes/NodeForm.vue'

const route = useRoute()
const nodeID = ref(route.params.id)
</script>

<style scoped></style>
